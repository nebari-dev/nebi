package contenthash

import (
	"crypto/sha256"
	"fmt"
	"sort"
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

// AssetRef names one asset layer for bundle hashing: bundle-relative
// path (forward slashes, matches OCI AnnotationTitle) plus its OCI
// content digest (e.g. "sha256:deadbeef…"). Used by HashBundle.
type AssetRef struct {
	Path   string
	Digest string
}

// HashBundle is the content-addressed tag input for bundles that carry
// asset layers. Output matches Hash's format ("sha-" + 12 hex) so the
// OCI tag shape is stable for both legacy and bundle publishes. Assets
// are sorted by (path, digest) so slice order does not affect the tag.
// A nil or empty assets slice reduces to Hash's output over the same
// pixi files — callers can migrate a single call site without
// regenerating existing tags.
func HashBundle(pixiToml, pixiLock string, assets []AssetRef) string {
	h := sha256.New()
	h.Write([]byte(pixiToml))
	h.Write([]byte("\n---\n"))
	h.Write([]byte(pixiLock))
	if len(assets) > 0 {
		cp := append([]AssetRef(nil), assets...)
		sort.Slice(cp, func(i, j int) bool {
			if cp[i].Path != cp[j].Path {
				return cp[i].Path < cp[j].Path
			}
			return cp[i].Digest < cp[j].Digest
		})
		h.Write([]byte("\n---assets---\n"))
		for _, a := range cp {
			h.Write([]byte(a.Path))
			h.Write([]byte{0})
			h.Write([]byte(a.Digest))
			h.Write([]byte{0})
		}
	}
	return fmt.Sprintf("sha-%x", h.Sum(nil)[:6])
}
