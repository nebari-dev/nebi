package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// Embed the frontend build output
//go:embed dist/*
var frontendFS embed.FS

// GetFileSystem returns the embedded filesystem for serving frontend files
func GetFileSystem() (http.FileSystem, error) {
	// Get the dist subdirectory
	dist, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(dist), nil
}
