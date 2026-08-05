package config

import (
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
