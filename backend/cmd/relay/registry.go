package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// sessionRegistryPrefix is the Redis key namespace for the global session
	// registry, shared by every relay replica.
	sessionRegistryPrefix = "camguid:session:"
	// streamStopChannel is the pub/sub channel used to ask the replica that
	// owns a session to stop it (cross-replica HA).
	streamStopChannel = "camguid:relay:stop"
	// stopAction marks a stop command payload on streamStopChannel.
	stopAction = "remote_stop"
)

// RegisteredSession is the shareable metadata published to Redis when an HLS
// session starts, so any relay replica can observe and target it.
type RegisteredSession struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"device_id"`
	Channel   int       `json:"channel"`
	Status    string    `json:"status"`
	HLSPath   string    `json:"hls_path"`
	StartedAt time.Time `json:"started_at"`
}

func sessionRegistryKey(id string) string {
	return sessionRegistryPrefix + id
}

// registrySet publishes a session to the shared registry.
func (r *RelayService) registrySet(ctx context.Context, s *StreamSession) error {
	reg := RegisteredSession{
		ID:        s.ID,
		DeviceID:  s.DeviceID,
		Channel:   s.Channel,
		Status:    s.Status,
		HLSPath:   s.HLSPath,
		StartedAt: s.StartedAt,
	}
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	return r.rdb.Set(ctx, sessionRegistryKey(s.ID), data, 0).Err()
}

// registryDelete removes a session entry from the shared registry.
func (r *RelayService) registryDelete(ctx context.Context, id string) error {
	return r.rdb.Del(ctx, sessionRegistryKey(id)).Err()
}

// registryExists reports whether a session is known to the shared registry.
func (r *RelayService) registryExists(ctx context.Context, id string) (bool, error) {
	n, err := r.rdb.Exists(ctx, sessionRegistryKey(id)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// registryList returns every session currently registered across all replicas.
func (r *RelayService) registryList(ctx context.Context) ([]RegisteredSession, error) {
	var out []RegisteredSession
	iter := r.rdb.Scan(ctx, 0, sessionRegistryPrefix+"*", 256).Iterator()
	for iter.Next(ctx) {
		data, err := r.rdb.Get(ctx, iter.Val()).Bytes()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			r.log.Warn().Err(err).Str("key", iter.Val()).Msg("Failed to read session registry entry")
			continue
		}
		var s RegisteredSession
		if err := json.Unmarshal(data, &s); err != nil {
			r.log.Warn().Err(err).Str("key", iter.Val()).Msg("Failed to decode session registry entry")
			continue
		}
		out = append(out, s)
	}
	if err := iter.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// subscribeToOps listens for cross-replica commands (e.g. "stop this session").
// Every replica receives every command, but only the one that actually owns the
// session in its local map reacts — the others ignore it harmlessly.
func (r *RelayService) subscribeToOps(ctx context.Context) {
	pubsub := r.rdb.Subscribe(ctx, streamStopChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	r.log.Info().Msg("Subscribed to cross-replica relay commands")

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var op struct {
				Action    string `json:"action"`
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &op); err != nil {
				r.log.Warn().Err(err).Msg("Ignoring malformed relay command")
				continue
			}
			if op.Action != stopAction || op.SessionID == "" {
				continue
			}
			if r.stopStream(op.SessionID) {
				r.log.Info().Str("session", op.SessionID).Msg("Stopped session via cross-replica command")
			}
		}
	}
}

// publishStop asks whichever replica owns the session to stop it.
func (r *RelayService) publishStop(ctx context.Context, sessionID string) error {
	op, err := json.Marshal(map[string]string{
		"action":     stopAction,
		"session_id": sessionID,
	})
	if err != nil {
		return err
	}
	return r.rdb.Publish(ctx, streamStopChannel, op).Err()
}

// sweepOrphanedSessions removes registry entries whose owning replica is no
// longer running (e.g. it crashed before Deregistering). Their shared HLS
// files and heartbeat leases are cleaned so the shared volume and Redis don't
// accumulate stale streams.
func (r *RelayService) sweepOrphanedSessions(ctx context.Context) {
	registered, err := r.registryList(ctx)
	if err != nil {
		r.log.Warn().Err(err).Msg("Orphan sweep: registry scan failed")
		return
	}
	staleAfter := time.Duration(r.cfg.StreamIdleTimeout*2) * time.Second

	for _, reg := range registered {
		r.mu.RLock()
		_, local := r.sessions[reg.ID]
		r.mu.RUnlock()
		if local {
			continue
		}
		if time.Since(reg.StartedAt) < staleAfter {
			continue
		}
		r.log.Info().
			Str("session", reg.ID).
			Str("device", reg.DeviceID).
			Msg("Cleaning up orphaned session (owner unreachable)")
		_ = r.registryDelete(ctx, reg.ID)
		_ = r.rdb.Del(ctx, heartbeatKey(reg.ID)).Err()
		_ = os.RemoveAll(filepath.Join(streamBaseDir, reg.ID))
	}
}