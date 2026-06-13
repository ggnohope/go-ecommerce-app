package database

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect opens a GORM connection using DATA_SOURCE_NAME. It is used by the
// standalone migrate/seed commands so they don't need to boot the full app
// (and its AWS clients). In development it loads variables from .env first.
func Connect() (*gorm.DB, error) {
	if os.Getenv("APP_ENV") == "development" {
		_ = godotenv.Load()
	}
	dsn := os.Getenv("DATA_SOURCE_NAME")
	if dsn == "" {
		return nil, errors.New("DATA_SOURCE_NAME is not set")
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
}
