package contenthash

import (
	"strings"
	"testing"
)

// TestHashBundle_AssetsAffectHash — same pixi.toml/lock, different
// asset content → different hash.
func TestHashBundle_AssetsAffectHash(t *testing.T) {
	toml := "t"
	lock := "l"
	a := HashBundle(toml, lock, []AssetRef{{Path: "README.md", Digest: "sha256:aaaa"}})
	b := HashBundle(toml, lock, []AssetRef{{Path: "README.md", Digest: "sha256:bbbb"}})
	if a == b {
		t.Fatalf("asset digest change must alter the hash, got %q for both", a)
	}
	noAssets := HashBundle(toml, lock, nil)
	if noAssets == a {
		t.Fatalf("adding an asset must alter the hash, got %q for both", a)
	}
}

// TestHashBundle_AssetOrderIndependent — the caller cannot rely on the
// asset slice being pre-sorted, so HashBundle must be order-independent.
func TestHashBundle_AssetOrderIndependent(t *testing.T) {
	toml, lock := "t", "l"
	ordered := []AssetRef{
		{Path: "a.txt", Digest: "sha256:1"},
		{Path: "b.txt", Digest: "sha256:2"},
	}
	shuffled := []AssetRef{
		{Path: "b.txt", Digest: "sha256:2"},
		{Path: "a.txt", Digest: "sha256:1"},
	}
	if HashBundle(toml, lock, ordered) != HashBundle(toml, lock, shuffled) {
		t.Fatal("HashBundle must be independent of asset slice order")
	}
}

// TestHashBundle_AssetPathAffectsHash — renaming an asset while keeping
// content identical must change the hash: the file's logical identity
// is part of the bundle.
func TestHashBundle_AssetPathAffectsHash(t *testing.T) {
	toml, lock := "t", "l"
	a := HashBundle(toml, lock, []AssetRef{{Path: "README.md", Digest: "sha256:deadbeef"}})
	b := HashBundle(toml, lock, []AssetRef{{Path: "docs.md", Digest: "sha256:deadbeef"}})
	if a == b {
		t.Fatalf("renamed asset must alter hash, got %q for both", a)
	}
}

// TestHashBundle_Format — output stays in the `sha-<12hex>` form Publish
// emits today (tag stability). 16 total chars: "sha-" + 12 hex.
func TestHashBundle_Format(t *testing.T) {
	got := HashBundle("t", "l", []AssetRef{{Path: "x", Digest: "sha256:1"}})
	if !strings.HasPrefix(got, "sha-") {
		t.Fatalf("expected sha- prefix, got %q", got)
	}
	if len(got) != 16 {
		t.Fatalf("expected 16 chars, got %d: %q", len(got), got)
	}
}
