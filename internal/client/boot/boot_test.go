package boot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
)

// fakePixi stands in for pixi: these tests never lock or install a
// project.
type fakePixi struct{}

func (fakePixi) Lock(context.Context, string, io.Writer) error      { return nil }
func (fakePixi) Install(context.Context, string, io.Writer) error   { return nil }
func (fakePixi) CheckLock(context.Context, string, io.Writer) error { return nil }

// brokenPixi is the PixiPath that makes the fake pixi runner fail.
const brokenPixi = "broken-pixi"

func TestMain(m *testing.M) {
	newPixiRunner = func(_ context.Context, path string) (local.PixiRunner, error) {
		if path == brokenPixi {
			return nil, errors.New("pixi does not run")
		}
		return fakePixi{}, nil
	}
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(runHelper(mode))
	}
	os.Exit(m.Run())
}

var testAssets = fstest.MapFS{
	"index.html": {Data: []byte("<!doctype html><html><head><title>Nebi</title></head><body></body></html>")},
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testOptions(t *testing.T, kind jobs.OwnerKind, dir string) Options {
	t.Helper()
	return Options{
		Kind:        kind,
		DataDir:     dir,
		ProjectsDir: filepath.Join(t.TempDir(), "projects"),
		PixiPath:    "pixi",
		Version:     api.VersionInfo{NebiVersion: "1.2.3"},
		Assets:      testAssets,
		Logger:      quietLogger(),
		Status:      io.Discard,
	}
}

func start(t *testing.T, opts Options) *Instance {
	t.Helper()
	inst, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = inst.Shutdown(context.Background()) })
	return inst
}

func readOwnerJSON(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, jobs.OwnerInfoFile))
	if err != nil {
		t.Fatalf("reading owner.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func requireLockHeld(t *testing.T, dir string) {
	t.Helper()
	o, err := jobs.TryAcquireOwner(dir)
	if err == nil {
		_ = o.Release()
		t.Fatal("the owner lock is free")
	}
	if !errors.Is(err, jobs.ErrOwnerLockHeld) {
		t.Fatalf("TryAcquireOwner: %v", err)
	}
}

// within waits for ch to close, and fails the test if that takes more than
// ten seconds.
func within(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func requireLockFree(t *testing.T, dir string) {
	t.Helper()
	o, err := jobs.TryAcquireOwner(dir)
	if err != nil {
		t.Fatalf("the owner lock is not free: %v", err)
	}
	_ = o.Release()
}

// seedInterruptedDelete records a project delete that a crash interrupted
// after moving the directory aside, as DeleteProject does, so that only
// RecoverDeletes can remove it. It returns the moved-aside directory.
func seedInterruptedDelete(t *testing.T, dir string) string {
	t.Helper()
	db, err := openDatabase(filepath.Join(dir, DatabaseFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Migrate(db); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(t.TempDir(), "gone")
	trash := filepath.Join(filepath.Dir(projectDir), ".gone.deleting-x")
	if err := os.MkdirAll(filepath.Join(trash, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO client_pending_removals (id, project_id, path, trash_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), uuid.NewString(), projectDir, trash, time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	return trash
}

// The steps run in the order of the issue: lock before the database,
// owner.json published without and then with the address and token, and
// recovery once, after taking the lock.
func TestStartupOrder(t *testing.T) {
	for _, kind := range []jobs.OwnerKind{jobs.OwnerWeb, jobs.OwnerDesktop} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			// An earlier owner's database with an interrupted delete. It is
			// moved out of the way, so the test can check that Start opens
			// no database before it holds the lock, and put back once the
			// lock is held.
			trash := seedInterruptedDelete(t, dir)
			dbPath := filepath.Join(dir, DatabaseFile)
			dbBefore, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(dbPath); err != nil {
				t.Fatal(err)
			}

			var steps []string
			testHook = func(s string) {
				steps = append(steps, s)
				switch s {
				case "locked":
					requireLockHeld(t, dir)
					if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("the database exists before the lock is taken: %v", err)
					}
					// Put the seeded database in place now that the lock
					// is held, as a database an earlier owner left.
					if err := os.WriteFile(dbPath, dbBefore, 0o600); err != nil {
						t.Fatal(err)
					}
				case "published":
					info := readOwnerJSON(t, dir)
					if info["kind"] != string(kind) || info["address"] != "" || info["token"] != "" ||
						info["hostname"] == "" || info["client_api_version"] != api.ClientAPIVersion {
						t.Errorf("first owner.json = %v, want the kind and no address", info)
					}
				case "recovered":
					if _, err := os.Stat(dbPath); err != nil {
						t.Errorf("the database is not open at recovery: %v", err)
					}
				}
			}
			t.Cleanup(func() { testHook = nil })

			inst := start(t, testOptions(t, kind, dir))
			testHook = nil
			want := []string{"locked", "published", "database", "recovered", "listening"}
			if !slices.Equal(steps, want) {
				t.Fatalf("steps %v, want %v", steps, want)
			}

			info := readOwnerJSON(t, dir)
			if info["address"] != inst.Addr() || info["token"] == "" || info["kind"] != string(kind) {
				t.Fatalf("second owner.json = %v, want the address %s and a token", info, inst.Addr())
			}
			if !strings.HasPrefix(inst.Addr(), "127.0.0.1:") {
				t.Fatalf("listening on %s, want 127.0.0.1", inst.Addr())
			}

			// RecoverDeletes ran: the interrupted delete's directory goes.
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(trash); errors.Is(err, os.ErrNotExist) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("%s is still there: RecoverDeletes did not run", trash)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

// End to end over the real listener: the version needs no token, the rest
// does, the sign-in URL sets the cookie, and a foreign Host is refused.
func TestServesGuardedAPIOnLoopback(t *testing.T) {
	dir := t.TempDir()
	inst := start(t, testOptions(t, jobs.OwnerWeb, dir))
	base := "http://" + inst.Addr()
	token := readOwnerJSON(t, dir)["token"].(string)

	get := func(path string, edit func(*http.Request)) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if edit != nil {
			edit(req)
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp
	}

	if resp := get("/api/v1/version", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /version = %d", resp.StatusCode)
	}
	if resp := get("/api/v1/capabilities", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /capabilities without a token = %d, want 401", resp.StatusCode)
	}
	if resp := get("/api/v1/capabilities", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /capabilities with the token = %d", resp.StatusCode)
	}
	if resp := get("/api/v1/version", func(r *http.Request) { r.Host = "evil.example:" + strings.Split(inst.Addr(), ":")[1] }); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET /version with a foreign Host = %d, want 403", resp.StatusCode)
	}
	if inst.SignInURL() != base+"/?token="+token {
		t.Fatalf("SignInURL = %q", inst.SignInURL())
	}
	resp := get("/?token="+token, nil)
	if resp.StatusCode != http.StatusSeeOther || len(resp.Cookies()) != 1 {
		t.Fatalf("sign-in URL = %d, cookies %v", resp.StatusCode, resp.Cookies())
	}
	c := resp.Cookies()[0]
	if resp := get("/api/v1/capabilities", func(r *http.Request) { r.AddCookie(c) }); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /capabilities with the cookie = %d", resp.StatusCode)
	}
	if resp := get("/", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d", resp.StatusCode)
	}

	// The desktop window's in-process handler needs no token.
	rec := &bytes.Buffer{}
	w := &recorder{header: http.Header{}, body: rec}
	inst.InProcessHandler().ServeHTTP(w, mustRequest(t, "/api/v1/capabilities"))
	if w.status != http.StatusOK {
		t.Fatalf("in-process GET /capabilities = %d %s", w.status, rec)
	}
}

type recorder struct {
	header http.Header
	body   *bytes.Buffer
	status int
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}
func (r *recorder) WriteHeader(status int) { r.status = status }

func mustRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://wails.localhost"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// Both roots open nebi-client.db and never open or change nebi.db.
func TestUsesClientDatabaseOnly(t *testing.T) {
	for _, kind := range []jobs.OwnerKind{jobs.OwnerWeb, jobs.OwnerDesktop} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			old := filepath.Join(dir, "nebi.db")
			content := []byte("the old local-mode database")
			if err := os.WriteFile(old, content, 0o600); err != nil {
				t.Fatal(err)
			}
			past := time.Now().Add(-time.Hour).Truncate(time.Second)
			if err := os.Chtimes(old, past, past); err != nil {
				t.Fatal(err)
			}

			inst := start(t, testOptions(t, kind, dir))
			if err := inst.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}

			got, err := os.ReadFile(old)
			if err != nil || !bytes.Equal(got, content) {
				t.Fatalf("nebi.db changed: %q, %v", got, err)
			}
			if fi, err := os.Stat(old); err != nil || !fi.ModTime().Equal(past) {
				t.Fatalf("nebi.db was touched: %v, %v", fi.ModTime(), err)
			}
			db, err := openDatabase(filepath.Join(dir, DatabaseFile))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
			if !db.Migrator().HasTable("client_projects") {
				t.Fatal("nebi-client.db has no client tables")
			}
			for _, name := range []string{"nebi.db-wal", "nebi.db-journal", "nebi.db-shm"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s exists: nebi.db was opened", name)
				}
			}
		})
	}
}

func TestEnvironment(t *testing.T) {
	data, projects := t.TempDir(), t.TempDir()
	t.Setenv(EnvDataDir, data)
	t.Setenv(EnvProjectsDir, projects)
	if got, err := DataDir(); err != nil || got != data {
		t.Fatalf("DataDir() = %q, %v; want %s", got, err, data)
	}
	if got := ProjectsDir(); got != projects {
		t.Fatalf("ProjectsDir() = %q, want %s", got, projects)
	}

	// The SDK puts projects in the directory the entry point passes.
	opts := testOptions(t, jobs.OwnerWeb, data)
	opts.ProjectsDir = ProjectsDir()
	inst := start(t, opts)
	path, err := inst.svc.DefaultProjectPath("demo")
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(projects)
	if path != filepath.Join(resolved, "demo") {
		t.Fatalf("default project path %s, want it in %s", path, projects)
	}

	t.Setenv(EnvDataDir, "")
	t.Setenv(EnvProjectsDir, "")
	if got, err := DataDir(); err != nil || !strings.HasSuffix(got, "nebi") {
		t.Fatalf("default DataDir() = %q, %v", got, err)
	}
	if ProjectsDir() != "" {
		t.Fatal("ProjectsDir() is set without NEBI_PROJECTS_DIR")
	}
}

func TestFindPixi(t *testing.T) {
	t.Setenv(EnvPixiPath, "/opt/pixi/bin/pixi")
	if got, err := FindPixi(); err != nil || got != "/opt/pixi/bin/pixi" {
		t.Fatalf("FindPixi with NEBI_PIXI_PATH = %q, %v", got, err)
	}
	t.Setenv(EnvPixiPath, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	if _, err := FindPixi(); !errors.Is(err, ErrPixiNotFound) {
		t.Fatalf("FindPixi without pixi = %v, want ErrPixiNotFound", err)
	}
	name := "pixi"
	if filepath.Separator == '\\' {
		name = "pixi.exe"
	}
	installed := filepath.Join(home, ".pixi", "bin", name)
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FindPixi(); err != nil || got != installed {
		t.Fatalf("FindPixi = %q, %v; want %s", got, err, installed)
	}
}

// Shutdown stops accepting requests, cancels running jobs and waits for
// them, removes owner.json and releases the lock.
func TestShutdown(t *testing.T) {
	dir := t.TempDir()
	inst := start(t, testOptions(t, jobs.OwnerWeb, dir))
	addr := inst.Addr()

	started, cancelled, proceed, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	// Let the job finish on a failure too, so cleanup does not wait for it.
	letJobFinish := sync.OnceFunc(func() { close(proceed) })
	t.Cleanup(letJobFinish)
	job, err := inst.runner.Submit(context.Background(), uuid.New(), jobs.TypeInstall, func(ctx context.Context, _ io.Writer) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-proceed // cleanup after cancel
		close(finished)
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	within(t, started, "the job to start")

	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- inst.Shutdown(context.Background()) }()
	within(t, cancelled, "Shutdown to cancel the running job")
	// While the job cleans up, the lock is still held and owner.json no
	// longer names the address, so a starting process waits for the lock.
	requireLockHeld(t, dir)
	if info := readOwnerJSON(t, dir); info["address"] != "" || info["token"] != "" || info["kind"] != "web" {
		t.Fatalf("owner.json during shutdown = %v, want no address", info)
	}
	letJobFinish()
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not return after the job finished")
	}
	select {
	case <-finished:
	default:
		t.Fatal("Shutdown returned before the running job finished")
	}
	if j, err := inst.runner.Get(job.ID); err != nil || j.Status != jobs.StatusCancelled {
		t.Fatalf("job after shutdown = %+v, %v; want cancelled", j, err)
	}
	if _, err := os.Stat(filepath.Join(dir, jobs.OwnerInfoFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner.json after shutdown: %v, want it removed", err)
	}
	requireLockFree(t, dir)
	if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		_ = conn.Close()
		t.Fatal("the listener still accepts connections")
	}
	select {
	case <-inst.Done():
	default:
		t.Fatal("Done is not closed after Shutdown")
	}
	if err := inst.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

// A job that ignores cancellation does not hold shutdown forever.
func TestShutdownIsBounded(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(t, jobs.OwnerWeb, dir)
	opts.ShutdownWait = 300 * time.Millisecond
	inst := start(t, opts)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{})
	if _, err := inst.runner.Submit(context.Background(), uuid.New(), jobs.TypeInstall, func(context.Context, io.Writer) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	within(t, started, "the job to start")
	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- inst.Shutdown(context.Background()) }()
	select {
	case err := <-shutdownErr:
		if err == nil || !strings.Contains(err.Error(), "did not stop") {
			t.Fatalf("Shutdown with a stuck job = %v, want a timeout error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown with a stuck job did not return: its wait is not bounded")
	}
	requireLockFree(t, dir)
}

// A start that fails after taking the lock leaves nothing held.
func TestFailedStartReleasesEverything(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(t, jobs.OwnerWeb, dir)
	opts.PixiPath = brokenPixi
	if _, err := Start(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "pixi does not run") {
		t.Fatalf("Start with broken pixi = %v", err)
	}
	requireLockFree(t, dir)
	if _, err := os.Stat(filepath.Join(dir, jobs.OwnerInfoFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner.json after a failed start: %v", err)
	}

	// The port is taken: Start says so and leaves nothing held either.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	opts = testOptions(t, jobs.OwnerWeb, dir)
	opts.Port = ln.Addr().(*net.TCPAddr).Port
	_, err = Start(context.Background(), opts)
	if !errors.Is(err, ErrPortInUse) || !strings.Contains(err.Error(), strconv.Itoa(opts.Port)) {
		t.Fatalf("Start on a taken port = %v, want ErrPortInUse naming the port", err)
	}
	requireLockFree(t, dir)
}

func TestStartValidatesOptions(t *testing.T) {
	dir := t.TempDir()
	for name, edit := range map[string]func(*Options){
		"cli kind":     func(o *Options) { o.Kind = jobs.OwnerCLI },
		"no data dir":  func(o *Options) { o.DataDir = "" },
		"no pixi":      func(o *Options) { o.PixiPath = "" },
		"no version":   func(o *Options) { o.Version.NebiVersion = "" },
		"no assets":    func(o *Options) { o.Assets = nil },
		"port too big": func(o *Options) { o.Port = 70000 },
	} {
		opts := testOptions(t, jobs.OwnerWeb, dir)
		edit(&opts)
		if _, err := Start(context.Background(), opts); err == nil {
			t.Errorf("Start with %s succeeded", name)
		}
	}
	requireLockFree(t, dir)
}
