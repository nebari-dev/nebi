package sandbox

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildEnv_DropsSecretsKeepsAllowlisted(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin:/bin",
		"NEBI_DATABASE_DSN=host=db user=nebi password=hunter2",
		"NEBI_AUTH_JWT_SECRET=supersecret",
		"NEBI_QUEUE_VALKEY_ADDR=valkey:6379",
		"AWS_SECRET_ACCESS_KEY=leakme",
		"LANG=en_US.UTF-8",
		"SSL_CERT_FILE=/etc/ssl/certs/org-ca.crt",
		"HTTPS_PROXY=http://proxy:3128",
	}

	got := buildEnv(parent, "/ws/home", "/ws/tmp")

	for _, banned := range []string{"NEBI_DATABASE_DSN", "NEBI_AUTH_JWT_SECRET", "NEBI_QUEUE_VALKEY_ADDR", "AWS_SECRET_ACCESS_KEY"} {
		for _, kv := range got {
			if strings.HasPrefix(kv, banned+"=") {
				t.Fatalf("secret %q leaked into sandbox env: %v", banned, got)
			}
		}
	}

	for _, want := range []string{
		"PATH=/usr/bin:/bin",
		"LANG=en_US.UTF-8",
		"SSL_CERT_FILE=/etc/ssl/certs/org-ca.crt",
		"HTTPS_PROXY=http://proxy:3128",
		"HOME=/ws/home",
		"TMPDIR=/ws/tmp",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("expected %q in sandbox env, got %v", want, got)
		}
	}
}

func TestBuildEnv_AlwaysSetsPathEvenWhenParentHasNone(t *testing.T) {
	got := buildEnv([]string{"NEBI_AUTH_JWT_SECRET=x"}, "/ws/home", "/ws/tmp")

	var path string
	for _, kv := range got {
		if strings.HasPrefix(kv, "PATH=") {
			path = kv
		}
	}
	if path == "" {
		t.Fatalf("expected a PATH entry, got %v", got)
	}
}

func TestBuildEnv_JobScopedHomeOverridesParentHome(t *testing.T) {
	got := buildEnv([]string{"HOME=/root", "PATH=/bin"}, "/ws/home", "/ws/tmp")

	if slices.Contains(got, "HOME=/root") {
		t.Fatalf("parent HOME must not survive: %v", got)
	}
	if !slices.Contains(got, "HOME=/ws/home") {
		t.Fatalf("expected job-scoped HOME, got %v", got)
	}
}
