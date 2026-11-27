// Package boot is the startup and shutdown that the two composition roots
// of the client, nebi-web (internal/client/web) and the desktop app
// (internal/client/desktop), share (issue #680). It makes the process the
// job-runner owner of its client database and serves the client API and
// the client UI on loopback:
//
//  1. The entry point resolves the data directory, the projects directory
//     and pixi (DataDir, ProjectsDir, FindPixi).
//  2. Start takes the owner lock on the data directory, before opening the
//     database. When another process holds it, Start follows the table in
//     acquire: it waits for a CLI command or an app that is still starting,
//     and returns a *RunningError for an app that serves its API.
//  3. It publishes owner.json with the PID, versions, kind and host name,
//     and no address yet.
//  4. It opens nebi-client.db in the data directory, creating it if
//     missing, and runs local.Migrate.
//  5. It builds the job runner, the PixiRunner and the local.Service.
//  6. It runs RecoverDeletes and RecoverInPlaceSolves, here and nowhere
//     else.
//  7. It listens on 127.0.0.1 and publishes owner.json again, with the
//     address and the session token.
//  8. It serves the client API, behind the token, and the client UI.
//
// Shutdown stops accepting requests, closes the job runner so running jobs
// are cancelled and waited for, closes the service and the database, and
// releases the lock, which removes owner.json. The wait is bounded; jobs
// that outlast it keep the database and the lock until they end.
package boot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk/exec"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/ui"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseFile is the client database, in the data directory (#678). The
// old nebi.db next to it is never opened.
const DatabaseFile = "nebi-client.db"

// Defaults for the zero values of Options.
const (
	// DefaultStartingWait bounds how long Start waits for another app that
	// holds the lock but does not serve its API yet.
	DefaultStartingWait = time.Minute
	// DefaultShutdownWait bounds Shutdown.
	DefaultShutdownWait = 30 * time.Second
)

// pollInterval is how often Start looks at a lock another process holds.
const pollInterval = 100 * time.Millisecond

// HTTP server timeouts. There is no write timeout: a client may follow a
// job's log for as long as the job runs.
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	maxHeaderBytes    = 1 << 20
)

// Options configures Start.
type Options struct {
	// Kind is jobs.OwnerWeb or jobs.OwnerDesktop.
	Kind jobs.OwnerKind
	// DataDir holds nebi-client.db and the owner lock. Required.
	DataDir string
	// ProjectsDir is where projects go when no path is given; see
	// local.Options.ProjectsDir. Empty means the SDK's default.
	ProjectsDir string
	// PixiPath is the pixi binary; see FindPixi. Required.
	PixiPath string
	// Port is the loopback port to listen on. Zero lets the OS pick one.
	Port int
	// Version is published in owner.json and served by GET /version.
	// NebiVersion is required.
	Version api.VersionInfo
	// Assets is the built client UI, usually frontend.ClientApp().
	// Required.
	Assets fs.FS
	// FolderPicker and Window are the desktop app's; nebi-web has neither.
	FolderPicker api.FolderPicker
	Window       api.Window
	// Logger receives progress and errors. It never receives the token.
	// Nil means slog.Default().
	Logger *slog.Logger
	// Status receives the messages a person waiting for Start should see,
	// such as the PID of the CLI command it waits for. Nil means os.Stderr.
	Status io.Writer
	// StartingWait and ShutdownWait override DefaultStartingWait and
	// DefaultShutdownWait.
	StartingWait time.Duration
	ShutdownWait time.Duration
}

// Instance is a running owner: it holds the lock, the database, the job
// runner and the listener until Shutdown.
type Instance struct {
	opts   Options
	logger *slog.Logger

	owner  *jobs.Owner
	info   jobs.OwnerInfo // as last published
	db     *gorm.DB
	runner *jobs.Memory
	svc    *local.Service

	listener  net.Listener
	server    *http.Server
	guard     *api.Guard
	inProcess http.Handler
	// cancelRequests cancels the context of every request the server is
	// handling, so Shutdown does not wait for requests that would end only
	// when their client goes away.
	cancelRequests context.CancelFunc

	serveDone chan struct{}
	serveErr  error

	shutdownOnce sync.Once
	shutdownErr  error
}

// newPixiRunner builds the PixiRunner. Tests replace it so they need no
// pixi binary.
var newPixiRunner = func(ctx context.Context, path string) (local.PixiRunner, error) {
	return exec.New(ctx, exec.Options{PixiPath: path})
}

// testHook, when a test sets it, is called after each startup step with
// the step's name. It is nil outside tests.
var testHook func(step string)

func step(name string) {
	if testHook != nil {
		testHook(name)
	}
}

// Start makes this process the owner of the client database in
// opts.DataDir and starts serving. See the package documentation for the
// steps. When another app owns the database and serves its API, Start
// returns a *RunningError; see HandleRunning. On any other error nothing is
// left held or open.
func Start(ctx context.Context, opts Options) (_ *Instance, err error) {
	switch {
	case !opts.Kind.ServesAPI():
		return nil, fmt.Errorf("boot: Options.Kind %q is not web or desktop", opts.Kind)
	case opts.DataDir == "":
		return nil, errors.New("boot: Options.DataDir is required")
	case opts.PixiPath == "":
		return nil, errors.New("boot: Options.PixiPath is required")
	case opts.Version.NebiVersion == "":
		return nil, errors.New("boot: Options.Version.NebiVersion is required")
	case opts.Assets == nil:
		return nil, errors.New("boot: Options.Assets is required")
	case opts.Port < 0 || opts.Port > 65535:
		return nil, fmt.Errorf("boot: port %d is out of range", opts.Port)
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Status == nil {
		opts.Status = os.Stderr
	}
	if opts.StartingWait <= 0 {
		opts.StartingWait = DefaultStartingWait
	}
	if opts.ShutdownWait <= 0 {
		opts.ShutdownWait = DefaultShutdownWait
	}

	// 2. The lock comes before the database: a process that does not get
	// it never opens the database.
	owner, err := acquire(ctx, opts)
	if err != nil {
		return nil, err
	}
	step("locked")
	inst := &Instance{opts: opts, logger: opts.Logger, owner: owner, serveDone: make(chan struct{})}
	defer func() {
		if err != nil {
			inst.release(context.Background())
		}
	}()

	// 3. Publish the kind at once, so a process that waits for the lock can
	// tell this app, still starting, from a CLI command.
	info := jobs.OwnerInfo{
		Kind:             opts.Kind,
		NebiVersion:      opts.Version.NebiVersion,
		ClientAPIVersion: api.ClientAPIVersion,
	}
	if err := owner.Publish(info); err != nil {
		return nil, fmt.Errorf("publishing owner info: %w", err)
	}
	inst.info = info
	step("published")

	// 4. The database sits next to the lock, in the directory the lock was
	// taken in, so every process that shares the database shares the lock.
	dbPath := filepath.Join(owner.Dir(), DatabaseFile)
	if inst.db, err = openDatabase(dbPath); err != nil {
		return nil, err
	}
	if err := local.Migrate(inst.db); err != nil {
		return nil, fmt.Errorf("migrating %s: %w", dbPath, err)
	}
	step("database")

	// 5. The runner, pixi and the SDK.
	inst.runner = jobs.NewMemory(jobs.Options{})
	pixi, err := newPixiRunner(ctx, opts.PixiPath)
	if err != nil {
		return nil, err
	}
	if inst.svc, err = local.New(local.Options{
		DB:          inst.db,
		Jobs:        inst.runner,
		Pixi:        pixi,
		ProjectsDir: opts.ProjectsDir,
	}); err != nil {
		return nil, err
	}

	// 6. Finish what an earlier owner left half done. Only the owner may do
	// this, and only once: it relies on knowing every active job.
	inst.recover(ctx)
	step("recovered")

	apiHandler, err := api.New(api.Options{
		Service:      inst.svc,
		Jobs:         inst.runner,
		Logger:       opts.Logger,
		Version:      opts.Version,
		FolderPicker: opts.FolderPicker,
		Window:       opts.Window,
	})
	if err != nil {
		return nil, err
	}
	uiInProcess, err := ui.New(ui.Options{Assets: opts.Assets, Logger: opts.Logger})
	if err != nil {
		return nil, err
	}
	inst.inProcess = mount(apiHandler, uiInProcess)

	// 7. Listen on loopback only, then publish the address and token.
	inst.listener, err = listen(opts.Port)
	if err != nil {
		return nil, err
	}
	session, err := api.NewSession()
	if err != nil {
		return nil, err
	}
	if inst.guard, err = api.NewGuard(session, inst.listener.Addr().String()); err != nil {
		return nil, err
	}
	uiNetwork, err := ui.New(ui.Options{Assets: opts.Assets, Guard: inst.guard, Logger: opts.Logger})
	if err != nil {
		return nil, err
	}
	info.Address = inst.listener.Addr().String()
	info.Token = session.Token()
	if err := owner.Publish(info); err != nil {
		return nil, fmt.Errorf("publishing owner info: %w", err)
	}
	inst.info = info
	step("listening")

	// 8. Serve.
	requestCtx, cancelRequests := context.WithCancel(context.Background())
	inst.cancelRequests = cancelRequests
	inst.server = &http.Server{
		Handler:           inst.guard.RequireHost(mount(inst.guard.RequireToken(apiHandler), uiNetwork)),
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(opts.Logger.Handler(), slog.LevelWarn),
	}
	go func() {
		defer close(inst.serveDone)
		if err := inst.server.Serve(inst.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			inst.serveErr = err
		}
	}()
	opts.Logger.Info("nebi client started", "kind", opts.Kind, "address", inst.Addr(),
		"data_dir", owner.Dir(), "pid", os.Getpid())
	return inst, nil
}

// recover runs the SDK's startup recovery. A failure is logged, not fatal:
// the records stay, and the next start tries again.
func (i *Instance) recover(ctx context.Context) {
	started, err := i.svc.RecoverDeletes(ctx)
	if err != nil {
		i.logger.Warn("finishing interrupted project deletes", "error", err)
	}
	for _, j := range started {
		i.logger.Info("removing the files of a project whose delete was interrupted", "job", j.ID, "project", j.ProjectID)
	}
	if err := i.svc.RecoverInPlaceSolves(ctx); err != nil {
		i.logger.Warn("restoring the files of interrupted version solves", "error", err)
	}
}

// mount serves the API under /api/ and the UI everywhere else.
func mount(apiHandler, uiHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", uiHandler)
	return mux
}

// ErrPortInUse is returned by Start when the port is taken.
var ErrPortInUse = errors.New("port is already in use")

// listen listens on 127.0.0.1. The client API never listens on any other
// address; owner.json would refuse it anyway.
func listen(port int) (net.Listener, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			return nil, fmt.Errorf("%w: another program is listening on %s", ErrPortInUse, addr)
		}
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	return ln, nil
}

// openDatabase opens the client database, creating the file if it is
// missing.
func openDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return db, nil
}

// Addr is the loopback address the client API listens on, as
// "127.0.0.1:port".
func (i *Instance) Addr() string { return i.listener.Addr().String() }

// SignInURL is the one-time sign-in URL for a browser. It holds the
// session token: print it for the user, never log it.
func (i *Instance) SignInURL() string { return i.guard.SignInURL() }

// InProcessHandler serves the client API and UI to the desktop app's own
// window, which reaches them through Wails in-process, with no socket in
// between. It checks no token.
func (i *Instance) InProcessHandler() http.Handler { return i.inProcess }

// Done is closed when the server stops serving on its own, such as when the
// listener fails; Err then says why. Shutdown also closes it.
func (i *Instance) Done() <-chan struct{} { return i.serveDone }

// Err is why the server stopped serving, or nil.
func (i *Instance) Err() error {
	select {
	case <-i.serveDone:
		return i.serveErr
	default:
		return nil
	}
}

// Shutdown stops the instance: it stops accepting requests, closes the job
// runner, which cancels running jobs and waits for them, closes the
// service and the database, and releases the lock, which removes
// owner.json. While it runs, owner.json names no address, so a process
// that starts meanwhile waits for the lock instead of pointing the user at
// an API that is going away.
//
// It waits at most Options.ShutdownWait, and less when ctx ends sooner.
// Requests still running are cancelled and drained while the jobs stop,
// so they do not use up the jobs' time. When jobs are still running at the
// end of the wait, Shutdown returns an error and leaves the service, the
// database and the lock to be released once those jobs end, so that no
// other process becomes the owner while they run; the OS releases the
// lock if the process exits first. It is safe to call more than once.
func (i *Instance) Shutdown(ctx context.Context) error {
	i.shutdownOnce.Do(func() {
		i.shutdownErr = i.release(ctx)
	})
	return i.shutdownErr
}

// release undoes whatever Start got to, in reverse order.
func (i *Instance) release(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, i.opts.ShutdownWait)
	defer cancel()
	var errs []error

	if i.info.Address != "" {
		stopping := i.info
		stopping.Address, stopping.Token = "", ""
		if err := i.owner.Publish(stopping); err != nil {
			errs = append(errs, fmt.Errorf("publishing owner info: %w", err))
		}
	}

	// Stop accepting requests first, then stop the jobs while the requests
	// in flight drain: both share the one wait instead of taking turns.
	requestsDone := i.stopServing(ctx)
	jobsDone := i.closeRunner()
	select {
	case <-jobsDone:
	case <-ctx.Done():
		errs = append(errs, fmt.Errorf("running jobs did not stop within %s; nebi releases the data directory when they do",
			i.opts.ShutdownWait))
		<-requestsDone
		// The owner rule: no other process may become the owner while this
		// one's jobs still run, so the lock is released only after them.
		go func() {
			<-jobsDone
			if err := i.closeStore(); err != nil {
				i.logger.Error("stopping nebi after its jobs ended", "error", err)
			}
		}()
		return errors.Join(errs...)
	}
	<-requestsDone
	if err := i.closeStore(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// stopServing closes the listener, cancels the requests in flight and
// drains them, cutting any still running when ctx ends. It returns once
// the listener is closed; the returned channel is closed once the requests
// are done.
func (i *Instance) stopServing(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	switch {
	case i.server != nil:
		i.cancelRequests()
		go func() {
			defer close(done)
			if err := i.server.Shutdown(ctx); err != nil {
				// Requests that ignore their context: cut them.
				_ = i.server.Close()
			}
		}()
		// Serve returns once Shutdown has closed the listener.
		<-i.serveDone
	case i.listener != nil:
		_ = i.listener.Close()
		close(i.serveDone)
		close(done)
	default:
		close(done)
	}
	return done
}

// closeRunner closes the job runner, which cancels the running jobs, and
// returns a channel that is closed once they have returned.
func (i *Instance) closeRunner() <-chan struct{} {
	done := make(chan struct{})
	if i.runner == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		i.runner.Close()
	}()
	return done
}

// closeStore closes the service and the database, then releases the lock,
// which removes owner.json. Call it only once the job runner is closed.
func (i *Instance) closeStore() error {
	var errs []error
	if i.svc != nil {
		i.svc.Close()
	}
	if i.db != nil {
		if sqlDB, err := i.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("closing the database: %w", err))
			}
		}
	}
	if err := i.owner.Release(); err != nil {
		errs = append(errs, fmt.Errorf("releasing the owner lock: %w", err))
	}
	return errors.Join(errs...)
}
