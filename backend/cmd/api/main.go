package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"camguid/internal/config"
	"camguid/internal/database"
	"camguid/internal/handler"
	"camguid/internal/middleware"
	"camguid/internal/pkg/logger"
	"camguid/internal/repository"
	auth "camguid/internal/service/auth"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
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

	// Repositories
	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	// Services
	authService := auth.NewService(userRepo, cfg.JWTSecret)

	// Bootstrap default admin
	if err := authService.EnsureDefaultAdmin(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure default admin")
	}

	// Handlers
	h := handler.New(authService, deviceRepo, userRepo)

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
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE",
	}))

	// Routes
	api := app.Group("/api/v1")

	// Health (public)
	api.Get("/health", h.HealthCheck)

	// Auth (public)
	authGroup := api.Group("/auth")
	authGroup.Post("/login", h.Login)
	authGroup.Post("/refresh", h.RefreshToken)

	// Protected routes
	protected := api.Group("", middleware.AuthMiddleware(cfg.JWTSecret))

	// Auth (protected)
	protected.Get("/auth/me", h.GetMe)
	protected.Put("/auth/password", h.ChangePassword)
	protected.Post("/auth/register", middleware.RequireRole("admin"), h.Register)

	// Dashboard
	protected.Get("/dashboard/stats", h.GetDashboardStats)

	// Devices
	devices := protected.Group("/devices")
	devices.Get("/", h.ListDevices)
	devices.Get("/:id", h.GetDevice)
	devices.Post("/", middleware.RequireRole("admin"), h.CreateDevice)
	devices.Put("/:id", middleware.RequireRole("admin"), h.UpdateDevice)
	devices.Delete("/:id", middleware.RequireRole("admin"), h.DeleteDevice)
	devices.Patch("/:id/toggle", middleware.RequireRole("admin"), h.ToggleDevice)
	devices.Post("/:id/channels", middleware.RequireRole("admin"), h.GenerateChannels)

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
