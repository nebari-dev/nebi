package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	// hkdfInfo provides domain separation so this derived key is independent
	// from keys derived for other purposes (e.g. JWT signing).
	hkdfInfo = "nebi/v1/field-encryption"

	// ciphertextPrefix is prepended to encrypted values for reliable detection.
	// Format: enc:v1:<base64(nonce+ciphertext+tag)>
	ciphertextPrefix = "enc:v1:"
)

// DeriveKey derives a 32-byte AES-256 key from the given secret using HKDF-SHA256.
// The info parameter provides domain separation per NIST SP 800-56C.
func DeriveKey(secret string) ([]byte, error) {
	if secret == "" {
		return nil, fmt.Errorf("crypto: secret must not be empty")
	}

	hkdfReader := hkdf.New(sha256.New, []byte(secret), nil, []byte(hkdfInfo))
	key := make([]byte, 32)
	if _, err := hkdfReader.Read(key); err != nil {
		return nil, fmt.Errorf("crypto: hkdf key derivation failed: %w", err)
	}
	return key, nil
}

// EncryptField encrypts a plaintext string and returns "enc:v1:<base64>" format.
// Returns "" for empty input (no credential configured).
func EncryptField(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("crypto: NewGCMWithRandomNonce: %w", err)
	}

	// Seal encrypts and appends authentication tag.
	// With NewGCMWithRandomNonce, the nonce is generated internally and
	// prepended to the output.
	ciphertext := gcm.Seal(nil, nil, []byte(plaintext), nil)

	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return ciphertextPrefix + encoded, nil
}

// DecryptField decrypts an "enc:v1:<base64>" value back to plaintext.
// Returns "" for empty input. Returns the value as-is if it lacks the "enc:"
// prefix (legacy plaintext — lazy migration on next write). Logs a warning for
// unencrypted values to track migration progress.
func DecryptField(value string, key []byte) (string, error) {
	if value == "" {
		return "", nil
	}

	// Plaintext fallback: value was stored before encryption was enabled.
	if !strings.HasPrefix(value, "enc:") {
		slog.Warn("crypto: encountered unencrypted field value, will be encrypted on next write")
		return value, nil
	}

	if !strings.HasPrefix(value, ciphertextPrefix) {
		return "", fmt.Errorf("crypto: unsupported encryption version in prefix %q", value[:min(len(value), 10)])
	}

	encoded := strings.TrimPrefix(value, ciphertextPrefix)
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("crypto: NewGCMWithRandomNonce: %w", err)
	}

	plaintext, err := gcm.Open(nil, nil, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decryption failed (wrong key or corrupted data): %w", err)
	}

	return string(plaintext), nil
}
