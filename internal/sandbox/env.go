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
// subset of parent, with HOME and TMPDIR handled separately.
//
// A non-empty home/tmpDir replaces the parent's, which is what an active
// sandbox wants: the pixi/rattler package cache then lands inside the
// workspace rather than somewhere shared across tenants.
//
// Empty home/tmpDir means "do not scope", which off mode passes. The
// parent's own values then survive the allowlist. That costs nothing in
// security terms, since an unconfined build could reach the real HOME
// regardless, and it avoids two concrete harms: for local workspaces the
// workspace directory is the user's own project folder, which would silently
// accumulate a multi-GB conda cache, and pixi and rattler read
// $HOME/.rattler/credentials.json and $HOME/.pixi, so a rewritten HOME
// breaks private-channel authentication.
func buildEnv(parent []string, home, tmpDir string) []string {
	allowed := make(map[string]bool, len(envAllowlist))
	for _, k := range envAllowlist {
		allowed[k] = true
	}
	// Only inherited when this build is not scoping them itself.
	allowed["HOME"] = home == ""
	allowed["TMPDIR"] = tmpDir == ""

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
	if home != "" {
		out = append(out, "HOME="+home)
	}
	if tmpDir != "" {
		out = append(out, "TMPDIR="+tmpDir)
	}
	return out
}

// dedupEnv collapses repeated keys, keeping the last occurrence. os/exec does
// this itself when it spawns the process, so doing it up front changes nothing
// about the child; it just makes cmd.Env exactly the environment the build will
// see, which matters for a list whose whole purpose is to be auditable.
func dedupEnv(env []string) []string {
	lastIndex := make(map[string]int, len(env))
	for i, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		lastIndex[key] = i
	}
	out := make([]string, 0, len(lastIndex))
	for i, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if lastIndex[key] == i {
			out = append(out, kv)
		}
	}
	return out
}
