// Command seed inserts initial reference and demo data. It is idempotent and
// expects the schema to already be migrated (run `make migrate-up` first).
//
//	go run ./cmd/seed
package main

import (
	"log"

	"go-ecommerce-app/internal/database"
)

func main() {
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("seed: get sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := database.Seed(db); err != nil {
		log.Fatalf("seed: %v", err)
	}
}
