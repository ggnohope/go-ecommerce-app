// @title           Go E-Commerce API
// @version         1.0
// @description     Production-grade e-commerce REST API built with Go, Fiber v2, GORM, and PostgreSQL. Backed by AWS SES/SNS for notifications, AWS S3 for image storage, AWS SQS for order event streaming, and Stripe for payments.
// @host            localhost:8001
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     JWT access token. Format: "Bearer <token>"
package main

import (
	"go-ecommerce-app/configs"
	"go-ecommerce-app/internal/api"
	"log"

	_ "go-ecommerce-app/docs"
)

func main() {
	config, err := configs.SetupEnv()

	if err != nil {
		log.Fatalf("failed to setup env: %v", err)
	}

	api.StartServer(config)
}
