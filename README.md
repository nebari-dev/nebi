# Darb

**Multi-User Environment Management System**

Darb is a REST API with web UI for managing pixi (and future uv) environments in multi-user systems. It supports local, Docker, and Kubernetes deployments with a container-first execution model.

## Features

### Phase 1: Core Infrastructure ✅

- **Single Binary Deployment**: Complete application ships as one Go binary
- **Structured Logging**: Using Go's standard library `log/slog` (JSON for production, text for development)
- **Flexible Database**: SQLite for local development, PostgreSQL for production
- **Authentication**: Basic Auth with bcrypt password hashing and JWT tokens
- **Job Queue**: In-memory queue system (extensible to Redis)
- **REST API**: Clean API built with Gin framework
- **Swagger Docs**: Auto-generated API documentation at `/docs`
- **Hot Reload**: Development mode with live reload using Air

## Quick Start

### Prerequisites

- Go 1.21 or higher
- Make (optional, for convenience)

### Installation

1. Clone the repository:
```bash
git clone https://github.com/aktech/darb.git
cd darb
```

2. Install development tools:
```bash
make install-tools
```

This installs:
- `swag` - Swagger documentation generator
- `air` - Live reload for Go apps

### Running Locally

#### Option 1: Using Make (with hot reload)

```bash
make dev
```

This will:
- Generate Swagger documentation
- Start the server with hot reload on port 8080
- Use SQLite database (`darb.db`)
- Enable text-based logging

#### Option 2: Standard Go run

```bash
make run
```

Or without Make:
```bash
swag init -g cmd/server/main.go -o docs
go run cmd/server/main.go
```

#### Option 3: Build binary

```bash
make build
./bin/darb
```

### Configuration

Darb uses a `config.yaml` file or environment variables. Example configuration:

```yaml
server:
  port: 8080
  mode: development

database:
  driver: sqlite
  dsn: ./darb.db

auth:
  type: basic
  jwt_secret: change-me-in-production

queue:
  type: memory

log:
  format: text
  level: info
```

#### Environment Variables

Override config with environment variables (prefix: `DARB_`):

```bash
export DARB_SERVER_PORT=9090
export DARB_DATABASE_DRIVER=postgres
export DARB_DATABASE_DSN="host=localhost user=darb password=secret dbname=darb"
export DARB_LOG_FORMAT=json
export DARB_LOG_LEVEL=debug
```

### First Steps

1. **Start the server**:
```bash
make dev
```

2. **Check health**:
```bash
curl http://localhost:8080/api/v1/health
```

3. **View API documentation**:
Open your browser to http://localhost:8080/docs

4. **Create a test user** (requires direct database access for now):
```bash
# Using SQLite CLI
sqlite3 darb.db
```

```sql
-- Create a user (password: "password123" hashed with bcrypt)
INSERT INTO users (username, password_hash, email, created_at, updated_at)
VALUES (
  'admin',
  '$2a$10$rMN8pGH8z7kI5KqYhJxF1.WQBqvXqL0W6XHlZ8xhF7KqYhJxF1.WQ',
  'admin@example.com',
  datetime('now'),
  datetime('now')
);
```

5. **Login**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}'
```

This returns a JWT token:
```json
{
  "token": "eyJhbGc...",
  "user": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com"
  }
}
```

6. **Use the token for authenticated requests**:
```bash
export TOKEN="<your-jwt-token>"

curl http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN"
```

## Project Structure

```
darb/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── router.go            # Gin router setup
│   │   └── handlers/            # HTTP handlers
│   ├── auth/
│   │   ├── auth.go              # Auth interface
│   │   └── basic.go             # Basic auth implementation
│   ├── config/
│   │   └── config.go            # Configuration management
│   ├── db/
│   │   └── db.go                # Database setup and migrations
│   ├── logger/
│   │   └── logger.go            # Structured logging setup
│   ├── models/                  # Database models
│   │   ├── user.go
│   │   ├── role.go
│   │   ├── environment.go
│   │   ├── job.go
│   │   ├── permission.go
│   │   ├── template.go
│   │   ├── package.go
│   │   └── audit_log.go
│   └── queue/
│       ├── queue.go             # Queue interface
│       └── memory.go            # In-memory queue
├── docs/                        # Generated Swagger docs
├── config.yaml                  # Configuration file
├── .air.toml                    # Air config for hot reload
├── Makefile                     # Build commands
└── README.md
```

## API Endpoints

### Public Endpoints

- `GET /api/v1/health` - Health check
- `POST /api/v1/auth/login` - User login

### Protected Endpoints (require JWT)

- `GET /api/v1/environments` - List environments (placeholder)
- `POST /api/v1/environments` - Create environment (placeholder)
- `GET /api/v1/environments/:id` - Get environment (placeholder)
- `DELETE /api/v1/environments/:id` - Delete environment (placeholder)
- `POST /api/v1/environments/:id/packages` - Install package (placeholder)
- `DELETE /api/v1/environments/:id/packages/:package` - Remove package (placeholder)
- `GET /api/v1/jobs` - List jobs (placeholder)
- `GET /api/v1/jobs/:id` - Get job status (placeholder)
- `GET /api/v1/templates` - List templates (placeholder)

### Admin Endpoints

- `GET /api/v1/admin/users` - List users (placeholder)
- `POST /api/v1/admin/users` - Create user (placeholder)
- `GET /api/v1/admin/roles` - List roles (placeholder)
- `POST /api/v1/admin/permissions` - Grant permissions (placeholder)
- `GET /api/v1/admin/audit-logs` - View audit logs (placeholder)

## Development

### Available Make Targets

```bash
make help          # Show all available targets
make install-tools # Install swag and air
make build         # Build the binary
make run           # Run without hot reload
make dev           # Run with hot reload (recommended)
make swagger       # Generate Swagger docs
make test          # Run tests
make clean         # Clean build artifacts
make tidy          # Tidy go.mod
make fmt           # Format code
make vet           # Run go vet
make lint          # Run formatters and linters
```

### Database Schema

The database includes the following tables:

- **users**: System users with authentication
- **roles**: User roles (admin, owner, editor, viewer)
- **environments**: Package manager environments
- **jobs**: Background tasks (create env, install package, etc.)
- **permissions**: User access to environments
- **templates**: Pre-configured environment templates
- **packages**: Installed packages in environments
- **audit_logs**: Compliance and security audit trail

### Production Deployment

For production, use PostgreSQL and configure appropriately:

```yaml
server:
  port: 8080
  mode: production

database:
  driver: postgres
  dsn: "host=db.example.com user=darb password=secret dbname=darb sslmode=require"

auth:
  type: basic
  jwt_secret: <strong-random-secret>

queue:
  type: memory  # or redis for distributed deployments

log:
  format: json
  level: info
```

## Roadmap

### Phase 1: Core Infrastructure ✅ COMPLETE
- Project structure, logging, database, queue, auth, Swagger

### Phase 2: Package Manager Abstraction (Next)
- Abstract interface for pixi/uv operations
- Pixi implementation
- Container base images
- 📋 **[See PHASE2.md for detailed implementation guide](./PHASE2.md)**

### Phase 3: Backend Execution Layer
- Docker and Kubernetes runtime support
- Volume management

### Phase 4: Environment Operations
- Queued jobs for create, install, remove, delete
- Real-time log streaming

### Phase 5: RBAC & Access Control
- Casbin-based RBAC
- Admin APIs for user/permission management

### Phase 6: API & Real-time Features
- Complete REST API implementation
- WebSocket log streaming

### Phase 7: User Interface
- React frontend with TypeScript and Tailwind
- IBM Plex Sans + Fira Code typography

### Phase 8: Admin Interface
- Admin dashboard
- User and permission management UI

### Phase 9: Single Binary Deployment
- Embedded frontend
- Multi-platform releases

## Contributing

Contributions are welcome! This is currently in early development (Phase 1).

## License

MIT License
