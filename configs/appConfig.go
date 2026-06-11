package configs

import (
	"errors"
	"go-ecommerce-app/pkg/notification"
	"go-ecommerce-app/pkg/payment"
	"go-ecommerce-app/pkg/queue"
	"go-ecommerce-app/pkg/storage"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	ServerPort         string
	DSN                string
	AppSecret          string
	EmailNotification  notification.NotificationClient
	S3Client           *storage.S3Client
	SQSClient          *queue.SQSClient
	StripeClient       *payment.StripeClient
}

func SetupEnv() (AppConfig, error) {
	if os.Getenv("APP_ENV") == "development" {
		godotenv.Load()
	}

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		return AppConfig{}, errors.New("missing environment variable: HTTP_PORT")
	}

	dsn := os.Getenv("DATA_SOURCE_NAME")
	if dsn == "" {
		return AppConfig{}, errors.New("missing environment variable: DATA_SOURCE_NAME")
	}

	appSecret := os.Getenv("APP_SECRET")
	if appSecret == "" {
		return AppConfig{}, errors.New("missing environment variable: APP_SECRET")
	}

	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		return AppConfig{}, errors.New("missing environment variable: AWS_REGION")
	}

	awsFromEmail := os.Getenv("AWS_SES_FROM_EMAIL")
	if awsFromEmail == "" {
		return AppConfig{}, errors.New("missing environment variable: AWS_SES_FROM_EMAIL")
	}

	emailClient, err := notification.NewAWSEmailNotification(awsRegion, awsFromEmail)
	if err != nil {
		return AppConfig{}, err
	}
	smsClient, err := notification.NewAWSSNSNotification(awsRegion)
	if err != nil {
		return AppConfig{}, err
	}
	notificationClient := notification.NewCompositeNotificationClient(smsClient, emailClient)

	// AWS S3 (optional — image uploads disabled if not configured)
	var s3Client *storage.S3Client
	if bucket := os.Getenv("AWS_S3_BUCKET"); bucket != "" {
		s3Client, err = storage.NewS3Client(awsRegion, bucket)
		if err != nil {
			log.Printf("WARNING: S3 client init failed: %v — image uploads disabled", err)
		}
	} else {
		log.Println("WARNING: AWS_S3_BUCKET not set — image uploads disabled")
	}

	// AWS SQS (optional — order events disabled if not configured)
	var sqsClient *queue.SQSClient
	if queueURL := os.Getenv("AWS_SQS_ORDER_QUEUE_URL"); queueURL != "" {
		sqsClient, err = queue.NewSQSClient(awsRegion, queueURL)
		if err != nil {
			log.Printf("WARNING: SQS client init failed: %v — order events disabled", err)
		}
	} else {
		log.Println("WARNING: AWS_SQS_ORDER_QUEUE_URL not set — order events disabled")
	}

	// Stripe (optional — payment intents disabled if not configured)
	var stripeClient *payment.StripeClient
	if secretKey := os.Getenv("STRIPE_SECRET_KEY"); secretKey != "" {
		webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
		stripeClient = payment.NewStripeClient(secretKey, webhookSecret)
	} else {
		log.Println("WARNING: STRIPE_SECRET_KEY not set — payment intents disabled")
	}

	return AppConfig{
		ServerPort:        httpPort,
		DSN:               dsn,
		AppSecret:         appSecret,
		EmailNotification: notificationClient,
		S3Client:          s3Client,
		SQSClient:         sqsClient,
		StripeClient:      stripeClient,
	}, nil
}
