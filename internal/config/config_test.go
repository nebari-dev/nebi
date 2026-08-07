package config

import (
	"os"
	"strings"
	"testing"
	"time"
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

func TestLoad_SandboxDefaultsToStrictInTeamMode(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", strings.Repeat("s", 32))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Mode != "strict" {
		t.Fatalf("expected sandbox mode strict in team mode, got %q", cfg.Sandbox.Mode)
	}
}

func TestLoad_SandboxDefaultsToOffInLocalMode(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Mode != "off" {
		t.Fatalf("expected sandbox mode off in local mode, got %q", cfg.Sandbox.Mode)
	}
}

func TestLoad_SandboxModeExplicitOverridesDefault(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("NEBI_SANDBOX_MODE", "permissive")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Mode != "permissive" {
		t.Fatalf("expected explicit permissive mode to win, got %q", cfg.Sandbox.Mode)
	}
}

func TestLoad_SandboxModeRejectsUnknownValue(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_SANDBOX_MODE", "banana")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for unknown sandbox mode")
	}
}

func TestLoad_SandboxDefaultPortsAndTimeout(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Sandbox.AllowedPorts) != 2 || cfg.Sandbox.AllowedPorts[0] != 80 || cfg.Sandbox.AllowedPorts[1] != 443 {
		t.Fatalf("expected default allowed ports [80 443], got %v", cfg.Sandbox.AllowedPorts)
	}
	if cfg.Sandbox.BuildTimeout != 30*time.Minute {
		t.Fatalf("expected default build timeout 30m, got %v", cfg.Sandbox.BuildTimeout)
	}
}

// viper's AutomaticEnv does not reach nested structs without an explicit
// BindEnv (see the comment in Load), so the non-string sandbox fields need
// coverage that exercises the env path rather than the SetDefault path.
func TestLoad_SandboxPortsAndTimeoutFromEnv(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_SANDBOX_ALLOWED_PORTS", "8080,9418")
	t.Setenv("NEBI_SANDBOX_BUILD_TIMEOUT", "45m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Sandbox.AllowedPorts) != 2 || cfg.Sandbox.AllowedPorts[0] != 8080 || cfg.Sandbox.AllowedPorts[1] != 9418 {
		t.Fatalf("expected allowed ports [8080 9418] from env, got %v", cfg.Sandbox.AllowedPorts)
	}
	if cfg.Sandbox.BuildTimeout != 45*time.Minute {
		t.Fatalf("expected build timeout 45m from env, got %v", cfg.Sandbox.BuildTimeout)
	}
}

// Ports are narrowed to uint16 when handed to the kernel, so an out-of-range
// entry would silently wrap (70000 becomes 4464) and open a port the operator
// never named. Reject it at load instead.
func TestLoad_SandboxRejectsOutOfRangePorts(t *testing.T) {
	for _, ports := range []string{"70000", "-1", "0", "80,65536"} {
		t.Run(ports, func(t *testing.T) {
			isolate(t)
			t.Setenv("NEBI_MODE", "local")
			t.Setenv("NEBI_SANDBOX_ALLOWED_PORTS", ports)

			if _, err := Load(); err == nil {
				t.Fatalf("expected error for out-of-range allowed_ports %q", ports)
			}
		})
	}
}

func TestLoad_SandboxRejectsSubSecondBuildTimeoutFromEnv(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_SANDBOX_BUILD_TIMEOUT", "500ms")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for a sub-second build timeout")
	}
}

// A bare int in config.yaml decodes as nanoseconds, so "build_timeout: 30"
// means 30ns rather than the 30 minutes an operator would expect. That is an
// easy mistake here because database.conn_max_lifetime is a bare-int-means-
// minutes field. Fail loudly instead of killing every build instantly with an
// opaque context deadline error.
func TestLoad_SandboxRejectsBareIntBuildTimeoutFromFile(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	if err := os.WriteFile("config.yaml", []byte("sandbox:\n  build_timeout: 30\n"), 0o600); err != nil {
		t.Fatalf("writing config.yaml: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected error for a bare-int build timeout that decodes to 30ns")
	}
}
