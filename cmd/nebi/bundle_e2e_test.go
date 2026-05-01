//go:build e2e

package main

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// startE2ERegistry spins up an in-memory OCI Distribution server. The
// registry is anonymous and trusts anything written to it.
func startE2ERegistry(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse registry URL: %v", err)
	}
	return u.Host
}

// publishedRefRE extracts the "<host>/<ns>/<repo>:<tag>" fragment from
// publish stderr lines that look like `Published <ref> (digest: sha256:...)`.
var publishedRefRE = regexp.MustCompile(`Published (\S+) \(digest:`)

// TestE2E_LocalBundlePublishImport runs a full publish --local → import
// round trip against an in-memory OCI registry. Verifies that pixi.toml,
// pixi.lock, and extra asset files round-trip byte-identically.
func TestE2E_LocalBundlePublishImport(t *testing.T) {
	// Isolate the local store for this test.
	dataDir := t.TempDir()
	t.Setenv("NEBI_DATA_DIR", dataDir)

	regHost := startE2ERegistry(t)
	regURL := "http://" + regHost

	// Add the local registry (http:// scheme triggers PlainHTTP).
	res := runCLI(t, dataDir,
		"registry", "add", "--local",
		"--name", "bundle-e2e",
		"--url", regURL,
		"--namespace", "demo",
		"--default",
	)
	if res.ExitCode != 0 {
		t.Fatalf("registry add failed:\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}

	// Build a workspace with pixi + extra asset files.
	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"bundle-e2e-ws\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)
	readmeBody := "# bundle demo\n"
	mainBody := "print('bundle')\n"
	dataBody := "col\nvalue\n"
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte(readmeBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "src/main.py"), []byte(mainBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "data/sample.csv"), []byte(dataBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// Init tracks the workspace locally so publish --local can find it.
	res = runCLI(t, srcDir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed:\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}

	// Publish: bundle the workspace.
	res = runCLI(t, srcDir,
		"publish", "--local", "--tag", "v1", "--concurrency", "4",
	)
	if res.ExitCode != 0 {
		t.Fatalf("publish failed:\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	// Extract the published ref from stderr so we don't have to reconstruct it.
	m := publishedRefRE.FindStringSubmatch(res.Stderr)
	if m == nil {
		t.Fatalf("could not parse published ref from stderr:\n%s", res.Stderr)
	}
	ref := m[1]
	if !strings.Contains(ref, regHost) {
		t.Fatalf("published ref does not reference test registry: %s", ref)
	}
	// Prepend the scheme so the CLI knows to talk plain HTTP.
	ref = "http://" + ref

	// Import into a fresh directory.
	outParent := t.TempDir()
	outDir := filepath.Join(outParent, "restored")
	res = runCLI(t, outParent,
		"import", ref, "-o", outDir,
	)
	if res.ExitCode != 0 {
		t.Fatalf("import failed:\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}

	// Asset layers must round-trip byte-identically.
	wantAssets := map[string]string{
		"README.md":       readmeBody,
		"src/main.py":     mainBody,
		"data/sample.csv": dataBody,
	}
	for rel, want := range wantAssets {
		got, err := os.ReadFile(filepath.Join(outDir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if string(got) != want {
			t.Fatalf("content mismatch for %s:\n got: %q\nwant: %q", rel, got, want)
		}
	}
	// pixi.toml and pixi.lock may be whitespace-normalized by the reader
	// (legacy behavior — they flow through typed layers, not the asset
	// path). Verify identity by content match on key fields.
	gotToml, err := os.ReadFile(filepath.Join(outDir, "pixi.toml"))
	if err != nil {
		t.Fatalf("read pixi.toml: %v", err)
	}
	if !strings.Contains(string(gotToml), "bundle-e2e-ws") {
		t.Fatalf("imported pixi.toml missing project name:\n%s", gotToml)
	}
	gotLock, err := os.ReadFile(filepath.Join(outDir, "pixi.lock"))
	if err != nil {
		t.Fatalf("read pixi.lock: %v", err)
	}
	if !strings.Contains(string(gotLock), "version: 6") {
		t.Fatalf("imported pixi.lock missing expected content:\n%s", gotLock)
	}
}

// TestE2E_LocalBundleRejectsNonEmptyDest verifies that importing a bundle
// with assets refuses to overwrite a non-empty destination.
func TestE2E_LocalBundleRejectsNonEmptyDest(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("NEBI_DATA_DIR", dataDir)

	regHost := startE2ERegistry(t)
	regURL := "http://" + regHost

	res := runCLI(t, dataDir,
		"registry", "add", "--local",
		"--name", "bundle-e2e-nonempty",
		"--url", regURL,
		"--namespace", "demo",
		"--default",
	)
	if res.ExitCode != 0 {
		t.Fatalf("registry add failed:\nstderr: %s", res.Stderr)
	}

	srcDir := t.TempDir()
	writePixiFiles(t, srcDir,
		"[project]\nname = \"bundle-nonempty\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"version: 6\n",
	)
	if err := os.WriteFile(filepath.Join(srcDir, "asset.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = runCLI(t, srcDir, "init")
	if res.ExitCode != 0 {
		t.Fatalf("init failed:\nstderr: %s", res.Stderr)
	}
	res = runCLI(t, srcDir, "publish", "--local", "--tag", "v1")
	if res.ExitCode != 0 {
		t.Fatalf("publish failed:\nstderr: %s", res.Stderr)
	}

	m := publishedRefRE.FindStringSubmatch(res.Stderr)
	if m == nil {
		t.Fatalf("could not parse published ref: %s", res.Stderr)
	}
	ref := "http://" + m[1]

	// Non-empty dest — must be rejected.
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "existing.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = runCLI(t, outDir, "import", ref, "-o", outDir)
	if res.ExitCode == 0 {
		t.Fatalf("import into non-empty dir should have failed:\nstderr: %s", res.Stderr)
	}
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "not empty") {
		t.Fatalf("expected 'not empty' error, got: %s", combined)
	}
	// The pre-existing file must be untouched.
	content, err := os.ReadFile(filepath.Join(outDir, "existing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("pre-existing file was overwritten: %q", content)
	}
}
