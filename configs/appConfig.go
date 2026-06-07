package configs

import (
	"errors"
	"go-ecommerce-app/pkg/notification"
	"os"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	ServerPort          string
	DSN                 string
	AppSecret           string
	EmailNotification   notification.NotificationClient
}

func SetupEnv() (config AppConfig, err error) {
	if os.Getenv("APP_ENV") == "development" {
		godotenv.Load()
	}

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		return AppConfig{}, errors.New("missing environment variable: HTTP_PORT")
	}

	DataSourceName := os.Getenv("DATA_SOURCE_NAME")
	if DataSourceName == "" {
		return AppConfig{}, errors.New("missing environment variable: DB_SOURCE_NAME")
	}

	appSecret := os.Getenv("APP_SECRET")
	if appSecret == "" {
		return AppConfig{}, errors.New("missing environment variable: APP_SECRET")
	}

	// AWS Region
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		return AppConfig{}, errors.New("missing environment variable: AWS_REGION")
	}

	// AWS SES Email Service
	awsFromEmail := os.Getenv("AWS_SES_FROM_EMAIL")
	if awsFromEmail == "" {
		return AppConfig{}, errors.New("missing environment variable: AWS_SES_FROM_EMAIL")
	}

	emailClient, err := notification.NewAWSEmailNotification(awsRegion, awsFromEmail)
	if err != nil {
		return AppConfig{}, err
	}

	// AWS SNS SMS Service
	smsClient, err := notification.NewAWSSNSNotification(awsRegion)
	if err != nil {
		return AppConfig{}, err
	}

	// Composite client (both SMS and Email)
	compositeClient := notification.NewCompositeNotificationClient(smsClient, emailClient)

	return AppConfig{
		ServerPort:        httpPort,
		DSN:               DataSourceName,
		AppSecret:         appSecret,
		EmailNotification: compositeClient,
	}, nil
}
