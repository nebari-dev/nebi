# Multi-stage Dockerfile
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --prefer-offline --no-audit
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Install swag for API docs generation
RUN go install github.com/swaggo/swag/cmd/swag@latest

# Copy source code
COPY . .

# Copy frontend build
COPY --from=frontend-builder /app/frontend/dist ./internal/web/dist

# Generate swagger docs
RUN swag init -g cmd/server/main.go -o ./docs

# Build pure Go binary with CGO disabled
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags '-s -w -X main.Version=latest' \
    -o /darb ./cmd/server

# Stage 3: Final minimal image
FROM alpine:3.19
WORKDIR /app

# Install CA certificates and create nonroot user
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 65532 nonroot

# Copy the static binary
COPY --from=backend-builder --chown=nonroot:nonroot /darb /app/darb

# Copy RBAC configuration
COPY --from=backend-builder --chown=nonroot:nonroot /app/internal/rbac/model.conf /app/internal/rbac/model.conf

# Expose port
EXPOSE 8460

# Environment variables
ENV GIN_MODE=release

# Run as non-root user
USER nonroot:nonroot

# Run the binary
ENTRYPOINT ["/app/darb"]
