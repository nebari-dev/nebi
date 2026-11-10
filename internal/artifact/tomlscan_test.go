package artifact

import (
	"slices"
	"strings"
	"testing"
)

func TestScanTomlRoundTripAndStructure(t *testing.T) {
	in := `# preamble
root = 1
[a]
x = [
  [1, 2],
  ["[b]"],
]
s = """
[not.a.header]
""""
l = '''
[also.not]
'''
esc = "quote \" [x]" # [comment]
[ "quoted.key" . sub ]
y = { a = 1, b = [
  2,
] }
[[arr]]
z = 'lit'
tail = "no newline"`
	d, err := scanToml(in)
	if err != nil {
		t.Fatal(err)
	}
	if d.String() != in {
		t.Fatalf("round trip mismatch:\n%q\nvs\n%q", d.String(), in)
	}
	var headers []string
	for _, s := range d.sections {
		h := strings.Join(s.key, "|")
		if s.array {
			h = "[[" + h + "]]"
		}
		headers = append(headers, h)
	}
	if want := []string{"a", "quoted.key|sub", "[[arr]]"}; !slices.Equal(headers, want) {
		t.Fatalf("headers = %q, want %q", headers, want)
	}
	var keys []string
	for _, e := range d.sections[0].entries {
		if e.key != nil {
			keys = append(keys, strings.Join(e.key, "."))
		}
	}
	if want := []string{"x", "s", "l", "esc"}; !slices.Equal(keys, want) {
		t.Fatalf("keys of [a] = %q, want %q", keys, want)
	}
	if len(d.preamble) != 2 || d.preamble[1].key[0] != "root" {
		t.Fatalf("preamble = %+v", d.preamble)
	}
}

func TestScanTomlErrors(t *testing.T) {
	for _, in := range []string{
		"x = [\n1,\n",        // unterminated array
		"[a\n",               // bad header
		"[a] junk\n",         // text after header
		"= 1\n",              // missing key
		"x 1\n",              // missing =
		"\"unterminated = 1", // bad quoted key
		"'lit = 1",           // bad literal key
		"x = \"\"\"\nopen\n", // unterminated multi-line string
	} {
		if _, err := scanToml(in); err == nil {
			t.Errorf("scanToml(%q) succeeded, want error", in)
		}
	}
}

func TestParseTomlKey(t *testing.T) {
	tests := []struct {
		in       string
		want     []string
		wantRest string
	}{
		{`a = 1`, []string{"a"}, "= 1"},
		{`a.b . c= 1`, []string{"a", "b", "c"}, "= 1"},
		{`"a.b".c = 1`, []string{"a.b", "c"}, "= 1"},
		{`'x y' = 1`, []string{"x y"}, "= 1"},
		{`"esc\"q" = 1`, []string{`esc"q`}, "= 1"},
	}
	for _, tt := range tests {
		got, rest, err := parseTomlKey(tt.in)
		if err != nil || !slices.Equal(got, tt.want) || rest != tt.wantRest {
			t.Errorf("parseTomlKey(%q) = %q, %q, %v", tt.in, got, rest, err)
		}
	}
}

func TestRenderHelpers(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{renderTomlKey("numpy"), "numpy"},
		{renderTomlKey("ruamel.yaml"), `"ruamel.yaml"`},
		{renderTomlKey(""), `""`},
		{tomlQuote("a\"b\\c\n\t\x01"), `"a\"b\\c\n\t\u0001"`},
		{renderStringArray(nil), "[]"},
		{renderStringArray([]string{"a", "b"}), `["a", "b"]`},
		{renderInlineValue(map[string]any{}), "{}"},
		{renderInlineValue(map[string]any{"b": true, "a": int64(1), "c": []any{"x", 1.5}, "d": 2.0}), `{ a = 1, b = true, c = ["x", 1.5], d = 2.0 }`},
		{renderInlineValue([]string{"x"}), `["x"]`},
		{renderInlineValue(3), "3"},
	}
	for i, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%d: got %s, want %s", i, tt.got, tt.want)
		}
	}
}
