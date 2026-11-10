package artifact

import (
	"regexp"
	"slices"
)

// KnownPlatforms lists every platform name pixi accepts in
// [workspace] platforms (rattler's Platform enum, minus "unknown").
var KnownPlatforms = []string{
	"noarch",
	"linux-32",
	"linux-64",
	"linux-aarch64",
	"linux-armv6l",
	"linux-armv7l",
	"linux-loongarch64",
	"linux-ppc64le",
	"linux-ppc64",
	"linux-ppc",
	"linux-s390x",
	"linux-riscv32",
	"linux-riscv64",
	"freebsd-64",
	"osx-64",
	"osx-arm64",
	"win-32",
	"win-64",
	"win-arm64",
	"emscripten-wasm32",
	"wasi-wasm32",
	"zos-z",
}

// CommonPlatforms are the platforms the form offers by default, in the
// prototype's order, with their human labels.
var CommonPlatforms = []string{"linux-64", "linux-aarch64", "osx-64", "osx-arm64", "win-64"}

var platformLabels = map[string]string{
	"linux-64":      "Linux",
	"linux-aarch64": "Linux ARM",
	"osx-64":        "macOS Intel",
	"osx-arm64":     "macOS Apple silicon",
	"win-64":        "Windows",
}

// platformPattern is the shape the form accepts for a typed platform name.
var platformPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

// IsKnownPlatform reports whether p is a platform name pixi accepts.
func IsKnownPlatform(p string) bool { return slices.Contains(KnownPlatforms, p) }

// PlatformLabel returns the human label for a common platform ("Linux"),
// or "" for others.
func PlatformLabel(p string) string { return platformLabels[p] }

// PlatformDisplayName mirrors the prototype's option text:
// "osx-arm64 · this machine", "linux-64 · Linux", or the bare name.
func PlatformDisplayName(p, machine string) string {
	if p == machine {
		return p + " · this machine"
	}
	if l := platformLabels[p]; l != "" {
		return p + " · " + l
	}
	return p
}

// DefaultPlatform maps Go's GOOS/GOARCH to the pixi platform name, or ""
// when there is no match. Callers pass runtime.GOOS and runtime.GOARCH.
func DefaultPlatform(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "linux-64"
	case "linux/386":
		return "linux-32"
	case "linux/arm64":
		return "linux-aarch64"
	case "linux/arm":
		return "linux-armv7l"
	case "linux/ppc64le":
		return "linux-ppc64le"
	case "linux/ppc64":
		return "linux-ppc64"
	case "linux/s390x":
		return "linux-s390x"
	case "linux/riscv64":
		return "linux-riscv64"
	case "linux/loong64":
		return "linux-loongarch64"
	case "darwin/amd64":
		return "osx-64"
	case "darwin/arm64":
		return "osx-arm64"
	case "windows/amd64":
		return "win-64"
	case "windows/386":
		return "win-32"
	case "windows/arm64":
		return "win-arm64"
	case "freebsd/amd64":
		return "freebsd-64"
	case "js/wasm":
		return "emscripten-wasm32"
	case "wasip1/wasm":
		return "wasi-wasm32"
	}
	return ""
}
