package oci

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadBundleConfig(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bundleConfig
	}{
		{
			name: "empty file",
			body: "",
			want: bundleConfig{},
		},
		{
			name: "no nebi section",
			body: `[project]` + "\n" + `name = "x"` + "\n",
			want: bundleConfig{},
		},
		{
			name: "include only",
			body: `[tool.nebi.bundle]` + "\n" +
				`include = ["src/**", "README.md"]` + "\n",
			want: bundleConfig{Include: []string{"src/**", "README.md"}},
		},
		{
			name: "exclude only",
			body: `[tool.nebi.bundle]` + "\n" +
				`exclude = ["*.log", "secrets/**"]` + "\n",
			want: bundleConfig{Exclude: []string{"*.log", "secrets/**"}},
		},
		{
			name: "both",
			body: `[tool.nebi.bundle]` + "\n" +
				`include = ["src/**"]` + "\n" +
				`exclude = ["*.log"]` + "\n",
			want: bundleConfig{Include: []string{"src/**"}, Exclude: []string{"*.log"}},
		},
		{
			name: "unknown keys ignored",
			body: `[tool.nebi.bundle]` + "\n" +
				`include = ["a"]` + "\n" +
				`mystery = "value"` + "\n" +
				`[tool.nebi.bundle.future]` + "\n" +
				`key = 1` + "\n",
			want: bundleConfig{Include: []string{"a"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			f := filepath.Join(dir, "pixi.toml")
			if err := os.WriteFile(f, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := loadBundleConfig(f)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestLoadBundleConfig_Missing(t *testing.T) {
	got, err := loadBundleConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, bundleConfig{}) {
		t.Fatalf("got %+v want zero", got)
	}
}
