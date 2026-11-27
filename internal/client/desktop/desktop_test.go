package desktop

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The desktop app exposes no Go methods to TypeScript: its window talks
// only to the client API, in-process.
func TestNoWailsBindings(t *testing.T) {
	handler := http.NotFoundHandler()
	stopped := 0
	opts := appOptions(fstest.MapFS{}, handler, &window{}, func() { stopped++ })
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
// Wails runtime. Before Wails has started the window, and after it has
// returned, they answer an error rather than call into a Wails runtime that
// is not there, which would end the process.
func TestWindowBeforeAndAfterWails(t *testing.T) {
	w := &window{}
	fe, wailsCtx := installFakeWails(t)
	if _, err := w.PickFolder(context.Background()); !errors.Is(err, errNotReady) {
		t.Fatalf("PickFolder before start = %v", err)
	}
	if err := w.Show(context.Background()); !errors.Is(err, errNotReady) {
		t.Fatalf("Show before start = %v", err)
	}

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
	if _, err := w.PickFolder(context.Background()); !errors.Is(err, errNotReady) {
		t.Fatalf("PickFolder after Wails returned = %v", err)
	}
	if err := w.Show(context.Background()); !errors.Is(err, errNotReady) {
		t.Fatalf("Show after Wails returned = %v", err)
	}
	w.quit() // a no-op once Wails has returned
	if got := fe.recorded(); len(got) != 4 {
		t.Fatalf("the window called Wails after it returned: %v", got)
	}
}

// A signal can ask the app to quit before Wails has started the window;
// the window then quits as soon as it starts.
func TestQuitBeforeWindowStarts(t *testing.T) {
	fe, wailsCtx := installFakeWails(t)
	w := &window{}
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
