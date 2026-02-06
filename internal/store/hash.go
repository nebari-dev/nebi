package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	toml "github.com/pelletier/go-toml/v2"
)

// ContentHash returns the hex-encoded SHA-256 hash of the given data.
func ContentHash(data string) string {
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}

// TomlContentHash parses data as TOML, canonicalizes it via sorted-key JSON,
// and returns the hex-encoded SHA-256 hash. This makes the hash insensitive
// to whitespace and formatting differences in the TOML source.
func TomlContentHash(data string) (string, error) {
	var m map[string]interface{}
	if err := toml.Unmarshal([]byte(data), &m); err != nil {
		return "", fmt.Errorf("parsing TOML for hashing: %w", err)
	}
	canonical, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("canonicalizing TOML for hashing: %w", err)
	}
	h := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", h), nil
}
