---
sidebar_position: 7
---

# Working as a Team

This page covers multi-person workflows — how teams use Nebi to share and collaborate on environments.

## A Typical Team Workflow

Here's how a team of data scientists might use Nebi:

**Alice** creates and pushes the team's environment:

```bash
mkdir data-science && cd data-science
nebi init
# ... edit pixi.toml, add dependencies with pixi ...
nebi push data-science:v1.0
```

**Bob** pulls Alice's environment to his machine:

```bash
nebi pull data-science:v1.0 -o ./data-science
cd data-science
pixi install
nebi shell data-science
```

Bob now has the exact same environment specification as Alice. Running `pixi install` resolves the locked dependencies on his machine.

**Alice** updates the environment and pushes again:

```bash
# Add a new dependency
pixi add scikit-learn
nebi push data-science:v1.1
```

**Bob** checks for changes and pulls the update:

```bash
cd data-science
nebi status
# Shows: data-science:v1.0 has changed on server since last sync

nebi pull data-science:v1.1
pixi install
```

## Using Tags for Stability

Teams often use tags to separate stable from in-progress environments:

```bash
# Push work-in-progress
nebi push data-science:dev

# When ready, tag for production use
nebi push data-science:stable
```

Team members who want the latest stable version always pull `:stable`, while developers working on environment changes use `:dev`.

## Browsing What's Available

New team members can see what environments are shared:

```bash
# List all workspaces on the server
nebi workspace list --remote

# See what tags are available for a workspace
nebi workspace tags data-science
```

## Removing Server Workspaces

To remove a workspace from the server (does not affect local copies):

```bash
nebi workspace remove data-science --remote
```

## Tips for Teams

**Push before you share.** Always push your latest changes before telling a teammate to pull. `nebi publish` reads from the server, not your local files.

**Use meaningful tags.** Tags like `stable`, `dev`, `experiment-lstm` are more useful than version numbers alone. Content-addressed `sha-*` tags are always available for exact reproducibility.

**Check status regularly.** Run `nebi status` to see if your local workspace has diverged from what's on the server.

**Lock files matter.** Always push with a `pixi.lock` file to ensure exact reproducibility. Without it, `pixi install` may resolve different package versions on different machines. Nebi warns you if the lock file is missing during push.

**Coordinate on names.** Workspace names come from `pixi.toml` and must be unique on the server. Agree on naming conventions with your team.
