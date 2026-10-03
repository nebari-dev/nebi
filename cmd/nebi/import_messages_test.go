package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/oci"
)

// TestImport_ZeroSizeLayerMessages pins what `nebi import` prints for a
// bundle with a layer that declares size 0 but has the digest of
// non-empty content. The registry serves the real content, as a real
// registry would.
func TestImport_ZeroSizeLayerMessages(t *testing.T) {
	blobs := map[string][]byte{}
	desc := func(mediaType, title, content string, zeroSize bool) map[string]any {
		sum := sha256.Sum256([]byte(content))
		digest := "sha256:" + hex.EncodeToString(sum[:])
		blobs[digest] = []byte(content)
		size := len(content)
		if zeroSize {
			size = 0
		}
		return map[string]any{
			"mediaType": mediaType, "digest": digest, "size": size,
			"annotations": map[string]string{"org.opencontainers.image.title": title},
		}
	}
	const (
		toml  = "[workspace]\nname = \"zero\"\n"
		lock  = "version: 6\n"
		asset = "asset bytes\n"
	)
	digestOf := func(content string) string {
		sum := sha256.Sum256([]byte(content))
		return "sha256:" + hex.EncodeToString(sum[:])
	}

	cases := map[string]struct {
		zeroToml, zeroLock, zeroAsset bool
		want                          func(host, outDir string) string
	}{
		"pixi.toml": {
			zeroToml: true,
			want: func(host, outDir string) string {
				return `failed to pull from registry: failed to fetch pixi.toml layer: GET "http://` + host +
					`/v2/demo/zero/blobs/` + digestOf(toml) + `": mismatch Content-Length`
			},
		},
		"pixi.lock": {
			zeroLock: true,
			want: func(host, outDir string) string {
				return `failed to pull from registry: failed to fetch pixi.lock layer: GET "http://` + host +
					`/v2/demo/zero/blobs/` + digestOf(lock) + `": mismatch Content-Length`
			},
		},
		"asset": {
			zeroAsset: true,
			want: func(host, outDir string) string {
				return `import failed: extract bundle: zero-size layer "data.bin" has non-empty digest ` +
					digestOf(asset) + `; partial files at ` + outDir
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			config := desc(oci.MediaTypePixiConfig, "", "{}", false)
			delete(config, "annotations")
			manifest, err := json.Marshal(map[string]any{
				"schemaVersion": 2,
				"mediaType":     "application/vnd.oci.image.manifest.v1+json",
				"config":        config,
				"layers": []map[string]any{
					desc(oci.MediaTypePixiToml, "pixi.toml", toml, tc.zeroToml),
					desc(oci.MediaTypePixiLock, "pixi.lock", lock, tc.zeroLock),
					desc(oci.MediaTypeNebiAsset, "data.bin", asset, tc.zeroAsset),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			manifestSum := sha256.Sum256(manifest)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := manifest
				if strings.Contains(r.URL.Path, "/manifests/") {
					w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
					w.Header().Set("Docker-Content-Digest", "sha256:"+hex.EncodeToString(manifestSum[:]))
				} else {
					var ok bool
					if body, ok = blobs[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]]; !ok {
						http.NotFound(w, r)
						return
					}
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
				if r.Method != http.MethodHead {
					_, _ = w.Write(body)
				}
			}))
			t.Cleanup(srv.Close)
			host := strings.TrimPrefix(srv.URL, "http://")

			savedOutput, savedForce := importOutput, importForce
			t.Cleanup(func() { importOutput, importForce = savedOutput, savedForce })
			importOutput, importForce = filepath.Join(t.TempDir(), "out"), true
			outDir, _ := filepath.Abs(importOutput)

			err = runImport(importCmd, []string{srv.URL + "/demo/zero:v1"})

			if want := tc.want(host, outDir); err == nil || err.Error() != want {
				t.Fatalf("message:\n got %v\nwant %s", err, want)
			}
		})
	}
}
