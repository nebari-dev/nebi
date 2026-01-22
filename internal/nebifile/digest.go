package nebifile

import (
	"crypto/sha256"
	"fmt"
)

// ComputeDigest computes the sha256 digest of content, returning it in the
// format "sha256:<hex>". This matches the OCI layer digest format used by
// Nebi's publisher.
func ComputeDigest(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", h)
}
