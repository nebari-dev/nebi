package config

import (
	"os"
	"strings"
	"testing"
)

// isolate runs Load() in a config-file-free temp directory so results only
// reflect the env vars this test sets (not any local config.yaml/.env).
func isolate(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

func TestLoad_TeamMode_RejectsDefaultJWTSecret(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "change-me-in-production")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when team mode uses the default JWT secret")
	}
}

func TestLoad_TeamMode_RejectsEmptyJWTSecret(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when team mode uses an empty JWT secret")
	}
}

func TestLoad_TeamMode_RejectsShortJWTSecret(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "too-short")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when team mode uses a JWT secret under the minimum length")
	}
}

func TestLoad_TeamMode_AcceptsStrongJWTSecret(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", strings.Repeat("s", 32))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error with a strong secret: %v", err)
	}
	if cfg.Auth.JWTSecret != strings.Repeat("s", 32) {
		t.Fatalf("expected configured secret to be loaded, got %q", cfg.Auth.JWTSecret)
	}
}

func TestLoad_LocalMode_AllowsDefaultJWTSecret(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "change-me-in-production")

	// Local mode never exposes the network-facing JWT auth path (see
	// router.go: local mode uses LocalAuthenticator, bypassing JWT
	// validation entirely), so the default secret is not a security issue.
	if _, err := Load(); err != nil {
		t.Fatalf("unexpected error in local mode: %v", err)
	}
}

func writeConfigYAML(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile("config.yaml", []byte(content), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

func TestLoad_Registries_Defaults(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Registries.SeedDefault {
		t.Error("registries.seed_default should default to true")
	}
	if len(cfg.Registries.Entries) != 0 {
		t.Errorf("expected no entries by default, got %d", len(cfg.Registries.Entries))
	}
}

func TestLoad_Registries_ParsesEntries(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	writeConfigYAML(t, `
registries:
  seed_default: false
  entries:
    - name: acme
      url: registry.acme.com
      namespace: acme-envs
      default: true
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Registries.SeedDefault {
		t.Error("seed_default should be false")
	}
	if len(cfg.Registries.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cfg.Registries.Entries))
	}
	e := cfg.Registries.Entries[0]
	if e.Name != "acme" || e.URL != "registry.acme.com" || e.Namespace != "acme-envs" || !e.Default {
		t.Errorf("entry fields not parsed correctly: %+v", e)
	}
}

func TestLoad_Registries_DuplicateNamesFail(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	writeConfigYAML(t, `
registries:
  entries:
    - name: acme
      url: a.io
    - name: acme
      url: b.io
`)

	if _, err := Load(); err == nil {
		t.Fatal("expected error for duplicate entry names")
	}
}

func TestLoad_Registries_MissingURLFails(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	writeConfigYAML(t, `
registries:
  entries:
    - name: acme
`)

	if _, err := Load(); err == nil {
		t.Fatal("expected error for entry without url")
	}
}

func TestLoad_Registries_MissingNameFails(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	writeConfigYAML(t, `
registries:
  entries:
    - url: registry.acme.com
`)

	if _, err := Load(); err == nil {
		t.Fatal("expected error for entry without name")
	}
}

func TestLoad_Registries_MultipleDefaultsFail(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	writeConfigYAML(t, `
registries:
  entries:
    - name: a
      url: a.io
      default: true
    - name: b
      url: b.io
      default: true
`)

	if _, err := Load(); err == nil {
		t.Fatal("expected error when two entries set default: true")
	}
}

func TestLoad_Registries_TrimsNameAndURL(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	writeConfigYAML(t, `
registries:
  entries:
    - name: "  acme  "
      url: "  registry.acme.com  "
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Registries.Entries[0].Name != "acme" {
		t.Errorf("expected trimmed name, got %q", cfg.Registries.Entries[0].Name)
	}
	if cfg.Registries.Entries[0].URL != "registry.acme.com" {
		t.Errorf("expected trimmed url, got %q", cfg.Registries.Entries[0].URL)
	}
}

func TestLoad_Server_AllowedOrigins(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_SERVER_ALLOWED_ORIGINS", "https://hub.example.com, https://other.example.com ,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := cfg.Server.AllowedOriginsList()
	want := []string{"https://hub.example.com", "https://other.example.com"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestAllowedOriginsList_Empty(t *testing.T) {
	c := ServerConfig{AllowedOrigins: "  "}
	if got := c.AllowedOriginsList(); got != nil {
		t.Errorf("expected nil for blank value, got %v", got)
	}
}
