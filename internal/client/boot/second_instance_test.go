package boot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// The second-instance tests run this test binary again as the other
// process: TestMain sees helperEnv and runs runHelper instead of the tests.
const (
	helperEnv         = "NEBI_TEST_BOOT_HELPER"
	helperDirEnv      = "NEBI_TEST_BOOT_DIR"
	helperKindEnv     = "NEBI_TEST_BOOT_KIND"
	helperStartingEnv = "NEBI_TEST_BOOT_STARTING_WAIT"

	// helperTimeout bounds every wait on a helper. It only trips when a
	// step is really stuck.
	helperTimeout = 30 * time.Second
)

// helperWindow is the desktop window of a helper that owns the database.
type helperWindow struct{}

func (helperWindow) Show(context.Context) error {
	fmt.Println("window shown")
	return nil
}

// runHelper plays the other process. Modes:
//   - app: Start as the kind in helperKindEnv, the way the roots do. While
//     serving it prints "serving <addr>" and stops on stdin EOF, printing
//     "stopped". When another app runs, it follows HandleRunning, where
//     notify prints "notify <holder kind> <url>", then prints "exit
//     running". Waiting messages go to stdout.
//   - cli: hold the lock as a CLI command until stdin EOF.
//   - starting: hold the lock as an app of helperKindEnv that has not
//     published its address; "publish" on stdin publishes one.
func runHelper(mode string) int {
	dir := os.Getenv(helperDirEnv)
	kind := jobs.OwnerKind(os.Getenv(helperKindEnv))
	out := func(format string, args ...any) { fmt.Printf(format+"\n", args...) }
	fail := func(err error) int {
		out("error: %v", err)
		return 2
	}
	waitStdin := func(onLine func(string) error) error {
		in := bufio.NewScanner(os.Stdin)
		for in.Scan() {
			if err := onLine(in.Text()); err != nil {
				return err
			}
		}
		return nil
	}

	switch mode {
	case "app":
		opts := Options{
			Kind:     kind,
			DataDir:  dir,
			PixiPath: "pixi",
			Version:  api.VersionInfo{NebiVersion: "1.2.3"},
			Assets:   testAssets,
			Logger:   quietLogger(),
			Status:   os.Stdout,
		}
		if kind == jobs.OwnerDesktop {
			opts.Window = helperWindow{}
		}
		if d, err := time.ParseDuration(os.Getenv(helperStartingEnv)); err == nil {
			opts.StartingWait = d
		}
		ctx := context.Background()
		inst, err := Start(ctx, opts)
		var running *RunningError
		if errors.As(err, &running) {
			if err := HandleRunning(ctx, kind, running, func(r *RunningError) {
				out("notify %s %s", r.Owner.Kind, r.SignInURL())
			}); err != nil {
				return fail(err)
			}
			out("exit running")
			return 0
		}
		if err != nil {
			return fail(err)
		}
		out("serving %s", inst.Addr())
		_ = waitStdin(func(string) error { return nil })
		if err := inst.Shutdown(context.Background()); err != nil {
			return fail(err)
		}
		out("stopped")
		return 0

	case "cli", "starting":
		o, err := jobs.AcquireOwner(context.Background(), dir)
		if err != nil {
			return fail(err)
		}
		info := jobs.OwnerInfo{Kind: jobs.OwnerCLI, NebiVersion: "1.2.3", ClientAPIVersion: api.ClientAPIVersion}
		if mode == "starting" {
			info.Kind = kind
		}
		if err := o.Publish(info); err != nil {
			return fail(err)
		}
		out("locked")
		if err := waitStdin(func(line string) error {
			if line != "publish" {
				return fmt.Errorf("unknown command %q", line)
			}
			info.Address, info.Token = "127.0.0.1:1", "starting-token"
			if err := o.Publish(info); err != nil {
				return err
			}
			out("published")
			return nil
		}); err != nil {
			return fail(err)
		}
		if err := o.Release(); err != nil {
			return fail(err)
		}
		out("released")
		return 0
	}
	return fail(fmt.Errorf("unknown helper mode %q", mode))
}

type helper struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	done  chan struct{}
}

func startHelper(t *testing.T, mode string, kind jobs.OwnerKind, dir string, env ...string) *helper {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, helperDirEnv+"="+dir, helperKindEnv+"="+string(kind))
	cmd.Env = append(cmd.Env, env...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &helper{t: t, cmd: cmd, stdin: stdin, lines: make(chan string, 64), done: make(chan struct{})}
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			h.lines <- s.Text()
		}
		close(h.lines)
		_ = cmd.Wait()
		close(h.done)
	}()
	t.Cleanup(func() {
		_ = h.stdin.Close()
		// Read what the test left unread, so the helper never blocks
		// writing to stdout and cleanup cannot hang.
		go func() {
			for range h.lines {
			}
		}()
		select {
		case <-h.done:
		case <-time.After(helperTimeout):
			_ = h.cmd.Process.Kill()
			select {
			case <-h.done:
			case <-time.After(helperTimeout):
				t.Errorf("helper %d did not exit after it was killed", h.cmd.Process.Pid)
			}
		}
	})
	return h
}

func (h *helper) next() string {
	h.t.Helper()
	select {
	case line, ok := <-h.lines:
		if !ok {
			h.t.Fatalf("helper %d exited before printing a line", h.cmd.Process.Pid)
		}
		if strings.HasPrefix(line, "error:") {
			h.t.Fatalf("helper %d: %s", h.cmd.Process.Pid, line)
		}
		return line
	case <-time.After(helperTimeout):
		h.t.Fatalf("helper %d printed nothing for %v", h.cmd.Process.Pid, helperTimeout)
		return ""
	}
}

func (h *helper) expectPrefix(prefix string) string {
	h.t.Helper()
	line := h.next()
	if !strings.HasPrefix(line, prefix) {
		h.t.Fatalf("helper %d printed %q, want %q...", h.cmd.Process.Pid, line, prefix)
	}
	return line
}

// stop closes the helper's stdin, which makes an owner shut down or a lock
// holder release, and expects its last line.
func (h *helper) stop(last string) {
	h.t.Helper()
	_ = h.stdin.Close()
	h.expectPrefix(last)
	h.exited(0)
}

func (h *helper) exited(code int) {
	h.t.Helper()
	select {
	case <-h.done:
	case <-time.After(helperTimeout):
		h.t.Fatalf("helper %d did not exit", h.cmd.Process.Pid)
	}
	if got := h.cmd.ProcessState.ExitCode(); got != code {
		h.t.Fatalf("helper %d exited with %d, want %d", h.cmd.Process.Pid, got, code)
	}
}

// startOwner starts a helper app that becomes the owner and returns it with
// its sign-in URL from owner.json.
func startOwner(t *testing.T, kind jobs.OwnerKind, dir string) (*helper, string) {
	t.Helper()
	h := startHelper(t, "app", kind, dir)
	h.expectPrefix("serving ")
	info, err := jobs.ReadOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != kind || info.PID != h.cmd.Process.Pid {
		t.Fatalf("owner.json = %v, want the %s helper %d", info, kind, h.cmd.Process.Pid)
	}
	return h, "http://" + info.Address + "/?token=" + info.Token
}

// Each row of the second-instance table, against a real second process.

func TestSecondDesktopRaisesRunningDesktopWindow(t *testing.T) {
	dir := t.TempDir()
	owner, _ := startOwner(t, jobs.OwnerDesktop, dir)

	second := startHelper(t, "app", jobs.OwnerDesktop, dir)
	second.expectPrefix("exit running")
	second.exited(0)
	owner.expectPrefix("window shown")

	owner.stop("stopped")
	requireLockFree(t, dir)
}

func TestDesktopShowsRunningWebURL(t *testing.T) {
	dir := t.TempDir()
	owner, url := startOwner(t, jobs.OwnerWeb, dir)

	second := startHelper(t, "app", jobs.OwnerDesktop, dir)
	if line := second.expectPrefix("notify "); line != "notify web "+url {
		t.Fatalf("desktop notified %q, want the running nebi-web's URL %s", line, url)
	}
	second.expectPrefix("exit running")
	second.exited(0)
	owner.stop("stopped")
}

func TestWebPrintsRunningURL(t *testing.T) {
	for _, holder := range []jobs.OwnerKind{jobs.OwnerDesktop, jobs.OwnerWeb} {
		t.Run(string(holder), func(t *testing.T) {
			dir := t.TempDir()
			owner, url := startOwner(t, holder, dir)

			second := startHelper(t, "app", jobs.OwnerWeb, dir)
			if line := second.expectPrefix("notify "); line != "notify "+string(holder)+" "+url {
				t.Fatalf("nebi-web printed %q, want the running %s's URL %s", line, holder, url)
			}
			second.expectPrefix("exit running")
			second.exited(0)
			owner.stop("stopped")
		})
	}
}

func TestWaitsForCLICommand(t *testing.T) {
	for _, kind := range []jobs.OwnerKind{jobs.OwnerDesktop, jobs.OwnerWeb} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			cli := startHelper(t, "cli", "", dir)
			cli.expectPrefix("locked")

			// A CLI command can hold the lock for long, as while it
			// installs a project: the app waits for it, however long, and
			// does not give up after StartingWait as it does on an app that
			// never serves.
			app := startHelper(t, "app", kind, dir, helperStartingEnv+"=200ms")
			want := "Waiting for process " + strconv.Itoa(cli.cmd.Process.Pid) + " (a nebi CLI command) to finish with "
			app.expectPrefix(want)
			select {
			case line := <-app.lines:
				t.Fatalf("app printed %q while the CLI command holds the lock", line)
			case <-time.After(time.Second):
			}

			cli.stop("released")
			app.expectPrefix("serving ")
			app.stop("stopped")
		})
	}
}

func TestWaitsForAppThatIsStarting(t *testing.T) {
	dir := t.TempDir()
	holder := startHelper(t, "starting", jobs.OwnerDesktop, dir)
	holder.expectPrefix("locked")

	app := startHelper(t, "app", jobs.OwnerWeb, dir)
	app.expectPrefix("Waiting for process " + strconv.Itoa(holder.cmd.Process.Pid) + " (the desktop app), which is starting or stopping")

	_, _ = io.WriteString(holder.stdin, "publish\n")
	holder.expectPrefix("published")
	if line := app.expectPrefix("notify "); line != "notify desktop http://127.0.0.1:1/?token=starting-token" {
		t.Fatalf("nebi-web printed %q once the desktop app published its address", line)
	}
	app.expectPrefix("exit running")
	app.exited(0)
	holder.stop("released")
}

func TestGivesUpOnAppThatNeverServes(t *testing.T) {
	dir := t.TempDir()
	holder := startHelper(t, "starting", jobs.OwnerWeb, dir)
	holder.expectPrefix("locked")

	app := startHelper(t, "app", jobs.OwnerDesktop, dir, helperStartingEnv+"=500ms")
	app.expectPrefix("Waiting for process")
	select {
	case line := <-app.lines:
		if !strings.Contains(line, ErrOwnerNotServing.Error()) || !strings.Contains(line, strconv.Itoa(holder.cmd.Process.Pid)) {
			t.Fatalf("app printed %q, want it to give up naming process %d", line, holder.cmd.Process.Pid)
		}
	case <-time.After(helperTimeout):
		t.Fatal("app did not give up")
	}
	app.exited(2)
	holder.stop("released")
}

// One owner per database, not per application: an app for another data
// directory starts while one runs.
func TestOneOwnerPerDatabase(t *testing.T) {
	owner, _ := startOwner(t, jobs.OwnerDesktop, filepath.Join(t.TempDir(), "a"))
	other, _ := startOwner(t, jobs.OwnerDesktop, filepath.Join(t.TempDir(), "b"))
	other.stop("stopped")
	owner.stop("stopped")
}

// RunningError carries the running app's token, for HandleRunning; its
// message, which callers print and log, must not.
func TestRunningErrorHidesToken(t *testing.T) {
	const token = "secret-session-token"
	err := error(&RunningError{
		Owner:   jobs.OwnerInfo{PID: 42, Kind: jobs.OwnerWeb, Address: "127.0.0.1:8460", Token: token},
		DataDir: "/data",
	})
	for _, verb := range []string{"%v", "%+v", "%s", "%q"} {
		if out := fmt.Sprintf(verb, err); strings.Contains(out, token) || !strings.Contains(out, "127.0.0.1:8460") {
			t.Errorf("%s printed %s, want the address without the token", verb, out)
		}
	}
	var running *RunningError
	if !errors.As(err, &running) || running.SignInURL() != "http://127.0.0.1:8460/?token="+token {
		t.Fatalf("SignInURL = %q", running.SignInURL())
	}
}
