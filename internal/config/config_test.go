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
	t.Setenv("NEBI_AUTH_AUTHORIZATION_STALE_AFTER_MINS", "30")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error with a strong secret: %v", err)
	}
	if cfg.Auth.JWTSecret != strings.Repeat("s", 32) {
		t.Fatalf("expected configured secret to be loaded, got %q", cfg.Auth.JWTSecret)
	}
	if cfg.Auth.AuthorizationStaleAfterMins != 30 {
		t.Fatalf("expected configured stale window to be loaded, got %d", cfg.Auth.AuthorizationStaleAfterMins)
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

func TestLoad_LimitsFromEnv(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_LIMITS_MAX_PACKAGES", "7")
	t.Setenv("NEBI_LIMITS_JOB_TIMEOUT_SECONDS", "9")
	t.Setenv("NEBI_LIMITS_JOB_LOG_BYTES", "1234")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Limits.MaxPackages != 7 {
		t.Fatalf("expected max_packages from env, got %d", cfg.Limits.MaxPackages)
	}
	if cfg.Limits.JobTimeoutSeconds != 9 {
		t.Fatalf("expected job_timeout_seconds from env, got %d", cfg.Limits.JobTimeoutSeconds)
	}
	if cfg.Limits.JobLogBytes != 1234 {
		t.Fatalf("expected job_log_bytes from env, got %d", cfg.Limits.JobLogBytes)
	}
}

func TestLoad_LimitsExplicitZeroIsPreserved(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_LIMITS_REQUEST_BODY_BYTES", "0")
	t.Setenv("NEBI_LIMITS_JOB_TIMEOUT_SECONDS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Limits.RequestBodyBytes != 0 {
		t.Fatalf("expected request_body_bytes=0, got %d", cfg.Limits.RequestBodyBytes)
	}
	if cfg.Limits.JobTimeoutSeconds != 0 {
		t.Fatalf("expected job_timeout_seconds=0, got %d", cfg.Limits.JobTimeoutSeconds)
	}
}

func TestLoad_ServerReadTimeoutFromEnv(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_SERVER_READ_TIMEOUT_SECONDS", "90")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.ReadTimeoutSeconds != 90 {
		t.Fatalf("expected read_timeout_seconds=90, got %d", cfg.Server.ReadTimeoutSeconds)
	}
}

func TestDefaultReadTimeoutSecondsScalesWithBodyCap(t *testing.T) {
	if got := DefaultReadTimeoutSeconds(20 * 1024 * 1024); got < 60 {
		t.Fatalf("expected default read timeout to scale above 60s for 20MiB, got %d", got)
	}
	if got := DefaultReadTimeoutSeconds(0); got != 0 {
		t.Fatalf("expected disabled body cap to disable default read timeout, got %d", got)
	}
}

func TestLoad_DefaultReadTimeoutUsesEffectiveBodyCap(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_LIMITS_REQUEST_BODY_BYTES", "104857600")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := DefaultReadTimeoutSeconds(cfg.Limits.RequestBodyBytes)
	if cfg.Server.ReadTimeoutSeconds != want {
		t.Fatalf("expected read timeout %d from effective body cap, got %d", want, cfg.Server.ReadTimeoutSeconds)
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
