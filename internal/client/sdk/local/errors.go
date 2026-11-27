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
	// removing files would remove another project's directory. When
	// another project's directory cannot be checked, DeleteProject refuses
	// with an error that does not match ErrPathTaken; see DeleteProject.
	ErrPathTaken = errors.New("path is already used by another project")
	// ErrPathNotEmpty: the directory already has a pixi.toml.
	ErrPathNotEmpty = errors.New("directory already contains a pixi.toml")
	// ErrProjectBusy: a job for the project is pending or running. The
	// returned error is a *BusyError carrying the job IDs.
	ErrProjectBusy = errors.New("project has a job in progress")
	// ErrFilesNotManaged: file deletion was asked for a directory nebi did
	// not create.
	ErrFilesNotManaged = errors.New("project directory was not created by nebi")
	// ErrInvalidRequest: a required field of the request is missing, such
	// as CreateVersionRequest.BaseVersionID, or a version to record has no
	// pixi.lock. Not in #661's error table; the client API maps it to 422,
	// like the other validation errors.
	ErrInvalidRequest = errors.New("invalid request")
	// ErrNotFound: the project, version or tag doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalidTag: the tag fails the OCI tag grammar, or is reserved
	// ("latest" or the "sha-" prefix).
	ErrInvalidTag = errors.New("invalid tag")
	// ErrOlderBase: the base version is not the latest and AllowOlderBase
	// is false.
	ErrOlderBase = errors.New("base version is not the latest version")
	// ErrTagExists: the tag is on another version and Move is false.
	ErrTagExists = errors.New("tag is already on another version")
	// ErrLockStale: the pixi.lock given to CreateResolvedVersion doesn't
	// match pixi.toml on disk, or is missing.
	ErrLockStale = errors.New("pixi.lock is out of date")
	// ErrWorkingCopyMissing: there is no pixi.toml at the project path, or
	// no project directory.
	ErrWorkingCopyMissing = errors.New("working copy is missing")
	// ErrWorkingCopyModified: pixi.toml or pixi.lock in the project
	// directory hold changes that are in no recorded version, and an
	// operation would overwrite them.
	ErrWorkingCopyModified = errors.New("working copy has changes that are in no version")
	// ErrNoVersions: InstallVersion was given no version, and the project
	// has none to install.
	ErrNoVersions = errors.New("project has no versions")
	// ErrNotInstalled: Uninstall found nothing to remove: no installed
	// version, no failed install and no .pixi/envs.
	ErrNotInstalled = errors.New("project is not installed")
)

// BusyError is returned by DeleteProject, CreateResolvedVersion,
// InstallVersion and Uninstall while jobs for the project are pending or
// running. It matches ErrProjectBusy
// with errors.Is. For CreateResolvedVersion it is an addition to #661's
// error table; the client API maps it to 409, as for delete.
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
