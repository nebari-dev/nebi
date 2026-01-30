package localstore

import (
	"crypto/sha256"
	"fmt"
)

// ContentHash returns the hex-encoded SHA-256 hash of the given data.
func ContentHash(data string) string {
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}
