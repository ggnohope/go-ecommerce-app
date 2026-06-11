package api

import (
	"go-ecommerce-app/configs"
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/api/rest/handlers"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/helper"
	"log"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func StartServer(config configs.AppConfig) {
	app := fiber.New()

	db, err := gorm.Open(postgres.Open(config.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Database connection error: %v\n", err)
	}
	log.Println("Database connection established")

	db.AutoMigrate(
		&domain.User{},
		&domain.Category{},
		&domain.Product{},
		&domain.ProductImage{},
		&domain.Cart{},
		&domain.CartItem{},
		&domain.Address{},
		&domain.Order{},
		&domain.OrderItem{},
	)

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

	log.Printf("Server starting on %s", config.ServerPort)
	app.Listen(config.ServerPort)
}
