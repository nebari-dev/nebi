# Nebi Server

The Nebi server is a hosted web interface to manage Nebi workspaces in a team. It has a similar interface as the local desktop, but with more features for teams and organizations.

This page covers how to run and configure it.

<!-- TODO: Embed video walkthrough of server UI, created with https://github.com/nebari-dev/nebi-video-demo-automation. Update the link in the following iframe. -->

<!-- <iframe width="560" height="315" src="" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe> -->

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

Once the server is running, authenticate from any client machine with [`nebi login`](./cli-team.md#connect-to-a-server).

## API Documentation

The Swagger API docs are available at [http://localhost:8460/docs](http://localhost:8460/docs).

## What's Next

- See the [CLI Team Workflows](./cli-team.md) for push/pull examples
- Check the [CLI Reference](./cli-reference.md) for all available commands
