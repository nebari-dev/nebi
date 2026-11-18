package artifact

import (
	"fmt"
	"regexp"
	"strings"
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
// right after the header when there is none. A value that is a multi-line
// string is replaced as a whole.
func setKey(lines []string, header int, key, value string) []string {
	end := tableEnd(lines, header)
	if i, m := findKey(lines, header, end, key); i >= 0 {
		last := i
		for _, delim := range []string{`"""`, `'''`} {
			if strings.HasPrefix(m[2], delim) && !strings.Contains(m[2][len(delim):], delim) {
				for j := i + 1; j < end; j++ {
					if strings.Contains(lines[j], delim) {
						last = j
						break
					}
				}
			}
		}
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
