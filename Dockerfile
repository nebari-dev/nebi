# Multi-stage Dockerfile
# Stage 1: Build frontend
# Keep the Node version in sync with NODE_VERSION in .github/tool-versions.env
FROM node:22.23.4-alpine3.23@sha256:6abbcf5dd6b231da109bcde1181db1d92bf78015ae1a9514d9807c1f857869db AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
COPY frontend/apps/client/package.json ./apps/client/package.json
COPY frontend/apps/server/package.json ./apps/server/package.json
COPY frontend/packages/ui/package.json ./packages/ui/package.json
RUN npm ci --prefer-offline --no-audit
COPY frontend/ ./
RUN npm run build

# Stage 2: Prepare Go source and generated assets
# Pinned by exact version + digest; keep in sync with the toolchain directive
# in go.mod (scripts/check-go-toolchain.sh gates supported lines in CI).
FROM golang:1.26.10-alpine@sha256:27f2d3b83cfc5803c8078f8e730af0f7c431bf067afaec8f60a36fb763187cba AS backend-base
WORKDIR /app

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Install build tools for API docs generation
COPY .github/tool-versions.env .github/tool-versions.env
RUN apk add --no-cache make && \
    . .github/tool-versions.env && \
    go install github.com/swaggo/swag/cmd/swag@${SWAG_VERSION}

# Copy source code
COPY . .

# Copy frontend build
COPY --from=frontend-builder /app/frontend/dist ./internal/frontend/dist

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
FROM ghcr.io/prefix-dev/pixi:0.82.1-noble@sha256:f83a5fc6d1bb9a2ced27faf2b26c04c5eb618eed22fe13eb60f9225db4817ce0 AS runtime-base
WORKDIR /app

# Pull in security updates for packages from the pinned base image (e.g.
# openssl), then install CA certificates (required for OIDC/HTTPS connections).
# CI passes the build date so a cached copy of this layer can't pin stale
# packages for more than a day.
ARG APT_REFRESH_DATE=unset
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

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
