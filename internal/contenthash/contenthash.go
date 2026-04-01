package contenthash

import (
	"crypto/sha256"
	"fmt"
)

// Hash computes a deterministic hash of pixi.toml + pixi.lock content.
// Returns "sha-" followed by the first 12 hex characters of the SHA-256 digest.
// This is used as the default OCI tag for publishing.
func Hash(pixiToml, pixiLock string) string {
	h := sha256.New()
	h.Write([]byte(pixiToml))
	h.Write([]byte("\n---\n"))
	h.Write([]byte(pixiLock))
	return fmt.Sprintf("sha-%x", h.Sum(nil)[:6])
}
