package sandbox

import (
	"strings"
)

// defaultPath is used when the parent process has no PATH. Without it pixi
// cannot find the tools it shells out to.
const defaultPath = "/usr/local/bin:/usr/bin:/bin"

// envAllowlist names the parent environment variables a build may inherit.
// Everything else is dropped, which is what keeps NEBI_DATABASE_DSN,
// NEBI_AUTH_JWT_SECRET, and registry credentials out of build code's reach.
var envAllowlist = []string{
	"PATH",
	"LANG",
	"LC_ALL",
	"SSL_CERT_FILE",
	"SSL_CERT_DIR",
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"http_proxy",
	"https_proxy",
	"no_proxy",
}

// buildEnv returns the environment for a build subprocess: the allowlisted
// subset of parent, plus a job-scoped HOME and TMPDIR. HOME is job-scoped so
// the pixi/rattler package cache lands inside the workspace rather than in a
// location shared across tenants.
func buildEnv(parent []string, home, tmpDir string) []string {
	allowed := make(map[string]bool, len(envAllowlist))
	for _, k := range envAllowlist {
		allowed[k] = true
	}

	out := make([]string, 0, len(envAllowlist)+2)
	havePath := false
	for _, kv := range parent {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || !allowed[key] {
			continue
		}
		if key == "PATH" {
			havePath = true
		}
		out = append(out, kv)
	}
	if !havePath {
		out = append(out, "PATH="+defaultPath)
	}
	out = append(out, "HOME="+home, "TMPDIR="+tmpDir)
	return out
}
