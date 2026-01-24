.PHONY: help build build-frontend build-backend run swagger migrate test clean install-tools dev build-docker-pixi build-docker-uv build-docker test-pkgmgr build-all up down

# Variables
BINARY_NAME=nebi
FRONTEND_DIR=frontend
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

install-tools: ## Install development tools (swag, air, golangci-lint)
	@echo "Installing swag..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "Installing air..."
	@go install github.com/air-verse/air@latest
	@echo "Installing golangci-lint v1.64.8..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.64.8
	@echo "Tools installed successfully"

swagger: ## Generate Swagger documentation
	@echo "Generating Swagger docs..."
	@command -v swag >/dev/null 2>&1 || { echo "swag not found, installing..."; go install github.com/swaggo/swag/cmd/swag@latest; }
	@PATH="$$PATH:$$(go env GOPATH)/bin" swag init -g cmd/nebi/serve.go -o docs
	@echo "Swagger docs generated at /docs"

build-frontend: ## Build frontend and copy to internal/web/dist
	@echo "Building frontend..."
	@cd $(FRONTEND_DIR) && npm install && npm run build
	@echo "Copying frontend build to internal/web/dist..."
	@rm -rf internal/web/dist
	@cp -r $(FRONTEND_DIR)/dist internal/web/dist
	@echo "Frontend build complete"

build-backend: swagger ## Build nebi binary
	@echo "Building nebi..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/nebi
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build: build-frontend build-backend ## Build complete single binary (frontend + backend)
	@echo "Single binary build complete: $(BUILD_DIR)/$(BINARY_NAME)"

run: build ## Run the server (without hot reload)
	@echo "Starting nebi server..."
	@if [ -f .env ]; then \
		echo "✓ Loading environment variables from .env..."; \
	fi
	@bash -c 'set -a; [ -f .env ] && source .env; set +a; $(BUILD_DIR)/$(BINARY_NAME) serve'

dev: swagger ## Run with hot reload (frontend + backend)
	@echo "Starting nebi in development mode with hot reload..."
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
	@go run cmd/nebi/main.go serve

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

test-e2e: ## Run end-to-end tests (starts server in-process)
	@echo "Running E2E tests..."
	@go test -v -count=1 -tags=e2e ./cmd/nebi/ -run TestE2E

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

lint: fmt ## Run formatters and linters (matches CI)
	@echo "Running golangci-lint..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found, installing..."; curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.64.8; }
	@PATH="$$PATH:$$(go env GOPATH)/bin" golangci-lint run ./...
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
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/nebi
	@echo "Building linux/arm64..."
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/nebi
	@echo "Building darwin/amd64..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/nebi
	@echo "Building darwin/arm64..."
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/nebi
	@echo "Building windows/amd64..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/nebi
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
