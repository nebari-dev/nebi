---
sidebar_position: 4
---

# Server Setup

Nebi includes a built-in server for team collaboration. This page covers how to run and configure it.

## Admin Credentials

Before starting the server for the first time, set `ADMIN_USERNAME` and `ADMIN_PASSWORD`. Nebi uses these to create the initial admin account for authentication.

![Nebi login screen](/img/login-nebi.png)

You (and your team) will use these credentials to log in via `nebi login` or the web UI.

You can set these credentials in your shell or in a `.env` file.

### Option A: Export in your shell

Set the variables directly in your terminal session before starting the server:

```bash
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=your-password
```

### Option B: Use a `.env` file

Create a `.env` file in the directory where you run `nebi serve`:

```bash
# .env
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your-password
```

## Running the Server

Start the server:

```bash
nebi serve
```

By default the server starts on [localhost:8460](http://localhost:8460). To use a different port, use the `--port` flag:

```bash
nebi serve --port 9000
```

Once the server is running, authenticate from any client machine with [`nebi login`](./cli-guide.md#connect-to-a-server).

## API Documentation

The Swagger API docs are available at [http://localhost:8460/docs](http://localhost:8460/docs).

## What's Next

- See the [CLI Guide](./cli-guide.md) for push/pull examples
- Check the [CLI Reference](./cli-reference.md) for all available commands
