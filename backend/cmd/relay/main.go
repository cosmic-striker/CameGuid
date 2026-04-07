package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"camguid/internal/config"
	"camguid/internal/database"
	"camguid/internal/pkg/logger"

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
)

type StreamSession struct {
	ID        string
	DeviceID  string
	Channel   int
	StreamURL string
	HLSPath   string
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	Status    string
	Retries   int
	StartedAt time.Time
}

type RelayService struct {
	pool     *pgxpool.Pool
	rdb      *redis.Client
	cfg      *config.Config
	log      zerolog.Logger
	sessions map[string]*StreamSession
	mu       sync.RWMutex
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

	relay := &RelayService{
		pool:     pool,
		rdb:      rdb,
		cfg:      cfg,
		log:      log,
		sessions: make(map[string]*StreamSession),
	}

	// Start idle session cleanup loop
	go relay.cleanupLoop(ctx)

	app := fiber.New(fiber.Config{AppName: "CamGuid Relay"})
	app.Use(cors.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		relay.mu.RLock()
		count := len(relay.sessions)
		relay.mu.RUnlock()
		return c.JSON(fiber.Map{
			"status":          "ok",
			"service":         "camguid-relay",
			"active_sessions": count,
		})
	})

	app.Post("/start", func(c *fiber.Ctx) error {
		var req struct {
			DeviceID  string `json:"device_id"`
			Channel   int    `json:"channel"`
			StreamURL string `json:"stream_url"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}
		if req.StreamURL == "" {
			return c.Status(400).JSON(fiber.Map{"error": "stream_url is required"})
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

		sessionID := uuid.New().String()
		session, err := relay.startStream(ctx, sessionID, req.DeviceID, req.Channel, req.StreamURL)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"session_id": session.ID,
			"hls_url":    fmt.Sprintf("/stream/hls/%s/index.m3u8", session.ID),
			"status":     session.Status,
		})
	})

	app.Delete("/stop/:id", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		relay.stopStream(sessionID)
		return c.JSON(fiber.Map{"status": "stopped"})
	})

	app.Get("/sessions", func(c *fiber.Ctx) error {
		relay.mu.RLock()
		defer relay.mu.RUnlock()

		var sessions []fiber.Map
		for _, s := range relay.sessions {
			sessions = append(sessions, fiber.Map{
				"id":         s.ID,
				"device_id":  s.DeviceID,
				"channel":    s.Channel,
				"status":     s.Status,
				"started_at": s.StartedAt,
			})
		}
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

	// Save to database
	r.pool.Exec(ctx,
		`INSERT INTO stream_sessions (id, device_id, channel, session_type, status, hls_path, pid, started_at)
		 VALUES ($1, $2, $3, 'hls', 'active', $4, $5, now())`,
		sessionID, deviceID, channel, hlsPath, cmd.Process.Pid)

	// Monitor FFmpeg process in background
	go r.monitorProcess(session, stderrPipe)

	r.log.Info().Str("session", sessionID).Str("url", streamURL).Msg("Stream started")
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
		if session.Retries < maxRetries {
			session.Retries++
			r.log.Info().Str("session", session.ID).Int("retry", session.Retries).Msg("Restarting stream")
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
}

func (r *RelayService) stopStream(sessionID string) {
	r.mu.Lock()
	session, ok := r.sessions[sessionID]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.sessions, sessionID)
	r.mu.Unlock()

	session.Cancel()
	session.Retries = maxRetries + 1 // Prevent auto-restart

	// Cleanup files
	sessionDir := filepath.Join(streamBaseDir, sessionID)
	os.RemoveAll(sessionDir)

	// Update DB
	r.pool.Exec(context.Background(),
		`UPDATE stream_sessions SET status = 'stopped', stopped_at = now() WHERE id = $1`, sessionID)

	r.log.Info().Str("session", sessionID).Msg("Stream stopped")
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
}

func (r *RelayService) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check for idle sessions (no viewers)
			// In production, would track viewer heartbeats via Redis
			r.log.Debug().Int("active", len(r.sessions)).Msg("Session cleanup tick")
		}
	}
}
