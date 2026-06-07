package rest

import (
	"go-ecommerce-app/internal/helper"
	"go-ecommerce-app/pkg/notification"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type RestHandler struct {
	App                *fiber.App
	DB                 *gorm.DB
	Auth               *helper.Auth
	NotificationClient notification.NotificationClient
}
