package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/boot"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The desktop app exposes no Go methods to TypeScript: its window talks
// only to the client API, in-process.
func TestNoWailsBindings(t *testing.T) {
	handler := http.NotFoundHandler()
	stopped := 0
	opts := appOptions(fstest.MapFS{}, handler, newWindow(), func() { stopped++ })
	if len(opts.Bind) != 0 || len(opts.EnumBind) != 0 {
		t.Fatalf("the desktop app binds %v and %v", opts.Bind, opts.EnumBind)
	}
	if opts.AssetServer == nil || opts.AssetServer.Handler == nil {
		t.Fatal("the window does not reach the in-process handler")
	}
	if opts.SingleInstanceLock != nil {
		t.Fatal("Wails' single-instance lock is set; the owner lock decides")
	}
	opts.OnShutdown(context.Background())
	if stopped != 1 {
		t.Fatalf("closing the window stopped the owner %d times, want 1", stopped)
	}
}

// wailsKey marks the context a fake Wails passes to OnStartup.
type wailsKey struct{}

// fakeWails replaces the Wails runtime calls of the window. It records them,
// and records "no window" for a call without the context Wails passed to
// OnStartup, where the real runtime would end the process.
type fakeWails struct {
	mu    sync.Mutex
	calls []string
	quit  chan struct{}
}

func installFakeWails(t *testing.T) (*fakeWails, context.Context) {
	t.Helper()
	f := &fakeWails{quit: make(chan struct{}, 1)}
	orig := wailsRuntime
	t.Cleanup(func() { wailsRuntime = orig })
	wailsRuntime.quit = func(ctx context.Context) {
		f.record(ctx, "quit")
		select {
		case f.quit <- struct{}{}:
		default:
		}
	}
	wailsRuntime.unminimise = func(ctx context.Context) { f.record(ctx, "unminimise") }
	wailsRuntime.show = func(ctx context.Context) { f.record(ctx, "show") }
	wailsRuntime.openDirectory = func(ctx context.Context, _ runtime.OpenDialogOptions) (string, error) {
		f.record(ctx, "folder dialog")
		return "/home/me/picked", nil
	}
	return f, context.WithValue(context.Background(), wailsKey{}, true)
}

func (f *fakeWails) record(ctx context.Context, call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Value(wailsKey{}) == nil {
		call = "no window: " + call
	}
	f.calls = append(f.calls, call)
}

func (f *fakeWails) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// While Wails runs the window, the folder picker and the window call the
// Wails runtime. After Wails has returned they answer window_not_ready
// rather than call into a Wails runtime that is not there, which would end
// the process.
func TestWindowBeforeAndAfterWails(t *testing.T) {
	w := newWindow()
	fe, wailsCtx := installFakeWails(t)

	w.setContext(wailsCtx)
	if err := w.Show(context.Background()); err != nil {
		t.Fatalf("Show = %v", err)
	}
	if path, err := w.PickFolder(context.Background()); err != nil || path != "/home/me/picked" {
		t.Fatalf("PickFolder = %q, %v", path, err)
	}
	w.quit()
	if got, want := fe.recorded(), []string{"unminimise", "show", "folder dialog", "quit"}; !slices.Equal(got, want) {
		t.Fatalf("Wails calls %v, want %v", got, want)
	}

	w.closed()
	requireNotReady(t, "PickFolder after Wails returned", func() error { _, err := w.PickFolder(context.Background()); return err })
	requireNotReady(t, "Show after Wails returned", func() error { return w.Show(context.Background()) })
	w.quit() // a no-op once Wails has returned
	if got := fe.recorded(); len(got) != 4 {
		t.Fatalf("the window called Wails after it returned: %v", got)
	}
}

// requireNotReady fails t unless call answers window_not_ready with 503.
func requireNotReady(t *testing.T, what string, call func() error) {
	t.Helper()
	err := call()
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable || apiErr.Code != api.CodeWindowNotReady {
		t.Fatalf("%s = %v, want 503 %s", what, err, api.CodeWindowNotReady)
	}
}

// boot.Start publishes the address before Wails opens the window, so a
// second desktop app can ask to raise it before it exists: Show waits for
// the window instead of failing, and gives up when its request ends or the
// window closes without opening.
func TestShowWaitsForWindow(t *testing.T) {
	fe, wailsCtx := installFakeWails(t)
	w := newWindow()

	shown := make(chan error, 1)
	go func() { shown <- w.Show(context.Background()) }()
	select {
	case err := <-shown:
		t.Fatalf("Show returned %v before the window opened", err)
	case <-time.After(100 * time.Millisecond):
	}
	w.setContext(wailsCtx)
	select {
	case err := <-shown:
		if err != nil {
			t.Fatalf("Show once the window opened = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Show did not return once the window opened")
	}
	if got, want := fe.recorded(), []string{"unminimise", "show"}; !slices.Equal(got, want) {
		t.Fatalf("Wails calls %v, want %v", got, want)
	}

	// A request that ends first.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	requireNotReady(t, "Show when the request ends first", func() error { return newWindow().Show(ctx) })
	requireNotReady(t, "PickFolder when the request ends first", func() error { _, err := newWindow().PickFolder(ctx); return err })

	// A window that closes without opening, as when Wails fails to start.
	never := newWindow()
	waiting := make(chan error, 1)
	go func() { waiting <- never.Show(context.Background()) }()
	never.closed()
	select {
	case err := <-waiting:
		requireNotReady(t, "Show when the window closes without opening", func() error { return err })
	case <-time.After(10 * time.Second):
		t.Fatal("Show did not return when the window closed")
	}
}

// A signal can ask the app to quit before Wails has started the window;
// the window then quits as soon as it starts.
func TestQuitBeforeWindowStarts(t *testing.T) {
	fe, wailsCtx := installFakeWails(t)
	w := newWindow()
	w.quit()
	if got := fe.recorded(); len(got) != 0 {
		t.Fatalf("quit before the window started called Wails: %v", got)
	}
	w.setContext(wailsCtx)
	select {
	case <-fe.quit:
	case <-time.After(10 * time.Second):
		t.Fatal("a quit asked before the window started was lost")
	}
	if got := fe.recorded(); !slices.Equal(got, []string{"quit"}) {
		t.Fatalf("Wails calls %v, want [quit]", got)
	}
}

// fakeAlerts replaces the dialogs and records them.
func fakeAlerts(t *testing.T) *[]string {
	t.Helper()
	var shown []string
	orig := showAlert
	t.Cleanup(func() { showAlert = orig })
	showAlert = func(_ fs.FS, title, _ string) { shown = append(shown, title) }
	return &shown
}

// A start that fails shows why, but only after it stops catching signals,
// so that a signal still ends the process while the dialog waits.
func TestStartFailedShowsDialog(t *testing.T) {
	shown := fakeAlerts(t)
	stopped := false
	orig := showAlert
	showAlert = func(assets fs.FS, title, message string) {
		if !stopped {
			t.Errorf("dialog %q shown while signals are still caught", title)
		}
		orig(assets, title, message)
	}
	startErr := errors.New("pixi is too old")
	if err := startFailed(context.Background(), fstest.MapFS{}, startErr, func() { stopped = true }); !errors.Is(err, startErr) {
		t.Fatalf("startFailed = %v, want the start error", err)
	}
	if !slices.Equal(*shown, []string{"Nebi cannot start"}) {
		t.Fatalf("dialogs %v, want [Nebi cannot start]", *shown)
	}
}

// A start that a signal stopped, such as while it waited for a CLI command
// to release the lock, shows no dialog: the user asked it to stop.
func TestStartStoppedBySignalShowsNothing(t *testing.T) {
	shown := fakeAlerts(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startErr := fmt.Errorf("waiting for the owner lock on /data: %w", ctx.Err())
	if err := startFailed(ctx, fstest.MapFS{}, startErr, func() {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("startFailed = %v, want the start error", err)
	}
	if len(*shown) != 0 {
		t.Fatalf("dialogs %v after a signal, want none", *shown)
	}
}

// With nebi-web running, the desktop app shows its URL, after it stops
// catching signals.
func TestStartFailedShowsRunningWebURL(t *testing.T) {
	shown := fakeAlerts(t)
	stopped := false
	orig := showAlert
	showAlert = func(assets fs.FS, title, message string) {
		if !stopped {
			t.Errorf("dialog %q shown while signals are still caught", title)
		}
		if !strings.Contains(message, "http://127.0.0.1:8460/?token=tok") {
			t.Errorf("dialog %q lacks the running nebi-web's URL: %s", title, message)
		}
		orig(assets, title, message)
	}
	running := &boot.RunningError{
		Owner:   jobs.OwnerInfo{PID: 42, Kind: jobs.OwnerWeb, Address: "127.0.0.1:8460", Token: "tok"},
		DataDir: "/data",
	}
	if err := startFailed(context.Background(), fstest.MapFS{}, running, func() { stopped = true }); err != nil {
		t.Fatalf("startFailed with nebi-web running = %v", err)
	}
	if !slices.Equal(*shown, []string{"Nebi is already running"}) {
		t.Fatalf("dialogs %v, want [Nebi is already running]", *shown)
	}
}
