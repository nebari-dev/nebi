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
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
)

// nvExec is a minimal executor whose solve writes a fake pixi.lock.
type nvExec struct{ staging string }

func (nvExec) CreateProject(context.Context, *models.Project, io.Writer, executor.CreateProjectOptions) error {
	return nil
}
func (nvExec) InstallPackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (nvExec) RemovePackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (nvExec) DeleteProject(context.Context, *models.Project, io.Writer) error { return nil }
func (nvExec) SolveEnvironment(_ context.Context, p *models.Project, _ io.Writer) error {
	return os.WriteFile(filepath.Join(p.Path, "pixi.lock"), []byte("version: 6\nenvironments: {}\npackages: []\n"), 0o644)
}
func (nvExec) CleanupJobArtifacts(context.Context, *models.Project, models.JobType, io.Writer) error {
	return nil
}
func (nvExec) InstallEnvironment(context.Context, *models.Project, io.Writer) error   { return nil }
func (nvExec) UninstallEnvironment(context.Context, *models.Project, io.Writer) error { return nil }
func (nvExec) IsEnvInstalled(*models.Project) bool                                    { return false }
func (nvExec) GetProjectPath(p *models.Project) string                                { return p.Path }
func (e nvExec) StagingRoot() string                                                  { return e.staging }

const nvToml = `[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.12.*"
numpy = ">=2.0"
`

func newNVRouter(t *testing.T) (http.Handler, *client.Stack, models.Project) {
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
		DB: gdb, Executor: nvExec{staging: t.TempDir()}, DataDir: dataDir,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	user := stack.SDK.Deps().LocalUserID
	p := models.Project{Name: "demo", OwnerID: user, Status: models.ProjectStatusReady}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	v := models.ProjectVersion{ProjectID: p.ID, ManifestContent: nvToml, LockFileContent: "version: 6\n", PackageMetadata: "[]", CreatedBy: user}
	if err := gdb.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	if err := local.ApplyTags(gdb, p.ID, 1, []string{"stable"}, user); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	stack.API().Register(r.Group("/api/v1"))
	return r, stack, p
}

func nvDo(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNewVersionRoutes(t *testing.T) {
	h, stack, p := newNVRouter(t)
	base := "/api/v1/client/projects/" + p.ID.String()

	// Draft for the latest version.
	rec := nvDo(t, h, http.MethodGet, base+"/versions/latest/draft", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("draft: %d %s", rec.Code, rec.Body)
	}
	var draft local.VersionDraft
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	if draft.BaseVersion != 1 || draft.LatestVersion != 1 || draft.PixiToml != nvToml || len(draft.Form.Requested) != 2 {
		t.Fatalf("draft = %+v", draft)
	}
	if rec := nvDo(t, h, http.MethodGet, base+"/versions/4/draft", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing draft: %d %s", rec.Code, rec.Body)
	}
	if rec := nvDo(t, h, http.MethodGet, base+"/versions/abc/draft", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad version: %d", rec.Code)
	}

	// Preview with a form (applied onto version 1's pixi.toml).
	form := draft.Form
	form.Requested = append(form.Requested[:1], shared.RequestedPackage{Name: "pandas", Constraint: ">=2.2"})
	rec = nvDo(t, h, http.MethodPost, base+"/versions/preview", map[string]any{"base_version": 1, "form": form})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	var diff shared.VersionDiff
	if err := json.Unmarshal(rec.Body.Bytes(), &diff); err != nil {
		t.Fatal(err)
	}
	if diff.RequestedCounts.Added != 1 || diff.RequestedCounts.Removed != 1 {
		t.Fatalf("diff = %+v", diff.RequestedCounts)
	}
	rec = nvDo(t, h, http.MethodPost, base+"/versions/preview", map[string]any{"base_version": 1, "pixi_toml": "[workspace"})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"manifest"`) {
		t.Fatalf("preview invalid: %d %s", rec.Code, rec.Body)
	}

	// Validation and no-change errors.
	rec = nvDo(t, h, http.MethodPost, base+"/versions", shared.CreateVersionRequest{BaseVersion: 1, Form: &form})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"description"`) {
		t.Fatalf("missing description: %d %s", rec.Code, rec.Body)
	}
	rec = nvDo(t, h, http.MethodPost, base+"/versions", shared.CreateVersionRequest{BaseVersion: 1, PixiToml: nvToml, Description: "Same"})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"no_changes"`) || !strings.Contains(rec.Body.String(), "Nothing changed in demo.") {
		t.Fatalf("no change: %d %s", rec.Code, rec.Body)
	}

	// Create (inline stack: the job finishes before the response).
	rec = nvDo(t, h, http.MethodPost, base+"/versions", shared.CreateVersionRequest{
		BaseVersion: 1, Form: &form, Description: "Swapped numpy for pandas", Tags: []string{"stable"},
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var job shared.JobSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.Type != models.JobTypeCreateVersion || job.VersionNumber == nil || *job.VersionNumber != 2 || job.ProjectName != "demo" {
		t.Fatalf("job = %+v", job)
	}
	v, err := stack.SDK.Local.LoadVersion(context.Background(), p.ID, 2)
	if err != nil || *v.BaseVersion != 1 || v.Description != "Swapped numpy for pandas" {
		t.Fatalf("version 2 = %+v, %v", v, err)
	}

	// Unknown project.
	rec = nvDo(t, h, http.MethodPost, "/api/v1/client/projects/00000000-0000-0000-0000-000000000009/versions", shared.CreateVersionRequest{BaseVersion: 1, PixiToml: nvToml, Description: "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown project: %d %s", rec.Code, rec.Body)
	}
}
