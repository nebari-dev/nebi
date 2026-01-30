package main

import (
	"path/filepath"
	"testing"
)

func TestIsPath(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		// Paths (contain a slash)
		{"./foo", true},
		{"../bar", true},
		{"/tmp/project", true},
		{"foo/bar", true},
		{".", false},  // no slash
		{"..", false}, // no slash

		// Names (no slash)
		{"data-science", false},
		{"myworkspace", false},
		{"my_env", false},

		// Server refs (colon but no slash)
		{"myworkspace:v1", false},
		{"env:latest", false},

		// Windows-style paths (backslash = filepath.Separator on Windows)
		{`foo` + string(filepath.Separator) + `bar`, true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := isPath(tt.ref)
			if got != tt.want {
				t.Errorf("isPath(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		// Valid names
		{"data-science", false},
		{"my_env", false},
		{"env123", false},
		{"ML-Project", false},

		// Invalid: contains path separators
		{"foo/bar", true},
		{"./foo", true},
		{`foo\bar`, true},

		// Invalid: contains colon (ambiguous with server refs)
		{"env:v1", true},

		// Invalid: empty
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspaceName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkspaceName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
