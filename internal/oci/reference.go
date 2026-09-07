package oci

import (
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
	orasregistry "oras.land/oras-go/v2/registry"
)

// ArtifactReference is a validated OCI artifact reference. Digest takes
// precedence over Tag when both were supplied, while Tag is retained as
// human-readable import metadata.
type ArtifactReference struct {
	Repository string
	Tag        string
	Digest     string
	PlainHTTP  bool
}

// Selector returns the immutable digest when present and otherwise the tag.
func (r ArtifactReference) Selector() string {
	if r.Digest != "" {
		return r.Digest
	}
	return r.Tag
}

// ParseArtifactReference validates an OCI reference in tag, digest, or
// tag-plus-digest form. An explicit http:// scheme opts into plain HTTP;
// scheme-less and https:// references use HTTPS.
func ParseArtifactReference(raw string) (ArtifactReference, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ArtifactReference{}, fmt.Errorf("invalid OCI reference: reference is empty")
	}

	stripped, plainHTTP := StripScheme(value)
	parsed, err := orasregistry.ParseReference(stripped)
	if err != nil {
		return ArtifactReference{}, fmt.Errorf("invalid OCI reference %q: %w", raw, err)
	}
	if parsed.Reference == "" {
		return ArtifactReference{}, fmt.Errorf("tag or digest is required in OCI reference %q", raw)
	}

	result := ArtifactReference{
		Repository: parsed.Registry + "/" + parsed.Repository,
		PlainHTTP:  plainHTTP,
	}
	if strings.Contains(stripped, "@") {
		result.Digest, err = ValidateManifestDigest(parsed.Reference)
		if err != nil {
			return ArtifactReference{}, fmt.Errorf("invalid OCI reference %q: %w", raw, err)
		}

		if tag, hasTag := tagFromDigestReference(stripped); hasTag {
			result.Tag = tag
			result.Tag, err = ValidateArtifactTag(result.Tag)
			if err != nil {
				return ArtifactReference{}, fmt.Errorf("invalid OCI reference %q: %w", raw, err)
			}
		}
	} else {
		result.Tag = parsed.Reference
	}

	return result, nil
}

// ValidateArtifactTag normalizes and validates an OCI distribution tag.
func ValidateArtifactTag(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	ref := orasregistry.Reference{Reference: value}
	if err := ref.ValidateReferenceAsTag(); err != nil {
		return "", fmt.Errorf("invalid tag %q: %w", value, err)
	}
	return value, nil
}

// ValidateManifestDigest normalizes and validates the sha256 manifest digest
// accepted by Nebi's import API.
func ValidateManifestDigest(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := digest.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid digest %q: %w", value, err)
	}
	if parsed.Algorithm() != digest.SHA256 {
		return "", fmt.Errorf("manifest digest must use sha256, got %q", parsed.Algorithm())
	}
	return parsed.String(), nil
}

// VerifyManifestDigest rejects an unexpected manifest after a digest-pinned
// pull. Empty expected values represent tag-only requests and require no check.
func VerifyManifestDigest(resolved, expected string) error {
	if expected == "" {
		return nil
	}
	if resolved != expected {
		return fmt.Errorf("registry resolved manifest digest %q, expected %q", resolved, expected)
	}
	return nil
}

func tagFromDigestReference(ref string) (tag string, present bool) {
	_, path, hasPath := strings.Cut(ref, "/")
	if !hasPath {
		return "", false
	}
	name, _, _ := strings.Cut(path, "@")
	colon := strings.IndexByte(name, ':')
	if colon == -1 {
		return "", false
	}
	return name[colon+1:], true
}
