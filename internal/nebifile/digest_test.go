package nebifile

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestComputeDigest(t *testing.T) {
	// Known sha256 of "hello world\n"
	content := []byte("hello world\n")
	h := sha256.Sum256(content)
	expected := fmt.Sprintf("sha256:%x", h)

	got := ComputeDigest(content)
	if got != expected {
		t.Errorf("ComputeDigest() = %q, want %q", got, expected)
	}
}

func TestComputeDigestEmpty(t *testing.T) {
	// sha256 of empty content
	content := []byte{}
	got := ComputeDigest(content)
	// Known sha256 of empty: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	expected := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != expected {
		t.Errorf("ComputeDigest(empty) = %q, want %q", got, expected)
	}
}

func TestComputeDigestDeterministic(t *testing.T) {
	content := []byte("test content for digest")
	d1 := ComputeDigest(content)
	d2 := ComputeDigest(content)
	if d1 != d2 {
		t.Errorf("ComputeDigest is not deterministic: %q != %q", d1, d2)
	}
}

func TestComputeDigestDifferentContent(t *testing.T) {
	d1 := ComputeDigest([]byte("content a"))
	d2 := ComputeDigest([]byte("content b"))
	if d1 == d2 {
		t.Error("Different content should produce different digests")
	}
}

func TestComputeDigestFormat(t *testing.T) {
	content := []byte("test")
	got := ComputeDigest(content)

	// Should start with "sha256:"
	if len(got) < 7 || got[:7] != "sha256:" {
		t.Errorf("ComputeDigest() = %q, should start with 'sha256:'", got)
	}

	// Should be sha256: + 64 hex chars = 71 chars total
	if len(got) != 71 {
		t.Errorf("ComputeDigest() length = %d, want 71", len(got))
	}
}
