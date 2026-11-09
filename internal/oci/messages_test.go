package oci

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// The CLI (`nebi import`) prints bundle errors as they are. Typing them
// for the API must not change what CLI users see, so these tests pin the
// exact messages.

func TestBundleErrorMessages_Unchanged(t *testing.T) {
	cases := map[string]struct {
		layers []ocispec.Descriptor
		want   string
	}{
		"missing core layers": {
			layers: []ocispec.Descriptor{layerDesc(MediaTypeNebiAsset, "README.md")},
			want:   "invalid bundle: missing pixi.{toml,lock}",
		},
		"duplicate core layer": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
			},
			want: "invalid bundle: duplicate core layer",
		},
		"wrong pixi.toml title": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "other.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
			},
			want: `invalid bundle: pixi.toml core layer has title "other.toml", expected "pixi.toml"`,
		},
		"wrong pixi.lock title": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "other.lock"),
			},
			want: `invalid bundle: pixi.lock core layer has title "other.lock", expected "pixi.lock"`,
		},
		"unknown media type": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc("application/vnd.example.future.v2", "future.bin"),
			},
			want: `invalid bundle: unknown media type "application/vnd.example.future.v2"`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := classifyBundleManifest(ocispec.Manifest{Layers: tc.layers})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("message:\n got %v\nwant %s", err, tc.want)
			}
		})
	}

	t.Run("unsafe asset path", func(t *testing.T) {
		_, err := classifyBundleManifest(ocispec.Manifest{Layers: []ocispec.Descriptor{
			layerDesc(MediaTypePixiToml, "pixi.toml"),
			layerDesc(MediaTypePixiLock, "pixi.lock"),
			layerDesc(MediaTypeNebiAsset, "../escape.txt"),
		}})
		const want = "unsafe path in bundle: ../escape.txt: parent segment not allowed"
		if err == nil || err.Error() != want {
			t.Fatalf("message:\n got %v\nwant %s", err, want)
		}
	})
}

// TestExtractBundleZeroSizeMessage_Unchanged pins the message `nebi
// import` prints for a zero-size layer whose digest is not the empty
// digest.
func TestExtractBundleZeroSizeMessage_Unchanged(t *testing.T) {
	const digest = "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"
	host := startManifestRegistry(t, zeroSizeManifest(MediaTypeNebiAsset, "data.bin", digest))

	_, err := ExtractBundle(context.Background(), host+"/demo/zero", "v1", t.TempDir(), PullOptions{PlainHTTP: true})

	const want = `extract bundle: zero-size layer "data.bin" has non-empty digest ` + digest
	if err == nil || err.Error() != want {
		t.Fatalf("message:\n got %v\nwant %s", err, want)
	}
}

func TestBundlePullMessages_Unchanged(t *testing.T) {
	host := startTestRegistry(t)
	pushRawArtifact(t, host, "demo/image", "v1", "application/vnd.oci.image.config.v1+json",
		map[string]string{"layer.tar": "application/vnd.oci.image.layer.v1.tar"})

	cases := map[string]struct {
		repo, tag, want string
	}{
		"not a Nebi artifact": {
			repo: host + "/demo/image", tag: "v1",
			want: "not a Nebi artifact",
		},
		"missing tag": {
			repo: host + "/demo/image", tag: "nope",
			want: "failed to resolve tag nope: " + host + "/demo/image:nope: not found",
		},
		"missing repository": {
			repo: host + "/demo/absent", tag: "v1",
			want: "failed to resolve tag v1: " + host + "/demo/absent:v1: not found",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := PullOptions{PlainHTTP: true}
			_, pullErr := PullBundle(context.Background(), tc.repo, tc.tag, opts)
			_, extractErr := ExtractBundle(context.Background(), tc.repo, tc.tag, t.TempDir(), opts)
			for path, err := range map[string]error{"PullBundle": pullErr, "ExtractBundle": extractErr} {
				if err == nil || err.Error() != tc.want {
					t.Errorf("%s message:\n got %v\nwant %s", path, err, tc.want)
				}
			}
		})
	}
}

// zeroSizeManifest is a bundle manifest with empty pixi.toml and
// pixi.lock layers plus, when mediaType is the asset type, one asset. The
// layer of the given mediaType declares Size 0 with a non-empty digest.
func zeroSizeManifest(mediaType, title string, lyingDigest digest.Digest) ocispec.Manifest {
	empty := func(mediaType, title string) ocispec.Descriptor {
		d := layerDesc(mediaType, title)
		d.Digest = emptyBlobDigest
		return d
	}
	m := ocispec.Manifest{
		Config: ocispec.Descriptor{MediaType: MediaTypePixiConfig, Digest: emptyBlobDigest},
		Layers: []ocispec.Descriptor{empty(MediaTypePixiToml, "pixi.toml"), empty(MediaTypePixiLock, "pixi.lock")},
	}
	lying := layerDesc(mediaType, title)
	lying.Digest = lyingDigest
	switch mediaType {
	case MediaTypePixiToml:
		m.Layers[0] = lying
	case MediaTypePixiLock:
		m.Layers[1] = lying
	default:
		m.Layers = append(m.Layers, lying)
	}
	return m
}

// startManifestRegistry serves m as the manifest of every reference and
// an empty body for every blob.
func startManifestRegistry(t *testing.T, m ocispec.Manifest) string {
	t.Helper()
	m.SchemaVersion = 2
	m.MediaType = ocispec.MediaTypeImageManifest
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/manifests/") {
			return
		}
		w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
		w.Header().Set("Docker-Content-Digest", digest.FromBytes(body).String())
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}
