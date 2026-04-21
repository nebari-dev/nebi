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
// duration of the test. Returns the host (no scheme) and a cleanup func.
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

func TestBundleRoundTrip(t *testing.T) {
	host := startTestRegistry(t)

	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"demo\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "README.md", "# demo\n")
	writeFile(t, src, "src/main.py", "print('hi')\n")
	writeFile(t, src, "src/util.py", "def f(): return 1\n")
	writeFile(t, src, "data/sample.csv", "a,b,c\n1,2,3\n")

	cfg, err := LoadBundleConfig(filepath.Join(src, "pixi.toml"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := WalkBundle(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Strip core files from asset list, the publisher emits them typed.
	assets := make([]AssetFile, 0, len(files))
	for _, f := range files {
		if f.RelPath == "pixi.toml" || f.RelPath == "pixi.lock" {
			continue
		}
		assets = append(assets, f)
	}

	repo := host + "/demo/bundle"
	_, err = PublishWorkspace(context.Background(), src, PublishOptions{
		Repository:   repo,
		Tag:          "v1",
		Username:     "",
		Password:     "",
		RegistryHost: host,
		Assets:       assets,
		PlainHTTP:    true,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	res, err := PullBundle(context.Background(), repo, "v1", PullOptions{
		Mode:      PullModeFull,
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}

	if !strings.Contains(res.PixiToml, `name = "demo"`) {
		t.Fatalf("unexpected pixi.toml: %q", res.PixiToml)
	}
	if !strings.Contains(res.PixiLock, "version: 6") {
		t.Fatalf("unexpected pixi.lock: %q", res.PixiLock)
	}
	// Every non-core source file must round-trip byte-identical.
	want := map[string]string{
		"README.md":       "# demo\n",
		"src/main.py":     "print('hi')\n",
		"src/util.py":     "def f(): return 1\n",
		"data/sample.csv": "a,b,c\n1,2,3\n",
	}
	if len(res.Assets) != len(want) {
		t.Fatalf("asset count: got %d want %d (%v)", len(res.Assets), len(want), res.Assets)
	}
	for _, a := range res.Assets {
		w, ok := want[a.Path]
		if !ok {
			t.Fatalf("unexpected asset path %q", a.Path)
		}
		if string(a.Bytes) != w {
			t.Fatalf("asset %q mismatch: got %q want %q", a.Path, a.Bytes, w)
		}
	}
}

func TestBundleRoundTrip_LegacyTwoLayer(t *testing.T) {
	host := startTestRegistry(t)

	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"legacy\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	repo := host + "/demo/legacy"
	_, err := PublishWorkspace(context.Background(), src, PublishOptions{
		Repository:   repo,
		Tag:          "v1",
		RegistryHost: host,
		PlainHTTP:    true,
		// Assets: nil — legacy zero-asset bundle.
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	res, err := PullBundle(context.Background(), repo, "v1", PullOptions{
		Mode:      PullModeFull,
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(res.Assets) != 0 {
		t.Fatalf("want zero assets, got %d", len(res.Assets))
	}
	if !strings.Contains(res.PixiToml, "legacy") {
		t.Fatalf("unexpected pixi.toml %q", res.PixiToml)
	}
}

func TestPixiOnlyMode_SkipsAssetBlobs(t *testing.T) {
	host := startTestRegistry(t)

	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	// Create a big-ish asset so we can detect if it was fetched by
	// looking at the registry server's request log (indirectly — via
	// hasAssets bytes length comparison).
	writeFile(t, src, "big.bin", strings.Repeat("Z", 4096))

	cfg, _ := LoadBundleConfig(filepath.Join(src, "pixi.toml"))
	files, _ := WalkBundle(src, cfg)
	var assets []AssetFile
	for _, f := range files {
		if f.RelPath == "pixi.toml" || f.RelPath == "pixi.lock" {
			continue
		}
		assets = append(assets, f)
	}

	repo := host + "/demo/bigasset"
	_, err := PublishWorkspace(context.Background(), src, PublishOptions{
		Repository:   repo,
		Tag:          "v1",
		RegistryHost: host,
		Assets:       assets,
		PlainHTTP:    true,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	res, err := PullBundle(context.Background(), repo, "v1", PullOptions{
		Mode:      PullModePixiOnly,
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(res.Assets) != 1 {
		t.Fatalf("expected 1 asset listed, got %d", len(res.Assets))
	}
	if res.Assets[0].Bytes != nil {
		t.Fatalf("pixi-only mode should not populate Bytes; got %d bytes", len(res.Assets[0].Bytes))
	}
	if res.Assets[0].Path != "big.bin" {
		t.Fatalf("unexpected asset path %q", res.Assets[0].Path)
	}
}

func TestBundleRoundTrip_ManyAssetsParallel(t *testing.T) {
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

	cfg, _ := LoadBundleConfig(filepath.Join(src, "pixi.toml"))
	files, _ := WalkBundle(src, cfg)
	var assets []AssetFile
	for _, f := range files {
		if f.RelPath == "pixi.toml" || f.RelPath == "pixi.lock" {
			continue
		}
		assets = append(assets, f)
	}
	if len(assets) != n {
		t.Fatalf("walker assets: got %d want %d", len(assets), n)
	}

	repo := host + "/demo/many"
	_, err := PublishWorkspace(context.Background(), src, PublishOptions{
		Repository:   repo,
		Tag:          "v1",
		RegistryHost: host,
		Assets:       assets,
		PlainHTTP:    true,
		Concurrency:  16,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	res, err := PullBundle(context.Background(), repo, "v1", PullOptions{
		Mode:        PullModeFull,
		Concurrency: 16,
		PlainHTTP:   true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(res.Assets) != n {
		t.Fatalf("asset count: got %d want %d", len(res.Assets), n)
	}
	got := make(map[string]string, n)
	for _, a := range res.Assets {
		got[a.Path] = string(a.Bytes)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("asset %q: got %q want %q", k, got[k], v)
		}
	}
}

func TestPublishRejectsUnsafeAssetPath(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	// Asset with unsafe path — the publisher must reject before any
	// network I/O. We construct the AssetFile manually to bypass walker
	// hardening.
	badAbs := filepath.Join(src, "ok.txt")
	writeFile(t, src, "ok.txt", "ok")
	assets := []AssetFile{
		{RelPath: "../escape.txt", AbsPath: badAbs, Size: 2},
	}
	repo := host + "/demo/unsafe"
	_, err := PublishWorkspace(context.Background(), src, PublishOptions{
		Repository:   repo,
		Tag:          "v1",
		RegistryHost: host,
		Assets:       assets,
		PlainHTTP:    true,
	})
	if err == nil {
		t.Fatal("expected publish to reject unsafe path")
	}
	if !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
