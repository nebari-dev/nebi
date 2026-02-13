---
sidebar_position: 4
---

# Server Setup

Nebi includes a built-in server for team collaboration. This page covers how to run and configure it.

## Running the Server

```bash
nebi serve
```

By default the server starts on port `8460`. You can change this with the `--port` flag:

```bash
nebi serve --port 9000
```

## Configuration

The server is configured via environment variables. Create a `.env` file in your working directory:

```bash
# .env
NEBI_PORT=8460
NEBI_JWT_SECRET=your-secret-key
NEBI_DB_PATH=nebi.db
```

## API Documentation

When running the server, Swagger API docs are available at:

```
http://localhost:8460/docs
```

## What's Next

- See the [CLI Workflows](./cli-workflows.md) guide for push/pull examples
- Check the [CLI Overview](./cli-overview.md) for all available commands
