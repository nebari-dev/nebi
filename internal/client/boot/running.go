package boot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// RunningError is returned by Start when another app, the desktop app or
// nebi-web, owns the database and serves its client API. The starting
// process does not become a second owner; see HandleRunning.
type RunningError struct {
	// Owner is the running app's owner.json. It holds the session token.
	Owner jobs.OwnerInfo
	// DataDir is the data directory both processes use.
	DataDir string
}

func (e *RunningError) Error() string {
	return fmt.Sprintf("nebi is already running for %s: %s, process %d, at http://%s",
		e.DataDir, KindName(e.Owner.Kind), e.Owner.PID, e.Owner.Address)
}

// SignInURL is the running app's one-time sign-in URL. It holds the
// session token: show it to the user, never log it.
func (e *RunningError) SignInURL() string {
	return "http://" + e.Owner.Address + "/?token=" + e.Owner.Token
}

// ShowWindow asks the running desktop app to bring its window to the
// front, through its client API.
func (e *RunningError) ShowWindow(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://"+e.Owner.Address+api.Prefix+"/desktop/show-window", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.Owner.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("asking process %d to show its window: %w", e.Owner.PID, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("asking process %d to show its window: %s", e.Owner.PID, resp.Status)
	}
	return nil
}

// HandleRunning does what a starting process of the given kind does when
// Start returns a *RunningError, and then the process exits:
//
//	starting  holder    action
//	desktop   desktop   asks the running app to raise its window
//	desktop   nebi-web  notify: a dialog with the running instance's URL
//	nebi-web  either    notify: prints the running instance's URL
//
// When the running desktop app cannot be asked to raise its window, the
// starting desktop app falls back to notify.
func HandleRunning(ctx context.Context, starting jobs.OwnerKind, running *RunningError, notify func(*RunningError)) error {
	if starting == jobs.OwnerDesktop && running.Owner.Kind == jobs.OwnerDesktop {
		err := running.ShowWindow(ctx)
		if err == nil {
			return nil
		}
		notify(running)
		return err
	}
	notify(running)
	return nil
}

// ErrOwnerNotServing is returned by Start when another app has held the
// lock for longer than Options.StartingWait without serving its API.
var ErrOwnerNotServing = errors.New("the nebi app that owns the data directory is not serving its API")

// acquire takes the owner lock on opts.DataDir. When another process holds
// it, what happens depends on that process, as owner.json describes it:
//
//	holder                          action
//	desktop app or nebi-web         return a *RunningError (HandleRunning)
//	a CLI command                   wait for the lock, naming the PID
//	an app that is still starting   read owner.json again until the address
//	  (kind set, no address yet)      appears, then as above; give up after
//	                                  Options.StartingWait. An app that is
//	                                  shutting down also has no address; its
//	                                  lock comes free and is taken instead.
//
// The owner lock, not Wails' single-instance support, decides whether
// another instance exists: the rule is one owner per database, not one per
// application.
func acquire(ctx context.Context, opts Options) (*jobs.Owner, error) {
	var (
		waitingFor    int // PID of the CLI command announced as waited for
		startingPID   = -1
		startingSince time.Time
	)
	for {
		o, err := jobs.TryAcquireOwner(opts.DataDir)
		if err == nil {
			return o, nil
		}
		if !errors.Is(err, jobs.ErrOwnerLockHeld) {
			return nil, err
		}

		info, err := jobs.ReadOwner(opts.DataDir)
		starting := false
		switch {
		case errors.Is(err, jobs.ErrNoOwnerInfo):
			// The holder took the lock and has not published yet.
			starting = true
		case errors.Is(err, jobs.ErrInvalidOwnerInfo):
			return nil, fmt.Errorf("process %d (nebi %s, client API %s) owns the data directory %s, and this nebi "+
				"cannot read its owner.json; stop that process or use the same nebi version: %w",
				info.PID, info.NebiVersion, info.ClientAPIVersion, opts.DataDir, err)
		case err != nil:
			return nil, err
		case info.Kind == jobs.OwnerCLI:
			if waitingFor != info.PID {
				waitingFor = info.PID
				_, _ = fmt.Fprintf(opts.Status, "Waiting for process %d (a nebi CLI command) to finish with %s...\n",
					info.PID, opts.DataDir)
			}
			startingPID = -1
		case info.Address == "":
			starting = true
		default:
			return nil, &RunningError{Owner: info, DataDir: opts.DataDir}
		}

		if starting {
			if info.PID != startingPID {
				startingPID, startingSince = info.PID, time.Now()
				if info.PID > 0 {
					_, _ = fmt.Fprintf(opts.Status, "Waiting for process %d (%s), which is starting or stopping...\n",
						info.PID, KindName(info.Kind))
				}
			} else if time.Since(startingSince) > opts.StartingWait {
				return nil, fmt.Errorf("%w: process %d (%s) has held the lock on %s for more than %s without serving; "+
					"stop it and start nebi again", ErrOwnerNotServing, info.PID, KindName(info.Kind), opts.DataDir, opts.StartingWait)
			}
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for the owner lock on %s: %w", opts.DataDir, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// KindName names an owner kind for people, as in "process 42 (nebi-web)".
func KindName(k jobs.OwnerKind) string {
	switch k {
	case jobs.OwnerDesktop:
		return "the desktop app"
	case jobs.OwnerWeb:
		return "nebi-web"
	case jobs.OwnerCLI:
		return "a nebi CLI command"
	}
	return "a nebi process"
}
