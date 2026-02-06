# Multi-Server Support: Pros and Cons Analysis

Should nebi support connecting a single local workspace to multiple servers, or should we simplify to a single-server model?

## Current Design

Each workspace can have multiple origins (one per server):
```go
type Workspace struct {
    Origins map[string]*Origin `json:"origins,omitempty"` // keyed by server name
}
```

This requires the `-s` flag on many commands and a "default server" concept.

---

## Pros of Multi-Server Support

### 1. Environment Promotion Workflows
Push to staging server for testing, then push same workspace to production server.
```bash
nebi push myenv:v1.0 -s staging
# test...
nebi push myenv:v1.0 -s production
```

### 2. Cross-Organization Collaboration
Pull from a vendor's public server, customize locally, push to your company's private server.
```bash
nebi pull vendor-ml:v2.0 -s public-hub
# customize...
nebi push our-ml:v1.0 -s company
```

### 3. Mirrors Git's Multi-Remote Model
Developers are already familiar with `git remote add` and pushing to multiple remotes. Similar mental model.

### 4. Personal + Team Servers
Keep personal experimental workspaces on one server, push polished ones to the team server.

### 5. Backup/Redundancy
Could sync important workspaces to multiple servers for redundancy.

---

## Cons of Multi-Server Support

### 1. CLI Complexity
Almost every command needs `-s server` flag consideration:
```bash
nebi push myenv:v1.0 -s work      # which server?
nebi pull myenv:v1.0 -s work      # which server?
nebi diff myenv:v1 myenv:v2 -s work
nebi workspace list -s work
nebi workspace tags myenv -s work
```

With single server, all of these could drop the `-s` flag entirely.

### 2. Mental Overhead
"Which server did I push this to?" "Is my local in sync with staging or production?" The cognitive load increases with each server.

### 3. Default Server Indirection
Need a "default server" concept to avoid typing `-s` everywhere, but this adds another layer:
- What's my default?
- Did I forget to set it?
- Is this command going to the server I think it is?

### 4. Origin Ambiguity
With multiple origins, `nebi status` shows multiple sync states:
```
Origins:
  staging → myenv:v1.0 (push, 2025-06-15)
  production → myenv:v1.0 (push, 2025-06-10)
```
Which one matters? Are they the same version? It's not immediately clear.

### 5. Shorthand Commands Become Ambiguous
Commands like `nebi push :v2.0` (reuse workspace name from origin) - which origin? Currently uses the default server's origin, but this is implicit.

### 6. Name Collision Confusion
Same workspace name could exist on multiple servers with completely different content. Easy to get confused.

### 7. Implementation Complexity
More code paths, more edge cases, more tests needed for multi-server scenarios.

---

## Single-Server Alternative

### What It Would Look Like
```bash
nebi server set https://nebi.company.com   # one server, period
nebi login
nebi push myenv:v1.0                        # no -s needed, ever
nebi pull myenv:v1.0
nebi diff
nebi workspace list                         # always shows server workspaces
```

### Simplified Mental Model
- One workspace ↔ one server relationship
- No ambiguity about which server
- `nebi status` shows one sync state
- All commands shorter and cleaner

### How to Handle Multiple Servers?
If someone truly needs multiple servers:
1. Use separate local directories for each server relationship
2. Or: `nebi server switch production` to change the active server (like kubectl context)

---

## Questions to Consider

1. **How common is multi-server actually?** In practice, do teams use multiple nebi servers, or is it typically one org = one server?

2. **Is environment promotion a real use case?** Or do teams just use tags (v1.0-staging, v1.0-prod) on a single server?

3. **Would context switching suffice?** Instead of multi-origin, could `nebi server use <name>` (like kubectl) cover the rare multi-server cases?

4. **What do similar tools do?**
   - Docker: single registry per image (you re-tag to push to different registries)
   - npm: single registry (you publish once)
   - Helm: single repo per chart
   - Most choose simplicity over multi-target flexibility

---

## Recommendation Options

### Option A: Keep Multi-Server (Current)
Keep the current design but improve UX:
- Better status output showing which server is "primary"
- Warnings when servers are out of sync
- Consider making `-s` required (no default) to force explicitness

### Option B: Single Server with Context Switching
Simplify to one active server at a time:
- `nebi server use <name>` switches context
- All commands work against the active server
- Workspace tracks origin for that one server only
- Simpler CLI, simpler mental model

### Option C: Single Server, Period
One server per nebi installation:
- `nebi server set <url>` (only one allowed)
- Absolute simplicity
- Multi-server users use separate machines/VMs/containers

---

## Summary Table

| Aspect | Multi-Server | Single Server |
|--------|--------------|---------------|
| CLI simplicity | `-s` flags everywhere | Clean, no flags |
| Mental model | Complex (which server?) | Simple (one server) |
| Flexibility | High | Lower |
| Real-world need | Uncertain | Covers most cases? |
| Implementation | More complex | Simpler |
| Error potential | Higher (wrong server) | Lower |
| Matches git model | Yes (multi-remote) | No |
| Matches npm/docker | No | Yes |
