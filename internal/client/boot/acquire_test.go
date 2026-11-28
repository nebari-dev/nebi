package boot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// holdLock takes the owner lock on dir in this process, as another owner,
// and writes content as its owner.json unless content is empty.
func holdLock(t *testing.T, dir, content string) *jobs.Owner {
	t.Helper()
	o, err := jobs.TryAcquireOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Release() })
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, jobs.OwnerInfoFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return o
}

// lockedWriter is a Status writer a test can read while Start writes.
type lockedWriter struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// What Start does with each owner.json another holder of the lock can
// leave, other than the running app and the CLI command the second-instance
// tests cover.
func TestStartWithOwnerInfoOfHolder(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		check   func(t *testing.T, err error)
	}{
		// The holder took the lock and has not published: Start waits, as
		// for an app that is starting, and gives up after StartingWait.
		"none yet": {"", func(t *testing.T, err error) {
			if !errors.Is(err, ErrOwnerNotServing) {
				t.Fatalf("Start = %v, want ErrOwnerNotServing", err)
			}
		}},
		// A newer nebi's owner.json: Start names its PID and versions.
		"invalid": {`{"pid":4242,"kind":"web","hostname":"h","nebi_version":"9.9.9","client_api_version":"2.0",` +
			`"address":"unix:/run/nebi.sock","token":"t"}`, func(t *testing.T, err error) {
			if !errors.Is(err, jobs.ErrInvalidOwnerInfo) {
				t.Fatalf("Start = %v, want ErrInvalidOwnerInfo", err)
			}
			for _, want := range []string{"4242", "9.9.9", "2.0", "stop that process"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
		}},
		"corrupt": {"{", func(t *testing.T, err error) {
			if err == nil || !strings.Contains(err.Error(), "decoding owner info") {
				t.Fatalf("Start = %v, want a decode error", err)
			}
		}},
		// A data directory shared with another machine, as over NFS.
		"other host": {`{"pid":4242,"kind":"web","hostname":"elsewhere.example","machine_id":"another-machine",` +
			`"nebi_version":"1.2.3","client_api_version":"` + api.ClientAPIVersion + `","address":"127.0.0.1:1","token":"t"}`,
			func(t *testing.T, err error) {
				var running *RunningError
				if !errors.Is(err, jobs.ErrOwnerOnOtherHost) || errors.As(err, &running) ||
					!strings.Contains(err.Error(), "elsewhere.example") {
					t.Fatalf("Start = %v, want ErrOwnerOnOtherHost naming the host", err)
				}
			}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			holdLock(t, dir, tc.content)
			opts := testOptions(t, jobs.OwnerWeb, dir)
			opts.StartingWait = 300 * time.Millisecond
			inst, err := Start(context.Background(), opts)
			if err == nil {
				_ = inst.Shutdown(context.Background())
			}
			tc.check(t, err)
			if _, statErr := os.Stat(filepath.Join(dir, DatabaseFile)); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("Start opened the database without the lock: %v", statErr)
			}
		})
	}
}

// Cancelling Start while it waits for the lock ends the wait with the
// context's error.
func TestStartCancelledWhileWaiting(t *testing.T) {
	dir := t.TempDir()
	o := holdLock(t, dir, "")
	if err := o.Publish(jobs.OwnerInfo{Kind: jobs.OwnerCLI, NebiVersion: "1.2.3", ClientAPIVersion: api.ClientAPIVersion}); err != nil {
		t.Fatal(err)
	}
	status := &lockedWriter{}
	opts := testOptions(t, jobs.OwnerWeb, dir)
	opts.Status = status
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Start(ctx, opts)
		done <- err
	}()
	deadline := time.Now().Add(helperTimeout)
	for !strings.Contains(status.String(), "Waiting for process") {
		if time.Now().After(deadline) {
			t.Fatal("Start did not wait for the CLI command")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start after cancel = %v, want context.Canceled", err)
		}
	case <-time.After(helperTimeout):
		t.Fatal("Start kept waiting after its context was cancelled")
	}
}

// Pixi is looked up only once Start holds the lock: a second instance that
// has no pixi still reaches the running one, and a first one fails without
// keeping anything.
func TestStartFindsPixiAfterLock(t *testing.T) {
	orig := findPixi
	t.Cleanup(func() { findPixi = orig })
	lookups := 0
	findPixi = func() (string, error) {
		lookups++
		return "", ErrPixiNotFound
	}

	dir := t.TempDir()
	owner, url := startOwner(t, jobs.OwnerWeb, dir)
	opts := testOptions(t, jobs.OwnerDesktop, dir)
	opts.PixiPath = ""
	_, err := Start(context.Background(), opts)
	var running *RunningError
	if !errors.As(err, &running) || running.SignInURL() != url {
		t.Fatalf("Start without pixi while nebi-web runs = %v, want a RunningError for %s", err, url)
	}
	if lookups != 0 {
		t.Fatalf("Start looked for pixi %d times before it held the lock", lookups)
	}
	owner.stop("stopped")

	opts = testOptions(t, jobs.OwnerWeb, dir)
	opts.PixiPath = ""
	opts.Status = io.Discard
	if _, err := Start(context.Background(), opts); !errors.Is(err, ErrPixiNotFound) {
		t.Fatalf("Start without pixi = %v, want ErrPixiNotFound", err)
	}
	if lookups != 1 {
		t.Fatalf("Start looked for pixi %d times, want once", lookups)
	}
	requireLockFree(t, dir)

	findPixi = func() (string, error) { return "found-pixi", nil }
	start(t, opts)
}
