FROM python:3.12-slim

# Install uv
RUN pip install --no-cache-dir uv

# Verify installation
RUN uv --version

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash"]
