FROM ubuntu:24.04

# Install dependencies
RUN apt-get update && \
    apt-get install -y curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Install pixi
RUN curl -fsSL https://pixi.sh/install.sh | bash

# Add pixi to PATH
ENV PATH="/root/.pixi/bin:${PATH}"

# Verify installation
RUN pixi --version

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash"]
