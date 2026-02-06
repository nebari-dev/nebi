package store

import "testing"

func TestTomlContentHash_WhitespaceInsensitive(t *testing.T) {
	compact := "[project]\nname = \"test\"\nchannels = [\"conda-forge\"]\n"
	spacious := "[project]\nname   =   \"test\"\nchannels   =   [\"conda-forge\"]\n"

	h1, err := TomlContentHash(compact)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := TomlContentHash(spacious)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("expected same hash, got %s vs %s", h1, h2)
	}
}

func TestTomlContentHash_DifferentContent(t *testing.T) {
	h1, _ := TomlContentHash("[project]\nname = \"alpha\"\n")
	h2, _ := TomlContentHash("[project]\nname = \"beta\"\n")
	if h1 == h2 {
		t.Error("expected different hashes")
	}
}

func TestTomlContentHash_InvalidTOML(t *testing.T) {
	_, err := TomlContentHash("not [valid toml ===")
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestContentHash(t *testing.T) {
	h := ContentHash("hello")
	if h == "" {
		t.Error("expected non-empty hash")
	}
	if ContentHash("hello") != ContentHash("hello") {
		t.Error("same input should produce same hash")
	}
	if ContentHash("hello") == ContentHash("world") {
		t.Error("different input should produce different hash")
	}
}
