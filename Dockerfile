# Multi-stage Dockerfile
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --prefer-offline --no-audit
COPY frontend/ ./
RUN npm run build

# Stage 2: Prepare Go source and generated assets
# Pinned by exact version + digest; keep in sync with the toolchain directive
# in go.mod (scripts/check-go-toolchain.sh gates supported lines in CI).
FROM golang:1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468 AS backend-base
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

# Build pure Go binaries with CGO disabled. TARGETOS/TARGETARCH are set by
# BuildKit to the build target platform; CI builds each platform on a native
# runner, so this is always a native compile there.
FROM backend-base AS nebi-server-builder
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /nebi-server ./cmd/nebi-server

FROM backend-base AS nebi-web-builder
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /nebi-web ./cmd/nebi-web

# Stage 3: Shared runtime image with pixi
FROM ghcr.io/prefix-dev/pixi:latest AS runtime-base
WORKDIR /app

# Install CA certificates (required for OIDC/HTTPS connections)
RUN apt-get update && apt-get install -y ca-certificates git && rm -rf /var/lib/apt/lists/*

# Copy RBAC configuration
COPY --from=backend-base /app/internal/rbac/model.conf /app/internal/rbac/model.conf

# Expose port
EXPOSE 8460

# Environment variables
ENV GIN_MODE=release

# Server image
FROM runtime-base AS nebi-server
COPY --from=nebi-server-builder /nebi-server /app/nebi-server
ENTRYPOINT ["/app/nebi-server"]

# Local web image
FROM runtime-base AS nebi-web
COPY --from=nebi-web-builder /nebi-web /app/nebi-web
ENTRYPOINT ["/app/nebi-web"]

# Intentionally fail if callers do not choose an image target.
FROM runtime-base AS no-default
RUN echo "Specify --target nebi-server or --target nebi-web." >&2; exit 1
