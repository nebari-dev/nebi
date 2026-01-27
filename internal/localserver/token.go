package localserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateToken creates a cryptographically secure random token.
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
