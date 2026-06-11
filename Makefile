.PHONY: server build dev install-dev swagger

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
	swag init -g main.go -o docs --parseDependency --parseInternal
