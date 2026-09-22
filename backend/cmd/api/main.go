package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"camguid/internal/config"
	"camguid/internal/database"
	"camguid/internal/handler"
	"camguid/internal/middleware"
	"camguid/internal/pkg/logger"
	"camguid/internal/pkg/streamtoken"
	"camguid/internal/repository"
	auth "camguid/internal/service/auth"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}

	// Logger
	log := logger.New(cfg.LogLevel)
	log.Info().Str("port", cfg.APIPort).Msg("Starting CamGuid API server")

	// Database
	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	// Run migrations
	if err := database.Migrate(ctx, pool, log); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Redis (rate limiting context; optional at runtime)
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid CAMGUID_REDIS_URL")
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	// Repositories
	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool, repository.NewSecretCipher(cfg.EncryptionKey))
	auditRepo := repository.NewAuditLogRepository(pool)

	// Services
	authService := auth.NewService(userRepo, cfg.JWTSecret)

	// Stream token signer (HTTPS-only concern: same secret used by relay)
	streamSigner, err := streamtoken.NewSigner(cfg.StreamSigningSecret)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid CAMGUID_STREAM_SIGNING_SECRET")
	}

	// Bootstrap default admin
	if err := authService.EnsureDefaultAdmin(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure default admin")
	}

	// Handlers
	h := handler.New(handler.HandlerOptions{
		AuthService:     authService,
		DeviceRepo:      deviceRepo,
		UserRepo:        userRepo,
		AuditRepo:       auditRepo,
		StreamSigner:    streamSigner,
		StreamTokenTTL:  5 * time.Minute,
		Redis:           rdb,
		LoginRateLimit:  cfg.LoginRateLimit,
		LoginRateWindow: time.Duration(cfg.LoginRateWindow) * time.Second,
	})

	// Audit retention sweep
	go auditRetentionSweep(ctx, auditRepo, cfg.AuditRetentionDays, log)

	// Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "CamGuid API",
		ErrorHandler: customErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "${time} | ${status} | ${latency} | ${method} ${path}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Stream-Token",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE",
	}))

	// Routes
	api := app.Group("/api/v1")

	// Health (public)
	api.Get("/health", h.HealthCheck)

	// Auth (public) — login is rate-limited per IP+username
	authGroup := api.Group("/auth")
	authGroup.Post("/login", middleware.LoginRateLimit(rdb, cfg.LoginRateLimit, time.Duration(cfg.LoginRateWindow)*time.Second), h.Login)
	authGroup.Post("/refresh", h.RefreshToken)

	// Protected routes
	protected := api.Group("", middleware.AuthMiddleware(cfg.JWTSecret))

	// Auth (protected)
	protected.Get("/auth/me", h.GetMe)
	protected.Put("/auth/password", h.ChangePassword)
	protected.Post("/auth/register", middleware.RequireRole("admin"), h.Register)

	// RBAC route matrix:
	//   viewer  — read-only: dashboard stats, devices list/get, stream token mint
	//   operator — device management (create/update/delete/toggle/channels)
	//   admin   — user management, audit logs
	protected.Get("/dashboard/stats", h.GetDashboardStats)

	// Devices
	devices := protected.Group("/devices")
	devices.Get("/", h.ListDevices)
	devices.Get("/:id", h.GetDevice)
	devices.Post("/", middleware.RequireRole("admin", "operator"), h.CreateDevice)
	devices.Put("/:id", middleware.RequireRole("admin", "operator"), h.UpdateDevice)
	devices.Delete("/:id", middleware.RequireRole("admin", "operator"), h.DeleteDevice)
	devices.Patch("/:id/toggle", middleware.RequireRole("admin", "operator"), h.ToggleDevice)
	devices.Post("/:id/channels", middleware.RequireRole("admin", "operator"), h.GenerateChannels)

	// Streams
	protected.Post("/stream/token", h.MintStreamToken)

	// Audit logs
	audit := protected.Group("/audit-logs", middleware.RequireRole("admin", "operator"))
	audit.Get("/", h.ListAuditLogs)
	audit.Get("/export", h.ExportAuditLogs)

	// Graceful shutdown
	go func() {
		if err := app.Listen(":" + cfg.APIPort); err != nil {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down API server...")
	app.Shutdown()
}

// auditRetentionSweep periodically purges audit entries older than the
// configured retention window.
func auditRetentionSweep(ctx context.Context, repo *repository.AuditLogRepository, days int, log zerolog.Logger) {
	cutoff := time.Now().AddDate(0, 0, -days)

	start := time.Now()
	deleted, err := repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		log.Err(err).Msg("Audit retention sweep failed")
		return
	}
	if deleted > 0 {
		log.Info().Int64("deleted", deleted).Dur("elapsed", time.Since(start)).Msg("Audit retention sweep completed")
	}

	// Re-run daily (24h). A simple interval keeps the sweep self-healing.
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -days)
			if n, err := repo.DeleteOlderThan(ctx, cutoff); err == nil && n > 0 {
				log.Info().Int64("deleted", n).Msg("Audit retention sweep completed")
			}
		}
	}
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    "INTERNAL_ERROR",
			"message": err.Error(),
		},
	})
}