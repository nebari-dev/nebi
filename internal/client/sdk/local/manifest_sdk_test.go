package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func TestParseManifestReportsUnrepresentable(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	toml := "[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\", \"osx-arm64\"]\n\n[dependencies]\npython = \">=3.11\"\nnumpy = \"*\"\n\n[tasks]\nstart = \"python main.py\"\n"
	res, err := stack.SDK.Local.ParseManifest(ctx, toml)
	if err != nil {
		t.Fatal(err)
	}
	if res.Form.Name != "demo" || len(res.Form.Platforms) != 2 || len(res.Form.Requested) != 2 {
		t.Fatalf("form: %+v", res.Form)
	}
	if len(res.Unrepresentable) != 1 || res.Unrepresentable[0] != "[tasks]" {
		t.Fatalf("unrepresentable: %v", res.Unrepresentable)
	}

	res, err = stack.SDK.Local.ParseManifest(ctx, "[workspace]\nname = \"x\"\nplatforms = [\"linux-64\"]\n")
	if err != nil || res.Unrepresentable == nil {
		t.Fatalf("unrepresentable must never be nil: %+v %v", res, err)
	}

	_, err = stack.SDK.Local.ParseManifest(ctx, "[workspace\n")
	wantValidation(t, err, "manifest")
}

func TestRenderManifest(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	form := shared.ManifestForm{
		Name:      "demo",
		Channels:  []string{"conda-forge"},
		Platforms: []string{"linux-64"},
		Requested: []shared.RequestedPackage{{Name: "python", Constraint: ""}, {Name: "rich", Constraint: ">=13", Source: "pypi"}},
	}
	res, err := stack.SDK.Local.RenderManifest(ctx, shared.ManifestRenderRequest{Form: form})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[workspace]", `name = "demo"`, `python = "*"`, "[pypi-dependencies]", `rich = ">=13"`} {
		if !strings.Contains(res.PixiToml, want) {
			t.Fatalf("missing %q in:\n%s", want, res.PixiToml)
		}
	}

	// Applying onto a base keeps what the form cannot show.
	base := "[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\npython = \"*\"\n\n[tasks]\nstart = \"python main.py\" # keep me\n"
	form.Requested = []shared.RequestedPackage{{Name: "python", Constraint: "3.12.*"}}
	res, err = stack.SDK.Local.RenderManifest(ctx, shared.ManifestRenderRequest{Form: form, BasePixiToml: base})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.PixiToml, `start = "python main.py" # keep me`) || !strings.Contains(res.PixiToml, `python = "3.12.*"`) {
		t.Fatalf("apply onto base:\n%s", res.PixiToml)
	}
}
