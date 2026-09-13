package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// startTestRegistryRejectingEmptyBlob spins up an in-memory registry
// wrapped with a shim that returns 404 for GET and HEAD on the empty
// blob, faithfully reproducing the Quay behaviour. Upload of the empty
// blob still succeeds so publish isn't blocked.
func startTestRegistryRejectingEmptyBlob(t *testing.T) string {
	t.Helper()
	inner := registry.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			strings.Contains(r.URL.Path, "/blobs/"+emptyBlobDigest.String()) {
			http.Error(w, "blob not found", http.StatusNotFound)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u.Host
}

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

// startBodyRewritingRegistry proxies an inner registry and lets tests
// replace selected GET bodies while keeping the rest of the registry behavior.
func startBodyRewritingRegistry(
	t *testing.T,
	innerHost string,
	shouldRewrite func(*http.Request) bool,
	rewrite func([]byte) ([]byte, error),
) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://"+innerHost+r.URL.RequestURI(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for k, vs := range r.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if r.Method == http.MethodGet && resp.StatusCode == http.StatusOK && shouldRewrite(r) {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			body, err = rewrite(body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for k, vs := range resp.Header {
				if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Docker-Content-Digest") {
					continue
				}
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(resp.StatusCode)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			_, _ = w.Write(body)
			return
		}

		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u.Host
}

// startManifestRewritingRegistry proxies an inner registry and lets tests
// replace manifest GET bodies while keeping the rest of the registry behavior.
func startManifestRewritingRegistry(t *testing.T, innerHost string, rewrite func([]byte) ([]byte, error)) string {
	t.Helper()
	return startBodyRewritingRegistry(t, innerHost, func(r *http.Request) bool {
		return strings.Contains(r.URL.Path, "/manifests/")
	}, rewrite)
}

func oversizedNebiManifest(body []byte) ([]byte, error) {
	var manifest ocispec.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, err
	}
	if manifest.Annotations == nil {
		manifest.Annotations = map[string]string{}
	}
	manifest.Annotations["dev.nebi.test.padding"] = strings.Repeat("X", int(maxManifestBytes)+1)
	return json.Marshal(manifest)
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

// readAsset reads an extracted asset from disk and compares against want.
// Used by tests that verify round-trip byte-identity after ExtractBundle.
func readAsset(t *testing.T, destDir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(destDir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
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

	dest := t.TempDir()
	pull, err := ExtractBundle(context.Background(), res.Repository, "v1", dest, PullOptions{
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
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
	for rel, w := range want {
		if got := readAsset(t, dest, rel); got != w {
			t.Fatalf("asset %q mismatch: got %q want %q", rel, got, w)
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

	dest := t.TempDir()
	pull, err := ExtractBundle(context.Background(), res.Repository, "v1", dest, PullOptions{
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(pull.Assets) != 0 {
		t.Fatalf("want zero assets, got %d", len(pull.Assets))
	}
	if !strings.Contains(pull.PixiToml, "legacy") {
		t.Fatalf("unexpected pixi.toml %q", pull.PixiToml)
	}
}

func TestPullBundle_ListsAssetsWithoutFetchingBytes(t *testing.T) {
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
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(pull.Assets) != 1 {
		t.Fatalf("expected 1 asset listed, got %d", len(pull.Assets))
	}
	if pull.Assets[0].Path != "big.bin" {
		t.Fatalf("unexpected asset path %q", pull.Assets[0].Path)
	}
}

func TestPullBundle_RejectsOversizedManifestBody(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "bigmanifest", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	shimHost := startManifestRewritingRegistry(t, host, oversizedNebiManifest)
	repoRef := strings.Replace(res.Repository, host, shimHost, 1)

	_, err = PullBundle(context.Background(), repoRef, "v1", PullOptions{PlainHTTP: true})
	if err == nil {
		t.Fatalf("expected oversized-manifest rejection, got nil")
	}
	if want := fmt.Sprintf("manifest exceeds %d bytes", maxManifestBytes); !strings.Contains(err.Error(), want) {
		t.Fatalf("expected manifest cap error, got: %v", err)
	}
}

func TestIsNebiRepository_RejectsOversizedManifestBody(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "filter", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	shimHost := startManifestRewritingRegistry(t, host, oversizedNebiManifest)
	repoRef := strings.Replace(res.Repository, host, shimHost, 1)

	if IsNebiRepository(context.Background(), repoRef, BrowseOptions{PlainHTTP: true}) {
		t.Fatal("expected oversized manifest to be rejected by repository filter")
	}
}

func TestBundleReads_RejectCoreLayerDeclaredAboveCap(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "hugecore", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	shimHost := startManifestRewritingRegistry(t, host, func(body []byte) ([]byte, error) {
		var manifest ocispec.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			return nil, err
		}
		for i := range manifest.Layers {
			if manifest.Layers[i].MediaType == MediaTypePixiLock {
				manifest.Layers[i].Size = defaultMaxCoreBytes + 1
			}
		}
		return json.Marshal(manifest)
	})
	repoRef := strings.Replace(res.Repository, host, shimHost, 1)
	wantErr := fmt.Sprintf("pixi.lock layer size %d bytes exceeds cap %d bytes", defaultMaxCoreBytes+1, defaultMaxCoreBytes)

	_, err = PullBundle(context.Background(), repoRef, "v1", PullOptions{PlainHTTP: true})
	if err == nil {
		t.Fatalf("expected oversized core layer rejection, got nil")
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected pixi.lock cap error, got: %v", err)
	}

	_, err = ExtractBundle(context.Background(), repoRef, "v1", t.TempDir(), PullOptions{PlainHTTP: true})
	if err == nil {
		t.Fatalf("expected extract to reject oversized core layer, got nil")
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected pixi.lock cap error from extract, got: %v", err)
	}
}

func TestBundleReads_UsesConfiguredCoreLayerCap(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", strings.Repeat("L", 33))

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "configuredcore", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	_, err = PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		PlainHTTP:         true,
		MaxCoreLayerBytes: 32,
	})
	if err == nil {
		t.Fatalf("expected configured core cap rejection, got nil")
	}
	if !strings.Contains(err.Error(), "pixi.lock layer size 33 bytes exceeds cap 32 bytes") {
		t.Fatalf("expected pixi.lock cap error, got: %v", err)
	}

	if _, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		PlainHTTP:         true,
		MaxCoreLayerBytes: 33,
	}); err != nil {
		t.Fatalf("pull with configured cap: %v", err)
	}
}

func TestBundleReads_AllowsCoreLayerAboveDefaultWithConfiguredCap(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", strings.Repeat("L", int(defaultMaxCoreBytes)+1))

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "raisedcore", "v1",
		WithMaxCoreLayerBytes(defaultMaxCoreBytes+1),
	)
	if err != nil {
		t.Fatalf("publish with raised cap: %v", err)
	}

	_, err = PullBundle(context.Background(), res.Repository, "v1", PullOptions{PlainHTTP: true})
	if err == nil {
		t.Fatal("expected default cap to reject oversized core layer")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("pixi.lock layer size %d bytes exceeds cap %d bytes", defaultMaxCoreBytes+1, defaultMaxCoreBytes)) {
		t.Fatalf("expected default pixi.lock cap error, got: %v", err)
	}

	if _, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		PlainHTTP:         true,
		MaxCoreLayerBytes: defaultMaxCoreBytes + 1,
	}); err != nil {
		t.Fatalf("pull with raised cap: %v", err)
	}
}

func TestBundleReads_RejectNegativeCoreLayerSize(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "negativecore", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	shimHost := startManifestRewritingRegistry(t, host, func(body []byte) ([]byte, error) {
		var manifest ocispec.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			return nil, err
		}
		for i := range manifest.Layers {
			if manifest.Layers[i].MediaType == MediaTypePixiLock {
				manifest.Layers[i].Size = -1
			}
		}
		return json.Marshal(manifest)
	})
	repoRef := strings.Replace(res.Repository, host, shimHost, 1)

	_, err = ExtractBundle(context.Background(), repoRef, "v1", t.TempDir(), PullOptions{PlainHTTP: true})
	if err == nil {
		t.Fatalf("expected negative core layer size rejection, got nil")
	}
	if !strings.Contains(err.Error(), "pixi.lock layer has invalid negative size -1 bytes") {
		t.Fatalf("expected negative size error, got: %v", err)
	}
}

// TestFetchLayerBytes_RejectsOversizedBody proves fetchLayerBytes rejects
// a registry that declares a small layer Size but streams more bytes
// (chunked, no Content-Length). Without the LimitReader bound the server
// would buffer the full body into RAM regardless of the declared size.
func TestFetchLayerBytes_RejectsOversizedBody(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "fakesize", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Wrap the inner registry with a shim that appends extra bytes to
	// blob fetches, simulating a hostile registry that lies about layer size.
	shimHost := startBodyRewritingRegistry(t, host, func(r *http.Request) bool {
		return strings.Contains(r.URL.Path, "/blobs/sha256:")
	}, func(body []byte) ([]byte, error) {
		return append(body, []byte(strings.Repeat("X", 4096))...), nil
	})

	// Rewrite repoRef from the original host to the shim host.
	repoRef := strings.Replace(res.Repository, host, shimHost, 1)

	_, err = PullBundle(context.Background(), repoRef, "v1", PullOptions{PlainHTTP: true})
	if err == nil {
		t.Fatalf("expected oversized-body rejection, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds declared size") {
		t.Fatalf("expected size-mismatch error, got: %v", err)
	}
}

// TestPullBundle_RejectsOversizedBundle proves MaxBundleBytes is
// enforced before any blob is fetched, so a registry serving a runaway
// asset cannot exhaust disk through the import path.
func TestPullBundle_RejectsOversizedBundle(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"big\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "asset.bin", strings.Repeat("Q", 8192))

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "big", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	_, err = PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		PlainHTTP:      true,
		MaxBundleBytes: 4096, // less than the asset alone
	})
	if err == nil {
		t.Fatalf("expected size-cap rejection, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds cap") {
		t.Fatalf("expected cap error, got: %v", err)
	}

	// Same call without cap should succeed.
	if _, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{
		PlainHTTP: true,
	}); err != nil {
		t.Fatalf("pull without cap: %v", err)
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

	dest := t.TempDir()
	pull, err := ExtractBundle(context.Background(), res.Repository, "v1", dest, PullOptions{
		Concurrency: 16, PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(pull.Assets) != n {
		t.Fatalf("asset count: got %d want %d", len(pull.Assets), n)
	}
	for k, v := range want {
		if got := readAsset(t, dest, k); got != v {
			t.Fatalf("asset %q: got %q want %q", k, got, v)
		}
	}
}

// TestPublish_RejectsSymlinkedCoreFile covers the Lstat-on-core-files
// fix. A workspace where pixi.toml is a symlink pointing outside the
// workspace must be rejected — file.Store.Add follows symlinks when it
// reads the core file, so without an Lstat guard the target's contents
// would be bundled under the innocent pixi.toml title.
func TestPublish_RejectsSymlinkedCoreFile(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	externalDir := t.TempDir()
	external := filepath.Join(externalDir, "secret.txt")
	if err := os.WriteFile(external, []byte("s3cret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(src, "pixi.toml")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	writeFile(t, src, "pixi.lock", "version: 6\n")

	_, err := Publish(context.Background(), src, testRegistry(host, "demo"), "symcore", "v1")
	if err == nil {
		t.Fatal("expected publish to reject symlinked pixi.toml")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected 'regular file' error, got %v", err)
	}
}

func TestPublish_RejectsCoreFileAboveConfiguredCap(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", strings.Repeat("L", 65))

	_, err := Publish(context.Background(), src, Registry{Host: "registry.invalid"}, "corecap", "v1",
		WithMaxCoreLayerBytes(64),
	)
	if err == nil {
		t.Fatal("expected publish to reject oversized core file")
	}
	if !strings.Contains(err.Error(), "pixi.lock layer size 65 bytes exceeds cap 64 bytes") {
		t.Fatalf("expected pixi.lock cap error, got %v", err)
	}
}

func TestPublish_RejectsManifestAboveCap(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "seed.bin", "x")

	const assetCount = 6000
	longName := strings.Repeat("a", 180)
	assets := make([]Asset, 0, assetCount)
	for i := 0; i < assetCount; i++ {
		assets = append(assets, Asset{
			RelPath: fmt.Sprintf("assets/%04d-%s.txt", i, longName),
			AbsPath: filepath.Join(src, "seed.bin"),
			Size:    1,
		})
	}

	_, err := Publish(context.Background(), src, Registry{Host: "registry.invalid"}, "bigmanifest", "v1",
		withAssets(assets),
	)
	if err == nil {
		t.Fatal("expected publish to reject oversized manifest")
	}
	want := "manifest size"
	if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), fmt.Sprintf("cap %d", maxManifestBytes)) {
		t.Fatalf("expected manifest cap error, got %v", err)
	}
}

func TestPublish_RejectsUnsafeAssetPath(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"x\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	writeFile(t, src, "ok.txt", "ok")

	// withAssets bypasses the walker; we inject a malicious path to verify
	// the publisher's pre-network validation fires.
	bad := []Asset{{RelPath: "../escape.txt", AbsPath: filepath.Join(src, "ok.txt"), Size: 2}}
	_, err := Publish(context.Background(), src, testRegistry(host, "demo"), "unsafe", "v1",
		withAssets(bad),
	)
	if err == nil {
		t.Fatal("expected publish to reject unsafe path")
	}
	if !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPublishBundle_EmptyAssetRoundTrip_RegistryRefusesEmptyBlob
// reproduces the Quay-in-the-wild behaviour: a registry that refuses to
// serve the canonical empty-blob digest on GET. Extraction must still
// produce a zero-byte file for the empty asset — oras.Copy's content
// store short-circuits on the known empty-blob digest and writes the
// file without ever issuing the blob GET.
func TestPublishBundle_EmptyAssetRoundTrip_RegistryRefusesEmptyBlob(t *testing.T) {
	host := startTestRegistryRejectingEmptyBlob(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"empty\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")
	// Zero-byte asset — the regression case.
	writeFile(t, src, "empty.txt", "")
	// Sibling non-empty asset so we still exercise the normal fetch path.
	writeFile(t, src, "README.md", "hi\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "empty", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	dest := t.TempDir()
	pull, err := ExtractBundle(context.Background(), res.Repository, "v1", dest, PullOptions{
		PlainHTTP: true,
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	want := map[string]string{
		"empty.txt": "",
		"README.md": "hi\n",
	}
	if len(pull.Assets) != len(want) {
		t.Fatalf("asset count: got %d want %d", len(pull.Assets), len(want))
	}
	for rel, w := range want {
		if got := readAsset(t, dest, rel); got != w {
			t.Fatalf("asset %q: got %q want %q", rel, got, w)
		}
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
		PlainHTTP: true,
	}); err == nil {
		t.Fatal("expected 'latest' tag to be missing when WithExtraTags was not supplied")
	}
}

// TestPullBundle_ReturnsManifestDigest covers the TOCTOU fix. For
// callers to pin the second network round (extract) to the same
// content they peeked, PullBundle must expose the resolved manifest
// digest so ExtractBundle can copy by digest instead of by tag.
func TestPullBundle_ReturnsManifestDigest(t *testing.T) {
	host := startTestRegistry(t)
	src := t.TempDir()
	writeFile(t, src, "pixi.toml", "[workspace]\nname = \"digest\"\n")
	writeFile(t, src, "pixi.lock", "version: 6\n")

	res, err := Publish(context.Background(), src, testRegistry(host, "demo"), "digest", "v1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	pull, err := PullBundle(context.Background(), res.Repository, "v1", PullOptions{PlainHTTP: true})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if pull.Digest == "" {
		t.Fatal("PullResult.Digest empty — callers need the digest to pin Extract across a tag move")
	}
	if pull.Digest != res.Digest {
		t.Fatalf("digest mismatch: pull=%q publish=%q", pull.Digest, res.Digest)
	}
}
