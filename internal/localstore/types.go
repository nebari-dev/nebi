package localstore

// Workspace represents a tracked pixi workspace in the local index.
type Workspace struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Global bool   `json:"global,omitempty"`
}

// Index is the top-level structure stored in index.json.
type Index struct {
	Workspaces map[string]*Workspace `json:"workspaces"`        // keyed by absolute path
	Servers    map[string]string     `json:"servers,omitempty"` // name -> server URL (global)
}
