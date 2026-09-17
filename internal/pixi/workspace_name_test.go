package pixi

import "testing"

func TestInitialWorkspaceName(t *testing.T) {
	for _, tc := range []struct {
		label, manifest, want string
		bad                   bool
	}{
		{"", "[workspace]\nname = 'manifest'", "manifest", false},
		{"", "[project]\nname = 'legacy'", "legacy", false},
		{"", "[workspace]\nchannels = []", "directory", false},
		{"chosen", "[workspace]\nname = 'manifest'", "chosen", false},
		{"chosen", "[workspace\n", "", true},
		{"bad/name", "[workspace]\n", "", true},
	} {
		got, err := InitialWorkspaceName(tc.label, "/tmp/directory", tc.manifest)
		if (err != nil) != tc.bad || got != tc.want {
			t.Errorf("%+v: got %q, %v", tc, got, err)
		}
	}
}
