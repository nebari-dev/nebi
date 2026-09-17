.PHONY: help build build-frontend build-cli build-server build-web build-backend run swagger migrate test clean install-tools dev build-docker-pixi build-docker test-pixi build-all build-platforms build-desktop

# Variables
CLI_BINARY=nebi
SERVER_BINARY=nebi-server
WEB_BINARY=nebi-web
FRONTEND_DIR=frontend
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"
BUILDFLAGS=-trimpath

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
	@echo "Installing golangci-lint v2.12.2..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.12.2
	@echo "Tools installed successfully"

swagger: ## Generate Swagger documentation
	@echo "Generating Swagger docs..."
	@PATH="$$PATH:$$(go env GOPATH)/bin"; command -v swag >/dev/null 2>&1 || { echo "swag not found, installing..."; go install github.com/swaggo/swag/cmd/swag@latest; }
	@PATH="$$PATH:$$(go env GOPATH)/bin" swag init -g main.go -d cmd/nebi-server,internal/api,internal/service,internal/models,internal/limits,internal/metrics,internal/auth -o internal/swagger --packageName swagger --exclude output,cross-platform-example
	@echo "Swagger docs generated at /internal/swagger"

build-frontend: ## Build frontend and copy to internal/web/dist
	@echo "Building frontend..."
	@cd $(FRONTEND_DIR) && npm install && npm run build
	@echo "Copying frontend build to internal/web/dist..."
	@rm -rf internal/web/dist
	@cp -r $(FRONTEND_DIR)/dist internal/web/dist
	@echo "Frontend build complete"

build-cli: ## Build CLI client binary
	@echo "Building $(CLI_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/nebi-cli
	@echo "Build complete: $(BUILD_DIR)/$(CLI_BINARY)"

build-server: build-frontend swagger ## Build team server binary
	@echo "Building $(SERVER_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY) ./cmd/nebi-server
	@echo "Build complete: $(BUILD_DIR)/$(SERVER_BINARY)"

build-web: build-frontend swagger ## Build local web binary
	@echo "Building $(WEB_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(WEB_BINARY) ./cmd/nebi-web
	@echo "Build complete: $(BUILD_DIR)/$(WEB_BINARY)"

build-backend: build-server build-web ## Build server and web binaries

build: build-cli build-server build-web ## Build CLI, server, and web binaries
	@echo "Build complete: $(BUILD_DIR)/$(CLI_BINARY), $(BUILD_DIR)/$(SERVER_BINARY), $(BUILD_DIR)/$(WEB_BINARY)"

run: build-server ## Run the team server (without hot reload)
	@echo "Starting nebi-server..."
	@if [ -f .env ]; then \
		echo "✓ Loading environment variables from .env..."; \
	fi
	@bash -c 'set -a; [ -f .env ] && source .env; set +a; $(BUILD_DIR)/$(SERVER_BINARY)'

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
	@go run ./cmd/nebi-server

test: ## Run tests (unit + e2e)
	@echo "Running tests..."
	@go test -tags=e2e -v ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf internal/swagger/
	@rm -rf internal/web/dist
	@rm -rf $(FRONTEND_DIR)/dist
	@rm -f nebi.db
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
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found, installing..."; curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.12.2; }
	@PATH="$$PATH:$$(go env GOPATH)/bin" golangci-lint run ./...
	@echo "Lint complete"

build-docker-pixi: ## Build pixi Docker image
	@echo "Building pixi Docker image..."
	@docker build -f docker/pixi.Dockerfile -t nebi-pixi:latest .
	@echo "Docker image built: nebi-pixi:latest"

build-docker: build-docker-pixi ## Build all Docker images
	@echo "All Docker images built successfully"

test-pixi: ## Test pixi operations
	@echo "Running pixi tests..."
	@go test -v ./internal/pixi/...

build-all: build build-desktop ## Build all four binaries
	@echo "All binaries built"

build-platforms: build-frontend swagger ## Build CLI/server/web binaries for all platforms
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@echo "Building linux/amd64..."
	@GOOS=linux GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-linux-amd64 ./cmd/nebi-cli
	@GOOS=linux GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-linux-amd64 ./cmd/nebi-server
	@GOOS=linux GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(WEB_BINARY)-linux-amd64 ./cmd/nebi-web
	@echo "Building linux/arm64..."
	@GOOS=linux GOARCH=arm64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-linux-arm64 ./cmd/nebi-cli
	@GOOS=linux GOARCH=arm64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-linux-arm64 ./cmd/nebi-server
	@GOOS=linux GOARCH=arm64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(WEB_BINARY)-linux-arm64 ./cmd/nebi-web
	@echo "Building darwin/amd64..."
	@GOOS=darwin GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-darwin-amd64 ./cmd/nebi-cli
	@GOOS=darwin GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-darwin-amd64 ./cmd/nebi-server
	@GOOS=darwin GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(WEB_BINARY)-darwin-amd64 ./cmd/nebi-web
	@echo "Building darwin/arm64..."
	@GOOS=darwin GOARCH=arm64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-darwin-arm64 ./cmd/nebi-cli
	@GOOS=darwin GOARCH=arm64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-darwin-arm64 ./cmd/nebi-server
	@GOOS=darwin GOARCH=arm64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(WEB_BINARY)-darwin-arm64 ./cmd/nebi-web
	@echo "Building windows/amd64..."
	@GOOS=windows GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-windows-amd64.exe ./cmd/nebi-cli
	@GOOS=windows GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY)-windows-amd64.exe ./cmd/nebi-server
	@GOOS=windows GOARCH=amd64 go build $(BUILDFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(WEB_BINARY)-windows-amd64.exe ./cmd/nebi-web
	@echo "All platform builds complete"

build-desktop: build-frontend ## Build Wails desktop app with version info
	@echo "Building desktop app..."
	@wails build -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"
	@echo "Desktop app built: build/bin/Nebi.app (executable: nebi-desktop)"
