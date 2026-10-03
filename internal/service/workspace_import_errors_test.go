package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gorm.io/gorm"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
)

// importModes runs fn once per server mode. Local mode imports through
// oci.ExtractBundle and team mode through oci.PullBundle, so every error
// mapping is exercised on both paths.
func importModes(t *testing.T, fn func(t *testing.T, isLocal bool)) {
	t.Helper()
	t.Run("local mode (extract bundle)", func(t *testing.T) { fn(t, true) })
	t.Run("team mode (pull bundle)", func(t *testing.T) { fn(t, false) })
}

// importFixture is a service wired to one registered registry.
type importFixture struct {
	svc    *WorkspaceService
	db     *gorm.DB
	userID uuid.UUID
	regID  string
	host   string
}

// newImportFixture registers a registry record pointing at host (plain
// HTTP) and grants the test user read access to it.
func newImportFixture(t *testing.T, isLocal bool, host, username, password string) *importFixture {
	t.Helper()
	svc, db := testSetup(t, isLocal)
	userID := createTestUser(t, db, "alice")
	dbReg := models.OCIRegistry{
		Name:      "import-src",
		URL:       "http://" + host,
		Namespace: "demo",
		Username:  username,
		Password:  password,
	}
	if err := db.Create(&dbReg).Error; err != nil {
		t.Fatalf("create registry: %v", err)
	}
	if !isLocal {
		grantRegistryAccessForTest(t, db, userID, dbReg.ID, "read")
	}
	return &importFixture{svc: svc, db: db, userID: userID, regID: dbReg.ID.String(), host: host}
}

// importExpectingError imports demo/<repo>:<tag> and returns the error,
// after checking that the failed import left nothing behind.
func (f *importFixture) importExpectingError(t *testing.T, repo, tag string) error {
	t.Helper()
	_, err := f.svc.ImportFromRegistry(context.Background(), f.regID, ImportFromRegistryRequest{
		RepositoryPath: "demo/" + repo,
		Tag:            tag,
		Name:           "imported",
	}, f.userID)
	if err == nil {
		t.Fatal("expected the import to fail")
	}

	var count int64
	f.db.Model(&models.Workspace{}).Count(&count)
	if count != 0 {
		t.Errorf("failed import created %d workspace(s)", count)
	}
	leftovers, globErr := filepath.Glob(filepath.Join(f.svc.executor.StagingRoot(), "import-*"))
	if globErr != nil {
		t.Fatalf("glob staging root: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Errorf("failed import left staging dirs behind: %v", leftovers)
	}
	return err
}

// startMemRegistry starts an in-memory OCI registry and returns its host.
// wrap, when non-nil, sees every request first and reports whether it
// answered it.
func startMemRegistry(t *testing.T, wrap func(w http.ResponseWriter, r *http.Request) bool) string {
	t.Helper()
	inner := registry.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wrap != nil && wrap(w, r) {
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// startStubRegistry starts a server that answers every request with h.
func startStubRegistry(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
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

// publishValidBundle publishes a well-formed bundle as demo/<repo>:v1.
func publishValidBundle(t *testing.T, host, repo string) {
	t.Helper()
	srcDir := t.TempDir()
	for name, body := range map[string]string{
		"pixi.toml": "[project]\nname = \"ok\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n",
		"pixi.lock": "version: 6\n",
		"asset.txt": "asset\n",
	} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reg := oci.Registry{Host: host, Namespace: "demo", PlainHTTP: true}
	if _, err := oci.Publish(context.Background(), srcDir, reg, repo, "v1"); err != nil {
		t.Fatalf("seed publish: %v", err)
	}
}

func TestImportFromRegistry_NotNebiArtifact_Unprocessable(t *testing.T) {
	importModes(t, func(t *testing.T, isLocal bool) {
		host := startMemRegistry(t, nil)
		pushRawArtifact(t, host, "demo/image", "v1", "application/vnd.oci.image.config.v1+json",
			map[string]string{"layer.tar": "application/vnd.oci.image.layer.v1.tar"})
		f := newImportFixture(t, isLocal, host, "", "")

		err := f.importExpectingError(t, "image", "v1")

		var unprocessable *UnprocessableError
		if !errors.As(err, &unprocessable) {
			t.Fatalf("want UnprocessableError, got %T: %v", err, err)
		}
		if unprocessable.Message != "not a Nebi artifact" {
			t.Errorf("message: got %q", unprocessable.Message)
		}
	})
}

func TestImportFromRegistry_InvalidBundle_Unprocessable(t *testing.T) {
	importModes(t, func(t *testing.T, isLocal bool) {
		host := startMemRegistry(t, nil)
		// Claims to be a Nebi bundle but carries no pixi.toml/pixi.lock.
		pushRawArtifact(t, host, "demo/no-core", "v1", oci.MediaTypePixiConfig,
			map[string]string{"README.md": oci.MediaTypeNebiAsset})
		f := newImportFixture(t, isLocal, host, "", "")

		err := f.importExpectingError(t, "no-core", "v1")

		var unprocessable *UnprocessableError
		if !errors.As(err, &unprocessable) {
			t.Fatalf("want UnprocessableError, got %T: %v", err, err)
		}
		if unprocessable.Message != "invalid bundle: missing pixi.{toml,lock}" {
			t.Errorf("message: got %q", unprocessable.Message)
		}
	})
}

func TestImportFromRegistry_UpstreamNotFound(t *testing.T) {
	importModes(t, func(t *testing.T, isLocal bool) {
		host := startMemRegistry(t, nil)
		publishValidBundle(t, host, "exists")
		f := newImportFixture(t, isLocal, host, "", "")

		for name, ref := range map[string][2]string{
			"missing tag":        {"exists", "nope"},
			"missing repository": {"absent", "v1"},
		} {
			t.Run(name, func(t *testing.T) {
				err := f.importExpectingError(t, ref[0], ref[1])

				var notFound *NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("want NotFoundError, got %T: %v", err, err)
				}
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("NotFoundError must match ErrNotFound")
				}
				want := "repository or tag not found: " + host + "/demo/" + ref[0] + ":" + ref[1]
				if notFound.Message != want {
					t.Errorf("message: got %q want %q", notFound.Message, want)
				}
			})
		}
	})
}

func TestImportFromRegistry_RegistryRefused_Upstream(t *testing.T) {
	const secret = "s3cr3t-registry-password"

	// Each refusal is a different way a registry says no.
	refusals := map[string]struct {
		handler    func(host *string) http.HandlerFunc
		wantStatus int
	}{
		"401 without challenge": {
			handler: func(*string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
			},
			wantStatus: http.StatusUnauthorized,
		},
		"403": {
			handler: func(*string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "denied", http.StatusForbidden)
				}
			},
			wantStatus: http.StatusForbidden,
		},
		"401 after basic credentials are rejected": {
			handler: func(*string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
					// Echo what the client sent, as a hostile or sloppy
					// registry might; none of it may reach the caller.
					http.Error(w, "bad credentials: "+r.Header.Get("Authorization"), http.StatusUnauthorized)
				}
			},
			wantStatus: http.StatusUnauthorized,
		},
		"401 from the token service": {
			handler: func(host *string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/token" {
						http.Error(w, "bad credentials", http.StatusUnauthorized)
						return
					}
					w.Header().Set("WWW-Authenticate", `Bearer realm="http://`+*host+`/token",service="registry"`)
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	importModes(t, func(t *testing.T, isLocal bool) {
		for name, tc := range refusals {
			t.Run(name, func(t *testing.T) {
				var host string
				host = startStubRegistry(t, tc.handler(&host))
				f := newImportFixture(t, isLocal, host, "robot", secret)

				err := f.importExpectingError(t, "private", "v1")

				var upstream *UpstreamError
				if !errors.As(err, &upstream) {
					t.Fatalf("want UpstreamError, got %T: %v", err, err)
				}
				if upstream.UpstreamStatus != tc.wantStatus {
					t.Errorf("upstream status: got %d want %d", upstream.UpstreamStatus, tc.wantStatus)
				}
				want := "registry refused access to " + host + "/demo/private"
				if upstream.Message != want || err.Error() != want {
					t.Errorf("message: got %q (Error() %q) want %q", upstream.Message, err.Error(), want)
				}
				if upstream.Err == nil {
					t.Fatal("UpstreamError must keep the cause for the server log")
				}
				// Neither the caller-facing message nor the logged cause
				// may carry the registry credentials.
				for _, leak := range []string{secret, "robot", "Basic ", "Bearer "} {
					if strings.Contains(err.Error(), leak) {
						t.Errorf("caller-facing message leaks %q: %s", leak, err.Error())
					}
				}
				if strings.Contains(upstream.Err.Error(), secret) {
					t.Errorf("logged cause leaks the registry password: %s", upstream.Err.Error())
				}
			})
		}
	})
}

// TestImportFromRegistry_RegistryRefusesLayers covers a refusal that only
// shows up after the manifest was served: the mode-specific layer fetch
// (oras.Copy in local mode, a direct blob GET in team mode) must be
// classified the same way as a refusal at tag resolution.
func TestImportFromRegistry_RegistryRefusesLayers(t *testing.T) {
	importModes(t, func(t *testing.T, isLocal bool) {
		refuse := false
		host := startMemRegistry(t, func(w http.ResponseWriter, r *http.Request) bool {
			if refuse && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
				http.Error(w, "denied", http.StatusForbidden)
				return true
			}
			return false
		})
		publishValidBundle(t, host, "layers-denied")
		refuse = true
		f := newImportFixture(t, isLocal, host, "", "")

		err := f.importExpectingError(t, "layers-denied", "v1")

		var upstream *UpstreamError
		if !errors.As(err, &upstream) {
			t.Fatalf("want UpstreamError, got %T: %v", err, err)
		}
		if upstream.UpstreamStatus != http.StatusForbidden {
			t.Errorf("upstream status: got %d want 403", upstream.UpstreamStatus)
		}
	})
}

func TestImportFromRegistry_OtherFailuresStayInternal(t *testing.T) {
	failures := map[string]func(t *testing.T) string{
		// 400 stands in for "some other upstream status"; the registry
		// client retries 5xx with backoff, which would slow the test.
		"unexpected upstream status": func(t *testing.T) string {
			return startStubRegistry(t, func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "bad request", http.StatusBadRequest)
			})
		},
		"registry unreachable": func(t *testing.T) string {
			srv := httptest.NewServer(http.NotFoundHandler())
			host := strings.TrimPrefix(srv.URL, "http://")
			srv.Close()
			return host
		},
	}

	importModes(t, func(t *testing.T, isLocal bool) {
		for name, start := range failures {
			t.Run(name, func(t *testing.T) {
				f := newImportFixture(t, isLocal, start(t), "", "")

				err := f.importExpectingError(t, "any", "v1")

				var (
					notFound      *NotFoundError
					unprocessable *UnprocessableError
					upstream      *UpstreamError
					validation    *ValidationError
				)
				if errors.As(err, &notFound) || errors.As(err, &unprocessable) ||
					errors.As(err, &upstream) || errors.As(err, &validation) || errors.Is(err, ErrNotFound) {
					t.Fatalf("want an untyped internal error, got %T: %v", err, err)
				}
				wantPrefix := "pull bundle: "
				if isLocal {
					wantPrefix = "extract bundle: "
				}
				if !strings.HasPrefix(err.Error(), wantPrefix) {
					t.Errorf("want prefix %q, got %q", wantPrefix, err.Error())
				}
			})
		}
	})
}

func TestDisplayRepoRef(t *testing.T) {
	for in, want := range map[string]string{
		"quay.io/org/repo":                  "quay.io/org/repo",
		"localhost:5000/repo":               "localhost:5000/repo",
		"user:secret@quay.io/org/repo":      "quay.io/org/repo",
		"user:p@ss@localhost:5000/org/repo": "localhost:5000/org/repo",
		"user:secret@quay.io":               "quay.io",
		"quay.io/org/re@po":                 "quay.io/org/re@po",
	} {
		if got := displayRepoRef(in); got != want {
			t.Errorf("displayRepoRef(%q) = %q, want %q", in, got, want)
		}
	}
}
