package oci

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
)

// TestClassifyBundleManifest_SafeReasons pins that every manifest
// rejection is an InvalidBundleError whose Reason is fixed text: it must
// not quote the layer title, media type or asset path that triggered it,
// because the registry controls those values.
func TestClassifyBundleManifest_SafeReasons(t *testing.T) {
	const planted = "Bearer PLANTED-SECRET"
	cases := map[string]struct {
		layers     []ocispec.Descriptor
		wantReason string
	}{
		"missing core layers": {
			layers:     []ocispec.Descriptor{layerDesc(MediaTypeNebiAsset, planted)},
			wantReason: "missing pixi.{toml,lock}",
		},
		"duplicate core layer": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
			},
			wantReason: "duplicate core layer",
		},
		"wrong pixi.toml title": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, planted),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
			},
			wantReason: "pixi.toml core layer has an unexpected title",
		},
		"wrong pixi.lock title": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, planted),
			},
			wantReason: "pixi.lock core layer has an unexpected title",
		},
		"unknown media type": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc("application/vnd."+planted, "future.bin"),
			},
			wantReason: "layer has an unknown media type",
		},
		"unsafe asset path": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc(MediaTypeNebiAsset, "../"+planted),
			},
			wantReason: "unsafe or colliding asset path",
		},
		"colliding asset paths": {
			layers: []ocispec.Descriptor{
				layerDesc(MediaTypePixiToml, "pixi.toml"),
				layerDesc(MediaTypePixiLock, "pixi.lock"),
				layerDesc(MediaTypeNebiAsset, planted),
				layerDesc(MediaTypeNebiAsset, planted),
			},
			wantReason: "unsafe or colliding asset path",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := classifyBundleManifest(ocispec.Manifest{Layers: tc.layers})
			if !errors.Is(err, ErrInvalidBundle) {
				t.Fatalf("want ErrInvalidBundle, got %v", err)
			}
			var invalid *InvalidBundleError
			if !errors.As(err, &invalid) {
				t.Fatalf("want *InvalidBundleError, got %T", err)
			}
			if invalid.Reason != tc.wantReason {
				t.Errorf("Reason: got %q want %q", invalid.Reason, tc.wantReason)
			}
			if strings.Contains(invalid.Reason, "PLANTED") {
				t.Errorf("Reason quotes manifest content: %q", invalid.Reason)
			}
		})
	}
}

// pushRawArtifact publishes an arbitrary OCI artifact (any config media
// type, any layers) so tests can present things that are not valid Nebi
// bundles. layers maps a title to its media type.
func pushRawArtifact(t *testing.T, host, repoPath, tag, configMediaType string, layers map[string]string) {
	t.Helper()
	ctx := context.Background()
	repo, err := remote.NewRepository(host + "/" + repoPath)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	repo.PlainHTTP = true
	cfg, err := oras.PushBytes(ctx, repo, configMediaType, []byte("{}"))
	if err != nil {
		t.Fatalf("push config: %v", err)
	}
	var descs []ocispec.Descriptor
	for title, mediaType := range layers {
		d, err := oras.PushBytes(ctx, repo, mediaType, []byte("content of "+title))
		if err != nil {
			t.Fatalf("push layer %s: %v", title, err)
		}
		d.Annotations = map[string]string{ocispec.AnnotationTitle: title}
		descs = append(descs, d)
	}
	manifest, err := oras.PackManifest(ctx, repo, oras.PackManifestVersion1_1, "", oras.PackManifestOptions{
		ConfigDescriptor: &cfg,
		Layers:           descs,
	})
	if err != nil {
		t.Fatalf("pack manifest: %v", err)
	}
	if err := repo.Tag(ctx, manifest, tag); err != nil {
		t.Fatalf("tag: %v", err)
	}
}

// startStatusRegistry answers every request with the given status and no
// authentication challenge, like a registry refusing an anonymous pull.
func startStatusRegistry(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(status), status)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// pullAndExtract runs both bundle entry points against the same
// reference and hands each error to check.
func pullAndExtract(t *testing.T, repoRef, tag string, check func(t *testing.T, err error)) {
	t.Helper()
	opts := PullOptions{PlainHTTP: true}
	t.Run("PullBundle", func(t *testing.T) {
		_, err := PullBundle(context.Background(), repoRef, tag, opts)
		check(t, err)
	})
	t.Run("ExtractBundle", func(t *testing.T) {
		_, err := ExtractBundle(context.Background(), repoRef, tag, t.TempDir(), opts)
		check(t, err)
	})
}

func TestBundlePull_NotNebiArtifact(t *testing.T) {
	host := startTestRegistry(t)
	pushRawArtifact(t, host, "demo/image", "v1", "application/vnd.oci.image.config.v1+json",
		map[string]string{"layer.tar": "application/vnd.oci.image.layer.v1.tar"})

	pullAndExtract(t, host+"/demo/image", "v1", func(t *testing.T, err error) {
		if !errors.Is(err, ErrNotNebiArtifact) {
			t.Fatalf("want ErrNotNebiArtifact, got %v", err)
		}
	})
}

func TestBundlePull_InvalidBundle(t *testing.T) {
	host := startTestRegistry(t)
	pushRawArtifact(t, host, "demo/no-core", "v1", MediaTypePixiConfig,
		map[string]string{"README.md": MediaTypeNebiAsset})

	pullAndExtract(t, host+"/demo/no-core", "v1", func(t *testing.T, err error) {
		if !errors.Is(err, ErrInvalidBundle) {
			t.Fatalf("want ErrInvalidBundle, got %v", err)
		}
	})
}

func TestBundlePull_ReferenceNotFound(t *testing.T) {
	host := startTestRegistry(t)
	pushRawArtifact(t, host, "demo/exists", "v1", MediaTypePixiConfig,
		map[string]string{"README.md": MediaTypeNebiAsset})

	for name, ref := range map[string][2]string{
		"missing tag":        {host + "/demo/exists", "nope"},
		"missing repository": {host + "/demo/absent", "v1"},
	} {
		t.Run(name, func(t *testing.T) {
			pullAndExtract(t, ref[0], ref[1], func(t *testing.T, err error) {
				if !errors.Is(err, ErrReferenceNotFound) {
					t.Fatalf("want ErrReferenceNotFound, got %v", err)
				}
				var refused *RegistryAccessError
				if errors.As(err, &refused) {
					t.Fatalf("a missing reference must not be a RegistryAccessError: %v", err)
				}
			})
		})
	}
}

func TestBundlePull_RegistryAccess(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			host := startStatusRegistry(t, status)
			pullAndExtract(t, host+"/demo/any", "v1", func(t *testing.T, err error) {
				var refused *RegistryAccessError
				if !errors.As(err, &refused) {
					t.Fatalf("want RegistryAccessError, got %T: %v", err, err)
				}
				if refused.StatusCode != status {
					t.Errorf("StatusCode: got %d want %d", refused.StatusCode, status)
				}
				if errors.Is(err, ErrReferenceNotFound) {
					t.Errorf("status %d must not be reported as not found: %v", status, err)
				}
			})
		})
	}

	// 400 stands in for "some other upstream failure"; a 5xx would work
	// too but the registry client retries those with backoff.
	t.Run("other statuses are not access errors", func(t *testing.T) {
		host := startStatusRegistry(t, http.StatusBadRequest)
		pullAndExtract(t, host+"/demo/any", "v1", func(t *testing.T, err error) {
			var refused *RegistryAccessError
			if err == nil || errors.As(err, &refused) || errors.Is(err, ErrReferenceNotFound) {
				t.Fatalf("want an unclassified error, got %T: %v", err, err)
			}
		})
	})
}
