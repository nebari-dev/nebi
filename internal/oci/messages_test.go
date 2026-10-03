package oci

import (
	"context"
	"strings"
	"testing"

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
		if err == nil || !strings.HasPrefix(err.Error(), "unsafe path in bundle: ../escape.txt: ") {
			t.Fatalf("message: got %v, want prefix %q", err, "unsafe path in bundle: ../escape.txt: ")
		}
	})
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
