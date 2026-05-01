package oci

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func layerDesc(mediaType, title string) ocispec.Descriptor {
	d := ocispec.Descriptor{MediaType: mediaType}
	if title != "" {
		d.Annotations = map[string]string{ocispec.AnnotationTitle: title}
	}
	return d
}

// TestClassifyBundleManifest_RejectsBadCoreTitle covers the core-title
// validation fix. Publisher always sets AnnotationTitle to pixi.toml /
// pixi.lock exactly; a crafted manifest could carry a hostile title
// like "../evil" on the core layer. ExtractBundle writes via that title
// through oras.Copy + file.Store. Belt-and-suspenders: reject at
// classify time regardless of file.Store's traversal guard.
func TestClassifyBundleManifest_RejectsBadCoreTitle(t *testing.T) {
	cases := []struct {
		name      string
		tomlTitle string
		lockTitle string
	}{
		{"toml title missing", "", "pixi.lock"},
		{"toml title traversal", "../evil.toml", "pixi.lock"},
		{"toml title absolute", "/etc/passwd", "pixi.lock"},
		{"toml title wrong name", "not-pixi.toml", "pixi.lock"},
		{"lock title missing", "pixi.toml", ""},
		{"lock title traversal", "pixi.toml", "../evil.lock"},
		{"lock title wrong name", "pixi.toml", "pixi.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := ocispec.Manifest{Layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, tc.tomlTitle),
				layerDesc(MediaTypePixiLock, tc.lockTitle),
			}}
			if _, err := classifyBundleManifest(m); err == nil {
				t.Fatalf("expected error for core titles %q/%q", tc.tomlTitle, tc.lockTitle)
			}
		})
	}
}

// TestClassifyBundleManifest_RejectsUnknownLayer isolates the unknown-
// media-type case. Layers with unfamiliar types previously slipped
// through classification but were still downloaded by oras.Copy in
// ExtractBundle, writing arbitrary blobs to disk. Classifier must
// reject the manifest instead of silently tolerating it.
func TestClassifyBundleManifest_RejectsUnknownLayer(t *testing.T) {
	m := ocispec.Manifest{Layers: []ocispec.Descriptor{
		layerDesc(MediaTypePixiToml, "pixi.toml"),
		layerDesc(MediaTypePixiLock, "pixi.lock"),
		layerDesc("application/vnd.example.future.v2", "future.bin"),
	}}
	_, err := classifyBundleManifest(m)
	if err == nil {
		t.Fatal("expected unknown media type to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown media type") {
		t.Fatalf("expected 'unknown media type' in error, got %v", err)
	}
}

func TestClassifyBundleManifest(t *testing.T) {
	cases := []struct {
		name       string
		layers     []ocispec.Descriptor
		wantErr    string
		wantAssets []string
	}{
		{
			name: "legacy 2-layer",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
			},
			wantAssets: nil,
		},
		{
			name: "with assets",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc(MediaTypeNebiAsset, "README.md"),
				layerDesc(MediaTypeNebiAsset, "src/main.go"),
			},
			wantAssets: []string{"README.md", "src/main.go"},
		},
		{
			name: "missing pixi.lock",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
			},
			wantErr: "missing pixi",
		},
		{
			name: "duplicate pixi.toml",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
			},
			wantErr: "duplicate core layer",
		},
		{
			name: "unsafe asset title",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc(MediaTypeNebiAsset, "../escape.txt"),
			},
			wantErr: "unsafe path",
		},
		{
			name: "dup asset title",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc(MediaTypeNebiAsset, "a.txt"),
				layerDesc(MediaTypeNebiAsset, "a.txt"),
			},
			wantErr: "collision",
		},
		{
			name: "unknown media type rejected",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc("application/vnd.example.future.v2", "future.bin"),
			},
			wantErr: "unknown media type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := ocispec.Manifest{Layers: tc.layers}
			got, err := classifyBundleManifest(m)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want err containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotPaths := make([]string, len(got.assets))
			for i, a := range got.assets {
				gotPaths[i] = a.Annotations[ocispec.AnnotationTitle]
			}
			if len(gotPaths) != len(tc.wantAssets) {
				t.Fatalf("asset count: got %v want %v", gotPaths, tc.wantAssets)
			}
			for i, p := range gotPaths {
				if p != tc.wantAssets[i] {
					t.Fatalf("asset[%d]: got %s want %s", i, p, tc.wantAssets[i])
				}
			}
		})
	}
}

// TestListRepositoriesViaQuayAPI_FollowsPagination verifies that the
// Quay REST client follows the `next_page` cursor. The old
// implementation fetched one page and returned, so namespaces with
// >50 repos saw silently truncated browse lists.
func TestListRepositoriesViaQuayAPI_FollowsPagination(t *testing.T) {
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		next := r.URL.Query().Get("next_page")
		w.Header().Set("Content-Type", "application/json")
		if next == "" {
			_, _ = io.WriteString(w, `{"repositories":[{"namespace":"ns","name":"r1","is_public":true}],"next_page":"cursor-2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"repositories":[{"namespace":"ns","name":"r2","is_public":false}]}`)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs,
	}}}

	repos, err := ListRepositoriesViaQuayAPIWithClient(context.Background(), u.Host, "ns", "", client)
	if err != nil {
		t.Fatalf("list repos: %v", err)
	}
	if hits < 2 {
		t.Fatalf("expected >=2 requests for pagination, got %d", hits)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos across pages, got %d (%v)", len(repos), repos)
	}
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, r.Name)
	}
	if !containsAll(names, "ns/r1", "ns/r2") {
		t.Fatalf("expected both pages' repos, got %v", names)
	}
}

func containsAll(haystack []string, needles ...string) bool {
	for _, n := range needles {
		found := false
		for _, h := range haystack {
			if strings.EqualFold(h, n) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
