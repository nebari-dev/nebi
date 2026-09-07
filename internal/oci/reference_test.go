package oci

import (
	"strings"
	"testing"
)

const testManifestDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseArtifactReference(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		repository string
		tag        string
		digest     string
		selector   string
		plainHTTP  bool
	}{
		{
			name:       "tag",
			input:      "quay.io/nebari/data-science:v1",
			repository: "quay.io/nebari/data-science",
			tag:        "v1",
			selector:   "v1",
		},
		{
			name:       "digest",
			input:      "quay.io/nebari/data-science@" + testManifestDigest,
			repository: "quay.io/nebari/data-science",
			digest:     testManifestDigest,
			selector:   testManifestDigest,
		},
		{
			name:       "tag and digest",
			input:      "quay.io/nebari/data-science:reviewed@" + testManifestDigest,
			repository: "quay.io/nebari/data-science",
			tag:        "reviewed",
			digest:     testManifestDigest,
			selector:   testManifestDigest,
		},
		{
			name:       "plain HTTP registry with port",
			input:      "http://localhost:5000/demo/data-science:v1",
			repository: "localhost:5000/demo/data-science",
			tag:        "v1",
			selector:   "v1",
			plainHTTP:  true,
		},
		{
			name:       "HTTPS registry with port and digest",
			input:      "https://registry.example.com:5443/demo/data-science@" + testManifestDigest,
			repository: "registry.example.com:5443/demo/data-science",
			digest:     testManifestDigest,
			selector:   testManifestDigest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArtifactReference(tt.input)
			if err != nil {
				t.Fatalf("ParseArtifactReference(%q): %v", tt.input, err)
			}
			if got.Repository != tt.repository {
				t.Errorf("Repository = %q, want %q", got.Repository, tt.repository)
			}
			if got.Tag != tt.tag {
				t.Errorf("Tag = %q, want %q", got.Tag, tt.tag)
			}
			if got.Digest != tt.digest {
				t.Errorf("Digest = %q, want %q", got.Digest, tt.digest)
			}
			if got.Selector() != tt.selector {
				t.Errorf("Selector() = %q, want %q", got.Selector(), tt.selector)
			}
			if got.PlainHTTP != tt.plainHTTP {
				t.Errorf("PlainHTTP = %t, want %t", got.PlainHTTP, tt.plainHTTP)
			}
		})
	}
}

func TestParseArtifactReferenceRejectsInvalidReferences(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "missing selector", input: "quay.io/nebari/data-science", wantErr: "tag or digest is required"},
		{name: "missing repository", input: "quay.io", wantErr: "invalid OCI reference"},
		{name: "truncated sha256", input: "quay.io/nebari/data-science@sha256:abc", wantErr: "invalid digest"},
		{name: "unsupported digest algorithm", input: "quay.io/nebari/data-science@sha512:" + strings.Repeat("0", 128), wantErr: "sha256"},
		{name: "invalid tag with digest", input: "quay.io/nebari/data-science:bad tag@" + testManifestDigest, wantErr: "invalid tag"},
		{name: "empty tag with digest", input: "quay.io/nebari/data-science:@" + testManifestDigest, wantErr: "invalid tag"},
		{name: "tag containing colon with digest", input: "quay.io/nebari/data-science:v1:extra@" + testManifestDigest, wantErr: "invalid tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseArtifactReference(tt.input)
			if err == nil {
				t.Fatalf("ParseArtifactReference(%q) unexpectedly succeeded", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateManifestDigest(t *testing.T) {
	if got, err := ValidateManifestDigest("  " + testManifestDigest + "  "); err != nil {
		t.Fatalf("ValidateManifestDigest: %v", err)
	} else if got != testManifestDigest {
		t.Fatalf("ValidateManifestDigest = %q, want %q", got, testManifestDigest)
	}
}

func TestVerifyManifestDigest(t *testing.T) {
	if err := VerifyManifestDigest(testManifestDigest, ""); err != nil {
		t.Fatalf("tag-only verification: %v", err)
	}
	if err := VerifyManifestDigest(testManifestDigest, testManifestDigest); err != nil {
		t.Fatalf("matching digest verification: %v", err)
	}
	if err := VerifyManifestDigest("sha256:"+strings.Repeat("a", 64), testManifestDigest); err == nil {
		t.Fatal("mismatched digest unexpectedly passed verification")
	}
}
