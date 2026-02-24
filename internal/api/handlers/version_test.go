package handlers

import "testing"

func TestParseGitDescribe(t *testing.T) {
	tests := []struct {
		input       string
		wantVersion string
		wantCommit  string
	}{
		// Release tags (exact match)
		{"v0.7", "0.7", ""},
		{"v0.7.0", "0.7.0", ""},
		{"v1.2.3", "1.2.3", ""},

		// Dev builds (commits after tag)
		{"v0.7-1-gf9e7962", "0.7.dev+f9e7962", "f9e7962"},
		{"v0.7.0-5-gabc1234", "0.7.0.dev+abc1234", "abc1234"},

		// Dirty working tree
		{"v0.7-dirty", "0.7.dev", ""},
		{"v0.7-1-gf9e7962-dirty", "0.7.dev+f9e7962", "f9e7962"},

		// Pre-release tags
		{"v0.7.0-rc1", "0.7.0-rc1", ""},
		{"v0.7.0-rc1-3-gabc1234", "0.7.0-rc1.dev+abc1234", "abc1234"},
		{"v0.7.0-beta.1-2-g1234567", "0.7.0-beta.1.dev+1234567", "1234567"},

		// Bare commit hash (no tags in repo)
		{"f9e7962", "dev+f9e7962", "f9e7962"},
		{"abc1234", "dev+abc1234", "abc1234"},

		// Without v prefix
		{"0.7.0", "0.7.0", ""},
		{"0.7.0-1-gf9e7962", "0.7.0.dev+f9e7962", "f9e7962"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotVersion, gotCommit := parseGitDescribe(tt.input)
			if gotVersion != tt.wantVersion {
				t.Errorf("parseGitDescribe(%q) version = %q, want %q", tt.input, gotVersion, tt.wantVersion)
			}
			if gotCommit != tt.wantCommit {
				t.Errorf("parseGitDescribe(%q) commit = %q, want %q", tt.input, gotCommit, tt.wantCommit)
			}
		})
	}
}
