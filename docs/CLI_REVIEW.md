# Server/Local Name Mismatch Confusion
It seems confusing that the name of the workspace on the server doesn't have to match the name of the workspace locally.  Why is that necessary again?  Maybe it could be okay if we add an easy way to see the the origin name(s).

```bash
[balast@nirvana my-pixi-env]$ nebi ws list -s test-server
NAME               STATUS  OWNER  UPDATED
test-ws            ready   admin  2026-02-05 15:18

[balast@nirvana my-pixi-env]$ nebi ws list
NAME         TYPE    PATH
my-pixi-env  local   /home/balast/eph/nebi/my-pixi-env

[balast@nirvana my-pixi-env]$ nebi status
Workspace: my-pixi-env
Type:      local
Path:      /home/balast/eph/nebi/my-pixi-env

Origins:
  test-server → test-ws:v1 (pull, 2026-02-05T15:23:14Z)
    In sync with test-ws:v1
```

Seems like this is a consequence of global envs needing a unique name.

## Explanation

This is intentional by design. Here's how it works:

### Local names are directory-based
In `cmd/nebi/init.go:64`, the local workspace name is automatically derived from the directory basename:
```go
name := filepath.Base(cwd)  // Name comes from directory name
```

### Server names are tracked via "Origin"
The `Origin` struct in `internal/localstore/types.go` stores server-side metadata separately:
```go
type Origin struct {
    Name      string `json:"name"`      // workspace name ON SERVER
    Tag       string `json:"tag"`       // tag that was pushed/pulled
    ...
}
```
Origins are stored per-server in the workspace's `Origins map[string]*Origin`.

### Use cases this enables
- **Multi-server**: One local workspace can map to different names on different servers (e.g., `ml-pipeline` on prod, `ml-test` on staging)
- **Team conventions**: Server names can follow team conventions while local directories reflect personal organization
- **Fork workflows**: Pull `alice/ml-project:v1`, work locally in `my-experiments/`, push back to `bob/ml-fork:v1`

### Current visibility
`nebi status` already shows the mapping via the Origins section:
```
Origins:
  test-server → test-ws:v1 (pull, 2026-02-05T15:23:14Z)
```

### Possible improvements
If we want to make this clearer:
1. Add origin info to `nebi ws list` output (show a column with server mappings)
2. Show a note on first push when local name ≠ server name
3. Add documentation explaining this is intentional

# 