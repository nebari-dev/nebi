---
sidebar_position: 5
---

# Set Up a Nebi Server

A Nebi server lets your team push, pull, and publish shared environments. This guide covers basic setup.

## Quick Start

The simplest way to run a server:

```bash
nebi serve
```

This starts the server on port 8460 with an SQLite database. The API docs are available at `http://localhost:8460/docs`.

### Set the Port

```bash
nebi serve --port 9000
```

### Create an Admin User

Set environment variables to bootstrap an admin account on first startup:

```bash
ADMIN_USERNAME=admin ADMIN_PASSWORD=changeme ADMIN_EMAIL=admin@company.com nebi serve
```

The admin user is only created if no users exist in the database.

## Configuration

Nebi can be configured via environment variables (with a `NEBI_` prefix), a `config.yaml` file, or both. Environment variables take precedence.

### Essential Settings

```bash
# Server
NEBI_PORT=8460

# Authentication
NEBI_AUTH_JWT_SECRET=your-secret-key    # Required for production

# Database
NEBI_DB_DRIVER=sqlite                   # or "postgres"
NEBI_DB_PATH=nebi.db                    # SQLite file path
```

### PostgreSQL (Recommended for Teams)

For multi-user deployments, use PostgreSQL instead of SQLite:

```bash
NEBI_DB_DRIVER=postgres
NEBI_DB_DSN="host=localhost port=5432 user=nebi password=secret dbname=nebi sslmode=disable"
```

### Job Queue

For distributed deployments with multiple workers, use Valkey (Redis-compatible):

```bash
NEBI_QUEUE_TYPE=valkey
NEBI_QUEUE_ADDR=localhost:6379
```

The default in-memory queue works for single-instance deployments.

### OIDC Authentication

To use an identity provider like Keycloak or Google:

```bash
NEBI_AUTH_TYPE=oidc
NEBI_AUTH_OIDC_ISSUER_URL=https://keycloak.company.com/realms/nebi
NEBI_AUTH_OIDC_CLIENT_ID=nebi
NEBI_AUTH_OIDC_CLIENT_SECRET=your-client-secret
NEBI_AUTH_OIDC_REDIRECT_URL=https://nebi.company.com/api/v1/auth/oidc/callback
```

## Docker Compose

For a production-like deployment with PostgreSQL and Valkey, the repository includes Docker Compose files:

- `docker-compose.dev.yml` — Simple single-container setup for development
- `docker-compose.yml` — Full stack with Traefik, PostgreSQL, Valkey, and multiple workers
- `docker-compose.prod.yml` — Production setup with HTTPS via Let's Encrypt

### Minimal Docker Compose

```bash
docker compose -f docker-compose.dev.yml up
```

This starts a single Nebi instance with SQLite, accessible on port 8470.

### Production Docker Compose

```bash
# Create .env from the example
cp .env.prod.example .env
# Edit .env with your domain, secrets, and OIDC config

docker compose -f docker-compose.prod.yml up -d
```

## Server Modes

The server can run in different modes:

```bash
nebi serve --mode server   # API only (no background job processing)
nebi serve --mode worker   # Worker only (no API)
nebi serve --mode both     # Both API and worker (default)
```

Split modes let you scale workers independently from the API server in larger deployments.

## Next Steps

Once the server is running, team members can connect with:

```bash
nebi login https://nebi.company.com
```

See the [CLI Guide](../cli-guide.md) for the full workflow.
