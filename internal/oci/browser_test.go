package oci

import (
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
			name: "unknown media type ignored",
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc("application/vnd.example.future.v2", "future.bin"),
			},
			wantAssets: nil,
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
