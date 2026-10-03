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

// TestClassifyBundleManifest_RejectionsAreInvalidBundle pins that every
// way a manifest can be rejected is reported as ErrInvalidBundle, so
// callers can classify with errors.Is.
func TestClassifyBundleManifest_RejectionsAreInvalidBundle(t *testing.T) {
	cases := map[string][]ocispec.Descriptor{
		"missing core layers": {
			layerDesc(MediaTypeNebiAsset, "README.md"),
		},
		"duplicate core layer": {
			layerDesc(MediaTypePixiToml, "pixi.toml"),
			layerDesc(MediaTypePixiToml, "pixi.toml"),
			layerDesc(MediaTypePixiLock, "pixi.lock"),
		},
		"wrong pixi.toml title": {
			layerDesc(MediaTypePixiToml, "other.toml"),
			layerDesc(MediaTypePixiLock, "pixi.lock"),
		},
		"wrong pixi.lock title": {
			layerDesc(MediaTypePixiToml, "pixi.toml"),
			layerDesc(MediaTypePixiLock, "other.lock"),
		},
		"unknown media type": {
			layerDesc(MediaTypePixiToml, "pixi.toml"),
			layerDesc(MediaTypePixiLock, "pixi.lock"),
			layerDesc("application/vnd.example.future.v2", "future.bin"),
		},
		"unsafe asset path": {
			layerDesc(MediaTypePixiToml, "pixi.toml"),
			layerDesc(MediaTypePixiLock, "pixi.lock"),
			layerDesc(MediaTypeNebiAsset, "../escape.txt"),
		},
	}
	for name, layers := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := classifyBundleManifest(ocispec.Manifest{Layers: layers})
			if !errors.Is(err, ErrInvalidBundle) {
				t.Fatalf("want ErrInvalidBundle, got %v", err)
			}
			if !strings.HasPrefix(err.Error(), "invalid bundle: ") {
				t.Fatalf("message should start with \"invalid bundle: \", got %q", err.Error())
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
				if status, ok := RegistryStatus(err); !ok || status != http.StatusNotFound {
					t.Fatalf("RegistryStatus: got (%d, %v), want (404, true)", status, ok)
				}
			})
		})
	}
}

func TestBundlePull_RegistryStatus(t *testing.T) {
	// 400 stands in for "some other upstream failure"; a 5xx would work
	// too but the registry client retries those with backoff.
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			host := startStatusRegistry(t, status)
			pullAndExtract(t, host+"/demo/any", "v1", func(t *testing.T, err error) {
				got, ok := RegistryStatus(err)
				if !ok || got != status {
					t.Fatalf("RegistryStatus: got (%d, %v), want (%d, true); err=%v", got, ok, status, err)
				}
				if errors.Is(err, ErrReferenceNotFound) {
					t.Fatalf("status %d must not be reported as not found: %v", status, err)
				}
			})
		})
	}
}

func TestRegistryStatus_NoStatus(t *testing.T) {
	for _, err := range []error{nil, errors.New("dial tcp: connection refused"), ErrInvalidBundle, ErrNotNebiArtifact} {
		if status, ok := RegistryStatus(err); ok {
			t.Errorf("RegistryStatus(%v) = (%d, true), want no status", err, status)
		}
	}
}
