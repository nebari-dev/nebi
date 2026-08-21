# Multi-stage Dockerfile
# Stage 1: Build frontend
FROM node:20.20.2-alpine3.23@sha256:fb4cd12c85ee03686f6af5362a0b0d56d50c58a04632e6c0fb8363f609372293 AS frontend-builder
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

# Build pure Go binary with CGO disabled
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /nebi ./cmd/nebi

# Stage 3: Final image with pixi
FROM ghcr.io/prefix-dev/pixi:0.76.2-noble@sha256:8b206ef57005a902cb53f50dbaa47893a4038ca269f0b00038b51f18b1313cd4
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
