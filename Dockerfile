# Multi-stage Dockerfile
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --prefer-offline --no-audit
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
# Pinned by exact version + digest; keep in sync with the toolchain directive
# in go.mod (scripts/check-go-toolchain.sh gates supported lines in CI).
FROM golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS backend-builder
WORKDIR /app

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Install build tools for API docs generation
RUN apk add --no-cache make && go install github.com/swaggo/swag/cmd/swag@latest

# Copy source code
COPY . .

# Copy frontend build
COPY --from=frontend-builder /app/frontend/dist ./internal/web/dist

# Generate swagger docs
RUN make swagger

# Build pure Go binary with CGO disabled. TARGETOS/TARGETARCH are set by
# BuildKit to the build's target platform; CI builds each platform on a
# native runner, so this is always a native compile there.
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /nebi ./cmd/nebi

# Stage 3: Final image with pixi
FROM ghcr.io/prefix-dev/pixi:latest
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
