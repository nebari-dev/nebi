.PHONY: help build build-frontend build-backend run swagger migrate test clean install-tools dev build-docker-pixi build-docker-uv build-docker test-pkgmgr build-all up down generate-cli-client build-cli build-cli-all build-desktop run-desktop-linux-amd64 build-wails-builder

# Variables
BINARY_NAME=darb-server
CLI_BINARY_NAME=darb
FRONTEND_DIR=frontend
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"
CLI_LDFLAGS=-ldflags "-X github.com/openteams-ai/darb/cmd/darb-cli/cmd.Version=$(VERSION)"
OPENAPI_GENERATOR_VERSION=7.10.0

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
	@command -v swag >/dev/null 2>&1 || { echo "swag not found, installing..."; go install github.com/swaggo/swag/cmd/swag@latest; }
	@PATH="$$PATH:$$(go env GOPATH)/bin" swag init -g cmd/server/main.go -o docs
	@echo "Swagger docs generated at /docs"

generate-cli-client: swagger ## Generate Go client from OpenAPI spec
	@echo "Generating CLI client (OpenAPI Generator v$(OPENAPI_GENERATOR_VERSION) via Docker)..."
	@mkdir -p cli/client
	@docker run --rm -u $(shell id -u):$(shell id -g) -v $(PWD):/local openapitools/openapi-generator-cli:v$(OPENAPI_GENERATOR_VERSION) generate \
		-i /local/docs/swagger.json \
		-g go \
		-o /local/cli/client \
		--additional-properties=packageName=client,isGoSubmodule=true,withGoMod=false \
		--global-property=apiTests=false,modelTests=false
	@echo "CLI client generated at cli/client/"

build-cli: generate-cli-client ## Build the CLI binary
	@echo "Building CLI..."
	@mkdir -p $(BUILD_DIR)
	@go build $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY_NAME) ./cmd/darb-cli
	@echo "CLI build complete: $(BUILD_DIR)/$(CLI_BINARY_NAME)"

build-cli-all: generate-cli-client ## Build CLI for all platforms
	@echo "Building CLI for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@echo "Building linux/amd64..."
	@GOOS=linux GOARCH=amd64 go build $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY_NAME)-linux-amd64 ./cmd/darb-cli
	@echo "Building linux/arm64..."
	@GOOS=linux GOARCH=arm64 go build $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY_NAME)-linux-arm64 ./cmd/darb-cli
	@echo "Building darwin/amd64..."
	@GOOS=darwin GOARCH=amd64 go build $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY_NAME)-darwin-amd64 ./cmd/darb-cli
	@echo "Building darwin/arm64..."
	@GOOS=darwin GOARCH=arm64 go build $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY_NAME)-darwin-arm64 ./cmd/darb-cli
	@echo "Building windows/amd64..."
	@GOOS=windows GOARCH=amd64 go build $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY_NAME)-windows-amd64.exe ./cmd/darb-cli
	@echo "All CLI platform builds complete"

build-frontend: ## Build frontend and copy to internal/web/dist
	@echo "Building frontend..."
	@cd $(FRONTEND_DIR) && npm install && npm run build
	@echo "Copying frontend build to internal/web/dist..."
	@rm -rf internal/web/dist
	@cp -r $(FRONTEND_DIR)/dist internal/web/dist
	@echo "Frontend build complete"

build-backend: swagger ## Build backend with embedded frontend
	@echo "Building backend with embedded frontend..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build: build-frontend build-backend ## Build complete single binary (frontend + backend)
	@echo "Single binary build complete: $(BUILD_DIR)/$(BINARY_NAME)"

run: build ## Run the server (without hot reload)
	@echo "Starting darb server..."
	@if [ -f .env ]; then \
		echo "✓ Loading environment variables from .env..."; \
	fi
	@bash -c 'set -a; [ -f .env ] && source .env; set +a; $(BUILD_DIR)/$(BINARY_NAME)'

dev: swagger ## Run with hot reload (frontend + backend)
	@echo "Starting darb in development mode with hot reload..."
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Frontend dependencies not found. Installing..."; \
		cd frontend && npm install; \
	fi
	@echo ""
	@if [ -f .env ]; then \
		echo "✓ Loading environment variables from .env..."; \
	else \
		echo "⚠️  Warning: .env file not found. Using defaults."; \
	fi
	@echo "🚀 Starting services..."
	@echo "  Frontend: http://localhost:8461"
	@echo "  Backend:  http://localhost:8460"
	@echo "  API Docs: http://localhost:8460/docs"
	@echo ""
	@echo "Press Ctrl+C to stop all services"
	@echo ""
	@command -v air >/dev/null 2>&1 || { echo "air not found, installing..."; go install github.com/air-verse/air@latest; }
	@bash -c 'export PATH="$$PATH:$$(go env GOPATH)/bin"; set -a; [ -f .env ] && source .env; set +a; trap "kill 0" EXIT; (cd frontend && npm run dev) & air'

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
	@rm -rf internal/web/dist
	@rm -rf $(FRONTEND_DIR)/dist
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

build-all: build-frontend ## Build binaries for all platforms
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@echo "Building linux/amd64..."
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/server
	@echo "Building linux/arm64..."
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/server
	@echo "Building darwin/amd64..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/server
	@echo "Building darwin/arm64..."
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/server
	@echo "Building windows/amd64..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/server
	@echo "All platform builds complete"

# K3d Development Environment
up: ## Create k3d cluster and start Tilt (recreates cluster if exists)
	@echo "Setting up Darb development environment..."
	@if k3d cluster list | grep -q darb-dev; then \
		echo ""; \
		echo "⚠️  Cluster 'darb-dev' already exists"; \
		read -p "Do you want to delete and recreate it? [y/N] " confirm; \
		if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
			echo "Deleting existing cluster..."; \
			k3d cluster delete darb-dev; \
		else \
			echo "Using existing cluster..."; \
		fi; \
	fi
	@if ! k3d cluster list | grep -q darb-dev; then \
		echo "Creating k3d cluster 'darb-dev'..."; \
		k3d cluster create -c k3d-config.yaml --wait; \
		kubectl wait --for=condition=ready node --all --timeout=60s; \
		echo "✓ Cluster ready!"; \
		kubectl get nodes; \
	fi
	@echo ""
	@echo "Starting Tilt..."
	@tilt up

down: ## Stop Tilt and delete k3d cluster
	@echo "Stopping Tilt..."
	@tilt down || true
	@echo "Deleting k3d cluster 'darb-dev'..."
	@k3d cluster delete darb-dev || true
	@echo "✓ Environment cleaned up!"

# Desktop App Targets (Docker-based builds for consistency)
DESKTOP_BINARY_NAME=darb-desktop
WAILS_BUILDER_IMAGE=ghcr.io/openteams-ai/darb-wails-builder:latest

build-wails-builder: ## Build the Wails builder Docker image locally
	@echo "Building Wails builder image..."
	@docker build -f docker/wails-builder.Dockerfile -t $(WAILS_BUILDER_IMAGE) .
	@echo "Builder image ready: $(WAILS_BUILDER_IMAGE)"

build-desktop: build-frontend swagger ## Build desktop app for Linux using Docker
	@echo "Building desktop app using Docker..."
	@mkdir -p $(BUILD_DIR) .cache/go-mod .cache/go-build
	@docker run --rm \
		-v $(PWD):/app \
		-v $(PWD)/.cache/go-mod:/go/pkg/mod \
		-v $(PWD)/.cache/go-build:/tmp/go-cache \
		-w /app \
		-e GOMODCACHE=/go/pkg/mod \
		-e GOCACHE=/tmp/go-cache \
		-u $(shell id -u):$(shell id -g) \
		$(WAILS_BUILDER_IMAGE) \
		bash -c "cd /app/cmd/desktop && wails build -tags webkit2_41 -platform linux/amd64 -ldflags '-X main.Version=$(VERSION) -X github.com/openteams-ai/darb/desktop.Version=$(VERSION)'"
	@cp cmd/desktop/build/bin/darb-desktop $(BUILD_DIR)/$(DESKTOP_BINARY_NAME)-linux-amd64
	@echo "Desktop build complete: $(BUILD_DIR)/$(DESKTOP_BINARY_NAME)-linux-amd64"

run-desktop-linux-amd64: build-desktop ## Build and run the desktop app
	@echo "Starting Darb Desktop..."
	# Use env -i to clear environment variables that can interfere with Wails/WebKit
	@env -i HOME="$$HOME" DISPLAY="$$DISPLAY" XAUTHORITY="$$XAUTHORITY" ./$(BUILD_DIR)/$(DESKTOP_BINARY_NAME)-linux-amd64
