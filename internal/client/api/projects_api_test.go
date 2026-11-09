package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/client"
	clientapi "github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
)

const projectsTestLock = `version: 6
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64: []
packages: []
`

// projectsFakeExecutor writes pixi.toml and a canned lock instead of
// running pixi.
type projectsFakeExecutor struct{ executor.Executor }

func (projectsFakeExecutor) CreateProject(ctx context.Context, p *models.Project, log io.Writer, opts executor.CreateProjectOptions) error {
	if err := os.MkdirAll(p.Path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.Path, "pixi.toml"), []byte(opts.PixiToml), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.Path, "pixi.lock"), []byte(projectsTestLock), 0o644)
}

func (projectsFakeExecutor) DeleteProject(ctx context.Context, p *models.Project, log io.Writer) error {
	return os.RemoveAll(p.Path)
}

func (projectsFakeExecutor) IsEnvInstalled(p *models.Project) bool {
	_, err := os.Stat(filepath.Join(p.Path, ".pixi", "envs"))
	return err == nil
}

func (projectsFakeExecutor) GetProjectPath(p *models.Project) string { return p.Path }

func newProjectsAPI(t *testing.T) (http.Handler, *client.Stack) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	stack, err := client.NewStack(client.StackOptions{
		DB:          gdb,
		Executor:    projectsFakeExecutor{},
		DataDir:     dataDir,
		ProjectsDir: filepath.Join(dataDir, "projects"),
		InlineLog:   io.Discard,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	clientapi.New(stack.SDK, stack.Jobs, stack.Desktop).Register(r.Group("/api/v1"))
	return r, stack
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

const createBody = `{"name":"api-proj","form":{"name":"","channels":[],"platforms":["linux-64"],"requested":[{"name":"python","constraint":">=3.11"}]},"description":"Initial project creation","tags":["latest"]}`

func TestProjectsAPI(t *testing.T) {
	h, _ := newProjectsAPI(t)

	rec := call(t, h, http.MethodGet, "/api/v1/client/projects", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body)
	}

	rec = call(t, h, http.MethodPost, "/api/v1/client/projects", createBody)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	created := decode[shared.CreateProjectResult](t, rec)
	if created.Project.Name != "api-proj" || created.Job.Type != models.JobTypeCreate {
		t.Fatalf("create result: %+v", created)
	}

	rec = call(t, h, http.MethodGet, "/api/v1/client/projects", "")
	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil || len(raw) != 1 {
		t.Fatalf("list: %s", rec.Body)
	}
	for _, key := range []string{"id", "name", "path", "status", "install_status", "installed_version", "last_installed_version",
		"latest_version", "size_bytes", "size", "remotes", "update_available", "server_latest_version",
		"failed_install_job_id", "active_job_id", "created_at", "updated_at"} {
		if _, ok := raw[0][key]; !ok {
			t.Fatalf("list item lacks %q: %v", key, raw[0])
		}
	}
	if raw[0]["remotes"] == nil {
		t.Fatal("remotes must never be null")
	}

	id := created.Project.ID.String()
	rec = call(t, h, http.MethodGet, "/api/v1/client/projects/"+id, "")
	if rec.Code != http.StatusOK || decode[shared.ProjectListItem](t, rec).ID != created.Project.ID {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/api/v1/client/projects/not-a-uuid", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", rec.Code)
	}
	if rec := call(t, h, http.MethodGet, "/api/v1/client/projects/00000000-0000-4000-8000-000000000001", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", rec.Code)
	}

	// Duplicate name: 409 name_taken.
	rec = call(t, h, http.MethodPost, "/api/v1/client/projects", createBody)
	if rec.Code != http.StatusConflict || decode[clientapi.ErrorResponse](t, rec).Code != "name_taken" {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body)
	}
	// Field errors: 422 with the field.
	rec = call(t, h, http.MethodPost, "/api/v1/client/projects", `{"name":"x","form":{"name":"x","channels":[],"platforms":[],"requested":[]},"description":"d"}`)
	if e := decode[clientapi.ErrorResponse](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field != "platforms" || e.Error != "Add at least one platform." {
		t.Fatalf("no platform: %d %s", rec.Code, rec.Body)
	}

	rec = call(t, h, http.MethodGet, "/api/v1/client/projects/name-check?name=API-PROJ", "")
	nc := decode[shared.NameCheckResult](t, rec)
	if rec.Code != http.StatusOK || nc.Available || nc.Error != "A project with that name already exists." {
		t.Fatalf("name-check: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, h, http.MethodGet, "/api/v1/client/projects/name-check?name=fresh", "")
	if !decode[shared.NameCheckResult](t, rec).Available {
		t.Fatalf("name-check fresh: %s", rec.Body)
	}

	rec = call(t, h, http.MethodDelete, "/api/v1/client/projects/"+id, "")
	if rec.Code != http.StatusAccepted || decode[shared.JobSummary](t, rec).Type != models.JobTypeDelete {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, h, http.MethodGet, "/api/v1/client/projects/"+id, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after delete: %d", rec.Code)
	}
}

func TestManifestAPI(t *testing.T) {
	h, _ := newProjectsAPI(t)
	rec := call(t, h, http.MethodPost, "/api/v1/client/manifest/parse", `{"pixi_toml":"[workspace]\nname = \"a\"\nplatforms = [\"linux-64\"]\n[tasks]\nx = \"y\"\n"}`)
	res := decode[shared.ManifestParseResult](t, rec)
	if rec.Code != http.StatusOK || res.Form.Name != "a" || len(res.Unrepresentable) != 1 {
		t.Fatalf("parse: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, h, http.MethodPost, "/api/v1/client/manifest/parse", `{"pixi_toml":"[workspace"}`)
	if rec.Code != http.StatusUnprocessableEntity || decode[clientapi.ErrorResponse](t, rec).Field != "manifest" {
		t.Fatalf("parse invalid: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, h, http.MethodPost, "/api/v1/client/manifest/render", `{"form":{"name":"a","channels":["conda-forge"],"platforms":["linux-64"],"requested":[{"name":"numpy","constraint":">=2"}]}}`)
	out := decode[shared.ManifestRenderResult](t, rec)
	if rec.Code != http.StatusOK || !strings.Contains(out.PixiToml, `numpy = ">=2"`) {
		t.Fatalf("render: %d %s", rec.Code, rec.Body)
	}
}
