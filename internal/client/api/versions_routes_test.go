package api_test

import (
	"bytes"
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
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
)

// detailExecutor is a pixi-free executor: installs create .pixi/envs.
type detailExecutor struct {
	root   string
	frozen int
}

func (e *detailExecutor) GetProjectPath(p *models.Project) string {
	return filepath.Join(e.root, p.ID.String())
}
func (e *detailExecutor) InstallEnvironment(_ context.Context, p *models.Project, _ io.Writer) error {
	return os.MkdirAll(filepath.Join(e.GetProjectPath(p), ".pixi", "envs", "default"), 0o755)
}
func (e *detailExecutor) InstallEnvironmentFrozen(ctx context.Context, p *models.Project, w io.Writer) error {
	e.frozen++
	return e.InstallEnvironment(ctx, p, w)
}
func (e *detailExecutor) UninstallEnvironment(_ context.Context, p *models.Project, _ io.Writer) error {
	return os.RemoveAll(filepath.Join(e.GetProjectPath(p), ".pixi", "envs"))
}
func (e *detailExecutor) IsEnvInstalled(p *models.Project) bool {
	_, err := os.Stat(filepath.Join(e.GetProjectPath(p), ".pixi", "envs"))
	return err == nil
}
func (e *detailExecutor) CreateProject(context.Context, *models.Project, io.Writer, executor.CreateProjectOptions) error {
	return nil
}
func (e *detailExecutor) InstallPackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (e *detailExecutor) RemovePackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (e *detailExecutor) DeleteProject(context.Context, *models.Project, io.Writer) error { return nil }
func (e *detailExecutor) SolveEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}
func (e *detailExecutor) CleanupJobArtifacts(context.Context, *models.Project, models.JobType, io.Writer) error {
	return nil
}
func (e *detailExecutor) StagingRoot() string { return e.root }

type detailEnv struct {
	h       http.Handler
	stack   *client.Stack
	exec    *detailExecutor
	project models.Project
	toml    string
	lock    string
}

// newDetailEnv builds the client API over an inline-job stack with one
// project ("artlock") holding versions 1 and 2.
func newDetailEnv(t *testing.T) *detailEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	ex := &detailExecutor{root: t.TempDir()}
	stack, err := client.NewStack(client.StackOptions{
		DB: gdb, Executor: ex, DataDir: dataDir, Desktop: desktop.Capabilities{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	stack.API().Register(r.Group("/api/v1"))

	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "artifact", "testdata", "v7", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	env := &detailEnv{h: r, stack: stack, exec: ex, toml: read("pixi.toml"), lock: read("pixi.lock")}
	user := stack.SDK.Deps().LocalUserID
	env.project = models.Project{Name: "artlock", OwnerID: user, Status: models.ProjectStatusReady}
	if err := gdb.Create(&env.project).Error; err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 2; n++ {
		v := models.ProjectVersion{ProjectID: env.project.ID, VersionNumber: n, ManifestContent: env.toml, LockFileContent: env.lock, PackageMetadata: "[]", CreatedBy: user, Description: "v" + string(rune('0'+n))}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		if err := local.ApplyTags(gdb, env.project.ID, n, nil, user); err != nil {
			t.Fatal(err)
		}
	}
	return env
}

func (e *detailEnv) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/client"+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func versionsDecode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func TestVersionsRoutes(t *testing.T) {
	env := newDetailEnv(t)
	base := "/projects/" + env.project.ID.String()

	rec := env.do(t, http.MethodGet, base+"/versions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	items := versionsDecode[[]shared.VersionItem](t, rec)
	if len(items) != 2 || items[0].Number != 2 || items[0].Tags[0] != "latest" {
		t.Fatalf("items = %+v", items)
	}
	if !strings.Contains(rec.Body.String(), `"published":[]`) || !strings.Contains(rec.Body.String(), `"sync":null`) {
		t.Fatalf("contract shape: %s", rec.Body)
	}

	rec = env.do(t, http.MethodGet, base+"/versions/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body)
	}
	detail := versionsDecode[shared.VersionDetail](t, rec)
	if detail.Number != 1 || len(detail.Spec.Platforms) != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	rec = env.do(t, http.MethodGet, base+"/versions/1/files/pixi.lock", "")
	if rec.Code != http.StatusOK || versionsDecode[shared.FileContent](t, rec).Content != env.lock {
		t.Fatalf("lock: %d", rec.Code)
	}
	if rec := env.do(t, http.MethodGet, base+"/versions/1/files/other.txt", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad file name: %d", rec.Code)
	}

	rec = env.do(t, http.MethodGet, base+"/versions/2/packages", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("packages: %d %s", rec.Code, rec.Body)
	}
	pkgs := versionsDecode[shared.ResolvedPackages](t, rec)
	if len(pkgs.Platforms) != 2 || len(pkgs.Packages["osx-arm64"]) == 0 || pkgs.TotalSize["osx-arm64"] == 0 {
		t.Fatalf("packages = %+v", pkgs.Platforms)
	}

	rec = env.do(t, http.MethodGet, base+"/versions/2/archive", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != artifact.ArchiveContentType {
		t.Fatalf("content type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename=artlock-v2.tar.gz` {
		t.Fatalf("content disposition %q", cd)
	}
	got, err := artifact.ReadArchive(bytes.NewReader(rec.Body.Bytes()), 0)
	if err != nil || string(got.Toml) != env.toml || got.Metadata.Version != 2 {
		t.Fatalf("archive content: %v", err)
	}

	// Missing versions are 404 (no server connected), bad params 400.
	for _, path := range []string{base + "/versions/9", base + "/versions/9/files/pixi.toml", base + "/versions/9/packages", base + "/versions/9/archive", "/projects/00000000-0000-0000-0000-000000000009/versions"} {
		if rec := env.do(t, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: want 404, got %d %s", path, rec.Code, rec.Body)
		}
	}
	if rec := env.do(t, http.MethodGet, base+"/versions/zero", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad version: %d", rec.Code)
	}
}

func TestInstallAndUninstallRoutes(t *testing.T) {
	env := newDetailEnv(t)
	base := "/projects/" + env.project.ID.String()

	rec := env.do(t, http.MethodPost, base+"/install", `{"version":1}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", rec.Code, rec.Body)
	}
	job := versionsDecode[shared.JobSummary](t, rec)
	if job.Type != models.JobTypeInstallVersion || job.VersionNumber == nil || *job.VersionNumber != 1 || job.Status != models.JobStatusCompleted {
		t.Fatalf("job = %+v", job)
	}
	if env.exec.frozen != 1 {
		t.Fatal("install is frozen")
	}
	items := versionsDecode[[]shared.VersionItem](t, env.do(t, http.MethodGet, base+"/versions", ""))
	if !items[1].Installed || items[0].Installed {
		t.Fatalf("v1 installed: %+v", items)
	}

	// Empty body: last installed (v1).
	rec = env.do(t, http.MethodPost, base+"/install", "")
	if rec.Code != http.StatusAccepted || *versionsDecode[shared.JobSummary](t, rec).VersionNumber != 1 {
		t.Fatalf("install default: %d %s", rec.Code, rec.Body)
	}

	if rec := env.do(t, http.MethodPost, base+"/install", `{"version":5}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing version: %d %s", rec.Code, rec.Body)
	}
	if rec := env.do(t, http.MethodPost, base+"/install", `{"version":`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", rec.Code)
	}

	rec = env.do(t, http.MethodPost, base+"/uninstall", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("uninstall: %d %s", rec.Code, rec.Body)
	}
	if job := versionsDecode[shared.JobSummary](t, rec); job.Type != models.JobTypeUninstall || *job.VersionNumber != 1 {
		t.Fatalf("uninstall job = %+v", job)
	}
	rec = env.do(t, http.MethodPost, base+"/uninstall", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not_installed") {
		t.Fatalf("uninstall twice: %d %s", rec.Code, rec.Body)
	}
}
