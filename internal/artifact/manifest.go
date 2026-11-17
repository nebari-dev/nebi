// Package artifact reads and writes pixi.toml manifests.
//
// It holds the subset the local project SDK needs to create a project
// (issue #660): a structured Spec and its renderer, a manifest parser, and a
// helper that sets [workspace].name and [workspace].description in a raw
// manifest while leaving the rest of the file as the user wrote it. Full
// form round-trip and validation of pixi.toml is tracked separately.
package artifact

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// File names inside a project directory.
const (
	ManifestFile = "pixi.toml"
	LockFile     = "pixi.lock"
)

// ErrInvalid is wrapped by every error that reports a manifest or spec that
// cannot be parsed, rendered or edited.
var ErrInvalid = errors.New("invalid pixi.toml")

// Package is one dependency of a Spec. Operator and Constraint together form
// the version spec: Operator ">=" and Constraint "1.26" give ">=1.26". An
// empty Operator uses Constraint as written (e.g. "1.2.*" or ">=1,<2"); both
// empty mean any version ("*").
type Package struct {
	Name       string
	Operator   string
	Constraint string
}

// Spec is the structured form of a new project's pixi.toml. The project
// name and description are passed to RenderManifest separately.
type Spec struct {
	Platforms []string
	Channels  []string
	Packages  []Package
}

// Manifest is what ParseManifest reads from a pixi.toml: the fields of the
// [workspace] table (or the legacy [project] table).
type Manifest struct {
	Name        string
	Description string
	// Channels are the channel names, from plain strings or from the
	// channel key of { channel = ..., priority = ... } tables.
	Channels  []string
	Platforms []string
}

// knownPlatforms are the platforms a pixi workspace can target.
var knownPlatforms = map[string]bool{
	"linux-32": true, "linux-64": true, "linux-aarch64": true, "linux-armv6l": true,
	"linux-armv7l": true, "linux-ppc64le": true, "linux-ppc64": true, "linux-s390x": true,
	"linux-riscv32": true, "linux-riscv64": true,
	"osx-64": true, "osx-arm64": true,
	"win-32": true, "win-64": true, "win-arm64": true,
	"emscripten-wasm32": true, "wasi-wasm32": true, "zos-z": true,
}

// ValidatePlatforms checks that every platform is one pixi knows and that
// none is listed twice. An empty list is not an error here; callers that
// need a platform check for one themselves.
func ValidatePlatforms(platforms []string) error {
	seen := make(map[string]bool, len(platforms))
	for _, p := range platforms {
		if !knownPlatforms[p] {
			return fmt.Errorf("%w: unknown platform %q", ErrInvalid, p)
		}
		if seen[p] {
			return fmt.Errorf("%w: platform %q is listed more than once", ErrInvalid, p)
		}
		seen[p] = true
	}
	return nil
}

// Validate checks what pixi would reject before solving: at least one
// channel, and known, distinct platforms.
func (m *Manifest) Validate() error {
	if len(m.Channels) == 0 {
		return fmt.Errorf("%w: at least one channel is required", ErrInvalid)
	}
	return ValidatePlatforms(m.Platforms)
}

var (
	packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
	bareKeyPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	versionOperators   = map[string]bool{
		"==": true, "=": true, "!=": true, ">=": true, "<=": true, ">": true, "<": true, "~=": true,
	}
)

// VersionSpec returns the pixi version spec of the package.
func (p Package) VersionSpec() (string, error) {
	op := strings.TrimSpace(p.Operator)
	constraint := strings.TrimSpace(p.Constraint)
	switch {
	case op == "" && constraint == "":
		return "*", nil
	case op == "":
		return constraint, nil
	case !versionOperators[op]:
		return "", fmt.Errorf("%w: package %q: unknown version operator %q", ErrInvalid, p.Name, op)
	case constraint == "":
		return "", fmt.Errorf("%w: package %q: operator %q needs a version", ErrInvalid, p.Name, op)
	case strings.ContainsAny(constraint[:1], "=<>!~"):
		return "", fmt.Errorf("%w: package %q: version %q already has an operator; leave Operator empty or drop it from the version", ErrInvalid, p.Name, constraint)
	default:
		return op + constraint, nil
	}
}

// ParseManifest parses a pixi.toml and returns its [workspace] fields,
// falling back to the legacy [project] table.
func ParseManifest(content string) (*Manifest, error) {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	table, ok := workspaceTable(doc)
	if !ok {
		return nil, fmt.Errorf("%w: missing [workspace] table", ErrInvalid)
	}
	m := &Manifest{}
	var err error
	if m.Name, err = optionalString(table, "name"); err != nil {
		return nil, err
	}
	if m.Description, err = optionalString(table, "description"); err != nil {
		return nil, err
	}
	if raw, present := table["channels"]; present {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("%w: channels must be a list", ErrInvalid)
		}
		for _, item := range list {
			switch c := item.(type) {
			case string:
				m.Channels = append(m.Channels, c)
			case map[string]any:
				name, ok := c["channel"].(string)
				if !ok {
					return nil, fmt.Errorf("%w: a channel table needs a channel name", ErrInvalid)
				}
				m.Channels = append(m.Channels, name)
			default:
				return nil, fmt.Errorf("%w: channels must be names or channel tables", ErrInvalid)
			}
		}
	}
	if raw, present := table["platforms"]; present {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("%w: platforms must be a list of strings", ErrInvalid)
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: platforms must be a list of strings", ErrInvalid)
			}
			m.Platforms = append(m.Platforms, s)
		}
	}
	return m, nil
}

func workspaceTable(doc map[string]any) (map[string]any, bool) {
	for _, key := range []string{"workspace", "project"} {
		if t, ok := doc[key].(map[string]any); ok {
			return t, true
		}
	}
	return nil, false
}

func optionalString(table map[string]any, key string) (string, error) {
	raw, present := table[key]
	if !present {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string", ErrInvalid, key)
	}
	return s, nil
}

// RenderManifest renders a Spec as a pixi.toml with the given workspace name
// and description. An empty description is left out.
func RenderManifest(name, description string, spec Spec) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if len(spec.Platforms) == 0 {
		return "", fmt.Errorf("%w: at least one platform is required", ErrInvalid)
	}
	for _, p := range spec.Platforms {
		if strings.TrimSpace(p) == "" {
			return "", fmt.Errorf("%w: platform names must not be empty", ErrInvalid)
		}
	}
	if err := ValidatePlatforms(spec.Platforms); err != nil {
		return "", err
	}
	if len(spec.Channels) == 0 {
		return "", fmt.Errorf("%w: at least one channel is required", ErrInvalid)
	}
	channels := make(map[string]bool, len(spec.Channels))
	for _, c := range spec.Channels {
		if strings.TrimSpace(c) == "" {
			return "", fmt.Errorf("%w: channel names must not be empty", ErrInvalid)
		}
		if channels[c] {
			return "", fmt.Errorf("%w: channel %q is listed more than once", ErrInvalid, c)
		}
		channels[c] = true
	}

	var b strings.Builder
	b.WriteString("[workspace]\n")
	fmt.Fprintf(&b, "name = %s\n", Quote(name))
	if description != "" {
		fmt.Fprintf(&b, "description = %s\n", Quote(description))
	}
	fmt.Fprintf(&b, "channels = %s\n", quoteList(spec.Channels))
	fmt.Fprintf(&b, "platforms = %s\n", quoteList(spec.Platforms))
	b.WriteString("\n[dependencies]\n")

	seen := make(map[string]bool, len(spec.Packages))
	for _, pkg := range spec.Packages {
		pkgName := strings.TrimSpace(pkg.Name)
		if !packageNamePattern.MatchString(pkgName) {
			return "", fmt.Errorf("%w: invalid package name %q", ErrInvalid, pkg.Name)
		}
		key := strings.ToLower(pkgName)
		if seen[key] {
			return "", fmt.Errorf("%w: package %q is listed more than once", ErrInvalid, pkgName)
		}
		seen[key] = true
		version, err := pkg.VersionSpec()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s = %s\n", tomlKey(pkgName), Quote(version))
	}

	out := b.String()
	if _, err := ParseManifest(out); err != nil {
		return "", fmt.Errorf("rendered manifest does not parse: %w", err)
	}
	return out, nil
}

// Quote returns s as a TOML basic string.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func quoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = Quote(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func tomlKey(k string) string {
	if bareKeyPattern.MatchString(k) {
		return k
	}
	return Quote(k)
}
