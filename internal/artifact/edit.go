package artifact

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	tableHeaderPattern = regexp.MustCompile(`^\s*\[\s*([A-Za-z0-9_.-]+)\s*\]\s*(#.*)?$`)
	anyHeaderPattern   = regexp.MustCompile(`^\s*\[`)
)

// SetWorkspaceFields returns content with [workspace].name set to name and,
// when description is not empty, [workspace].description set to
// description. The legacy [project] table is used when there is no
// [workspace] table. Every other line is kept as written.
//
// The result is parsed again and checked, so a manifest whose layout the
// line editor does not understand (for example dotted keys such as
// workspace.name = "x" outside a [workspace] table) returns ErrInvalid
// instead of a corrupted file.
func SetWorkspaceFields(content, name, description string) (string, error) {
	if _, err := ParseManifest(content); err != nil {
		return "", err
	}

	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	start := findTable(lines, "workspace")
	if start < 0 {
		start = findTable(lines, "project")
	}
	if start < 0 {
		return "", fmt.Errorf("%w: the manifest needs a [workspace] table header", ErrInvalid)
	}

	lines = setKey(lines, start, "name", Quote(name))
	if description != "" {
		lines = setKey(lines, start, "description", Quote(description))
	}
	out := strings.Join(lines, newline)

	m, err := ParseManifest(out)
	if err != nil {
		return "", fmt.Errorf("%w: could not set the workspace name: %v", ErrInvalid, err)
	}
	if m.Name != name || (description != "" && m.Description != description) {
		return "", fmt.Errorf("%w: could not set the workspace name and description", ErrInvalid)
	}
	return out, nil
}

// findTable returns the index of the "[table]" header line, or -1.
func findTable(lines []string, table string) int {
	for i, line := range lines {
		if m := tableHeaderPattern.FindStringSubmatch(line); m != nil && m[1] == table {
			return i
		}
	}
	return -1
}

// tableEnd returns the index of the first line after the table that starts
// at header (the next table header, or len(lines)).
func tableEnd(lines []string, header int) int {
	for i := header + 1; i < len(lines); i++ {
		if anyHeaderPattern.MatchString(lines[i]) {
			return i
		}
	}
	return len(lines)
}

// setKey replaces `key = ...` in the table that starts at header with
// `key = value`. A missing key is inserted after the table's name line, or
// right after the header when there is none. A value that spans lines (a
// multi-line string, array or inline table) is replaced as a whole.
func setKey(lines []string, header int, key, value string) []string {
	end := tableEnd(lines, header)
	if i, m := findKey(lines, header, end, key); i >= 0 {
		last := valueEnd(lines, i, end, m[2])
		replaced := append([]string{}, lines[:i]...)
		replaced = append(replaced, m[1]+key+" = "+value)
		return append(replaced, lines[last+1:]...)
	}
	at := header + 1
	if key != "name" {
		if i, _ := findKey(lines, header, end, "name"); i >= 0 {
			at = i + 1
		}
	}
	inserted := append([]string{}, lines[:at]...)
	inserted = append(inserted, key+" = "+value)
	return append(inserted, lines[at:]...)
}

// findKey returns the line index of `key = ...` between header and end, and
// the regexp match (indentation, value), or -1.
func findKey(lines []string, header, end int, key string) (int, []string) {
	keyPattern := regexp.MustCompile(`^(\s*)` + regexp.QuoteMeta(key) + `\s*=\s*(.*)$`)
	for i := header + 1; i < end; i++ {
		if m := keyPattern.FindStringSubmatch(lines[i]); m != nil {
			return i, m
		}
	}
	return -1, nil
}

// valueEnd returns the index of the last line of the value that starts on
// line i with text value (the part after `key =`), before end. Multi-line
// strings, arrays and inline tables can span lines; brackets inside strings
// and comments are ignored.
func valueEnd(lines []string, i, end int, value string) int {
	depth := 0
	quote := "" // the open string delimiter, if any
	text := value
	for line := i; ; {
		for j := 0; j < len(text); j++ {
			rest := text[j:]
			switch quote {
			case `"""`, `"`:
				if rest[0] == '\\' {
					j++ // skip the escaped character
				} else if strings.HasPrefix(rest, quote) {
					j += len(quote) - 1
					quote = ""
				}
			case `'''`, `'`:
				if strings.HasPrefix(rest, quote) {
					j += len(quote) - 1
					quote = ""
				}
			default:
				switch {
				case strings.HasPrefix(rest, `"""`), strings.HasPrefix(rest, `'''`):
					quote = rest[:3]
					j += 2
				case rest[0] == '"' || rest[0] == '\'':
					quote = rest[:1]
				case rest[0] == '#':
					j = len(text)
				case rest[0] == '[' || rest[0] == '{':
					depth++
				case rest[0] == ']' || rest[0] == '}':
					depth--
				}
			}
		}
		// A single-line string cannot continue on the next line.
		if quote == `"` || quote == `'` {
			quote = ""
		}
		if quote == "" && depth <= 0 {
			return line
		}
		line++
		if line >= end {
			return end - 1
		}
		text = lines[line]
	}
}

// ApplySpec returns content with spec applied: [workspace].channels and
// [workspace].platforms (or those of the legacy [project] table) set to
// the Spec's, and the top-level [dependencies] table replaced by the Spec's
// packages, or added when there is none. Every other table and key,
// including the workspace name and description, features, targets,
// [pypi-dependencies] and tasks, is kept as written. Lines inside the
// replaced [dependencies] table are not kept, apart from comments and
// blank lines after its last entry.
//
// A Spec holds only names and version specs. When a package or channel the
// Spec keeps has settings it cannot express, such as a dependency's channel
// or build, or a channel's priority, ApplySpec returns ErrInvalid instead
// of dropping them; such a pixi.toml is edited as a raw manifest.
//
// The result is parsed again and checked, so a layout the line editor
// does not understand (for example dotted keys, or dependencies written
// as an inline table) returns ErrInvalid instead of a corrupted file.
func ApplySpec(content string, spec Spec) (string, error) {
	if _, err := ParseManifest(content); err != nil {
		return "", err
	}
	if err := spec.validateLists(); err != nil {
		return "", err
	}
	if err := checkNothingDropped(content, spec); err != nil {
		return "", err
	}
	deps, err := spec.dependencyLines()
	if err != nil {
		return "", err
	}

	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	start := findTable(lines, "workspace")
	if start < 0 {
		start = findTable(lines, "project")
	}
	if start < 0 {
		return "", fmt.Errorf("%w: the manifest needs a [workspace] table header", ErrInvalid)
	}
	lines = setKey(lines, start, "channels", quoteList(spec.Channels))
	lines = setKey(lines, start, "platforms", quoteList(spec.Platforms))

	if header := findTable(lines, "dependencies"); header >= 0 {
		end := tableEnd(lines, header)
		last := header
		for i := header + 1; i < end; i++ {
			if t := strings.TrimSpace(lines[i]); t != "" && !strings.HasPrefix(t, "#") {
				last = i
			}
		}
		replaced := append([]string{}, lines[:header+1]...)
		replaced = append(replaced, deps...)
		lines = append(replaced, lines[last+1:]...)
	} else {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "", "[dependencies]")
		lines = append(lines, deps...)
		lines = append(lines, "")
	}
	out := strings.Join(lines, newline)

	if err := checkSpecApplied(out, spec); err != nil {
		return "", err
	}
	return out, nil
}

// checkNothingDropped returns ErrInvalid when applying spec to content
// would drop settings of a top-level dependency or a workspace channel that
// the Spec keeps: any key of a dependency table other than version, or of a
// channel table other than channel.
func checkNothingDropped(content string, spec Spec) error {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	deps, _ := doc["dependencies"].(map[string]any)
	for _, pkg := range spec.Packages {
		name := strings.TrimSpace(pkg.Name)
		for key, value := range deps {
			if !strings.EqualFold(key, name) {
				continue
			}
			if extra := extraKey(value, "version"); extra != "" {
				return fmt.Errorf("%w: dependency %q sets %q, which a spec cannot hold; edit pixi.toml directly", ErrInvalid, key, extra)
			}
		}
	}
	table, _ := workspaceTable(doc)
	for _, c := range asList(table["channels"]) {
		entry, ok := c.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["channel"].(string)
		if !slices.Contains(spec.Channels, name) {
			continue
		}
		if extra := extraKey(entry, "channel"); extra != "" {
			return fmt.Errorf("%w: channel %q sets %q, which a spec cannot hold; edit pixi.toml directly", ErrInvalid, name, extra)
		}
	}
	return nil
}

// extraKey returns a key of value, when it is a table, other than allowed,
// or "" when there is none.
func extraKey(value any, allowed string) string {
	table, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	keys := make([]string, 0, len(table))
	for key := range table {
		if key != allowed {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	slices.Sort(keys)
	return keys[0]
}

// checkSpecApplied parses out and checks that its workspace channels and
// platforms and its top-level dependencies are exactly spec's.
func checkSpecApplied(out string, spec Spec) error {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(out), &doc); err != nil {
		return fmt.Errorf("%w: could not apply the spec: %v", ErrInvalid, err)
	}
	failed := fmt.Errorf("%w: could not apply the spec to this pixi.toml layout", ErrInvalid)
	table, ok := workspaceTable(doc)
	if !ok || !sameStrings(table["channels"], spec.Channels) || !sameStrings(table["platforms"], spec.Platforms) {
		return failed
	}
	deps, _ := doc["dependencies"].(map[string]any)
	if len(deps) != len(spec.Packages) {
		return failed
	}
	for _, pkg := range spec.Packages {
		want, err := pkg.VersionSpec()
		if err != nil {
			return err
		}
		if got, ok := deps[strings.TrimSpace(pkg.Name)].(string); !ok || got != want {
			return failed
		}
	}
	return nil
}

// sameStrings reports whether raw, a parsed TOML value, is a list of
// exactly the strings in want, in order.
func sameStrings(raw any, want []string) bool {
	list, ok := raw.([]any)
	if !ok || len(list) != len(want) {
		return false
	}
	for i, item := range list {
		if s, ok := item.(string); !ok || s != want[i] {
			return false
		}
	}
	return true
}

// HasLocalPaths reports whether the manifest refers to local files that
// pixi resolves against the manifest's directory, so that it can only be
// locked in its own directory:
//   - a dependency given by a path, such as an editable PyPI package with
//     path = ".", in any dependency table (top level, feature or target);
//   - a channel given by a relative path or relative file: URL, in a
//     channels list or a dependency's channel;
//   - a relative find-links path or relative file: URL, or a relative
//     file: index-url or extra-index-urls entry, in [pypi-options];
//   - a mapping file given by a relative path or file: URL in
//     conda-pypi-map.
//
// Absolute paths and remote URLs resolve the same from any directory and do
// not count.
func HasLocalPaths(content string) (bool, error) {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(content), &doc); err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return hasLocalPath(doc), nil
}

func hasLocalPath(table map[string]any) bool {
	for key, value := range table {
		switch key {
		case "channels":
			for _, c := range asList(value) {
				switch c := c.(type) {
				case string:
					if relativePath(c, false) {
						return true
					}
				case map[string]any:
					if name, ok := c["channel"].(string); ok && relativePath(name, false) {
						return true
					}
				}
			}
		case "channel":
			if name, ok := value.(string); ok && relativePath(name, false) {
				return true
			}
		case "find-links":
			for _, l := range asList(value) {
				link, ok := l.(map[string]any)
				if !ok {
					continue
				}
				if path, ok := link["path"].(string); ok && relativePath(path, true) {
					return true
				}
				if url, ok := link["url"].(string); ok && relativePath(url, false) {
					return true
				}
			}
		case "index-url":
			if url, ok := value.(string); ok && relativePath(url, false) {
				return true
			}
		case "extra-index-urls":
			for _, u := range asList(value) {
				if url, ok := u.(string); ok && relativePath(url, false) {
					return true
				}
			}
		case "conda-pypi-map":
			// Each channel maps to a location (a URL or a file), given as a
			// string or as the location of a table; false disables it.
			entries, _ := value.(map[string]any)
			for _, entry := range entries {
				if table, ok := entry.(map[string]any); ok {
					entry = table["location"]
				}
				if location, ok := entry.(string); ok && relativePath(location, true) {
					return true
				}
			}
			continue // its keys are channels, not manifest tables
		}
		sub, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if key == "dev" || strings.HasSuffix(key, "dependencies") {
			for _, dep := range sub {
				if spec, ok := dep.(map[string]any); ok {
					if _, ok := spec["path"]; ok {
						return true
					}
				}
			}
		}
		if hasLocalPath(sub) {
			return true
		}
	}
	return false
}

func asList(value any) []any {
	list, _ := value.([]any)
	return list
}

// relativePath reports whether s is a path relative to the manifest's
// directory, either written as a path or as a file: URL. A bare name with
// no leading "./" or "../" counts as a path only when bareIsPath is set:
// a channel called "conda-forge" is a name, a find-links path "wheels" is
// a directory.
func relativePath(s string, bareIsPath bool) bool {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "file://"):
		s, bareIsPath = s[len("file://"):], true
	case strings.HasPrefix(lower, "file:"):
		s, bareIsPath = s[len("file:"):], true
	case strings.Contains(s, "://"):
		return false // a remote URL
	}
	if s == "" || strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\`) || strings.HasPrefix(s, "~") {
		return false
	}
	if len(s) >= 2 && s[1] == ':' {
		return false // a Windows drive path, such as C:\channel
	}
	for _, prefix := range []string{"./", "../", `.\`, `..\`} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	if s == "." || s == ".." {
		return true
	}
	return bareIsPath
}
