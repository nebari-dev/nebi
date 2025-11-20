# Multi-stage Dockerfile for ultra-lightweight final image
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy frontend build
COPY --from=frontend-builder /app/frontend/dist ./internal/web/dist

# Generate swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    swag init -g cmd/server/main.go -o ./docs

# Build statically linked binary
# TARGETARCH is automatically set by Docker buildx based on --platform
ARG TARGETARCH
RUN CGO_ENABLED=1 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -a \
    -ldflags '-linkmode external -extldflags "-static" -s -w -X main.Version=latest' \
    -o /darb ./cmd/server

# Stage 3: Final minimal image
# Using debian:12-slim instead of distroless to provide system dependencies for pixi
FROM debian:12-slim

# Install CA certificates and create nonroot user
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    useradd -u 65532 -m -s /bin/bash nonroot

# Copy the static binary
COPY --from=backend-builder --chown=nonroot:nonroot /darb /app/darb

# Copy RBAC configuration
COPY --from=backend-builder --chown=nonroot:nonroot /app/internal/rbac/model.conf /app/internal/rbac/model.conf

# Set working directory
WORKDIR /app

# Expose port
EXPOSE 8460

# Environment variables
ENV GIN_MODE=release \
    DATABASE_PATH=/app/data/darb.db \
    LOG_LEVEL=info

# Run as non-root user (uid:gid 65532:65532)
USER nonroot:nonroot

# Run the binary
ENTRYPOINT ["/app/darb"]
