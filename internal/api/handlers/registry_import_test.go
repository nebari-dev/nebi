package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests drive POST /registries/:id/import end to end (route, service,
// OCI client) against in-process registries, and check the three things a
// caller or operator can observe: the status, the response body, and the
// server log.

const (
	importRegistryUser     = "robot"
	importRegistryPassword = "s3cr3t-registry-password"
	genericInternalError   = `{"error":"Internal server error"}`
)

// importModes runs fn once per server mode: local mode imports through
// oci.ExtractBundle, team mode through oci.PullBundle.
func importModes(t *testing.T, fn func(t *testing.T, isLocal bool)) {
	t.Helper()
	t.Run("local mode (extract bundle)", func(t *testing.T) { fn(t, true) })
	t.Run("team mode (pull bundle)", func(t *testing.T) { fn(t, false) })
}

// importResult is what one import request produced.
type importResult struct {
	status int
	body   string
	logs   string
}

// runImport registers registryHost (plain HTTP) and posts one import for
// demo/<repo>:v1 through the real route, capturing the server log.
func runImport(t *testing.T, isLocal bool, registryHost, repo string, withCredentials bool) importResult {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=busy_timeout(10000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.FederatedIdentity{}, &models.FederatedIdentityReview{},
		&models.Role{}, &models.Workspace{}, &models.Job{}, &models.Permission{},
		&models.WorkspaceVersion{}, &models.WorkspaceTag{}, &models.AuditLog{},
		&models.Package{}, &models.OCIRegistry{}, &models.Publication{},
		&models.Group{}, &models.GroupMember{}, &models.GroupPermission{},
		&models.ResourceLock{}, &models.ResourceMetric{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init rbac: %v", err)
	}
	q := queue.NewMemoryQueue(10)
	t.Cleanup(func() { q.Close() })
	exec, err := executor.NewLocalExecutor(&config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	provider := rbac.NewDefaultProvider()
	wsSvc := service.New(db, q, exec, isLocal, nil, provider, limits.Defaults())
	h := NewRegistryBrowseHandler(service.NewRegistryService(db, nil, isLocal, provider), wsSvc)

	user := models.User{Username: "alice", Email: "alice@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	reg := models.OCIRegistry{Name: "import-src", URL: "http://" + registryHost, Namespace: "demo"}
	if withCredentials {
		reg.Username, reg.Password = importRegistryUser, importRegistryPassword
	}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatalf("create registry: %v", err)
	}

	router := gin.New()
	router.POST("/registries/:id/import", func(c *gin.Context) {
		c.Set("user", &user)
		h.ImportEnvironment(c)
	})

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	body, _ := json.Marshal(ImportRequest{RepositoryPath: "demo/" + repo, Tag: "v1", Name: "imported"})
	req := httptest.NewRequest(http.MethodPost, "/registries/"+reg.ID.String()+"/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return importResult{status: w.Code, body: strings.TrimSpace(w.Body.String()), logs: logs.String()}
}

func (r importResult) expect(t *testing.T, status int, body string) {
	t.Helper()
	if r.status != status {
		t.Errorf("status: got %d want %d (body %s)", r.status, status, r.body)
	}
	if r.body != body {
		t.Errorf("body:\n got %s\nwant %s", r.body, body)
	}
}

// expectNoLeak fails if any of the given secrets shows up in the response
// body or, when checkLogs is set, in the server log.
func (r importResult) expectNoLeak(t *testing.T, checkLogs bool, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		if strings.Contains(r.body, s) {
			t.Errorf("response body leaks %q: %s", s, r.body)
		}
		if checkLogs && strings.Contains(r.logs, s) {
			t.Errorf("server log leaks %q:\n%s", s, r.logs)
		}
	}
}

// startServer starts an HTTP server and returns its host:port.
func startServer(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// seenAuth collects every Authorization header a stub received, so tests
// can assert that exactly what the client sent never comes back out.
type seenAuth struct {
	mu     sync.Mutex
	values []string
}

func (s *seenAuth) record(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if v != "" {
		s.mu.Lock()
		s.values = append(s.values, v)
		s.mu.Unlock()
	}
	return v
}

func (s *seenAuth) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.values...)
}

// registryErrorBody is a distribution-spec JSON error body. The registry
// client keeps message and detail in the error it returns.
func registryErrorBody(code, message, detail string) string {
	b, _ := json.Marshal(map[string]any{
		"errors": []map[string]string{{"code": code, "message": message, "detail": detail}},
	})
	return string(b)
}

func writeRegistryError(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func blobDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// artifact is a hand-built OCI artifact served by artifactHandler. It lets
// tests present manifests that a real registry or the publisher would
// never produce.
type artifact struct {
	manifest []byte
	blobs    map[string][]byte // digest → content
}

// layer describes one manifest layer for newArtifact.
type layer struct {
	mediaType string
	title     string
	content   string
	// lieAboutSize makes the descriptor claim Size 0 for non-empty content.
	lieAboutSize bool
}

func newArtifact(t *testing.T, configMediaType string, layers ...layer) artifact {
	t.Helper()
	a := artifact{blobs: map[string][]byte{}}
	config := []byte("{}")
	a.blobs[blobDigest(config)] = config
	m := map[string]any{
		"schemaVersion": 2,
		"mediaType":     ocispec.MediaTypeImageManifest,
		"config": map[string]any{
			"mediaType": configMediaType, "digest": blobDigest(config), "size": len(config),
		},
	}
	var descs []map[string]any
	for _, l := range layers {
		content := []byte(l.content)
		a.blobs[blobDigest(content)] = content
		size := len(content)
		if l.lieAboutSize {
			size = 0
		}
		descs = append(descs, map[string]any{
			"mediaType": l.mediaType, "digest": blobDigest(content), "size": size,
			"annotations": map[string]string{ocispec.AnnotationTitle: l.title},
		})
	}
	m["layers"] = descs
	var err error
	if a.manifest, err = json.Marshal(m); err != nil {
		t.Fatal(err)
	}
	return a
}

// validBundle is a well-formed Nebi bundle with one asset.
func validBundle(t *testing.T) artifact {
	return newArtifact(t, oci.MediaTypePixiConfig,
		layer{mediaType: oci.MediaTypePixiToml, title: "pixi.toml", content: "[project]\nname = \"ok\"\n"},
		layer{mediaType: oci.MediaTypePixiLock, title: "pixi.lock", content: "version: 6\n"},
		layer{mediaType: oci.MediaTypeNebiAsset, title: "README.md", content: "# hi\n"},
	)
}

// artifactHandler serves a as every repository and tag of a registry.
func artifactHandler(a artifact) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/manifests/"):
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", blobDigest(a.manifest))
			w.Header().Set("Content-Length", strconv.Itoa(len(a.manifest)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(a.manifest)
			}
		case strings.Contains(r.URL.Path, "/blobs/"):
			digest := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			content, ok := a.blobs[digest]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", digest)
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(content)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}
}

// TestImport_RegistryRefusal_DoesNotLeak covers refusals whose underlying
// client error is full of things that must stay out of both the response
// and the log: a token-service URL with a signed query string, and JSON
// error bodies that echo the credentials the client presented.
func TestImport_RegistryRefusal_DoesNotLeak(t *testing.T) {
	const (
		signedQuery  = "X-Signature=SIGNED-URL-SECRET"
		echoedBearer = "Bearer ECHOED-TOKEN-SECRET"
	)
	basicCreds := base64.StdEncoding.EncodeToString([]byte(importRegistryUser + ":" + importRegistryPassword))

	importModes(t, func(t *testing.T, isLocal bool) {
		t.Run("token service on another origin rejects the credentials", func(t *testing.T) {
			var seen seenAuth
			tokenHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent := seen.record(r)
				writeRegistryError(w, http.StatusUnauthorized,
					registryErrorBody("UNAUTHORIZED", "bad credentials: "+sent, echoedBearer))
			}))
			registryHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("WWW-Authenticate",
					`Bearer realm="http://`+tokenHost+`/token?`+signedQuery+`",service="registry"`)
				writeRegistryError(w, http.StatusUnauthorized, registryErrorBody("UNAUTHORIZED", "authentication required", ""))
			}))

			res := runImport(t, isLocal, registryHost, "private", true)

			res.expect(t, http.StatusBadGateway,
				`{"error":"registry refused access to `+registryHost+`/demo/private","upstream_status":401}`)
			if len(seen.all()) == 0 {
				t.Fatal("token service never saw the registry credentials; the fixture is not exercising the leak path")
			}
			secrets := append(seen.all(), importRegistryPassword, basicCreds, signedQuery, "SIGNED-URL-SECRET",
				echoedBearer, "ECHOED-TOKEN-SECRET", tokenHost, "/token")
			res.expectNoLeak(t, true, secrets...)
			for _, want := range []string{"upstream_status=401", registryHost + "/demo/private"} {
				if !strings.Contains(res.logs, want) {
					t.Errorf("log should record %q:\n%s", want, res.logs)
				}
			}
		})

		t.Run("registry denies a layer with a body echoing credentials", func(t *testing.T) {
			var seen seenAuth
			bundle := artifactHandler(validBundle(t))
			registryHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent := seen.record(r)
				if r.Header.Get("Authorization") == "" {
					w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
					writeRegistryError(w, http.StatusUnauthorized, registryErrorBody("UNAUTHORIZED", "authentication required", ""))
					return
				}
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					writeRegistryError(w, http.StatusForbidden,
						registryErrorBody("DENIED", "denied for "+sent, echoedBearer))
					return
				}
				bundle(w, r)
			}))

			res := runImport(t, isLocal, registryHost, "layers-denied", true)

			res.expect(t, http.StatusBadGateway,
				`{"error":"registry refused access to `+registryHost+`/demo/layers-denied","upstream_status":403}`)
			if len(seen.all()) == 0 {
				t.Fatal("registry never saw the credentials; the fixture is not exercising the leak path")
			}
			secrets := append(seen.all(), importRegistryPassword, basicCreds, echoedBearer, "ECHOED-TOKEN-SECRET", "/blobs/")
			res.expectNoLeak(t, true, secrets...)
			if !strings.Contains(res.logs, "upstream_status=403") {
				t.Errorf("log should record the upstream status:\n%s", res.logs)
			}
		})
	})
}

// TestImport_InvalidBundle_DoesNotEchoManifest covers the 422 body: the
// reason is fixed text, never the manifest values that triggered it, since
// whoever controls the registry controls those values.
func TestImport_InvalidBundle_DoesNotEchoManifest(t *testing.T) {
	const planted = "Bearer PLANTED-MANIFEST-SECRET"
	toml := layer{mediaType: oci.MediaTypePixiToml, title: "pixi.toml", content: "[project]\n"}
	lock := layer{mediaType: oci.MediaTypePixiLock, title: "pixi.lock", content: "version: 6\n"}

	cases := map[string]struct {
		artifact func(t *testing.T) artifact
		want     string
	}{
		"pixi.toml title": {
			artifact: func(t *testing.T) artifact {
				return newArtifact(t, oci.MediaTypePixiConfig,
					layer{mediaType: oci.MediaTypePixiToml, title: planted, content: "[project]\n"}, lock)
			},
			want: "invalid bundle: pixi.toml core layer has an unexpected title",
		},
		"pixi.lock title": {
			artifact: func(t *testing.T) artifact {
				return newArtifact(t, oci.MediaTypePixiConfig, toml,
					layer{mediaType: oci.MediaTypePixiLock, title: planted, content: "version: 6\n"})
			},
			want: "invalid bundle: pixi.lock core layer has an unexpected title",
		},
		"layer media type": {
			artifact: func(t *testing.T) artifact {
				return newArtifact(t, oci.MediaTypePixiConfig, toml, lock,
					layer{mediaType: "application/vnd." + planted, title: "x.bin", content: "x"})
			},
			want: "invalid bundle: layer has an unknown media type",
		},
		"asset path": {
			artifact: func(t *testing.T) artifact {
				return newArtifact(t, oci.MediaTypePixiConfig, toml, lock,
					layer{mediaType: oci.MediaTypeNebiAsset, title: "../" + planted, content: "x"})
			},
			want: "invalid bundle: unsafe or colliding asset path",
		},
		"duplicate core layer": {
			artifact: func(t *testing.T) artifact {
				return newArtifact(t, oci.MediaTypePixiConfig, toml, toml, lock)
			},
			want: "invalid bundle: duplicate core layer",
		},
		"missing core layers": {
			artifact: func(t *testing.T) artifact {
				return newArtifact(t, oci.MediaTypePixiConfig,
					layer{mediaType: oci.MediaTypeNebiAsset, title: "README.md", content: "x"})
			},
			want: "invalid bundle: missing pixi.{toml,lock}",
		},
	}

	importModes(t, func(t *testing.T, isLocal bool) {
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				registryHost := startServer(t, artifactHandler(tc.artifact(t)))

				res := runImport(t, isLocal, registryHost, "bad", false)

				want, _ := json.Marshal(ErrorResponse{Error: tc.want})
				res.expect(t, http.StatusUnprocessableEntity, string(want))
				res.expectNoLeak(t, false, planted, "PLANTED-MANIFEST-SECRET")
			})
		}
	})
}

// TestImport_MalformedBundleContent covers invalid bundles that are not
// caught by layer classification.
func TestImport_MalformedBundleContent(t *testing.T) {
	importModes(t, func(t *testing.T, isLocal bool) {
		t.Run("manifest is not JSON", func(t *testing.T) {
			registryHost := startServer(t, artifactHandler(artifact{
				manifest: []byte(`{"schemaVersion": 2, "config": PLANTED-SECRET`),
			}))

			res := runImport(t, isLocal, registryHost, "bad-json", false)

			res.expect(t, http.StatusUnprocessableEntity, `{"error":"invalid bundle: manifest is not valid JSON"}`)
			res.expectNoLeak(t, false, "PLANTED-SECRET")
		})

		t.Run("not a Nebi artifact", func(t *testing.T) {
			registryHost := startServer(t, artifactHandler(newArtifact(t, ocispec.MediaTypeImageConfig,
				layer{mediaType: ocispec.MediaTypeImageLayer, title: "layer.tar", content: "x"})))

			res := runImport(t, isLocal, registryHost, "image", false)

			res.expect(t, http.StatusUnprocessableEntity, `{"error":"not a Nebi artifact"}`)
		})
	})

	// A layer that declares size 0 with the digest of non-empty content
	// can never verify, whichever layer it is and whether or not the
	// mode downloads it.
	zeroSize := map[string][]layer{
		"zero-size core layer with a non-empty digest": {
			{mediaType: oci.MediaTypePixiToml, title: "pixi.toml", content: "[project]\n", lieAboutSize: true},
			{mediaType: oci.MediaTypePixiLock, title: "pixi.lock", content: "version: 6\n"},
		},
		"zero-size asset layer with a non-empty digest": {
			{mediaType: oci.MediaTypePixiToml, title: "pixi.toml", content: "[project]\n"},
			{mediaType: oci.MediaTypePixiLock, title: "pixi.lock", content: "version: 6\n"},
			{mediaType: oci.MediaTypeNebiAsset, title: "PLANTED-SECRET.bin", content: "not empty", lieAboutSize: true},
		},
	}
	importModes(t, func(t *testing.T, isLocal bool) {
		for name, layers := range zeroSize {
			t.Run(name, func(t *testing.T) {
				registryHost := startServer(t, artifactHandler(newArtifact(t, oci.MediaTypePixiConfig, layers...)))

				res := runImport(t, isLocal, registryHost, "zero-size", false)

				res.expect(t, http.StatusUnprocessableEntity, `{"error":"invalid bundle: zero-size layer has a non-empty digest"}`)
				res.expectNoLeak(t, false, "PLANTED-SECRET")
			})
		}
	})
}

// bearerRegistry wraps next so that it demands a bearer token from the
// token service at realm before answering.
func bearerRegistry(realm string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+realm+`",service="registry"`)
			writeRegistryError(w, http.StatusUnauthorized, registryErrorBody("UNAUTHORIZED", "authentication required", ""))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TestImport_ReferenceNotFound covers the only failure reported as 404:
// the registry's own 404 for the manifest. A 404 from anywhere else in
// the pull says nothing about the reference and stays a generic 500.
func TestImport_ReferenceNotFound(t *testing.T) {
	manifestUnknown := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/manifests/") {
			writeRegistryError(w, http.StatusNotFound, registryErrorBody("MANIFEST_UNKNOWN", "manifest unknown", ""))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	importModes(t, func(t *testing.T, isLocal bool) {
		t.Run("registry answers 404 for the manifest", func(t *testing.T) {
			registryHost := startServer(t, manifestUnknown)

			res := runImport(t, isLocal, registryHost, "absent", false)

			res.expect(t, http.StatusNotFound,
				`{"error":"repository or tag not found: `+registryHost+`/demo/absent:v1"}`)
		})

		t.Run("token request is redirected and succeeds, then the registry answers 404 for the manifest", func(t *testing.T) {
			issuer := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"anonymous-token"}`))
			}))
			tokenHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "http://"+issuer+"/issue?"+r.URL.RawQuery, http.StatusTemporaryRedirect)
			}))
			registryHost := startServer(t, bearerRegistry("http://"+tokenHost+"/token", manifestUnknown))

			res := runImport(t, isLocal, registryHost, "absent", false)

			res.expect(t, http.StatusNotFound,
				`{"error":"repository or tag not found: `+registryHost+`/demo/absent:v1"}`)
		})

		t.Run("tag resolves, then the registry answers 404 for the manifest itself", func(t *testing.T) {
			bundle := artifactHandler(validBundle(t))
			registryHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/manifests/") {
					writeRegistryError(w, http.StatusNotFound, registryErrorBody("MANIFEST_UNKNOWN", "manifest unknown", ""))
					return
				}
				bundle(w, r)
			}))

			res := runImport(t, isLocal, registryHost, "deleted", false)

			res.expect(t, http.StatusNotFound,
				`{"error":"repository or tag not found: `+registryHost+`/demo/deleted:v1"}`)
		})

		t.Run("token service answers 404", func(t *testing.T) {
			tokenHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeRegistryError(w, http.StatusNotFound, registryErrorBody("NOT_FOUND", "no", ""))
			}))
			registryHost := startServer(t, bearerRegistry("http://"+tokenHost+"/token", manifestUnknown))

			res := runImport(t, isLocal, registryHost, "any", false)

			res.expect(t, http.StatusInternalServerError, genericInternalError)
		})

		t.Run("registry has the manifest but not its layers", func(t *testing.T) {
			bundle := artifactHandler(validBundle(t))
			registryHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					http.NotFound(w, r)
					return
				}
				bundle(w, r)
			}))

			res := runImport(t, isLocal, registryHost, "any", false)

			res.expect(t, http.StatusInternalServerError, genericInternalError)
		})
	})
}

// TestImport_AnonymousBasicChallenge covers a registry that asks for
// Basic credentials when none are configured. The registry client gives
// up without sending a second request, so there is no error response to
// read the status from, but it is still the registry answering 401.
func TestImport_AnonymousBasicChallenge(t *testing.T) {
	importModes(t, func(t *testing.T, isLocal bool) {
		registryHost := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			writeRegistryError(w, http.StatusUnauthorized, registryErrorBody("UNAUTHORIZED", "authentication required", ""))
		}))

		res := runImport(t, isLocal, registryHost, "private", false)

		res.expect(t, http.StatusBadGateway,
			`{"error":"registry refused access to `+registryHost+`/demo/private","upstream_status":401}`)
		if !strings.Contains(res.logs, "upstream_status=401") {
			t.Errorf("log should record the upstream status:\n%s", res.logs)
		}
	})
}
