package api

import (
	"go-ecommerce-app/configs"
	"go-ecommerce-app/internal/api/middleware"
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/api/rest/handlers"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/helper"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func StartServer(config configs.AppConfig) {
	app := fiber.New(fiber.Config{
		// Return structured JSON on unhandled errors instead of HTML.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	middleware.SetupGlobal(app)

	db, err := gorm.Open(postgres.Open(config.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	slog.Info("database connection established")

	db.AutoMigrate(
		&domain.User{},
		&domain.RefreshToken{},
		&domain.Address{},
		&domain.Category{},
		&domain.Product{},
		&domain.ProductImage{},
		&domain.Cart{},
		&domain.CartItem{},
		&domain.Order{},
		&domain.OrderItem{},
	)

	// Health check — probes the DB on every call.
	sqlDB, _ := db.DB()
	app.Get("/health", func(c *fiber.Ctx) error {
		if err := sqlDB.Ping(); err != nil {
			slog.Error("health check db ping failed", "err", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "down",
				"db":     "unreachable",
			})
		}
		return c.JSON(fiber.Map{
			"status":  "ok",
			"db":      "up",
			"version": "1.0.0",
		})
	})

	auth := helper.NewAuth(config.AppSecret)
	h := rest.RestHandler{
		App:                app,
		DB:                 db,
		Auth:               auth,
		NotificationClient: config.EmailNotification,
		S3Client:           config.S3Client,
		SQSClient:          config.SQSClient,
		StripeClient:       config.StripeClient,
	}

	handlers.SetupUserRoutes(&h)
	handlers.SetupProductRoutes(&h)
	handlers.SetupSellerRoutes(&h)
	handlers.SetupOrderRoutes(&h)

	// Graceful shutdown: wait for SIGINT / SIGTERM, then drain in-flight requests.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "addr", config.ServerPort)
		if err := app.Listen(config.ServerPort); err != nil {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutdown signal received — draining connections")

	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	} else {
		slog.Info("server stopped cleanly")
	}
}
