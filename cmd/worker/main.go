package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-ecommerce-app/configs"
	"go-ecommerce-app/internal/worker"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	config, err := configs.SetupEnv()
	if err != nil {
		slog.Error("worker: failed to setup env", "err", err)
		os.Exit(1)
	}
	if config.SQSClient == nil {
		slog.Error("worker: AWS_SQS_ORDER_QUEUE_URL not set — nothing to consume")
		os.Exit(1)
	}

	db, err := gorm.Open(postgres.Open(config.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		slog.Error("worker: database connection failed", "err", err)
		os.Exit(1)
	}

	dispatcher := worker.NewDispatcher(worker.NewGormOrderStore(db), config.EmailNotification)
	consumer := worker.NewConsumer(config.SQSClient, dispatcher.Handle)

	// Graceful shutdown: stop polling on SIGINT/SIGTERM. The in-flight batch
	// finishes; any message not yet deleted is safely redelivered later.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	consumer.Run(ctx)
	slog.Info("worker: stopped cleanly")
}
