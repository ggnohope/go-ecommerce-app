.PHONY: server build dev install-dev swagger migrate-up migrate-down migrate-status migrate-create seed

build:
	go build -o bin/ecommerce main.go

server:
	APP_ENV=development go run main.go

dev:
	@command -v air >/dev/null 2>&1 || { echo "Installing air..."; go install github.com/air-verse/air@latest; }
	APP_ENV=development air

install-dev:
	go install github.com/air-verse/air@latest

swagger:
	@command -v swag >/dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	$(shell go env GOPATH)/bin/swag init -g main.go -o docs --parseDependency --parseInternal

# ── Database migrations & seeding ───────────────────────────────────────────
migrate-up:
	APP_ENV=development go run ./cmd/migrate up

# Roll back the last migration, or N with: make migrate-down n=3
migrate-down:
	APP_ENV=development go run ./cmd/migrate down $(n)

migrate-status:
	APP_ENV=development go run ./cmd/migrate status

# Scaffold a new migration pair: make migrate-create name=create_reviews
migrate-create:
	@test -n "$(name)" || { echo "usage: make migrate-create name=<title>"; exit 1; }
	@dir=internal/database/migrations; \
	 next=$$(printf "%03d" $$(($$(ls $$dir/*.up.sql 2>/dev/null | wc -l | tr -d ' ') + 1))); \
	 up=$$dir/$${next}_$(name).up.sql; down=$$dir/$${next}_$(name).down.sql; \
	 touch $$up $$down; \
	 echo "created $$up"; echo "created $$down"

seed:
	APP_ENV=development go run ./cmd/seed
