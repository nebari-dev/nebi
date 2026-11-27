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
// dialog and returned.
func Run(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	pixiPath, err := boot.FindPixi()
	if err != nil {
		alert(opts.Assets, "Nebi cannot start", err.Error())
		return err
	}
	win := &window{}
	// SIGINT and SIGTERM stop the app as closing the window does. Before
	// the window exists they interrupt Start, such as while it waits for a
	// CLI command to release the lock.
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	inst, err := boot.Start(ctx, boot.Options{
		Kind:         jobs.OwnerDesktop,
		DataDir:      opts.DataDir,
		ProjectsDir:  opts.ProjectsDir,
		PixiPath:     pixiPath,
		Port:         0, // the OS picks: this listener only serves the CLI
		Version:      opts.Version,
		Assets:       opts.Assets,
		FolderPicker: win,
		Window:       win,
		Logger:       opts.Logger,
	})
	var running *boot.RunningError
	switch {
	case errors.As(err, &running):
		return boot.HandleRunning(ctx, jobs.OwnerDesktop, running, func(r *boot.RunningError) {
			alert(opts.Assets, "Nebi is already running", fmt.Sprintf(
				"Nebi is already running for %s in %s (process %d). Open it in your browser:\n\n%s",
				r.DataDir, boot.KindName(r.Owner.Kind), r.Owner.PID, r.SignInURL()))
		})
	case err != nil:
		alert(opts.Assets, "Nebi cannot start", err.Error())
		return err
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

	return wails.Run(appOptions(opts.Assets, inst.InProcessHandler(), win, stop))
}

// appOptions configures the Wails app. It binds no Go methods: the window
// reaches the client API and UI through handler, in-process.
func appOptions(assets fs.FS, handler http.Handler, win *window, stop func()) *options.App {
	return &options.App{
		Title:  "Nebi - Environment Manager",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: handler,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        win.setContext,
		// Closing the window stops the owner: running jobs are cancelled
		// and waited for, and the lock is released, before the app exits.
		OnShutdown: func(context.Context) { stop() },
	}
}

// errNotReady is returned while the window has not started yet.
var errNotReady = errors.New("the desktop window is not ready yet")

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
type window struct {
	mu  sync.Mutex
	ctx context.Context
	// quitAsked is set when quit is called before the window started.
	quitAsked bool
	// done is set once Wails has returned; the context is dead then.
	done bool
}

func (w *window) setContext(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctx = ctx
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
	case w.done:
	case w.ctx != nil:
		wailsRuntime.quit(w.ctx)
	default:
		w.quitAsked = true
	}
}

func (w *window) closed() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.done = true
}

func (w *window) wailsContext() (context.Context, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx == nil || w.done {
		return nil, errNotReady
	}
	return w.ctx, nil
}

// PickFolder shows the native folder dialog.
func (w *window) PickFolder(context.Context) (string, error) {
	ctx, err := w.wailsContext()
	if err != nil {
		return "", err
	}
	return wailsRuntime.openDirectory(ctx, runtime.OpenDialogOptions{
		Title:                "Choose a folder",
		CanCreateDirectories: true,
	})
}

// Show brings the window to the front.
func (w *window) Show(context.Context) error {
	ctx, err := w.wailsContext()
	if err != nil {
		return err
	}
	wailsRuntime.unminimise(ctx)
	wailsRuntime.show(ctx)
	return nil
}

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
