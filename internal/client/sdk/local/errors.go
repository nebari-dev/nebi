package local

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Errors returned by the SDK. Callers match them with errors.Is; the
// returned error usually wraps one of them with detail.
var (
	// ErrInvalidName: the name fails validation.
	ErrInvalidName = errors.New("invalid project name")
	// ErrNoPlatform: no platform given.
	ErrNoPlatform = errors.New("at least one platform is required")
	// ErrInvalidManifest: pixi.toml doesn't parse or validate.
	ErrInvalidManifest = errors.New("invalid pixi.toml")
	// ErrNameTaken: the name is already used.
	ErrNameTaken = errors.New("project name is already taken")
	// ErrPathTaken: the path is already tracked, or is inside or contains
	// a tracked project's directory. DeleteProject also returns it when
	// removing files would remove another project's directory.
	ErrPathTaken = errors.New("path is already used by another project")
	// ErrPathNotEmpty: the directory already has a pixi.toml.
	ErrPathNotEmpty = errors.New("directory already contains a pixi.toml")
	// ErrProjectBusy: a job for the project is queued or running. The
	// returned error is a *BusyError carrying the job IDs.
	ErrProjectBusy = errors.New("project has a job in progress")
	// ErrFilesNotManaged: file deletion was asked for a directory nebi did
	// not create.
	ErrFilesNotManaged = errors.New("project directory was not created by nebi")
	// ErrNotFound: the project doesn't exist.
	ErrNotFound = errors.New("project not found")
)

// BusyError is returned by DeleteProject while jobs for the project are
// queued or running. It matches ErrProjectBusy with errors.Is.
type BusyError struct {
	ProjectID uuid.UUID
	JobIDs    []uuid.UUID
}

func (e *BusyError) Error() string {
	ids := make([]string, len(e.JobIDs))
	for i, id := range e.JobIDs {
		ids[i] = id.String()
	}
	return fmt.Sprintf("%v: project %s, jobs %s", ErrProjectBusy, e.ProjectID, strings.Join(ids, ", "))
}

// Unwrap makes errors.Is(err, ErrProjectBusy) true.
func (e *BusyError) Unwrap() error { return ErrProjectBusy }
