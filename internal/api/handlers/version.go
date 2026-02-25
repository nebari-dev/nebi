package handlers

import (
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Version is set via ldflags at build time
var Version = "dev"

// Commit is the git commit hash, set via ldflags at build time
var Commit = ""

var versionOnce sync.Once
var resolvedVersion string
var resolvedCommit string

// gitDescribeRe matches the git describe suffix: -N-g<hash> with optional -dirty.
// Examples: "-1-gf9e7962", "-5-gabc1234-dirty"
var gitDescribeRe = regexp.MustCompile(`-(\d+)-g([0-9a-f]+)(-dirty)?$`)

// parseGitDescribe parses a git describe string into a clean version and commit.
// Examples:
//
//	"v0.7-1-gf9e7962-dirty" → ("0.7.dev+f9e7962", "f9e7962")
//	"v0.7-dirty"            → ("0.7.dev", "")
//	"v0.7"                  → ("0.7", "")
//	"v0.7.0-rc1-3-gabc1234" → ("0.7.0-rc1.dev+abc1234", "abc1234")
//	"f9e7962"               → ("dev+f9e7962", "f9e7962")
func parseGitDescribe(desc string) (version string, commit string) {
	desc = strings.TrimSpace(desc)
	desc = strings.TrimPrefix(desc, "v")

	// Match -N-g<hash>(-dirty)? suffix (commits after a tag)
	if m := gitDescribeRe.FindStringSubmatchIndex(desc); m != nil {
		tag := desc[:m[0]]
		hash := desc[m[4]:m[5]]
		return tag + ".dev+" + hash, hash
	}

	// Exact tag with -dirty
	if clean, ok := strings.CutSuffix(desc, "-dirty"); ok {
		return clean + ".dev", ""
	}

	// Exact tag (e.g. "0.7.0", "0.7.0-rc1") or bare hash (e.g. "f9e7962")
	if strings.Contains(desc, ".") {
		return desc, ""
	}
	return "dev+" + desc, desc
}

// resolveVersion computes version and commit strings once.
// It handles three cases:
//  1. ldflags set a raw git describe string → parse it
//  2. Version == "dev" (no ldflags) → run git describe at runtime
//  3. ldflags set a clean version (e.g. release tag) → use as-is
func resolveVersion() (string, string) {
	versionOnce.Do(func() {
		v := Version
		c := Commit

		if v == "dev" {
			// No ldflags — try git at runtime
			out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
			if err == nil {
				v = strings.TrimSpace(string(out))
			} else {
				out, err = exec.Command("git", "rev-parse", "--short", "HEAD").Output()
				if err == nil {
					c = strings.TrimSpace(string(out))
					resolvedVersion = "dev+" + c
					resolvedCommit = c
					return
				}
				resolvedVersion = "dev"
				return
			}
		}

		// Parse git describe format (works for both ldflags and runtime git)
		resolvedVersion, resolvedCommit = parseGitDescribe(v)
		if c != "" && resolvedCommit == "" {
			resolvedCommit = c
		}
	})
	return resolvedVersion, resolvedCommit
}

// Mode is set by the router based on config (e.g. "local" or "team")
var Mode = "team"

// GetVersion godoc
// @Summary Get version information
// @Description Returns version information about the Nebi server
// @Tags system
// @Produce json
// @Success 200 {object} map[string]string
// @Router /version [get]
func GetVersion(c *gin.Context) {
	features := map[string]bool{
		"auth":          Mode != "local",
		"rbac":          Mode != "local",
		"remote_proxy":  Mode == "local",
		"local_storage": Mode == "local",
	}

	version, commit := resolveVersion()

	c.JSON(http.StatusOK, gin.H{
		"version":    version,
		"commit":     commit,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"mode":       Mode,
		"features":   features,
	})
}
