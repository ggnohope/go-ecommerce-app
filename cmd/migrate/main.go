// Command migrate applies or rolls back SQL migrations.
//
//	go run ./cmd/migrate up         # apply all pending migrations (default)
//	go run ./cmd/migrate down [n]   # roll back the last n migrations (default 1)
//	go run ./cmd/migrate status     # show applied / pending state
package main

import (
	"log"
	"os"
	"strconv"

	"go-ecommerce-app/internal/database"
)

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	db, err := database.Connect()
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("migrate: get sql.DB: %v", err)
	}
	defer sqlDB.Close()

	switch cmd {
	case "up":
		if err := database.Up(sqlDB); err != nil {
			log.Fatalf("migrate up: %v", err)
		}
	case "down":
		steps := 1
		if len(os.Args) > 2 {
			n, convErr := strconv.Atoi(os.Args[2])
			if convErr != nil {
				log.Fatalf("migrate down: invalid step count %q", os.Args[2])
			}
			steps = n
		}
		if err := database.Down(sqlDB, steps); err != nil {
			log.Fatalf("migrate down: %v", err)
		}
	case "status":
		if err := database.Status(sqlDB); err != nil {
			log.Fatalf("migrate status: %v", err)
		}
	default:
		log.Fatalf("unknown command %q (use: up | down [n] | status)", cmd)
	}
}
