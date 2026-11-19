package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// OwnerLockFile is the file the owner holds the OS lock on.
	OwnerLockFile = "owner.lock"
	// OwnerInfoFile describes the current owner. It only exists while an
	// owner holds the lock and has published.
	OwnerInfoFile = "owner.json"

	// ownerTempPrefix names the files owner.json is written through before
	// the rename. A new owner removes any a crashed owner left behind.
	ownerTempPrefix = ".owner.json."
	ownerTempSuffix = ".tmp"
)

// lockPollInterval is how often AcquireOwner retries a held lock.
const lockPollInterval = 50 * time.Millisecond

// heldOwners keeps every Owner reachable until Release. The lock lives in an
// *os.File, so a dropped handle would let the garbage collector close the file
// and the OS free the lock while the process still runs jobs.
var heldOwners sync.Map // *Owner -> struct{}

var (
	// ErrOwnerLockHeld is returned by TryAcquireOwner when another owner
	// holds the lock.
	ErrOwnerLockHeld = errors.New("job runner owner lock is held by another owner")
	// ErrNoOwnerInfo is returned by ReadOwner when owner.json does not exist.
	// After failing to take the lock it means the owner has not published
	// yet: wait and try again. It never means there is no owner.
	ErrNoOwnerInfo = errors.New("job runner owner has not published owner.json")
	// ErrOwnerReleased is returned by Owner methods after Release.
	ErrOwnerReleased = errors.New("job runner owner lock was released")
)

// OwnerInfo is the content of owner.json.
type OwnerInfo struct {
	// PID is the owner's process ID, for messages such as "waiting for
	// process 1234". It is not used to decide whether the owner is alive.
	PID int `json:"pid"`
	// NebiVersion is the owner's nebi version.
	NebiVersion string `json:"nebi_version"`
	// ClientAPIVersion is the version of the client API the owner speaks.
	ClientAPIVersion string `json:"client_api_version"`
	// Address is the loopback address of the owner's client API. Empty for
	// an owner that serves no API, or that is not listening yet.
	Address string `json:"address"`
	// Token authenticates requests to the owner's client API. Empty exactly
	// when Address is empty.
	Token string `json:"token"`
}

// Owner is the handle of the process that holds the owner lock for a
// directory. It is safe for concurrent use.
//
// Job state lives in the memory of one process, so only one process may run
// jobs for a database at a time. That process is the owner: it holds an OS
// advisory lock (flock on Unix, LockFileEx on Windows) on owner.lock, in the
// directory that holds nebi.db, for as long as it runs, and describes itself
// in owner.json next to it. Every other process sends its project actions to
// the owner instead of running them.
//
// The OS lock is the only source of truth for whether an owner exists. The OS
// releases it when the owner exits or dies, so there are no stale locks and
// no PID checks. owner.lock itself is never removed: removing a lock file
// lets two processes lock two different files of the same name.
//
// The lock is held until Release or until the process exits, even if the
// caller drops the handle.
type Owner struct {
	dir string

	mu   sync.Mutex
	lock *os.File // nil after Release
}

// TryAcquireOwner takes the owner lock for dir without waiting. dir is the
// directory that holds nebi.db; it is created if missing. It returns
// ErrOwnerLockHeld when another owner holds the lock, including another handle
// in the same process.
//
// A new owner removes owner.json and any temporary file a crashed owner left
// behind before returning, so ReadOwner never reports a dead owner.
func TryAcquireOwner(dir string) (*Owner, error) {
	if dir == "" {
		return nil, errors.New("owner lock directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating owner lock directory: %w", err)
	}

	path := filepath.Join(dir, OwnerLockFile)
	// os.OpenFile opens close-on-exec on Unix and non-inheritable on
	// Windows, so jobs the owner starts do not keep the lock after it dies.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening owner lock file: %w", err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errWouldBlock) {
			return nil, ErrOwnerLockHeld
		}
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}

	o := &Owner{dir: dir, lock: f}
	if err := removeOwnerInfo(dir); err != nil {
		_ = o.unlock()
		return nil, err
	}
	heldOwners.Store(o, struct{}{})
	return o, nil
}

// AcquireOwner takes the owner lock for dir, waiting until it is free or ctx
// ends. On ctx ending it returns an error that wraps ctx.Err().
func AcquireOwner(ctx context.Context, dir string) (*Owner, error) {
	t := time.NewTicker(lockPollInterval)
	defer t.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("waiting for job runner owner lock: %w", err)
		}
		o, err := TryAcquireOwner(dir)
		if !errors.Is(err, ErrOwnerLockHeld) {
			return o, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for job runner owner lock: %w", ctx.Err())
		case <-t.C:
		}
	}
}

// Dir returns the directory the lock was taken in.
func (o *Owner) Dir() string {
	return o.dir
}

// Publish writes owner.json with info. The PID is always the calling
// process's; info.PID is ignored. An owner that serves the client API calls
// Publish once it holds the lock and again once it is listening, to add its
// address and token. The file has mode 0600 and is replaced with an atomic
// rename, so a reader sees either the old content or the new, never part.
func (o *Owner) Publish(info OwnerInfo) error {
	if (info.Address == "") != (info.Token == "") {
		return errors.New("owner address and token must be set together")
	}
	info.PID = os.Getpid()
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding owner info: %w", err)
	}
	data = append(data, '\n')

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lock == nil {
		return ErrOwnerReleased
	}
	return writeFileAtomic(o.dir, data)
}

// Release removes owner.json and releases the lock. It is safe to call more
// than once; later calls return nil.
func (o *Owner) Release() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lock == nil {
		return nil
	}
	// owner.json goes first, while the lock is still held: removed after
	// unlocking, it could be the next owner's file.
	return errors.Join(removeOwnerInfo(o.dir), o.unlock())
}

func (o *Owner) unlock() error {
	f := o.lock
	o.lock = nil
	heldOwners.Delete(o)
	unlockErr := unlockFile(f)
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlocking owner lock: %w", unlockErr)
	}
	// Closing releases the lock even if the explicit unlock failed.
	return errors.Join(unlockErr, f.Close())
}

// ReadOwner returns the content of owner.json in dir. Call it only after
// failing to take the lock. It returns ErrNoOwnerInfo when the file does not
// exist, which means the owner has not published yet; an empty Address means
// the owner is not serving the client API (yet). Neither means there is no
// owner.
func ReadOwner(dir string) (OwnerInfo, error) {
	data, err := readFile(filepath.Join(dir, OwnerInfoFile))
	if errors.Is(err, os.ErrNotExist) {
		return OwnerInfo{}, ErrNoOwnerInfo
	}
	if err != nil {
		return OwnerInfo{}, fmt.Errorf("reading owner info: %w", err)
	}
	var info OwnerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return OwnerInfo{}, fmt.Errorf("decoding owner info: %w", err)
	}
	return info, nil
}

// writeFileAtomic writes data to a temporary file in dir and renames it over
// owner.json.
func writeFileAtomic(dir string, data []byte) (err error) {
	tmp, err := os.CreateTemp(dir, ownerTempPrefix+"*"+ownerTempSuffix)
	if err != nil {
		return fmt.Errorf("creating owner info: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = removeFile(tmp.Name())
		}
	}()
	// CreateTemp already uses 0600; Chmod keeps it so under any umask.
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting owner info mode: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing owner info: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing owner info: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing owner info: %w", err)
	}
	if err := replaceFile(tmp.Name(), filepath.Join(dir, OwnerInfoFile)); err != nil {
		return fmt.Errorf("replacing owner info: %w", err)
	}
	return nil
}

// removeOwnerInfo removes owner.json and any temporary files it was written
// through. The caller holds the lock, so the files are its own (on Release)
// or were left by an owner that crashed (on acquire).
func removeOwnerInfo(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading owner lock directory: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		stale := name == OwnerInfoFile ||
			(strings.HasPrefix(name, ownerTempPrefix) && strings.HasSuffix(name, ownerTempSuffix))
		if !stale {
			continue
		}
		if err := removeFile(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing owner info %s: %w", name, err)
		}
	}
	return nil
}
