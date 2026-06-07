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

	db.AutoMigrate(&domain.User{})

	auth := helper.NewAuth(config.AppSecret)
	restHandler := rest.RestHandler{
		App:                app,
		DB:                 db,
		Auth:               auth,
		NotificationClient: config.EmailNotification,
	}

	handlers.SetupUserRoutes(&restHandler)

	app.Listen(config.ServerPort)
}
