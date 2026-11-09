package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"gorm.io/gorm"
)

type jobsAPI struct {
	h  http.Handler
	db *gorm.DB
	p  *models.Project
}

func newJobsAPI(t *testing.T) *jobsAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	gdb, err := local.OpenDB(dir)
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
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Storage.ProjectsDir = t.TempDir()
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := client.NewStack(client.StackOptions{
		DB: gdb, Executor: exec, DataDir: dir, Queue: queue.NewMemoryQueue(4),
		Desktop: desktop.Capabilities{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	stack.API().Register(r.Group("/api/v1"))
	p := &models.Project{Name: "proj", OwnerID: stack.SDK.Deps().LocalUserID, Status: models.ProjectStatusReady}
	if err := gdb.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	return &jobsAPI{h: r, db: gdb, p: p}
}

func (a *jobsAPI) job(t *testing.T, typ models.JobType, st models.JobStatus, logs string) *models.Job {
	t.Helper()
	now := time.Now()
	j := &models.Job{ProjectID: a.p.ID, Type: typ, Status: st, Logs: logs}
	if st != models.JobStatusPending {
		j.StartedAt = &now
	}
	if st == models.JobStatusCompleted || st == models.JobStatusFailed {
		j.CompletedAt = &now
	}
	if err := a.db.Create(j).Error; err != nil {
		t.Fatal(err)
	}
	return j
}

func (a *jobsAPI) do(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestJobsRoutes(t *testing.T) {
	a := newJobsAPI(t)
	done := a.job(t, models.JobTypeInstallVersion, models.JobStatusFailed, "line 1\nError: boom\n")
	pending := a.job(t, models.JobTypeCreateVersion, models.JobStatusPending, "")

	rec := a.do(t, http.MethodGet, "/api/v1/client/jobs?status=failed&type=install_version&project_id="+a.p.ID.String()+"&sort=-started_at&limit=10")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var list []shared.JobSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != done.ID || list[0].ProjectName != "proj" || list[0].TypeLabel != "Install version" {
		t.Fatalf("list = %+v", list)
	}

	// Empty result is [] not null.
	rec = a.do(t, http.MethodGet, "/api/v1/client/jobs?status=running")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list body = %s", rec.Body)
	}

	for path, want := range map[string]int{
		"/api/v1/client/jobs?status=bogus":        http.StatusUnprocessableEntity,
		"/api/v1/client/jobs?sort=name":           http.StatusUnprocessableEntity,
		"/api/v1/client/jobs?project_id=nope":     http.StatusBadRequest,
		"/api/v1/client/jobs?limit=x":             http.StatusBadRequest,
		"/api/v1/client/jobs/" + uuid.NewString(): http.StatusNotFound,
		"/api/v1/client/jobs/not-a-uuid":          http.StatusBadRequest,
	} {
		if rec := a.do(t, http.MethodGet, path); rec.Code != want {
			t.Errorf("GET %s = %d, want %d (%s)", path, rec.Code, want, rec.Body)
		}
	}

	rec = a.do(t, http.MethodGet, "/api/v1/client/jobs/"+done.ID.String())
	var detail shared.JobDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.Logs != "line 1\nError: boom\n" || detail.Status != models.JobStatusFailed {
		t.Fatalf("detail = %+v, %v", detail, err)
	}

	rec = a.do(t, http.MethodPost, "/api/v1/client/jobs/"+pending.ID.String()+"/cancel")
	var cancelled shared.JobSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelled); err != nil || rec.Code != http.StatusOK || cancelled.Status != models.JobStatusCancelled {
		t.Fatalf("cancel pending = %d %s", rec.Code, rec.Body)
	}
	rec = a.do(t, http.MethodPost, "/api/v1/client/jobs/"+done.ID.String()+"/cancel")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "job_finished") {
		t.Fatalf("cancel finished = %d %s", rec.Code, rec.Body)
	}
}

func TestJobLogStream(t *testing.T) {
	a := newJobsAPI(t)
	done := a.job(t, models.JobTypeInstallVersion, models.JobStatusFailed, "line 1\nline 2\n")
	rec := a.do(t, http.MethodGet, "/api/v1/client/jobs/"+done.ID.String()+"/logs/stream")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	want := "data: line 1\ndata: line 2\ndata: \n\nevent: done\ndata: failed\n\n"
	if rec.Body.String() != want {
		t.Fatalf("stream body %q, want %q", rec.Body.String(), want)
	}

	// A running job run elsewhere streams its stored log until it ends.
	running := a.job(t, models.JobTypeInstallVersion, models.JobStatusRunning, "start\n")
	go func() {
		time.Sleep(100 * time.Millisecond)
		now := time.Now()
		a.db.Model(&models.Job{}).Where("id = ?", running.ID).Updates(map[string]any{"logs": "start\nend\n", "status": models.JobStatusCompleted, "completed_at": now})
	}()
	rec = a.do(t, http.MethodGet, "/api/v1/client/jobs/"+running.ID.String()+"/logs/stream")
	body := rec.Body.String()
	if !strings.HasPrefix(body, "data: start\n") || !strings.Contains(body, "data: end\n") || !strings.HasSuffix(body, "event: done\ndata: completed\n\n") {
		t.Fatalf("running stream body %q", body)
	}

	if rec := a.do(t, http.MethodGet, "/api/v1/client/jobs/"+uuid.NewString()+"/logs/stream"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing job stream = %d", rec.Code)
	}
}
