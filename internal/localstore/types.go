package localstore

// Origin records the last push/pull sync point for a workspace on a specific server.
type Origin struct {
	Name      string `json:"name"`                // workspace name on server
	Tag       string `json:"tag"`                 // tag that was pushed/pulled
	Action    string `json:"action"`              // "push" or "pull"
	TomlHash  string `json:"toml_hash"`           // SHA-256 of pixi.toml at sync time
	LockHash  string `json:"lock_hash,omitempty"` // SHA-256 of pixi.lock at sync time
	Timestamp string `json:"timestamp"`           // ISO 8601
}

// Workspace represents a tracked pixi workspace in the local index.
type Workspace struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Path    string             `json:"path"`
	Global  bool               `json:"global,omitempty"`
	Origins map[string]*Origin `json:"origins,omitempty"` // keyed by server name
}

// Index is the top-level structure stored in index.json.
type Index struct {
	Workspaces map[string]*Workspace `json:"workspaces"`        // keyed by absolute path
	Servers    map[string]string     `json:"servers,omitempty"` // name -> server URL (global)
}
