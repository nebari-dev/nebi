package localstore

import (
	"testing"
)

func TestTomlContentHash_WhitespaceInsensitive(t *testing.T) {
	compact := `[project]
name = "test"
channels = ["conda-forge"]
platforms = ["linux-64"]
`
	spacious := `[project]
name   =   "test"
channels   =   ["conda-forge"]
platforms   =   ["linux-64"]
`

	h1, err := TomlContentHash(compact)
	if err != nil {
		t.Fatalf("hash compact: %v", err)
	}
	h2, err := TomlContentHash(spacious)
	if err != nil {
		t.Fatalf("hash spacious: %v", err)
	}
	if h1 != h2 {
		t.Errorf("expected same hash for semantically identical TOML, got %s vs %s", h1, h2)
	}
}

func TestTomlContentHash_DifferentContent(t *testing.T) {
	a := `[project]
name = "alpha"
`
	b := `[project]
name = "beta"
`
	h1, err := TomlContentHash(a)
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	h2, err := TomlContentHash(b)
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if h1 == h2 {
		t.Error("expected different hashes for different content")
	}
}

func TestTomlContentHash_InvalidTOML(t *testing.T) {
	_, err := TomlContentHash("not [valid toml ===")
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestTomlContentHash_Empty(t *testing.T) {
	h, err := TomlContentHash("")
	if err != nil {
		t.Fatalf("unexpected error for empty string: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash for empty TOML")
	}
}

func TestTomlContentHash_TrailingNewline(t *testing.T) {
	a := `[project]
name = "test"
`
	b := `[project]
name = "test"`

	h1, err := TomlContentHash(a)
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	h2, err := TomlContentHash(b)
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if h1 != h2 {
		t.Errorf("trailing newline should not affect hash, got %s vs %s", h1, h2)
	}
}

func TestTomlContentHash_CommentInsensitive(t *testing.T) {
	withComment := `# This is a comment
[project]
name = "test"
`
	withoutComment := `[project]
name = "test"
`
	h1, err := TomlContentHash(withComment)
	if err != nil {
		t.Fatalf("hash with comment: %v", err)
	}
	h2, err := TomlContentHash(withoutComment)
	if err != nil {
		t.Fatalf("hash without comment: %v", err)
	}
	if h1 != h2 {
		t.Errorf("comments should not affect hash, got %s vs %s", h1, h2)
	}
}
