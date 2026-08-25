package server

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/config"
)

func TestResolveBindHost(t *testing.T) {
	tests := []struct {
		name      string
		localMode bool
		host      string
		want      string
	}{
		{name: "local mode defaults to loopback", localMode: true, host: "", want: "127.0.0.1"},
		{name: "local mode whitespace defaults to loopback", localMode: true, host: "  ", want: "127.0.0.1"},
		{name: "local mode explicit host respected", localMode: true, host: "0.0.0.0", want: "0.0.0.0"},
		{name: "team mode keeps all-interface default", localMode: false, host: "", want: ""},
		{name: "team mode explicit host respected", localMode: false, host: "10.0.0.5", want: "10.0.0.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBindHost(tt.localMode, tt.host)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestListenAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "empty host uses all interfaces", host: "", port: 8460, want: ":8460"},
		{name: "whitespace host uses all interfaces", host: "   ", port: 9000, want: ":9000"},
		{name: "ipv4 host", host: "127.0.0.1", port: 8460, want: "127.0.0.1:8460"},
		{name: "ipv6 host", host: "::1", port: 8460, want: "[::1]:8460"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listenAddress(tt.host, tt.port)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDisplayHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "empty host", host: "", want: "localhost"},
		{name: "whitespace host", host: "  ", want: "localhost"},
		{name: "all interfaces ipv4", host: "0.0.0.0", want: "localhost"},
		{name: "all interfaces ipv6", host: "::", want: "localhost"},
		{name: "loopback ipv4", host: "127.0.0.1", want: "127.0.0.1"},
		{name: "loopback ipv6", host: "::1", want: "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayHost(tt.host)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestServerURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		basePath string
		want     string
	}{
		{name: "empty host defaults to localhost display", host: "", port: 8460, want: "http://localhost:8460"},
		{name: "ipv4 host", host: "127.0.0.1", port: 8460, want: "http://127.0.0.1:8460"},
		{name: "ipv6 host is bracketed", host: "::1", port: 8460, want: "http://[::1]:8460"},
		{name: "all interfaces display localhost", host: "0.0.0.0", port: 9000, basePath: "/api/v1", want: "http://localhost:9000/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serverURL(tt.host, tt.port, tt.basePath)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// The shim's per-build warning only reaches the job log, so an operator
// running a fleet in permissive mode has no server-side signal that
// confinement is not being enforced. This pins the startup warning that
// supplies one.
func TestWarnIfSandboxNotEnforcing(t *testing.T) {
	tests := []struct {
		name        string
		deployMode  string
		sandboxMode string
		wantLogged  string
	}{
		{name: "strict is the enforcing default and says nothing", deployMode: "team", sandboxMode: "strict"},
		{name: "permissive warns that builds may run unconfined", deployMode: "team", sandboxMode: "permissive", wantLogged: "UNCONFINED"},
		{name: "off in team mode warns that confinement is disabled", deployMode: "team", sandboxMode: "off", wantLogged: "DISABLED"},
		{name: "off in local mode is the intended default", deployMode: "local", sandboxMode: "off"},
		{name: "permissive in local mode still warns", deployMode: "local", sandboxMode: "permissive", wantLogged: "UNCONFINED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			previous := slog.Default()
			t.Cleanup(func() { slog.SetDefault(previous) })
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

			cfg := &config.Config{Mode: tt.deployMode}
			cfg.Sandbox.Mode = tt.sandboxMode
			warnIfSandboxNotEnforcing(cfg)

			got := buf.String()
			if tt.wantLogged == "" {
				if got != "" {
					t.Fatalf("expected no warning, got: %s", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantLogged) {
				t.Fatalf("expected a warning mentioning %q, got: %s", tt.wantLogged, got)
			}
			if !strings.Contains(got, "sandbox_mode="+tt.sandboxMode) {
				t.Fatalf("expected the configured mode in the warning, got: %s", got)
			}
		})
	}
}
