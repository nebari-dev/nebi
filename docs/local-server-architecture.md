# Nebi Local Server Architecture

## Overview

The Nebi CLI and desktop app (Wails) share a single local server instance. Whichever starts first spawns the server; the other connects to it.

## Architecture

```
┌─────────────┐         ┌─────────────────┐         ┌─────────────┐
│   CLI       │         │  server.state   │         │  Wails App  │
└──────┬──────┘         └────────┬────────┘         └──────┬──────┘
       │                         │                         │
       │    ┌────────────────────┴────────────────────┐    │
       │    │  {                                      │    │
       │    │    "pid": 12345,                        │    │
       │    │    "port": 8463,                        │    │
       │    │    "token": "nebi_local_a7b3x9...",     │    │
       │    │    "started_at": "2026-01-20T10:30:00Z" │    │
       │    │  }                                      │    │
       │    └────────────────────┬────────────────────┘    │
       │                         │                         │
       ▼                         ▼                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Local Nebi Server                          │
│                                                                 │
│   - Listens on localhost:PORT (127.0.0.1 only)                  │
│   - SQLite database (~/.local/share/nebi/nebi.db)               │
│   - Validates token from server.state                           │
└─────────────────────────────────────────────────────────────────┘
```

## Connection Flow (CLI or Wails)

```
Any Nebi command
       │
       ▼
┌──────────────────────────┐
│ Read server.state        │
└────────────┬─────────────┘
             │
     ┌───────┴───────┐
     │ File exists?  │
     └───────┬───────┘
             │
      No ────┴──── Yes
      │             │
      │             ▼
      │      ┌────────────────┐
      │      │ Is PID alive?  │
      │      └───────┬────────┘
      │              │
      │       No ────┴──── Yes─|
      │       │                │
      ▼       ▼                │
┌────────────────────────────┐ │
│ Acquire spawn.lock         │ │
│ Find available port        │ │
│ Spawn server process       │ │
│ Wait for server.state      │ │
└────────────┬───────────────┘ │
             │                 │
             └────────┬────────┘
                      ▼
              ┌───────────────┐
              │ Read token    │
              │ Connect       │
              └───────────────┘
```

## Key Files

| File | Purpose | Written By |
|------|---------|------------|
| `~/.config/nebi/config.yaml` | User config (current server, remote tokens) | CLI |
| `~/.local/share/nebi/server.state` | Runtime state (pid, port, local token) | Server |
| `~/.local/share/nebi/nebi.db` | Local SQLite database | Server |
| `~/.local/share/nebi/spawn.lock` | Prevents race condition on spawn | CLI/Wails |

## Authentication

**Local server**: Token is auto-generated on server startup and written to `server.state`. CLI/Wails read this token - no user login required.

**Remote server**: User runs `nebi server login <url>`, enters credentials, token stored in `config.yaml`.

## Race Condition Handling

If CLI and Wails both try to spawn simultaneously:

1. Both attempt to acquire `spawn.lock`
2. One wins, spawns server, writes `server.state`
3. Other waits, then reads `server.state` and connects

## Port Selection

Server tries port 8460 first. If taken, tries 8461, 8462, etc. The actual port is written to `server.state`, so clients always know where to connect.