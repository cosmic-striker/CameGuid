package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"camguid/internal/config"
	"camguid/internal/database"
	"camguid/internal/middleware"
	"camguid/internal/pkg/logger"
	"camguid/internal/pkg/streamtoken"
	"camguid/internal/repository"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	streamBaseDir   = "/tmp/streams"
	snapshotBaseDir = "/tmp/snapshots"
	maxRetries      = 3

	heartbeatKeyPrefix = "camguid:heartbeat:"
)

func heartbeatKey(sessionID string) string {
	return heartbeatKeyPrefix + sessionID
}

// redactStreamURL strips userinfo credentials from a URL for safe logging
// (e.g. rtsp://user:pass@host -> rtsp://***@host).
func redactStreamURL(raw string) string {
	idx := strings.Index(raw, "://")
	if idx == -1 {
		return raw
	}
	scheme := raw[:idx+3]
	rest := raw[idx+3:]
	at := strings.Index(rest, "@")
	if at == -1 {
		return raw
	}
	return scheme + "***@" + rest[at+1:]
}

// fmtInt renders an int64 for Prometheus text exposition.
func fmtInt(v int64) fmt.Stringer {
	return intStringer(v)
}

type intStringer int64

func (i intStringer) String() string {
	return fmt.Sprintf("%d", int64(i))
}

type StreamSession struct {
	ID        string
	DeviceID  string
	Channel   int
	StreamURL string
	HLSPath   string
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	Status    string
	Retries   atomic.Int32
	StartedAt time.Time
}

type RelayService struct {
	pool     *pgxpool.Pool
	rdb      *redis.Client
	cfg      *config.Config
	log      zerolog.Logger
	cipher   *repository.SecretCipher // decrypts stream_url at rest
	verifier *streamtoken.Signer      // validates HMAC stream tokens
	sessions map[string]*StreamSession
	mu       sync.RWMutex
	snapshots *SnapshotManager

	// Observability counters (Prometheus-style /metrics)
	startedTotal     atomic.Int64
	stoppedTotal     atomic.Int64
	restartsTotal    atomic.Int64
	idleReclaims     atomic.Int64
	registeredSess   atomic.Int64 // global registry count refreshed by cleanupLoop
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info().Str("port", cfg.RelayPort).Msg("Starting CamGuid Stream Relay")

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse Redis URL")
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()

	// Ensure directories exist
	os.MkdirAll(streamBaseDir, 0755)
	os.MkdirAll(snapshotBaseDir, 0755)
	ensureSnapshotDir(snapshotBaseDir)

	verifier, err := streamtoken.NewSigner(cfg.StreamSigningSecret)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid CAMGUID_STREAM_SIGNING_SECRET")
	}

	relay := &RelayService{
		pool:      pool,
		rdb:       rdb,
		cfg:       cfg,
		log:       log,
		cipher:    repository.NewSecretCipher(cfg.EncryptionKey),
		verifier:  verifier,
		sessions:  make(map[string]*StreamSession),
		snapshots: NewSnapshotManager(),
	}

	// Start idle session cleanup loop
	go relay.cleanupLoop(ctx)
	// Listen for cross-replica commands (e.g. stop a session owned remotely)
	go relay.subscribeToOps(ctx)

	// Relay API auth: every stream/snapshot control call must present a
	// short-lived HMAC token minted by the API for the target device, so the
	// relay never needs (and never receives) raw RTSP URLs.
	guard := middleware.StreamTokenMiddleware(verifier)

	app := fiber.New(fiber.Config{AppName: "CamGuid Relay"})
	app.Use(cors.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		relay.mu.RLock()
		count := len(relay.sessions)
		relay.mu.RUnlock()
		return c.JSON(fiber.Map{
			"status":          "ok",
			"service":         "camguid-relay",
			"instance":        cfg.RelayInstanceID,
			"active_sessions": count,
		})
	})

	// Prometheus-style metrics for per-replica monitoring.
	app.Get("/metrics", func(c *fiber.Ctx) error {
		relay.mu.RLock()
		active := len(relay.sessions)
		relay.mu.RUnlock()

		var b bytes.Buffer
		w := func(name, help, typ string, value fmt.Stringer) {
			fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n%s %s\n", name, help, name, typ, name, value.String())
		}

		w("camguid_relay_sessions_active", "Local active HLS sessions on this replica",
			"gauge", fmtInt(int64(active)))
		w("camguid_relay_sessions_registered", "Sessions registered across all relay replicas",
			"gauge", fmtInt(relay.registeredSess.Load()))
		w("camguid_relay_snapshot_jobs", "Periodic snapshot jobs running on this replica",
			"gauge", fmtInt(relay.snapshots.running()))
		w("camguid_relay_sessions_started_total", "Cumulative stream sessions started", "counter",
			fmtInt(relay.startedTotal.Load()))
		w("camguid_relay_sessions_stopped_total", "Cumulative stream sessions stopped", "counter",
			fmtInt(relay.stoppedTotal.Load()))
		w("camguid_relay_ffmpeg_restarts_total", "Cumulative FFmpeg auto-restarts", "counter",
			fmtInt(relay.restartsTotal.Load()))
		w("camguid_relay_idle_reclaims_total", "Cumulative idle sessions reclaimed via lease expiry", "counter",
			fmtInt(relay.idleReclaims.Load()))
		w("camguid_relay_snapshot_captures_total", "Cumulative snapshot captures attempted", "counter",
			fmtInt(relay.snapshots.capturesTotal.Load()))
		w("camguid_relay_snapshot_failures_total", "Cumulative snapshot capture failures", "counter",
			fmtInt(relay.snapshots.failuresTotal.Load()))

		c.Set("Content-Type", "text/plain; version=0.0.4")
		return c.SendString(b.String())
	})

	app.Post("/start", guard, func(c *fiber.Ctx) error {
		// The device id is taken from the signed token, never from the body.
		deviceID := middleware.GetStreamDeviceID(c)
		if deviceID == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Missing valid stream token"})
		}
		deviceUUID, err := uuid.Parse(deviceID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid device id in token"})
		}

		var req struct {
			Channel int `json:"channel"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}
		if req.Channel == 0 {
			req.Channel = 1
		}

		// Check max sessions
		relay.mu.RLock()
		if len(relay.sessions) >= cfg.StreamMaxSessions {
			relay.mu.RUnlock()
			return c.Status(429).JSON(fiber.Map{"error": "Maximum stream sessions reached"})
		}
		relay.mu.RUnlock()

		// Resolve the (possibly credential-bearing) RTSP URL from the DB.
		streamURL, enabled, ok, err := relay.deviceRepo().ResolveStreamURL(ctx, deviceUUID)
		if err != nil {
			relay.log.Error().Err(err).Str("device", deviceID).Msg("Failed to resolve stream URL")
			return c.Status(500).JSON(fiber.Map{"error": "Failed to resolve device stream"})
		}
		if !ok {
			return c.Status(404).JSON(fiber.Map{"error": "Device not found"})
		}
		if !enabled {
			return c.Status(403).JSON(fiber.Map{"error": "Device is disabled"})
		}
		if streamURL == "" {
			return c.Status(409).JSON(fiber.Map{"error": "Device has no stream URL configured"})
		}

		sessionID := uuid.New().String()
		session, err := relay.startStream(ctx, sessionID, deviceID, req.Channel, streamURL)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"session_id": session.ID,
			"hls_url":    fmt.Sprintf("/stream/hls/%s/index.m3u8", session.ID),
			"status":     session.Status,
		})
	})

	app.Delete("/stop/:id", guard, func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		// Stop locally if this replica owns the session...
		if relay.stopStream(sessionID) {
			return c.JSON(fiber.Map{"status": "stopped"})
		}
		// ...otherwise ask the owning replica via Redis pub/sub (multi-replica HA).
		if exists, _ := relay.registryExists(ctx, sessionID); exists {
			if err := relay.publishStop(ctx, sessionID); err != nil {
				relay.log.Warn().Err(err).Str("session", sessionID).Msg("Failed to publish stop command")
				return c.Status(500).JSON(fiber.Map{"error": "Failed to reach session owner"})
			}
			return c.JSON(fiber.Map{"status": "stopping"})
		}
		return c.Status(404).JSON(fiber.Map{"error": "Session not found"})
	})

	app.Get("/sessions", guard, func(c *fiber.Ctx) error {
		// Primary view: the global Redis registry (all replicas).
		var sessions []fiber.Map
		registered, err := relay.registryList(ctx)
		seen := make(map[string]bool)
		if err == nil {
			for _, reg := range registered {
				seen[reg.ID] = true
				sessions = append(sessions, fiber.Map{
					"id":         reg.ID,
					"device_id":  reg.DeviceID,
					"channel":    reg.Channel,
					"status":     reg.Status,
					"started_at": reg.StartedAt,
				})
			}
		} else {
			relay.log.Warn().Err(err).Msg("Registry list failed, falling back to local sessions")
		}

		// Merge any local sessions missing from the registry (e.g. right at
		// startup, or if the registry write failed).
		relay.mu.RLock()
		for _, s := range relay.sessions {
			if seen[s.ID] {
				continue
			}
			sessions = append(sessions, fiber.Map{
				"id":         s.ID,
				"device_id":  s.DeviceID,
				"channel":    s.Channel,
				"status":     s.Status,
				"started_at": s.StartedAt,
			})
		}
		relay.mu.RUnlock()

		return c.JSON(fiber.Map{"sessions": sessions})
	})

	app.Get("/snapshot/:deviceId", func(c *fiber.Ctx) error {
		deviceID := c.Params("deviceId")
		path := filepath.Join(snapshotBaseDir, deviceID+".jpg")
		if _, err := os.Stat(path); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Snapshot not found"})
		}
		return c.SendFile(path)
	})

	// Heartbeat: the Live Wall calls this periodically for sessions currently
	// being viewed, renewing the session's lease in Redis. Sessions whose lease
	// expires (no viewers) are reclaimed by the cleanup loop, so HLS sessions
	// are on-demand rather than persistent.
	app.Post("/heartbeat/:id", guard, func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		// Defer to the shared registry so heartbeats arriving at a different
		// replica still renew the owner's session lease.
		exists, err := relay.registryExists(ctx, sessionID)
		if err != nil || !exists {
			relay.mu.RLock()
			_, local := relay.sessions[sessionID]
			relay.mu.RUnlock()
			if err != nil || !local {
				return c.Status(404).JSON(fiber.Map{"error": "Session not found"})
			}
		}
		ttl := time.Duration(relay.cfg.StreamIdleTimeout) * time.Second
		relay.rdb.Set(context.Background(), heartbeatKey(sessionID), "1", ttl)
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// One-shot snapshot capture — grab a single frame now.
	app.Post("/snapshot/capture", guard, func(c *fiber.Ctx) error {
		var req struct {
			Channel int `json:"channel"`
			Width   int `json:"width"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}
		deviceID := middleware.GetStreamDeviceID(c)
		if deviceID == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Missing valid stream token"})
		}
		deviceUUID, err := uuid.Parse(deviceID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid device id in token"})
		}
		if req.Width == 0 {
			req.Width = relay.cfg.SnapshotWidth
		}

		streamURL, enabled, ok, err := relay.deviceRepo().ResolveStreamURL(ctx, deviceUUID)
		if err != nil || !ok || !enabled || streamURL == "" {
			if err != nil {
				relay.log.Error().Err(err).Str("device", deviceID).Msg("Snapshot: stream resolution failed")
			}
			code := fiber.StatusNotFound
			msg := "Device not found or no stream configured"
			switch {
			case !ok:
				code = fiber.StatusNotFound
			case !enabled:
				code = fiber.StatusForbidden
				msg = "Device is disabled"
			case streamURL == "":
				code = fiber.StatusConflict
				msg = "Device has no stream URL configured"
			}
			return c.Status(code).JSON(fiber.Map{"error": msg})
		}

		go func() {
			if err := relay.snapshots.CaptureNow(deviceID, streamURL, req.Channel, req.Width); err != nil {
				relay.log.Warn().Str("device", deviceID).Err(err).Msg("One-shot snapshot capture failed")
			}
		}()

		return c.JSON(fiber.Map{
			"status":       "queued",
			"snapshot_url": fmt.Sprintf("/stream/snapshots/%s.jpg", deviceID),
		})
	})

	// Periodic snapshot job — registers a repeating capture loop for a device.
	app.Post("/snapshot/start", guard, func(c *fiber.Ctx) error {
		var req struct {
			Channel  int `json:"channel"`
			Width    int `json:"width"`
			Interval int `json:"interval"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}
		deviceID := middleware.GetStreamDeviceID(c)
		if deviceID == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Missing valid stream token"})
		}
		deviceUUID, err := uuid.Parse(deviceID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid device id in token"})
		}
		if req.Width == 0 {
			req.Width = relay.cfg.SnapshotWidth
		}
		if req.Interval == 0 {
			req.Interval = relay.cfg.SnapshotInterval
		}

		streamURL, enabled, ok, err := relay.deviceRepo().ResolveStreamURL(ctx, deviceUUID)
		if err != nil || !ok || !enabled || streamURL == "" {
			if err != nil {
				relay.log.Error().Err(err).Str("device", deviceID).Msg("Snapshot job: stream resolution failed")
			}
			code := fiber.StatusNotFound
			msg := "Device not found or no stream configured"
			switch {
			case !ok:
				code = fiber.StatusNotFound
			case !enabled:
				code = fiber.StatusForbidden
				msg = "Device is disabled"
			case streamURL == "":
				code = fiber.StatusConflict
				msg = "Device has no stream URL configured"
			}
			return c.Status(code).JSON(fiber.Map{"error": msg})
		}

		job := relay.snapshots.Start(
			deviceID,
			streamURL,
			req.Channel,
			req.Width,
			time.Duration(req.Interval)*time.Second,
		)

		return c.JSON(fiber.Map{
			"status":       "started",
			"device_id":    job.DeviceID,
			"interval_sec": int(job.Interval / time.Second),
			"snapshot_url": fmt.Sprintf("/stream/snapshots/%s.jpg", job.DeviceID),
		})
	})

	// Stop a periodic snapshot job.
	app.Delete("/snapshot/stop/:deviceId", guard, func(c *fiber.Ctx) error {
		deviceID := c.Params("deviceId")
		if mid := middleware.GetStreamDeviceID(c); mid != "" && mid != deviceID {
			return c.Status(403).JSON(fiber.Map{"error": "Token not valid for this device"})
		}
		if !relay.snapshots.Stop(deviceID) {
			return c.Status(404).JSON(fiber.Map{"error": "No active snapshot job"})
		}
		return c.JSON(fiber.Map{"status": "stopped"})
	})

	// List active snapshot jobs.
	app.Get("/snapshot/jobs", guard, func(c *fiber.Ctx) error {
		relay.snapshots.mu.Lock()
		defer relay.snapshots.mu.Unlock()
		jobs := make([]fiber.Map, 0, len(relay.snapshots.jobs))
		for _, j := range relay.snapshots.jobs {
			jobs = append(jobs, fiber.Map{
				"device_id":     j.DeviceID,
				"channel":       j.Channel,
				"interval_sec":  int(j.Interval / time.Second),
				"last_capture":  j.LastCapture,
				"last_error":    j.LastError,
				"active":        j.Active,
			})
		}
		return c.JSON(fiber.Map{"jobs": jobs})
	})

	go func() {
		if err := app.Listen(":" + cfg.RelayPort); err != nil {
			log.Fatal().Err(err).Msg("Relay server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down relay — stopping all streams...")
	relay.stopAll()
	app.Shutdown()
}

// deviceRepo returns a DeviceRepository bound to this replica's pool and
// decryption cipher, used to resolve stream URLs for a signed device id.
func (r *RelayService) deviceRepo() *repository.DeviceRepository {
	return repository.NewDeviceRepository(r.pool, r.cipher)
}

func (r *RelayService) startStream(ctx context.Context, sessionID, deviceID string, channel int, streamURL string) (*StreamSession, error) {
	sessionDir := filepath.Join(streamBaseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	hlsPath := filepath.Join(sessionDir, "index.m3u8")
	segPattern := filepath.Join(sessionDir, "seg_%03d.ts")

	fctx, cancel := context.WithCancel(ctx)

	args := []string{}
	if len(streamURL) >= 4 && streamURL[:4] == "rtsp" {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args, 
		"-i", streamURL,
		"-c:v", "copy",
		"-an",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", r.cfg.HLSSegmentDuration),
		"-hls_list_size", "5",
		"-hls_flags", "delete_segments+append_list",
		"-hls_segment_filename", segPattern,
		hlsPath,
	)

	cmd := exec.CommandContext(fctx, "ffmpeg", args...)

	// Capture FFmpeg stderr for logging
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		os.RemoveAll(sessionDir)
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	session := &StreamSession{
		ID:        sessionID,
		DeviceID:  deviceID,
		Channel:   channel,
		StreamURL: streamURL,
		HLSPath:   hlsPath,
		Cmd:       cmd,
		Cancel:    cancel,
		Status:    "starting",
		StartedAt: time.Now(),
	}

	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(sessionDir)
		return nil, fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	session.Status = "active"

	r.mu.Lock()
	r.sessions[sessionID] = session
	r.mu.Unlock()

	// Publish to the shared registry so any relay replica can observe/stop it.
	if err := r.registrySet(ctx, session); err != nil {
		r.log.Warn().Err(err).Str("session", session.ID).Msg("Failed to register session in Redis")
	}

	// Save to database
	r.pool.Exec(ctx,
		`INSERT INTO stream_sessions (id, device_id, channel, session_type, status, hls_path, pid, started_at)
		 VALUES ($1, $2, $3, 'hls', 'active', $4, $5, now())`,
		sessionID, deviceID, channel, hlsPath, cmd.Process.Pid)

	// Monitor FFmpeg process in background
	go r.monitorProcess(session, stderrPipe)

	// Grant the session an initial lease in Redis. It is renewed by viewer
	// heartbeats; when it expires (no viewers) the cleanup loop reclaims the
	// session so on-demand streams free their FFmpeg slot automatically.
	ttl := time.Duration(r.cfg.StreamIdleTimeout) * time.Second
	r.rdb.Set(ctx, heartbeatKey(sessionID), "1", ttl)

	r.log.Info().Str("session", sessionID).Str("url", redactStreamURL(streamURL)).Msg("Stream started")
	r.startedTotal.Add(1)
	return session, nil
}

func (r *RelayService) monitorProcess(session *StreamSession, stderr io.ReadCloser) {
	// Drain stderr in a separate goroutine
	if stderr != nil {
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := stderr.Read(buf)
				if n > 0 {
					r.log.Debug().Str("session", session.ID).Str("ffmpeg", string(buf[:n])).Msg("FFmpeg output")
				}
				if err != nil {
					break
				}
			}
		}()
	}

	err := session.Cmd.Wait()
	if err != nil {
		r.log.Warn().Str("session", session.ID).Err(err).Msg("FFmpeg process exited")

		// Auto-restart if under retry limit
		if session.Retries.Load() < maxRetries {
			session.Retries.Add(1)
			r.log.Info().Str("session", session.ID).Int("retry", int(session.Retries.Load())).Msg("Restarting stream")
			time.Sleep(2 * time.Second)
			r.restartStream(session)
			return
		}
	}

	// Mark as stopped
	r.mu.Lock()
	if s, ok := r.sessions[session.ID]; ok {
		s.Status = "stopped"
	}
	r.mu.Unlock()

	// Update DB
	r.pool.Exec(context.Background(),
		`UPDATE stream_sessions SET status = 'stopped', stopped_at = now() WHERE id = $1`, session.ID)
}

func (r *RelayService) restartStream(session *StreamSession) {
	fctx, cancel := context.WithCancel(context.Background())

	sessionDir := filepath.Join(streamBaseDir, session.ID)
	segPattern := filepath.Join(sessionDir, "seg_%03d.ts")

	args := []string{}
	if len(session.StreamURL) >= 4 && session.StreamURL[:4] == "rtsp" {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args,
		"-i", session.StreamURL,
		"-c:v", "copy",
		"-an",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", r.cfg.HLSSegmentDuration),
		"-hls_list_size", "5",
		"-hls_flags", "delete_segments+append_list",
		"-hls_segment_filename", segPattern,
		session.HLSPath,
	)

	cmd := exec.CommandContext(fctx, "ffmpeg", args...)

	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		r.log.Error().Str("session", session.ID).Err(err).Msg("Failed to restart stream")
		cancel()
		return
	}

	r.mu.Lock()
	session.Cmd = cmd
	session.Cancel = cancel
	session.Status = "active"
	r.mu.Unlock()

	go r.monitorProcess(session, stderrPipe)
	r.restartsTotal.Add(1)
}

func (r *RelayService) stopStream(sessionID string) bool {
	r.mu.Lock()
	session, ok := r.sessions[sessionID]
	if !ok {
		r.mu.Unlock()
		return false
	}
	delete(r.sessions, sessionID)
	r.mu.Unlock()

	session.Cancel()
	session.Retries.Store(maxRetries + 1) // Prevent auto-restart

	// Clear the session's lease and registry entry so cross-replica views are
	// accurate and cleanup bookkeeping stays consistent.
	r.rdb.Del(context.Background(), heartbeatKey(sessionID))
	r.registryDelete(context.Background(), sessionID)

	// Cleanup files
	sessionDir := filepath.Join(streamBaseDir, sessionID)
	os.RemoveAll(sessionDir)

	// Update DB
	r.pool.Exec(context.Background(),
		`UPDATE stream_sessions SET status = 'stopped', stopped_at = now() WHERE id = $1`, sessionID)

	r.log.Info().Str("session", sessionID).Msg("Stream stopped")
	r.stoppedTotal.Add(1)
	return true
}

func (r *RelayService) stopAll() {
	r.mu.Lock()
	sessions := make([]*StreamSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	r.mu.Unlock()

	for _, s := range sessions {
		r.stopStream(s.ID)
	}

	// Stop every periodic snapshot job
	r.snapshots.mu.Lock()
	for deviceID, job := range r.snapshots.jobs {
		job.Cancel()
		delete(r.snapshots.jobs, deviceID)
		_ = deviceID
	}
	r.snapshots.mu.Unlock()
}

func (r *RelayService) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reclaimIdleSessions(ctx)
		}
	}
}

// reclaimIdleSessions stops any HLS session whose Redis lease has expired,
// i.e. no viewer has sent a heartbeat within StreamIdleTimeout seconds. This
// is what keeps heavy FFmpeg usage proportional to actual viewers instead of
// one process per camera 24/7.
func (r *RelayService) reclaimIdleSessions(ctx context.Context) {
	r.mu.RLock()
	sessions := make([]*StreamSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	r.mu.RUnlock()

	for _, s := range sessions {
		exists, err := r.rdb.Exists(ctx, heartbeatKey(s.ID)).Result()
		if err != nil {
			r.log.Warn().Err(err).Str("session", s.ID).Msg("Heartbeat check failed")
			continue
		}
		if exists == 0 {
			r.log.Info().
				Str("session", s.ID).
				Str("device", s.DeviceID).
				Msg("Reclaiming idle stream session (lease expired, no viewers)")
			r.stopStream(s.ID)
			r.idleReclaims.Add(1)
		}
	}

	// Refresh the global registry gauge used by /metrics.
	if registered, err := r.registryList(ctx); err == nil {
		r.registeredSess.Store(int64(len(registered)))
	} else {
		r.log.Warn().Err(err).Msg("Registry count refresh failed")
	}

	// Clean up sessions left behind by replicas that crashed — their FFmpeg
	// processes are gone but registry entries + HLS files linger.
	r.sweepOrphanedSessions(ctx)
}
