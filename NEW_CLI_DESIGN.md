# RESTless Nebi CLI Design
- no local server

# Commands used by a local solo developer (don't interact with any nebi server)
- nebi init
  - registers an existing pixi workspace in the local index and snapshots the current spec files
- nebi workspace list
  - lists what local envs you have in the local registry, with
- nebi diff
  - compare your two workspace spec files
- nebi shell

  - some of this can be implicit (e.g. wrap `pixi add`), but not like `vim pixi.toml` which is a totally valid way to modify a pixi manifest.

# Commands used by developer that interact with a server
- nebi login <server-url>
  - authenticates with a nebi server via username/password or API token (--token flag)
  - stores credentials in ~/.config/nebi/credentials.json keyed by server URL
- nebi server add <server-name> <server-url>
  - registers a nebi server globally so it can be referenced by name
  - servers are global (not per-workspace); specify which server at push/pull time
- nebi registry list -s <server-name>
  - queries a server for which registries are configured there that they have access to
- nebi workspace list -s <server-name>
  - queries a server for available workspaces
- nebi workspace tags <workspace-name> -s <server-name>
  - list tags for remote workspace
- nebi push ws:tag <path-or-local-dir-by-default>
  - pushes local dir
- nebi pull ws:tag or @sha256:... or ws:@version_num?
  - pulls a ws spec locally
- nebi publish 
  - e.g. nebi publish -s <local-server-name> -r <registry-url-or-name> <oci-repo>:<oci-tag> local-dir-or-path-or-some-ws-on-server
- nebi shell

# Local storage layout (no .nebi/ directory in tracked repos)
- ~/.local/share/nebi/ (platform equivalent on macOS/Windows)
  - index.json — registry of all tracked workspaces (paths, IDs) and global servers
  - snapshots/<workspace-id>/pixi.toml — last committed spec file copy
  - snapshots/<workspace-id>/pixi.lock — last committed lock file copy
- ~/.config/nebi/ (platform equivalent on macOS/Windows)
  - credentials.json — auth tokens keyed by server URL
