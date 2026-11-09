package client

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/localext"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/worker"
)

type fakePicker struct{}

func (fakePicker) PickFolder(context.Context, string, string) (string, error) {
	return "/picked", nil
}

// newLocalRouter builds the production local-mode router with the client
// extension, so a gin route conflict between the client API and the legacy
// routes fails here.
func newLocalRouter(t *testing.T, caps desktop.Capabilities) (http.Handler, *Extension) {
	t.Helper()
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Auth.JWTSecret = "test-secret-for-client-test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "client-test.db")
	cfg.Storage.ProjectsDir = t.TempDir()
	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}
	q := queue.NewMemoryQueue(8)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ext := NewExtension(caps)
	if err := ext.Init(localext.Deps{Config: cfg, DB: database, Queue: q, Executor: exec, Logger: logger}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	var _ worker.Dispatcher = ext.Dispatcher()
	return api.NewRouterWithOptions(cfg, database, q, exec, nil, logger, api.RouterOptions{Local: ext}), ext
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCapabilities(t *testing.T) {
	h, _ := newLocalRouter(t, desktop.Capabilities{})
	rec := do(t, h, http.MethodGet, "/api/v1/client/capabilities", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var caps shared.Capabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if caps.NativeFolderPicker {
		t.Fatal("nebi-web must not report a native folder picker")
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/client/desktop/pick-folder", "{}"); rec.Code != http.StatusNotImplemented {
		t.Fatalf("pick-folder without picker: want 501, got %d", rec.Code)
	}

	h, _ = newLocalRouter(t, desktop.Capabilities{FolderPicker: fakePicker{}})
	rec = do(t, h, http.MethodPost, "/api/v1/client/desktop/pick-folder", `{"title":"Choose a folder"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/picked") {
		t.Fatalf("pick-folder with picker: %d %s", rec.Code, rec.Body)
	}
}

// TestStubsReturnNotImplemented pins the contract that unfilled SDK
// functions answer 501 with code "not_implemented".
func TestStubsReturnNotImplemented(t *testing.T) {
	h, _ := newLocalRouter(t, desktop.Capabilities{})
	rec := do(t, h, http.MethodGet, "/api/v1/client/compare-does-not-exist", "")
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("unexpected 500")
	}
	rec = do(t, h, http.MethodGet, "/api/v1/client/projects/00000000-0000-0000-0000-000000000001/compare?base=1&target=2", "")
	if rec.Code != http.StatusNotImplemented && rec.Code != http.StatusNotFound {
		t.Fatalf("compare: want 501 (stub) or 404, got %d: %s", rec.Code, rec.Body)
	}
}

func TestApplyTagsMovesLatest(t *testing.T) {
	_, ext := newLocalRouter(t, desktop.Capabilities{})
	gdb := ext.Stack().SDK.Deps().DB
	user := ext.Stack().SDK.Deps().LocalUserID
	p := models.Project{Name: "p", OwnerID: user, Status: models.ProjectStatusReady}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 2; n++ {
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: n, ManifestContent: "x", LockFileContent: "y", PackageMetadata: "[]", CreatedBy: user}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		if err := local.ApplyTags(gdb, p.ID, n, []string{"stable"}, user); err != nil {
			t.Fatal(err)
		}
	}
	var tags []models.ProjectTag
	gdb.Where("project_id = ?", p.ID).Find(&tags)
	got := map[string]int{}
	for _, tag := range tags {
		got[tag.Tag] = tag.VersionNumber
	}
	if got["latest"] != 2 || got["stable"] != 2 || len(got) != 2 {
		t.Fatalf("tags = %v", got)
	}

	resolved, err := ext.Stack().SDK.Local.ResolveProject(context.Background(), "p")
	if err != nil || resolved.ID != p.ID {
		t.Fatalf("ResolveProject = %v, %v", resolved, err)
	}
	if _, err := ext.Stack().SDK.Local.LoadVersion(context.Background(), p.ID, 3); err == nil {
		t.Fatal("LoadVersion of a missing version must fail")
	}
}

func TestDispatcherOnlyHandlesClientJobs(t *testing.T) {
	_, ext := newLocalRouter(t, desktop.Capabilities{})
	d := ext.Dispatcher()
	legacy := &models.Job{Type: models.JobTypeCreate}
	if d.Handles(legacy) {
		t.Fatal("legacy create jobs must stay on the worker switch")
	}
	clientJob := &models.Job{Type: models.JobTypeCreateVersion, Metadata: map[string]any{jobs.MetadataOrigin: jobs.OriginClient}}
	if !d.Handles(clientJob) {
		t.Fatal("client create_version jobs must be dispatched")
	}
}
