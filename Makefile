.PHONY: help build run swagger migrate test clean install-tools dev build-docker-pixi build-docker-uv build-docker test-pkgmgr

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

install-tools: ## Install development tools (swag, air)
	@echo "Installing swag..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "Installing air..."
	@go install github.com/air-verse/air@latest
	@echo "Tools installed successfully"

swagger: ## Generate Swagger documentation
	@echo "Generating Swagger docs..."
	@swag init -g cmd/server/main.go -o docs
	@echo "Swagger docs generated at /docs"

build: swagger ## Build the binary
	@echo "Building darb..."
	@go build -o bin/darb cmd/server/main.go
	@echo "Build complete: bin/darb"

run: swagger ## Run the server (without hot reload)
	@echo "Starting darb server..."
	@go run cmd/server/main.go

dev: swagger ## Run with hot reload using Air
	@echo "Starting darb in development mode with hot reload..."
	@air

migrate: ## Run database migrations
	@echo "Running migrations..."
	@go run cmd/server/main.go migrate

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf docs/
	@rm -f darb.db
	@echo "Clean complete"

tidy: ## Tidy go.mod
	@echo "Tidying go.mod..."
	@go mod tidy

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

lint: fmt vet ## Run formatters and linters
	@echo "Lint complete"

build-docker-pixi: ## Build pixi Docker image
	@echo "Building pixi Docker image..."
	@docker build -f docker/pixi.Dockerfile -t darb-pixi:latest .
	@echo "Docker image built: darb-pixi:latest"

build-docker-uv: ## Build uv Docker image
	@echo "Building uv Docker image..."
	@docker build -f docker/uv.Dockerfile -t darb-uv:latest .
	@echo "Docker image built: darb-uv:latest"

build-docker: build-docker-pixi build-docker-uv ## Build all Docker images
	@echo "All Docker images built successfully"

test-pkgmgr: ## Test package manager operations
	@echo "Running package manager tests..."
	@go test -v ./internal/pkgmgr/...
