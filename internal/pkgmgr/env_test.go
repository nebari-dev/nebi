package pkgmgr

import (
	"context"
	"testing"
)

func TestWithEnvironmentCopiesValues(t *testing.T) {
	env := map[string]string{"GITLAB_TOKEN": "secret"}
	ctx := WithEnvironment(context.Background(), env)
	env["GITLAB_TOKEN"] = "mutated"

	got := environmentFromContext(ctx)
	if got["GITLAB_TOKEN"] != "secret" {
		t.Fatalf("expected copied environment value, got %q", got["GITLAB_TOKEN"])
	}

	got["GITLAB_TOKEN"] = "mutated-again"
	got = environmentFromContext(ctx)
	if got["GITLAB_TOKEN"] != "secret" {
		t.Fatalf("expected returned environment to be copied, got %q", got["GITLAB_TOKEN"])
	}
}

func TestMergeEnvironmentOverlaysBase(t *testing.T) {
	got := mergeEnvironment(
		[]string{"PATH=/bin", "GITLAB_TOKEN=old", "BROKEN"},
		map[string]string{"GITLAB_TOKEN": "new", "UV_INDEX_TOKEN": "uv"},
	)

	want := []string{"GITLAB_TOKEN=new", "PATH=/bin", "UV_INDEX_TOKEN=uv"}
	if len(got) != len(want) {
		t.Fatalf("unexpected env length: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected env at %d: got %q want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}
