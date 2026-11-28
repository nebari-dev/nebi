// Package desktop is the composition root of the desktop app: it makes the
// process the owner of the client database, serves the client API and the
// client UI on loopback for the CLI, and opens the Wails window, which
// reaches the same API and UI in-process (issue #680). The startup and
// shutdown are internal/client/boot's, shared with nebi-web.
//
// The window exposes no Go methods to TypeScript: the UI talks only to the
// client API. The native folder dialog reaches the UI through
// POST /api/v1/desktop/pick-folder instead of a Wails binding.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/boot"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Options configures Run.
type Options struct {
	// DataDir and ProjectsDir come from boot.DataDir and boot.ProjectsDir.
	DataDir     string
	ProjectsDir string
	// Version is the build's version.
	Version api.VersionInfo
	// Assets is the built client UI, frontend.ClientApp().
	Assets fs.FS
	// Logger receives progress and errors. Nil means slog.Default().
	Logger *slog.Logger
}

// Run starts the desktop app and returns when its window is closed. When
// another app already owns the database it does not open a window: it
// raises the running desktop app's window, or shows a dialog with the URL
// of the running nebi-web, and returns. A startup error is shown in a
// dialog and returned. A start that a signal stopped returns nil.
func Run(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	win := newWindow()
	// SIGINT and SIGTERM stop the app as closing the window does. Before
	// the window exists they interrupt Start, such as while it waits for a
	// CLI command to release the lock.
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	inst, err := boot.Start(ctx, boot.Options{
		Kind:         jobs.OwnerDesktop,
		DataDir:      opts.DataDir,
		ProjectsDir:  opts.ProjectsDir,
		Port:         0, // the OS picks: this listener only serves the CLI
		Version:      opts.Version,
		Assets:       opts.Assets,
		FolderPicker: win,
		Window:       win,
		Logger:       opts.Logger,
	})
	if err != nil {
		return startFailed(ctx, opts.Assets, err, stopSignals)
	}

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			if err := inst.Shutdown(context.Background()); err != nil {
				opts.Logger.Error("stopping nebi", "error", err)
			}
		})
	}
	defer stop()

	runDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			win.quit()
		case <-runDone:
		}
	}()
	defer close(runDone)
	defer win.closed()

	return wails.Run(appOptions(inst.InProcessHandler(), win, stop))
}

// startFailed handles an error from boot.Start: it raises the running
// desktop app's window, or tells the user why this one does not start, and
// returns what Run returns. A start that a signal stopped, such as while it
// waited for a CLI command, shows nothing and returns nil: the user asked
// it to stop.
// Before a dialog, which waits for the user, it stops catching signals, so
// that a signal still ends the process.
func startFailed(ctx context.Context, assets fs.FS, err error, stopSignals func()) error {
	var running *boot.RunningError
	if errors.As(err, &running) {
		return boot.HandleRunning(ctx, jobs.OwnerDesktop, running, func(r *boot.RunningError) {
			stopSignals()
			showAlert(assets, "Nebi is already running", fmt.Sprintf(
				"Nebi is already running for %s in %s (process %d). Open it in your browser:\n\n%s",
				r.DataDir, boot.KindName(r.Owner.Kind), r.Owner.PID, r.SignInURL()))
		})
	}
	if ctx.Err() != nil {
		return nil
	}
	stopSignals()
	showAlert(assets, "Nebi cannot start", err.Error())
	return err
}

// appOptions configures the Wails app. It binds no Go methods: the window
// reaches the client API and UI through handler, in-process.
//
// handler serves every request, the UI's files included. Given Assets too,
// Wails would serve the files it finds there itself, index.html among them,
// and the window would get none of the UI's headers, such as its
// Content-Security-Policy. Wails still adds its runtime scripts to
// index.html, keeping handler's headers.
func appOptions(handler http.Handler, win *window, stop func()) *options.App {
	return &options.App{
		Title:  "Nebi - Environment Manager",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Handler: handler,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        win.setContext,
		// Closing the window stops the owner: running jobs are cancelled
		// and waited for, and the lock is released, before the app exits.
		OnShutdown: func(context.Context) { stop() },
	}
}

// errNotReady is returned by the window once it has closed, or when a
// request ends before the window has opened.
var errNotReady error = &api.Error{
	Status:  http.StatusServiceUnavailable,
	Code:    api.CodeWindowNotReady,
	Message: "the desktop window is not open",
}

// wailsRuntime holds the Wails runtime calls the window makes. They need
// the context Wails passes to OnStartup, and end the process when called
// without it. Tests replace them.
var wailsRuntime = struct {
	quit          func(ctx context.Context)
	unminimise    func(ctx context.Context)
	show          func(ctx context.Context)
	openDirectory func(ctx context.Context, opts runtime.OpenDialogOptions) (string, error)
}{runtime.Quit, runtime.WindowUnminimise, runtime.WindowShow, runtime.OpenDirectoryDialog}

// window is the Wails window. It implements api.FolderPicker and
// api.Window; both need the context Wails passes to OnStartup.
//
// boot.Start publishes the address before Wails opens the window, so a
// second desktop app can ask to raise the window before it exists. The
// window then waits until it has opened.
type window struct {
	// started is closed when Wails has opened the window (setContext),
	// ended when Wails has returned (closed).
	started chan struct{}
	ended   chan struct{}

	mu  sync.Mutex
	ctx context.Context
	// quitAsked is set when quit is called before the window started.
	quitAsked bool
}

func newWindow() *window {
	return &window{started: make(chan struct{}), ended: make(chan struct{})}
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (w *window) setContext(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if isClosed(w.started) {
		return
	}
	w.ctx = ctx
	close(w.started)
	if w.quitAsked {
		go wailsRuntime.quit(ctx)
	}
}

// quit closes the window, which runs OnShutdown, or closes it as soon as
// it has started.
func (w *window) quit() {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case isClosed(w.ended):
	case w.ctx != nil:
		wailsRuntime.quit(w.ctx)
	default:
		w.quitAsked = true
	}
}

func (w *window) closed() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !isClosed(w.ended) {
		close(w.ended)
	}
}

// wailsContext returns the context Wails passed to OnStartup. It waits for
// the window to open, until ctx ends, and fails once the window has closed.
func (w *window) wailsContext(ctx context.Context) (context.Context, error) {
	select {
	case <-w.started:
	case <-w.ended:
	case <-ctx.Done():
		return nil, errNotReady
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if isClosed(w.ended) {
		return nil, errNotReady
	}
	return w.ctx, nil
}

// PickFolder shows the native folder dialog.
func (w *window) PickFolder(ctx context.Context) (string, error) {
	ctx, err := w.wailsContext(ctx)
	if err != nil {
		return "", err
	}
	return wailsRuntime.openDirectory(ctx, runtime.OpenDialogOptions{
		Title:                "Choose a folder",
		CanCreateDirectories: true,
	})
}

// Show brings the window to the front.
func (w *window) Show(ctx context.Context) error {
	ctx, err := w.wailsContext(ctx)
	if err != nil {
		return err
	}
	wailsRuntime.unminimise(ctx)
	wailsRuntime.show(ctx)
	return nil
}

// showAlert is alert; tests replace it.
var showAlert = alert

// alert shows a message in a native dialog when the app has no window of
// its own, such as when another app owns the database, and returns when
// the user dismisses it. It runs a hidden Wails app for the dialog; Wails
// needs assets for its window, which is never shown.
func alert(assets fs.FS, title, message string) {
	err := wails.Run(&options.App{
		Title:       title,
		Width:       400,
		Height:      200,
		StartHidden: true,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			go func() {
				// Give the app a moment to finish starting before the
				// dialog, which is modal, takes over the main thread.
				time.Sleep(100 * time.Millisecond)
				_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
					Type:    runtime.InfoDialog,
					Title:   title,
					Message: message,
				})
				runtime.Quit(ctx)
			}()
		},
	})
	if err != nil {
		slog.Error("showing a dialog", "title", title, "error", err)
	}
}
