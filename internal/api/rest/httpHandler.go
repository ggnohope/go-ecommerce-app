package rest

import (
	"go-ecommerce-app/internal/helper"
	"go-ecommerce-app/pkg/notification"
	"go-ecommerce-app/pkg/payment"
	"go-ecommerce-app/pkg/queue"
	"go-ecommerce-app/pkg/storage"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type RestHandler struct {
	App                *fiber.App
	DB                 *gorm.DB
	Auth               *helper.Auth
	NotificationClient notification.NotificationClient
	S3Client           *storage.S3Client
	SQSClient          *queue.SQSClient
	StripeClient       *payment.StripeClient
}
