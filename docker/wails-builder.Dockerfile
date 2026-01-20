# Wails Desktop Builder Image
# Used for building Darb desktop app in CI and locally
FROM ubuntu:24.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies for Wails
RUN apt-get update && apt-get install -y \
    # Build essentials
    build-essential \
    pkg-config \
    # GTK and WebKit for Wails (4.1 for Ubuntu 24.04+)
    libgtk-3-dev \
    libwebkit2gtk-4.1-dev \
    # Additional Wails dependencies
    libayatana-appindicator3-dev \
    # Git for version info
    git \
    # Utilities
    curl \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 20.x (required by Vite)
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    rm -rf /var/lib/apt/lists/*

# Install Go
ARG GO_VERSION=1.24.0
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/opt/go/bin:${PATH}"
ENV GOPATH="/opt/go"
ENV GOCACHE="/tmp/go-cache"

# Install Wails CLI and swag to shared location
RUN mkdir -p /opt/go/bin && \
    GOPATH=/opt/go go install github.com/wailsapp/wails/v2/cmd/wails@latest && \
    GOPATH=/opt/go go install github.com/swaggo/swag/cmd/swag@latest && \
    chmod -R 755 /opt/go

# Verify installations
RUN go version && wails version && node --version && npm --version

WORKDIR /app

# Default command shows help
CMD ["wails", "doctor"]
