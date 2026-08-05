# Multi-stage Dockerfile
# Stage 1: Build frontend
FROM node:20.20.2-alpine3.23@sha256:fb4cd12c85ee03686f6af5362a0b0d56d50c58a04632e6c0fb8363f609372293 AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --prefer-offline --no-audit
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24.13-alpine3.23@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 AS backend-builder
WORKDIR /app

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Install swag for API docs generation
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Copy source code
COPY . .

# Copy frontend build
COPY --from=frontend-builder /app/frontend/dist ./internal/web/dist

# Generate swagger docs
RUN swag init -g cmd/nebi/main.go -o ./docs --exclude output

# Build pure Go binary with CGO disabled
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /nebi ./cmd/nebi

# Stage 3: Final image with pixi
FROM ghcr.io/prefix-dev/pixi:0.40.3-noble@sha256:2ce403ab03b9882e6aeda6c1f6dce6ce83a82db213202f2708eee9059c973893
WORKDIR /app

# Install CA certificates (required for OIDC/HTTPS connections)
RUN apt-get update && apt-get install -y ca-certificates git && rm -rf /var/lib/apt/lists/*

# Copy the static binary
COPY --from=backend-builder /nebi /app/nebi

# Copy RBAC configuration
COPY --from=backend-builder /app/internal/rbac/model.conf /app/internal/rbac/model.conf

# Expose port
EXPOSE 8460

# Environment variables
ENV GIN_MODE=release

# Run the binary
ENTRYPOINT ["/app/nebi", "serve"]
