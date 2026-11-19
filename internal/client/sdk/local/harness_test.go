package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestDB opens a fresh SQLite file database (not :memory:, whose
// connections would each see a different database).
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nebi.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// testVersion is the table behind fakeVersions. It is a real table so the
// delete transaction can be checked for atomicity. Like #661's
// ProjectVersion it has no job column.
type testVersion struct {
	ID          uuid.UUID `gorm:"type:text;primaryKey"`
	ProjectID   uuid.UUID `gorm:"type:text;index"`
	Number      int
	Manifest    string
	Lock        string
	Origin      VersionOrigin
	Base        *uuid.UUID `gorm:"type:text"`
	Author      string
	Description string
	Tags        string
}

func (testVersion) TableName() string { return "test_versions" }

// fakeVersions stands in for the version CRUD issue (#661).
type fakeVersions struct {
	db *gorm.DB

	mu          sync.Mutex
	recordErr   error
	deleteErr   error
	deleteCalls []uuid.UUID
	recordTx    []*gorm.DB
}

func (f *fakeVersions) RecordVersion(tx *gorm.DB, req RecordVersionRequest) (uuid.UUID, error) {
	f.mu.Lock()
	err := f.recordErr
	f.recordTx = append(f.recordTx, tx)
	f.mu.Unlock()
	if err != nil {
		return uuid.Nil, err
	}
	var n int64
	if err := tx.Model(&testVersion{}).Where("project_id = ?", req.ProjectID).Count(&n).Error; err != nil {
		return uuid.Nil, err
	}
	v := testVersion{
		ID:          uuid.New(),
		ProjectID:   req.ProjectID,
		Number:      int(n) + 1,
		Manifest:    req.Manifest,
		Lock:        req.Lock,
		Origin:      req.Origin,
		Base:        req.BaseVersionID,
		Author:      req.Author,
		Description: req.Description,
		Tags:        strings.Join(req.Tags, ","),
	}
	return v.ID, tx.Create(&v).Error
}

func (f *fakeVersions) VersionInfo(ctx context.Context, projectID uuid.UUID, installedID *uuid.UUID) (VersionInfo, error) {
	var list []testVersion
	if err := f.db.WithContext(ctx).Where("project_id = ?", projectID).Order("number").Find(&list).Error; err != nil {
		return VersionInfo{}, err
	}
	info := VersionInfo{Count: len(list)}
	if len(list) > 0 {
		latest := list[len(list)-1].Number
		info.Latest = &latest
	}
	for _, v := range list {
		if installedID != nil && v.ID == *installedID {
			n := v.Number
			info.Installed = &n
		}
	}
	return info, nil
}

func (f *fakeVersions) DeleteProjectVersions(tx *gorm.DB, projectID uuid.UUID) error {
	f.mu.Lock()
	f.deleteCalls = append(f.deleteCalls, projectID)
	err := f.deleteErr
	f.mu.Unlock()
	if err := tx.Where("project_id = ?", projectID).Delete(&testVersion{}).Error; err != nil {
		return err
	}
	return err
}

func (f *fakeVersions) versionsOf(t *testing.T, projectID uuid.UUID) []testVersion {
	t.Helper()
	var list []testVersion
	if err := f.db.Where("project_id = ?", projectID).Order("number").Find(&list).Error; err != nil {
		t.Fatal(err)
	}
	return list
}

// fakePixi stands in for sdk/exec. Lock writes a pixi.lock; Install
// creates .pixi/envs. Either can fail or block, and both write pixi-like
// output to the job's log. A blocked call returns ctx's error when ctx is
// cancelled, as the real runner does when it stops pixi.
type fakePixi struct {
	mu           sync.Mutex
	lockErr      error
	installErr   error
	partialEnv   bool          // a failing install still leaves .pixi/envs behind
	lockGate     chan struct{} // when set, Lock waits until it is closed
	lockStarted  chan struct{} // receives once per Lock call that waits on lockGate
	lockCalls    []string
	installCalls []string

	// installGate, when set, makes Install create a partial .pixi/envs and
	// then wait until it is closed or the job is cancelled, like a long
	// install that is interrupted.
	installGate    chan struct{}
	installStarted chan struct{} // receives once per Install call that waits on installGate
}

const fakeLockContent = "version: 6\n# fake lock\n"

// fakeLockOutput and fakeInstallOutput are what the fake writes to the
// job's log for dir.
func fakeLockOutput(dir string) string    { return fmt.Sprintf("pixi lock: solving %s\n", dir) }
func fakeInstallOutput(dir string) string { return fmt.Sprintf("pixi install: installing %s\n", dir) }

func (f *fakePixi) Lock(ctx context.Context, dir string, out io.Writer) error {
	f.mu.Lock()
	f.lockCalls = append(f.lockCalls, dir)
	gate, started, err := f.lockGate, f.lockStarted, f.lockErr
	f.mu.Unlock()
	if _, werr := io.WriteString(out, fakeLockOutput(dir)); werr != nil {
		return werr
	}
	if gate != nil {
		if started != nil {
			started <- struct{}{}
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return err
	}
	return os.WriteFile(filepath.Join(dir, artifact.LockFile), []byte(fakeLockContent), 0o644)
}

func (f *fakePixi) Install(ctx context.Context, dir string, out io.Writer) error {
	f.mu.Lock()
	f.installCalls = append(f.installCalls, dir)
	err, partial := f.installErr, f.partialEnv
	gate, started := f.installGate, f.installStarted
	f.mu.Unlock()
	if _, werr := io.WriteString(out, fakeInstallOutput(dir)); werr != nil {
		return werr
	}
	if gate != nil {
		if mkErr := os.MkdirAll(filepath.Join(dir, ".pixi", "envs", "default"), 0o755); mkErr != nil {
			return mkErr
		}
		if started != nil {
			started <- struct{}{}
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		if partial {
			_ = os.MkdirAll(filepath.Join(dir, ".pixi", "envs"), 0o755)
		}
		return err
	}
	envDir := filepath.Join(dir, ".pixi", "envs", "default")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(envDir, "marker"), []byte("installed"), 0o644)
}

func (f *fakePixi) calls() (lock, install []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lockCalls...), append([]string(nil), f.installCalls...)
}

// failingSubmitter is a JobRunner whose Submit always fails.
type failingSubmitter struct{ *jobs.Memory }

func (failingSubmitter) Submit(context.Context, uuid.UUID, jobs.Type, jobs.Func) (*jobs.Job, error) {
	return nil, errors.New("queue unavailable")
}

type harness struct {
	t           *testing.T
	db          *gorm.DB
	svc         *Service
	runner      *jobs.Memory
	pixi        *fakePixi
	versions    *fakeVersions
	projectsDir string // as configured (may contain symlinks on macOS)
}

func newHarness(t *testing.T, mods ...func(*Options)) *harness {
	t.Helper()
	return newHarnessWithRunner(t, jobs.Options{}, mods...)
}

// newHarnessWithRunner is newHarness with a job runner configured by ro.
func newHarnessWithRunner(t *testing.T, ro jobs.Options, mods ...func(*Options)) *harness {
	t.Helper()
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.AutoMigrate(&testVersion{}); err != nil {
		t.Fatalf("migrate test versions: %v", err)
	}
	h := &harness{
		t:           t,
		db:          db,
		runner:      jobs.NewMemory(ro),
		pixi:        &fakePixi{},
		versions:    &fakeVersions{db: db},
		projectsDir: filepath.Join(t.TempDir(), "projects"),
	}
	opts := Options{
		DB:          db,
		Versions:    h.versions,
		Jobs:        h.runner,
		Pixi:        h.pixi,
		ProjectsDir: h.projectsDir,
		Author:      "tester",
	}
	for _, m := range mods {
		m(&opts)
	}
	svc, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.svc = svc
	// Registered after every TempDir, so it runs first: background work
	// stops before the directories it may touch are removed.
	t.Cleanup(func() {
		svc.Close()
		h.runner.Close()
	})
	return h
}

// resolved returns p with symlinks resolved, as the SDK stores paths
// (t.TempDir lives under a symlink on macOS).
func (h *harness) resolved(p string) string {
	h.t.Helper()
	r, err := resolveSymlinks(p)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

func (h *harness) wait(job *jobs.Job) jobs.Job {
	h.t.Helper()
	if job == nil {
		h.t.Fatal("wait: nil job")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := h.runner.Wait(ctx, job.ID)
	if err != nil {
		h.t.Fatalf("waiting for job %s: %v", job.ID, err)
	}
	return got
}

func (h *harness) mustCreate(req CreateProjectRequest) (*Project, jobs.Job) {
	h.t.Helper()
	p, job, err := h.svc.CreateProject(context.Background(), req)
	if err != nil {
		h.t.Fatalf("CreateProject(%q): %v", req.Name, err)
	}
	return p, h.wait(job)
}

func (h *harness) projectCount() int64 {
	h.t.Helper()
	var n int64
	if err := h.db.Model(&Project{}).Count(&n).Error; err != nil {
		h.t.Fatal(err)
	}
	return n
}

func (h *harness) detail(id uuid.UUID) *ProjectDetail {
	h.t.Helper()
	d, err := h.svc.GetProject(context.Background(), id)
	if err != nil {
		h.t.Fatalf("GetProject: %v", err)
	}
	return d
}

// insertRow inserts a project row directly, bypassing validation (for
// legacy names and fixtures).
func (h *harness) insertRow(p *Project) {
	h.t.Helper()
	if err := h.db.Create(p).Error; err != nil {
		h.t.Fatalf("insert project row: %v", err)
	}
}

func specRequest(name string) CreateProjectRequest {
	return CreateProjectRequest{
		Name: name,
		Spec: &artifact.Spec{
			Platforms: []string{"linux-64", "osx-arm64"},
			Channels:  []string{"conda-forge"},
			Packages:  []artifact.Package{{Name: "numpy", Operator: ">=", Constraint: "1.26"}},
		},
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
