package main

import (
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/store"
)

func TestSplitImportRef(t *testing.T) {
	tests := []struct {
		ref         string
		wantAlias   string
		wantRepo    string
		wantHasHost bool
	}{
		// Host-bearing references are passed through untouched.
		{"quay.io/nebari/my-env", "", "quay.io/nebari/my-env", true},
		{"ghcr.io/myorg/data-science", "", "ghcr.io/myorg/data-science", true},
		{"localhost/my-env", "", "localhost/my-env", true},
		{"localhost:5000/my-env", "", "localhost:5000/my-env", true},
		{"registry.example.com:5000/org/my-env", "", "registry.example.com:5000/org/my-env", true},

		// A dotless host with a port is still a host: the digits after
		// the colon are what separate it from an alias prefix. This is
		// the in-cluster and docker-compose form.
		{"registry:5000/env", "", "registry:5000/env", true},
		{"registry:5000/org/env", "", "registry:5000/org/env", true},
		{"192.168.1.10:5000/env", "", "192.168.1.10:5000/env", true},

		// A slash with no alias prefix means the first segment is a
		// host, because a namespace-relative path has the same shape
		// and nothing can tell the two apart.
		{"registry/my-env", "", "registry/my-env", true},
		{"internal-reg/team/env", "", "internal-reg/team/env", true},
		{"myorg/my-env", "", "myorg/my-env", true},

		// Explicit alias prefix. The prefix is what reaches a nested
		// repository through a configured registry.
		{"myreg:my-env", "myreg", "my-env", false},
		{"myreg:myorg/my-env", "myreg", "myorg/my-env", false},

		// No alias, no host, no slash: the default registry.
		{"my-env", "", "my-env", false},

		// An all-numeric repository is indistinguishable from a port,
		// so it loses to the host reading. Degenerate, documented here
		// so a change in the rule shows up as a test failure.
		{"myreg:2024", "", "myreg:2024", true},

		// Alias named with nothing after it, from "nebi import myreg::v1".
		{"myreg:", "myreg", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			alias, repo, hasHost := splitImportRef(tt.ref)
			if alias != tt.wantAlias || repo != tt.wantRepo || hasHost != tt.wantHasHost {
				t.Errorf("splitImportRef(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.ref, alias, repo, hasHost, tt.wantAlias, tt.wantRepo, tt.wantHasHost)
			}
		})
	}
}

// Every reference that names a host must report hasHost, because those are
// the references that work today and whose meaning must not shift.
func TestSplitImportRefNeverResolvesHostReferences(t *testing.T) {
	hostRefs := []string{
		"quay.io/nebari/my-env",
		"ghcr.io/myorg/env",
		"registry.example.com/env",
		"localhost:5000/env",
		"127.0.0.1:5000/env",
		"registry:5000/env",
		"registry/my-env",
	}
	for _, ref := range hostRefs {
		if _, _, hasHost := splitImportRef(ref); !hasHost {
			t.Errorf("splitImportRef(%q) did not detect a host", ref)
		}
	}
}

func TestHasScheme(t *testing.T) {
	tests := map[string]bool{
		"http://localhost:5000/env:v1": true,
		"https://quay.io/org/env:v1":   true,
		"quay.io/org/env:v1":           false,
		"myreg:env:v1":                 false,
	}
	for ref, want := range tests {
		if got := hasScheme(ref); got != want {
			t.Errorf("hasScheme(%q) = %v, want %v", ref, got, want)
		}
	}
}

// newRegistryStore points store.New at a temp data dir and returns an open
// store, so registry resolution can be exercised without touching the
// user's real nebi state.
func newRegistryStore(t *testing.T) *store.Store {
	t.Helper()
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	s, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestResolveImportRefUsesDefaultRegistry(t *testing.T) {
	s := newRegistryStore(t)
	if err := s.CreateRegistry(&store.LocalRegistry{
		Name:      "myreg",
		URL:       "https://registry.example.com",
		Namespace: "myorg",
		IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, plainHTTP, err := resolveImportRef("", "my-env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "registry.example.com/myorg/my-env"; got != want {
		t.Errorf("resolveImportRef = %q, want %q", got, want)
	}
	if plainHTTP {
		t.Error("https registry URL should not select plain HTTP")
	}
}

func TestResolveImportRefUsesNamedRegistry(t *testing.T) {
	s := newRegistryStore(t)
	if err := s.CreateRegistry(&store.LocalRegistry{
		Name:      "default-reg",
		URL:       "https://default.example.com",
		IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRegistry(&store.LocalRegistry{
		Name: "other",
		URL:  "https://other.example.com/base",
	}); err != nil {
		t.Fatal(err)
	}

	got, _, err := resolveImportRef("other", "my-env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "other.example.com/base/my-env"; got != want {
		t.Errorf("resolveImportRef = %q, want %q", got, want)
	}
}

func TestResolveImportRefCarriesPlainHTTP(t *testing.T) {
	s := newRegistryStore(t)
	if err := s.CreateRegistry(&store.LocalRegistry{
		Name:      "local",
		URL:       "http://localhost:5000",
		IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, plainHTTP, err := resolveImportRef("", "my-env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "localhost:5000/my-env"; got != want {
		t.Errorf("resolveImportRef = %q, want %q", got, want)
	}
	if !plainHTTP {
		t.Error("http registry URL should select plain HTTP")
	}
}

func TestResolveImportRefUnknownAlias(t *testing.T) {
	newRegistryStore(t)

	_, _, err := resolveImportRef("nope", "my-env")
	if err == nil {
		t.Fatal("expected an error for an unconfigured registry name")
	}
	if !strings.Contains(err.Error(), "not found in local store") {
		t.Errorf("error should say the registry is unknown, got: %v", err)
	}
}

func TestResolveImportRefNoDefaultRegistry(t *testing.T) {
	newRegistryStore(t)

	_, _, err := resolveImportRef("", "my-env")
	if err == nil {
		t.Fatal("expected an error when no default registry is configured")
	}
	if !strings.Contains(err.Error(), "no default registry") {
		t.Errorf("error should mention the missing default, got: %v", err)
	}
}

// "nebi import myreg:my-env" parses as repository "myreg", tag "my-env",
// because the reference parser splits on the last colon. Resolving that
// against the default registry would quietly pull the wrong thing, so it
// has to be an error that names the full form.
func TestResolveImportRefRejectsRegistryNameAsRepository(t *testing.T) {
	s := newRegistryStore(t)
	if err := s.CreateRegistry(&store.LocalRegistry{
		Name:      "myreg",
		URL:       "https://registry.example.com",
		IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveImportRef("", "myreg")
	if err == nil {
		t.Fatal("expected an error when a registry name is used as a repository")
	}
	if !strings.Contains(err.Error(), "myreg:<repository>:<tag>") {
		t.Errorf("error should show the full reference form, got: %v", err)
	}
}

func TestResolveImportRefAliasWithoutRepository(t *testing.T) {
	newRegistryStore(t)

	_, _, err := resolveImportRef("myreg", "")
	if err == nil {
		t.Fatal("expected an error for an alias with no repository")
	}
}

func TestIsAllDigits(t *testing.T) {
	tests := map[string]bool{
		"5000": true,
		"0":    true,
		// Empty must be false, otherwise "myreg:" reads as a host and
		// an alias naming no repository loses its error.
		"":       false,
		"my-env": false,
		"50a0":   false,
		"5000 ":  false,
	}
	for in, want := range tests {
		if got := isAllDigits(in); got != want {
			t.Errorf("isAllDigits(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveImportRefEmptyReference(t *testing.T) {
	newRegistryStore(t)

	_, _, err := resolveImportRef("", "")
	if err == nil {
		t.Fatal("expected an error for an empty reference")
	}
	if strings.Contains(err.Error(), `""`) {
		t.Errorf("error should not quote an empty alias, got: %v", err)
	}
}
