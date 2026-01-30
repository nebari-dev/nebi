package localstore

import "time"

// Workspace represents a tracked pixi workspace in the local index.
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Index is the top-level structure stored in index.json.
type Index struct {
	Workspaces map[string]*Workspace `json:"workspaces"`          // keyed by absolute path
	Servers    map[string]string     `json:"servers,omitempty"`   // name -> server URL (global)
}
