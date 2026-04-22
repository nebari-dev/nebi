package oci

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// startTestRegistry spins up an in-memory OCI Distribution server for the
// duration of the test. Returns the host (no scheme).
func startTestRegistry(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u.Host
}

// writeFile writes content under root/rel, creating parents.
func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// testRegistry returns an oci.Registry wired to a local in-memory server.
func testRegistry(host, namespace string) Registry {
	return Registry{Host: host, Namespace: namespace, PlainHTTP: true}
}

func TestPublishBundle_RoundTrip(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"demo\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "README.md", "# demo\n")
	writeFile(t, src, "src/main.py", "print('hi')\n")
	writeFile(t, src, "src/util.py", "def f(): return 1\n")
	writeFile(t, src, "data/sample.csv", "a,b,c\n1,2,3\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "bundle", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.AssetCount != 4 {
		t.Fatalf("AssetCount: got %d want 4", res.AssetCount)
	}

	pull, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		Mode: PullModeFull, PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !strings.Contains(pull.PixiToml, `name = "demo"`) {
		t.Fatalf("unexpected pixi.toml: %q", pull.PixiToml)
	}
	if !strings.Contains(pull.PixiLock, "version: 6") {
		t.Fatalf("unexpected pixi.lock: %q", pull.PixiLock)
	}
	want := map[string]string{
		"README.md":       "# demo\n",
		"src/main.py":     "print('hi')\n",
		"src/util.py":     "def f(): return 1\n",
		"data/sample.csv": "a,b,c\n1,2,3\n",
	}
	if len(pull.Assets) != len(want) {
		t.Fatalf("asset count: got %d want %d (%v)", len(pull.Assets), len(want), pull.Assets)
	}
	for _, a := range pull.Assets {
		w, ok := want[a.Path]
		if !ok {
			t.Fatalf("unexpected asset path %q", a.Path)
		}
		if string(a.Bytes) != w {
			t.Fatalf("asset %q mismatch: got %q want %q", a.Path, a.Bytes, w)
		}
	}
}

func TestPublishPixiOnly_LegacyTwoLayer(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"legacy\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	// Stray file — PublishPixiOnly must ignore it (no walker).
	writeFile(t, src, "ignored.txt", "should not ship")

	res, err := PublishPixiOnly(context.Background(), src, testRegistry(host, "demo"), "legacy", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.AssetCount != 0 {
		t.Fatalf("AssetCount: got %d want 0", res.AssetCount)
	}

	pull, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		Mode: PullModeFull, PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(pull.Assets) != 0 {
		t.Fatalf("want zero assets, got %d", len(pull.Assets))
	}
	if !strings.Contains(pull.PixiToml, "legacy") {
		t.Fatalf("unexpected pixi.toml %q", pull.PixiToml)
	}
}

func TestPullBundle_PixiOnlyModeSkipsAssetBlobs(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "big.bin", strings.Repeat("Z", 4096))

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "bigasset", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	pull, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		Mode: PullModePixiOnly, PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(pull.Assets) != 1 {
		t.Fatalf("expected 1 asset listed, got %d", len(pull.Assets))
	}
	if pull.Assets[0].Bytes != nil {
		t.Fatalf("pixi-only mode should not populate Bytes; got %d bytes", len(pull.Assets[0].Bytes))
	}
	if pull.Assets[0].Path != "big.bin" {
		t.Fatalf("unexpected asset path %q", pull.Assets[0].Path)
	}
}

func TestPublishBundle_ManyAssetsParallel(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"many\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	const n = 50
	want := make(map[string]string, n)
	for i := 0; i < n; i++ {
		rel := fmt.Sprintf("files/f%03d.txt", i)
		body := fmt.Sprintf("content-%d\n", i)
		writeFile(t, src, rel, body)
		want[rel] = body
	}

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "many", "v1",
		WithConcurrency(16),
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.AssetCount != n {
		t.Fatalf("AssetCount: got %d want %d", res.AssetCount, n)
	}

	pull, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		Mode: PullModeFull, Concurrency: 16, PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(pull.Assets) != n {
		t.Fatalf("asset count: got %d want %d", len(pull.Assets), n)
	}
	got := make(map[string]string, n)
	for _, a := range pull.Assets {
		got[a.Path] = string(a.Bytes)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("asset %q: got %q want %q", k, got[k], v)
		}
	}
}

func TestPublish_RejectsUnsafeAssetPath(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "ok.txt", "ok")

	// WithAssets bypasses the walker; we inject a malicious path to verify
	// the publisher's pre-network validation fires.
	bad := []Asset{{RelPath: "../escape.txt", AbsPath: filepath.Join(src, "ok.txt"), Size: 2}}
	_, err := Publish(context.Background(), src, testRegistry(host, "demo"), "unsafe", "v1",
		WithAssets(bad),
	)
	if err == nil {
		t.Fatal("expected publish to reject unsafe path")
	}
	if !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPreview_MatchesPublishAssetOrder(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"p\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "README.md", "# p\n")
	writeFile(t, src, "src/a.py", "a")
	writeFile(t, src, "src/b.py", "b")

	got, err := Preview(context.Background(), src)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	want := []string{"README.md", "pixi.lock", "pixi.toml", "src/a.py", "src/b.py"}
	if len(got) != len(want) {
		t.Fatalf("count: got %d want %d", len(got), len(want))
	}
	for i, a := range got {
		if a.RelPath != want[i] {
			t.Fatalf("order[%d]: got %q want %q", i, a.RelPath, want[i])
		}
	}
}

func TestPublish_ExtraTags_NoImplicitLatest(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"t\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "tagtest", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	// "latest" must not resolve — the implicit default is gone.
	if _, err := PullBundle(context.Background(), res.Repository, "latest", PullOptions{
		Mode: PullModePixiOnly, PlainHTTP: true,
	}); err == nil {
		t.Fatal("expected 'latest' tag to be missing when WithExtraTags was not supplied")
	}
}
