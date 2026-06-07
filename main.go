package main

import (
	"go-ecommerce-app/configs"
	"go-ecommerce-app/internal/api"
	"log"
)

func main() {
	config, err := configs.SetupEnv()

	if err != nil {
		log.Fatalf("failed to setup env: %v", err)
	}

	api.StartServer(config)
}
