// Package web is the composition root of nebi-web: it makes the process the
// owner of the client database and serves the client API and the client UI
// on loopback, for a browser the user brings (issue #680). The startup and
// shutdown are internal/client/boot's, shared with the desktop app.
package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/boot"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// DefaultPort is the port nebi-web listens on when --port is not given.
const DefaultPort = 8460

// Options configures Run.
type Options struct {
	// Port is the loopback port. Zero means DefaultPort.
	Port int
	// DataDir and ProjectsDir come from boot.DataDir and boot.ProjectsDir.
	DataDir     string
	ProjectsDir string
	// PixiPath is the pixi binary. Empty means boot.FindPixi, once the
	// process holds the lock.
	PixiPath string
	// Version is the build's version.
	Version api.VersionInfo
	// Assets is the built client UI, frontend.ClientApp().
	Assets fs.FS
	// Stdout receives the sign-in URL, or the running instance's URL. Nil
	// means os.Stdout.
	Stdout io.Writer
	// Stderr receives waiting messages. Nil means os.Stderr.
	Stderr io.Writer
	// Logger receives progress and errors. Nil means slog.Default().
	Logger *slog.Logger
}

// Run starts nebi-web and serves until ctx ends, then shuts down. When
// another app already owns the database it prints that app's URL and
// returns nil without starting. When ctx ends before it starts, such as on
// Ctrl+C while it waits for a CLI command, it returns nil: the user asked
// it to stop.
func Run(ctx context.Context, opts Options) error {
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	inst, err := boot.Start(ctx, boot.Options{
		Kind:        jobs.OwnerWeb,
		DataDir:     opts.DataDir,
		ProjectsDir: opts.ProjectsDir,
		PixiPath:    opts.PixiPath,
		Port:        opts.Port,
		Version:     opts.Version,
		Assets:      opts.Assets,
		Logger:      opts.Logger,
		Status:      opts.Stderr,
	})
	var running *boot.RunningError
	switch {
	case errors.As(err, &running):
		return boot.HandleRunning(ctx, jobs.OwnerWeb, running, func(r *boot.RunningError) {
			_, _ = fmt.Fprintf(opts.Stdout, "Nebi is already running (%s, process %d) for %s.\nOpen it in your browser:\n\n    %s\n\n",
				boot.KindName(r.Owner.Kind), r.Owner.PID, r.DataDir, r.SignInURL())
		})
	case errors.Is(err, boot.ErrPortInUse):
		return fmt.Errorf("%w; choose another port with --port", err)
	case err != nil && ctx.Err() != nil:
		return nil
	case err != nil:
		return err
	}

	_, _ = fmt.Fprintf(opts.Stdout, "Nebi is running. Open it in your browser:\n\n    %s\n\nPress Ctrl+C to stop.\n", inst.SignInURL())

	var serveErr error
	select {
	case <-ctx.Done():
	case <-inst.Done():
		serveErr = inst.Err()
	}
	_, _ = fmt.Fprintln(opts.Stderr, "Stopping nebi...")
	return errors.Join(serveErr, inst.Shutdown(context.Background()))
}
