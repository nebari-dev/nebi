# Contributing to Nebi

## Development

### Running the Server Locally

To develop or test the Nebi server on your machine, install the required tools and start the dev environment:

```bash
# Install development tools
make install-tools

# Run with hot reload (frontend + backend)
# Frontend dependencies will be automatically installed if needed
make dev
```

This will start:

- **Frontend dev server** at http://localhost:8461 (with hot reload)
- **Backend API** at http://localhost:8460 (with hot reload)
- **API docs** at http://localhost:8460/docs

**Authentication:**
The checked-in `config.yaml` runs the dev server with `auth.type: none`, so there is no login and every request is an admin. To develop against a real identity provider, start the Keycloak stack from `docker-compose.yml` (see [Server Setup](docs/src/content/docs/server-setup.md#docker-compose-deployment)) and point the dev server at it with the `NEBI_AUTH_*` variables in `.env.example`. The identity provider's client must then also allow the dev frontend's `http://localhost:8461/auth/callback` redirect URI and web origin.

> **Tip**: Access the app at http://localhost:8461 for the best development experience with instant hot reload of frontend changes!

### Make Targets

```bash
make help           # Show all targets
make dev            # Run with hot reload
make build          # Build CLI, server, and local web binaries
make test           # Run tests
make swagger        # Generate API docs
```

### Desktop App

Nebi includes a desktop application built with [Wails](https://wails.io/).

**Prerequisites:**
- Go 1.25+
- Node.js 20+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)

```bash
# Install wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Ensure Go bin is in PATH (add to ~/.zshrc or ~/.bashrc for persistence)
export PATH="$PATH:$(go env GOPATH)/bin"

# Run in development mode (with hot reload)
wails dev

# Build for production
wails build
```

**Platform-specific notes:**

- **Linux (Ubuntu 24.04+):** Requires webkit2gtk-4.1 and the `webkit2_41` build tag:
  ```bash
  sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev
  wails build -tags webkit2_41
  ```

- **macOS:** No additional dependencies required.

- **Windows:** No additional dependencies required.

The built application will be in `build/bin/`.

## Project Structure

```
nebi/
├── cmd/nebi-cli/         # CLI client entry point
├── cmd/nebi-server/      # Team server entry point
├── cmd/nebi-web/         # Local web entry point
├── internal/
│   ├── api/              # HTTP handlers and routing
│   ├── auth/             # Authentication (JWT, basic auth)
│   ├── cliclient/        # HTTP client for CLI-to-server communication
│   ├── store/            # Local index, config, and credentials
│   ├── db/               # Database models and migrations
│   ├── executor/         # Job execution (local/docker)
│   ├── queue/            # Job queue (in-memory)
│   ├── server/           # Server initialization logic
│   ├── worker/           # Background job processor
│   └── pixi/             # Pixi CLI integration
└── frontend/             # React web UI
```
