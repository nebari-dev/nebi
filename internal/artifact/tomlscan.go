package artifact

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The scanner below splits a TOML document into top-level sections and
// key/value entries while keeping every byte of the original text. It is not
// a TOML parser: values are not decoded (go-toml does that), the scanner only
// tracks strings, comments and bracket depth so it can tell where one entry
// ends and the next header begins. That lets the form round-trip edit the
// tables it owns and leave everything else (comments, formatting, unknown
// tables) exactly as written.

// tomlEntry is one logical line of a section: a key/value pair (possibly
// spanning several physical lines), a comment line or a blank line.
type tomlEntry struct {
	raw string   // exact source text, including the trailing newline
	key []string // dotted key parts; nil for blank lines and comments
}

// tomlSection is a [table] or [[array-of-tables]] header plus its entries.
type tomlSection struct {
	header  string
	key     []string
	array   bool
	entries []tomlEntry
}

// tomlDoc is a scanned document. String() reproduces the input exactly.
type tomlDoc struct {
	preamble []tomlEntry
	sections []*tomlSection
}

func (d *tomlDoc) String() string {
	var sb strings.Builder
	for _, e := range d.preamble {
		sb.WriteString(e.raw)
	}
	for _, s := range d.sections {
		sb.WriteString(s.header)
		for _, e := range s.entries {
			sb.WriteString(e.raw)
		}
	}
	return sb.String()
}

// findSection returns the index of the first plain (non-array) table whose
// dotted key equals name, or -1.
func (d *tomlDoc) findSection(name string) int {
	for i, s := range d.sections {
		if !s.array && strings.Join(s.key, ".") == name {
			return i
		}
	}
	return -1
}

func splitLinesKeep(s string) []string {
	var lines []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}

func scanToml(content string) (*tomlDoc, error) {
	d := &tomlDoc{}
	cur := &d.preamble
	lines := splitLinesKeep(content)
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			*cur = append(*cur, tomlEntry{raw: line})
			i++
			continue
		}
		if trimmed[0] == '[' {
			key, array, err := parseTomlHeader(trimmed)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			sec := &tomlSection{header: line, key: key, array: array}
			d.sections = append(d.sections, sec)
			cur = &sec.entries
			i++
			continue
		}
		key, rest, err := parseTomlKey(trimmed)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if !strings.HasPrefix(rest, "=") {
			return nil, fmt.Errorf("line %d: expected '=' after key", i+1)
		}
		var st tomlScanState
		st.scan(rest[1:])
		raw := line
		start := i
		i++
		for !st.clean() {
			if i >= len(lines) {
				return nil, fmt.Errorf("line %d: unterminated value", start+1)
			}
			st.scan(lines[i])
			raw += lines[i]
			i++
		}
		*cur = append(*cur, tomlEntry{raw: raw, key: key})
	}
	return d, nil
}

// tomlScanState tracks whether a value continues onto the next line.
type tomlScanState struct {
	ml    string // open multi-line string delimiter, if any
	depth int    // open [ and { count
}

func (st *tomlScanState) clean() bool { return st.ml == "" && st.depth <= 0 }

func (st *tomlScanState) scan(s string) {
	i := 0
	for i < len(s) {
		if st.ml != "" {
			idx := indexMultilineClose(s[i:], st.ml)
			if idx < 0 {
				return
			}
			i += idx + 3
			// """a"""" is a valid string ending in a quote: up to two extra
			// delimiter characters belong to the content.
			for k := 0; k < 2 && i < len(s) && s[i] == st.ml[0]; k++ {
				i++
			}
			st.ml = ""
			continue
		}
		c := s[i]
		switch c {
		case '#':
			return
		case '"', '\'':
			triple := strings.Repeat(string(c), 3)
			if strings.HasPrefix(s[i:], triple) {
				st.ml = triple
				i += 3
				continue
			}
			i++
			for i < len(s) && s[i] != c && s[i] != '\n' {
				if c == '"' && s[i] == '\\' {
					i++
				}
				i++
			}
			i++
		case '[', '{':
			st.depth++
			i++
		case ']', '}':
			st.depth--
			i++
		default:
			i++
		}
	}
}

func indexMultilineClose(s, delim string) int {
	if delim == `'''` {
		return strings.Index(s, delim)
	}
	for i := 0; i+3 <= len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i:i+3] == delim {
			return i
		}
	}
	return -1
}

func isBareKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// parseTomlKey parses a (dotted, possibly quoted) key at the start of s and
// returns its parts and the remainder with leading whitespace removed.
func parseTomlKey(s string) ([]string, string, error) {
	var parts []string
	for {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return nil, "", errors.New("missing key")
		}
		switch s[0] {
		case '"':
			end := 1
			for end < len(s) && s[end] != '"' {
				if s[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(s) {
				return nil, "", errors.New("unterminated quoted key")
			}
			v, err := strconv.Unquote(s[:end+1])
			if err != nil {
				return nil, "", fmt.Errorf("invalid quoted key: %w", err)
			}
			parts = append(parts, v)
			s = s[end+1:]
		case '\'':
			end := strings.IndexByte(s[1:], '\'')
			if end < 0 {
				return nil, "", errors.New("unterminated quoted key")
			}
			parts = append(parts, s[1:end+1])
			s = s[end+2:]
		default:
			j := 0
			for j < len(s) && isBareKeyChar(s[j]) {
				j++
			}
			if j == 0 {
				return nil, "", fmt.Errorf("invalid key near %q", truncate(s, 20))
			}
			parts = append(parts, s[:j])
			s = s[j:]
		}
		t := strings.TrimLeft(s, " \t")
		if strings.HasPrefix(t, ".") {
			s = t[1:]
			continue
		}
		return parts, t, nil
	}
}

func parseTomlHeader(trimmed string) ([]string, bool, error) {
	array := strings.HasPrefix(trimmed, "[[")
	open, closer := "[", "]"
	if array {
		open, closer = "[[", "]]"
	}
	key, rest, err := parseTomlKey(trimmed[len(open):])
	if err != nil {
		return nil, false, err
	}
	if !strings.HasPrefix(rest, closer) {
		return nil, false, fmt.Errorf("malformed table header %q", trimmed)
	}
	rest = strings.TrimSpace(rest[len(closer):])
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return nil, false, fmt.Errorf("unexpected text after table header %q", trimmed)
	}
	return key, array, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// renderTomlKey renders a single key part, quoting it when it is not bare.
// Package names with dots (ruamel.yaml) must be quoted.
func renderTomlKey(k string) string {
	if k == "" {
		return `""`
	}
	for i := 0; i < len(k); i++ {
		if !isBareKeyChar(k[i]) {
			return tomlQuote(k)
		}
	}
	return k
}

// tomlQuote renders s as a TOML basic string.
func tomlQuote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		case '\r':
			sb.WriteString(`\r`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&sb, `\u%04X`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// renderStringArray renders ["a", "b"].
func renderStringArray(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = tomlQuote(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// renderInlineValue renders a decoded go-toml value as inline TOML.
// Tables become inline tables with sorted keys.
func renderInlineValue(v any) string {
	switch t := v.(type) {
	case string:
		return tomlQuote(t)
	case bool:
		return strconv.FormatBool(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case float64:
		switch {
		case math.IsInf(t, 1):
			return "inf"
		case math.IsInf(t, -1):
			return "-inf"
		case math.IsNaN(t):
			return "nan"
		}
		s := strconv.FormatFloat(t, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	case []any:
		parts := make([]string, len(t))
		for i, x := range t {
			parts[i] = renderInlineValue(x)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		return renderStringArray(t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		if len(keys) == 0 {
			return "{}"
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = renderTomlKey(k) + " = " + renderInlineValue(t[k])
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	default:
		// Dates and times: go-toml's types print in TOML-compatible form.
		return fmt.Sprint(t)
	}
}
