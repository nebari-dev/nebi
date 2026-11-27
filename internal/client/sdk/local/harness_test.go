package local

import (
	"context"
	"crypto/sha256"
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

// versionProbe watches the real version and tag tables of a harness
// database through GORM callbacks: it can fail version inserts or deletes,
// records whether inserts ran in a transaction, counts deletes, and
// records any update of a version row, which must never happen.
type versionProbe struct {
	db *gorm.DB

	mu          sync.Mutex
	recordErr   error // fails inserts into client_project_versions
	deleteErr   error // fails deletes from client_project_versions
	deleteCalls int
	recordInTx  []bool
	updates     []string // statements that updated client_project_versions
}

const versionTable = "client_project_versions"

func newVersionProbe(t *testing.T, db *gorm.DB) *versionProbe {
	t.Helper()
	p := &versionProbe{db: db}
	cb := db.Callback()
	must := func(err error) {
		if err != nil {
			t.Fatalf("registering version probe: %v", err)
		}
	}
	must(cb.Create().Before("gorm:create").Register("test:version_create", func(tx *gorm.DB) {
		if tx.Statement.Table != versionTable {
			return
		}
		_, inTx := tx.Statement.ConnPool.(gorm.TxCommitter)
		p.mu.Lock()
		p.recordInTx = append(p.recordInTx, inTx)
		err := p.recordErr
		p.mu.Unlock()
		if err != nil {
			_ = tx.AddError(err)
		}
	}))
	must(cb.Delete().Before("gorm:delete").Register("test:version_delete", func(tx *gorm.DB) {
		if tx.Statement.Table != versionTable {
			return
		}
		p.mu.Lock()
		p.deleteCalls++
		err := p.deleteErr
		p.mu.Unlock()
		if err != nil {
			_ = tx.AddError(err)
		}
	}))
	must(cb.Update().Before("gorm:update").Register("test:version_update", func(tx *gorm.DB) {
		if tx.Statement.Table == versionTable {
			p.mu.Lock()
			p.updates = append(p.updates, "gorm update of "+versionTable)
			p.mu.Unlock()
		}
	}))
	must(cb.Raw().Before("gorm:raw").Register("test:version_raw", func(tx *gorm.DB) {
		sql := strings.ToLower(strings.TrimSpace(tx.Statement.SQL.String()))
		if strings.HasPrefix(sql, "update") && strings.Contains(sql, versionTable) {
			p.mu.Lock()
			p.updates = append(p.updates, sql)
			p.mu.Unlock()
		}
	}))
	// Versions are immutable: no test may see a version row updated.
	t.Cleanup(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.updates) > 0 {
			t.Errorf("version rows must never be updated, saw: %v", p.updates)
		}
	})
	return p
}

func (p *versionProbe) setRecordErr(err error) {
	p.mu.Lock()
	p.recordErr = err
	p.mu.Unlock()
}

func (p *versionProbe) setDeleteErr(err error) {
	p.mu.Lock()
	p.deleteErr = err
	p.mu.Unlock()
}

func (p *versionProbe) deletes() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.deleteCalls
}

func (p *versionProbe) inserts() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bool(nil), p.recordInTx...)
}

// versionsOf returns the project's versions, whole, oldest first.
func (p *versionProbe) versionsOf(t *testing.T, projectID uuid.UUID) []ProjectVersion {
	t.Helper()
	var list []ProjectVersion
	if err := p.db.Where("project_id = ?", projectID).Order("number").Find(&list).Error; err != nil {
		t.Fatal(err)
	}
	return list
}

// tagsOf maps each stored tag of the project to its version number.
func (p *versionProbe) tagsOf(t *testing.T, projectID uuid.UUID) map[string]int {
	t.Helper()
	var rows []struct {
		Name   string
		Number int
	}
	if err := p.db.Table("client_project_tags AS t").
		Select("t.name AS name, v.number AS number").
		Joins("JOIN client_project_versions v ON v.id = t.version_id").
		Where("t.project_id = ?", projectID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Name] = r.Number
	}
	return out
}

// fakePixi stands in for sdk/exec. Lock writes a pixi.lock derived from
// pixi.toml (fakeLockFor); Install creates .pixi/envs; CheckLock returns a
// fakeStaleLock unless pixi.lock is the one Lock would write. Lock and Install can fail
// or block, and both write pixi-like output to the job's log. A blocked
// call returns ctx's error when ctx is cancelled, as the real runner does
// when it stops pixi.
type fakePixi struct {
	mu         sync.Mutex
	lockErr    error
	installErr error
	checkErr   error
	// lockInputs records, per Lock call, the pixi.lock that was in the
	// directory when the call started ("" when there was none).
	lockInputs   []string
	checkCalls   []string
	checkGate    chan struct{} // when set, CheckLock waits until it is closed
	checkStarted chan struct{} // receives once per CheckLock call that waits on checkGate
	partialEnv   bool          // a failing install still leaves .pixi/envs behind
	lockGate     chan struct{} // when set, Lock waits until it is closed
	lockStarted  chan struct{} // receives once per Lock call that waits on lockGate
	lockCalls    []string
	installCalls []string
	// installSaw records, per Install call, the pixi.toml and pixi.lock
	// that were in the directory when the call started, so tests can check
	// what pixi installed from.
	installSaw []EnvironmentSpecification

	// installGate, when set, makes Install create a partial .pixi/envs and
	// then wait until it is closed or the job is cancelled, like a long
	// install that is interrupted.
	installGate    chan struct{}
	installStarted chan struct{} // receives once per Install call that waits on installGate
}

// fakeLockFor is the pixi.lock the fake writes for a manifest.
func fakeLockFor(manifest string) string {
	sum := sha256.Sum256([]byte(manifest))
	return fmt.Sprintf("version: 6\n# fake lock for %x\n", sum[:8])
}

// fakeLockOutput and fakeInstallOutput are what the fake writes to the
// job's log for dir.
func fakeLockOutput(dir string) string    { return fmt.Sprintf("pixi lock: solving %s\n", dir) }
func fakeInstallOutput(dir string) string { return fmt.Sprintf("pixi install: installing %s\n", dir) }

func (f *fakePixi) Lock(ctx context.Context, dir string, out io.Writer) error {
	input, _ := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	f.mu.Lock()
	f.lockCalls = append(f.lockCalls, dir)
	f.lockInputs = append(f.lockInputs, string(input))
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
	manifest, err := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, artifact.LockFile), []byte(fakeLockFor(string(manifest))), 0o644)
}

// fakeStaleLock is the fake's stale-lock error. Like the real runner's,
// it is its own type with a StaleLock method (see StaleLockError).
type fakeStaleLock struct{ dir string }

func (e fakeStaleLock) Error() string   { return "lock file is not up to date in " + e.dir }
func (e fakeStaleLock) StaleLock() bool { return true }

func (f *fakePixi) CheckLock(_ context.Context, dir string, out io.Writer) error {
	f.mu.Lock()
	f.checkCalls = append(f.checkCalls, dir)
	err, gate, started := f.checkErr, f.checkGate, f.checkStarted
	f.mu.Unlock()
	fmt.Fprintf(out, "pixi lock --check: %s\n", dir)
	if gate != nil {
		if started != nil {
			started <- struct{}{}
		}
		<-gate
	}
	if err != nil {
		return err
	}
	manifest, err := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
	if err != nil {
		return err
	}
	lock, err := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("pixi lock --check: %w", fakeStaleLock{dir})
	}
	if err != nil {
		return err
	}
	if string(lock) != fakeLockFor(string(manifest)) {
		return fmt.Errorf("pixi lock --check: %w", fakeStaleLock{dir})
	}
	return nil
}

func (f *fakePixi) Install(ctx context.Context, dir string, out io.Writer) error {
	manifest, _ := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
	lock, _ := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	f.mu.Lock()
	f.installCalls = append(f.installCalls, dir)
	f.installSaw = append(f.installSaw, EnvironmentSpecification{Manifest: string(manifest), Lock: string(lock)})
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

// lastInstallSaw returns the files the latest Install call found.
func (f *fakePixi) lastInstallSaw() (EnvironmentSpecification, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.installSaw) == 0 {
		return EnvironmentSpecification{}, false
	}
	return f.installSaw[len(f.installSaw)-1], true
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
	versions    *versionProbe
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
	h := &harness{
		t:           t,
		db:          db,
		runner:      jobs.NewMemory(ro),
		pixi:        &fakePixi{},
		versions:    newVersionProbe(t, db),
		projectsDir: filepath.Join(t.TempDir(), "projects"),
	}
	opts := Options{
		DB:          db,
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

// insertRow inserts a project row directly, bypassing validation, for
// fixtures that CreateProject would refuse.
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
