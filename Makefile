APP_NAME := whatsapp-engine
BACKEND_DIR := backend
CMD_PATH := cmd/api/main.go
BUILD_DIR := bin

ifneq (,$(wildcard ./.env))
    include .env
    export
endif

DB_URL ?= postgres://$(WHATSAPP_GO_DB_USER):$(WHATSAPP_GO_DB_PASSWORD)@localhost:$(WHATSAPP_GO_DB_PORT)/$(WHATSAPP_GO_DB_NAME)?sslmode=disable

.PHONY: help all build run test clean deps swagger migrate-up migrate-down migrate-create docker-build docker-run generate-keys backup backup-manager restore

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

all: clean swagger build

run: swagger ## Run the API server locally with updated swagger docs
	@echo "🚀 Starting $(APP_NAME)..."
	cd $(BACKEND_DIR) && go run $(CMD_PATH)

build: ## Build the native binary
	@echo "📦 Building..."
	@mkdir -p $(BACKEND_DIR)/$(BUILD_DIR)
	cd $(BACKEND_DIR) && go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)
	@echo "✅ Build completed: $(BACKEND_DIR)/$(BUILD_DIR)/$(APP_NAME)"

clean: ## Clean build artifacts and Go cache
	@echo "🧹 Cleaning..."
	@rm -rf $(BACKEND_DIR)/$(BUILD_DIR)
	cd $(BACKEND_DIR) && go clean

test: ## Run unit and integration tests
	@echo "🧪 Running tests..."
	cd $(BACKEND_DIR) && go test -v ./...

deps: ## Tidy and download Go dependencies
	@echo "📥 Managing dependencies..."
	cd $(BACKEND_DIR) && go mod tidy && go mod download

swagger: ## Regenerate Swagger documentation in backend/docs/
	@echo "📝 Generating Swagger documentation..."
	@if ! command -v swag > /dev/null; then \
		echo "❌ swag not found. Install with: go install github.com/swaggo/swag/cmd/swag@latest"; \
		exit 1; \
	fi
	cd $(BACKEND_DIR) && swag init -g $(CMD_PATH) --output docs --parseDependency --parseInternal
	@echo "✅ Documentation updated in $(BACKEND_DIR)/docs/"

migrate-up: ## Run database migrations UP
	@echo "⬆️  Running migrations UP..."
	migrate -path database/migrations -database "$(DB_URL)" -verbose up

migrate-down: ## Rollback 1 database migration DOWN
	@echo "⬇️  Running migrations DOWN..."
	migrate -path database/migrations -database "$(DB_URL)" -verbose down 1

migrate-create: ## Create a new migration file (usage: make migrate-create name=add_column)
	@if [ -z "$(name)" ]; then echo "❌ Error: Define migration name (e.g., make migrate-create name=add_column)"; exit 1; fi
	@echo "🆕 Creating new migration..."
	migrate create -ext sql -dir database/migrations -seq $(name)

docker-build: ## Build Docker core image
	@echo "🐳 Building Docker image..."
	docker build -f $(BACKEND_DIR)/Dockerfile -t $(APP_NAME):latest $(BACKEND_DIR)

docker-run: ## Run Docker core image locally
	docker run -p 8080:8080 --env-file .env $(APP_NAME):latest

generate-keys: ## Generate OpenGPG asymmetric key pair for backup
	@echo "🔐 Generating OpenGPG keypair for backup..."
	@./deployments/scripts/generate-gpg-keys.sh

backup: ## Trigger manual WhatsApp backup (OpenGPG + GFS)
	@echo "🔄 Triggering WhatsApp backup..."
	@docker compose exec chatnip-backup /usr/local/bin/backup.sh

backup-manager: ## Launch interactive Backup Manager
	@echo "🔄 Launching Backup Manager..."
	@cd deployments/scripts && ./manager.sh

restore: ## Restore backup (usage: make restore file=daily/whatsapp-backup-...)
	@echo "♻️  Restoring backup..."
	@cd deployments/scripts && ./restore.sh $(filter-out $@,$(MAKECMDGOALS))

%:
	@:
