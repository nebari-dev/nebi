# Multi-stage Dockerfile
# Stage 1: Build frontend
FROM node:22.23.4-alpine3.23@sha256:6abbcf5dd6b231da109bcde1181db1d92bf78015ae1a9514d9807c1f857869db AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --prefer-offline --no-audit
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
# Pinned by exact version + digest; keep in sync with the toolchain directive
# in go.mod (scripts/check-go-toolchain.sh gates supported lines in CI).
FROM golang:1.26.10-alpine@sha256:27f2d3b83cfc5803c8078f8e730af0f7c431bf067afaec8f60a36fb763187cba AS backend-builder
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
FROM ghcr.io/prefix-dev/pixi:0.82.1-noble@sha256:f83a5fc6d1bb9a2ced27faf2b26c04c5eb618eed22fe13eb60f9225db4817ce0
WORKDIR /app

# Pull in security updates for packages from the pinned base image (e.g.
# openssl), then install CA certificates (required for OIDC/HTTPS connections)
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

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
