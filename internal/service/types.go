package service

// CreateRequest holds parameters for creating a workspace.
type CreateRequest struct {
	Name           string
	PackageManager string
	PixiToml       string
	Source         string
	Path           string
}

// PushRequest holds parameters for pushing a new version.
type PushRequest struct {
	Tag      string
	PixiToml string
	PixiLock string
	Force    bool
}

// PushResult is returned after a successful push.
type PushResult struct {
	VersionNumber int
	Tags          []string
	ContentHash   string
	Deduplicated  bool
	Tag           string // kept for backwards compatibility
}
