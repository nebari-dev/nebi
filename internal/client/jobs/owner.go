package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// OwnerLockFile is the file the owner holds the OS lock on.
	OwnerLockFile = "owner.lock"
	// OwnerInfoFile describes the current owner once it has published. A
	// file left by an owner that died, or that Release could not remove,
	// stays until the next owner takes the lock and removes it.
	OwnerInfoFile = "owner.json"

	// ownerTempPrefix names the files owner.json is written through before
	// the rename. A new owner removes any a crashed owner left behind.
	ownerTempPrefix = ".owner.json."
	ownerTempSuffix = ".tmp"
)

// lockPollInterval is how often AcquireOwner retries a held lock.
const lockPollInterval = 50 * time.Millisecond

// testHookBeforeUnlock, when a test sets it, runs in Release after owner.json
// is removed and before the lock is released, so the test can check that
// order. It is nil outside tests.
var testHookBeforeUnlock func(dir string)

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
	// ErrInvalidOwnerInfo is returned by ReadOwner when owner.json decodes
	// but its content is not valid owner info, such as an address form this
	// build does not accept. The OwnerInfo returned with it still holds the
	// PID and versions the file gave, so a caller can report a version skew,
	// but never its Address or Token.
	ErrInvalidOwnerInfo = errors.New("invalid job runner owner info")
	// ErrOwnerReleased is returned by Owner methods after Release.
	ErrOwnerReleased = errors.New("job runner owner lock was released")
	// ErrOwnerOnOtherHost is returned by ReadOwner when owner.json was
	// written on another machine, as with a home directory shared over
	// NFS. Its loopback address would reach this machine, not the owner, so
	// the reader must fail instead of connecting. The error names the other
	// host.
	ErrOwnerOnOtherHost = errors.New("job runner owner runs on another host")

	errEmptyOwnerDir = errors.New("owner lock directory is empty")
)

// OwnerKind says which kind of process holds the owner lock.
type OwnerKind string

const (
	// OwnerDesktop is the desktop app. It serves the client API once it is
	// listening.
	OwnerDesktop OwnerKind = "desktop"
	// OwnerWeb is nebi-web. It serves the client API once it is listening.
	OwnerWeb OwnerKind = "web"
	// OwnerCLI is a CLI command. It never serves the client API, so a
	// process that finds it holding the lock waits for the lock instead of
	// waiting for an address.
	OwnerCLI OwnerKind = "cli"
)

func (k OwnerKind) valid() bool {
	switch k {
	case OwnerDesktop, OwnerWeb, OwnerCLI:
		return true
	}
	return false
}

// ServesAPI reports whether an owner of this kind serves the client API
// once it is listening.
func (k OwnerKind) ServesAPI() bool {
	return k == OwnerDesktop || k == OwnerWeb
}

// OwnerInfo is the content of owner.json.
type OwnerInfo struct {
	// PID is the owner's process ID, for messages such as "waiting for
	// process 1234". It is not used to decide whether the owner is alive.
	PID int `json:"pid"`
	// Kind is the kind of process that holds the lock. It is published as
	// soon as the lock is taken, so a waiting process can tell an app that
	// is still starting (an app kind with no Address yet) from a CLI command
	// that will never serve the client API. Never empty.
	Kind OwnerKind `json:"kind"`
	// Hostname is the host name of the machine the owner runs on. Publish
	// sets it; info.Hostname is ignored. ReadOwner fails with
	// ErrOwnerOnOtherHost when it is not this machine's. Never empty.
	Hostname string `json:"hostname"`
	// NebiVersion is the owner's nebi version. Never empty.
	NebiVersion string `json:"nebi_version"`
	// ClientAPIVersion is the version of the client API the owner speaks.
	// Never empty.
	ClientAPIVersion string `json:"client_api_version"`
	// Address is the loopback address of the owner's client API, as
	// "127.0.0.1:port" or "[::1]:port". Empty for an owner that serves no
	// API, or that is not listening yet.
	Address string `json:"address"`
	// Token authenticates requests to the owner's client API. Empty exactly
	// when Address is empty. String, GoString and LogValue never print it.
	Token string `json:"token"`
}

// redactedToken stands in for a set Token when OwnerInfo is printed.
const redactedToken = "REDACTED"

func (info OwnerInfo) redactedToken() string {
	if info.Token == "" {
		return ""
	}
	return redactedToken
}

// String formats info with the token redacted. It is also what %v and %+v
// print.
func (info OwnerInfo) String() string {
	return fmt.Sprintf("{PID:%d Kind:%s Hostname:%s NebiVersion:%s ClientAPIVersion:%s Address:%s Token:%s}",
		info.PID, info.Kind, info.Hostname, info.NebiVersion, info.ClientAPIVersion, info.Address, info.redactedToken())
}

// GoString formats info for %#v with the token redacted.
func (info OwnerInfo) GoString() string {
	return fmt.Sprintf("jobs.OwnerInfo{PID:%d, Kind:%q, Hostname:%q, NebiVersion:%q, ClientAPIVersion:%q, Address:%q, Token:%q}",
		info.PID, info.Kind, info.Hostname, info.NebiVersion, info.ClientAPIVersion, info.Address, info.redactedToken())
}

// LogValue implements slog.LogValuer with the token redacted.
func (info OwnerInfo) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("pid", info.PID),
		slog.String("kind", string(info.Kind)),
		slog.String("hostname", info.Hostname),
		slog.String("nebi_version", info.NebiVersion),
		slog.String("client_api_version", info.ClientAPIVersion),
		slog.String("address", info.Address),
		slog.String("token", info.redactedToken()),
	)
}

// Owner is the handle of the process that holds the owner lock for a
// directory. It is safe for concurrent use.
//
// Job state lives in the memory of one process, so only one process may run
// jobs for a database at a time. That process is the owner: it holds an OS
// advisory lock (flock on Unix, LockFileEx on Windows) on owner.lock, in the
// directory that holds the client database (nebi-client.db), for as long as
// it runs, and describes itself
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
//
// Callers should know:
//   - The lock directory must be the directory of the nebi-client.db the
//     process opens, so that every binary that shares a database shares the
//     lock. Take the lock before opening the database: a process that does
//     not get it must not open the database at all. That is also why
//     TryAcquireOwner creates the directory.
//   - Publish once right after taking the lock, with the Kind, so a waiting
//     process can tell an app that is still starting from a CLI command.
//   - Close the job runner before Release. Memory.Close waits for running
//     jobs. Released first, another process can become the owner and start
//     jobs while this process's jobs still run.
//   - owner.json can describe an owner that has died: one that died after a
//     reader failed to take the lock, or one whose file the next owner has
//     not removed yet. A reader that gets "connection refused" or a rejected
//     token from Address must wait and try again, not report an error.
//   - On Windows a file has no 0600 mode, and owner.json gets no access
//     control list of its own: it, and the token in it, rely on the access
//     rules of the data directory, which is under the user profile by
//     default. This is an accepted, documented limitation.
//   - With a data directory shared between machines, as with a home
//     directory on NFS, owner.json can describe an owner on another host.
//     ReadOwner then fails with ErrOwnerOnOtherHost rather than let the
//     reader connect to its own loopback.
//   - On Windows the OS can take a moment to free the lock of a process that
//     died. TryAcquireOwner may report the lock as held during that time;
//     AcquireOwner waits it out.
type Owner struct {
	dir string

	mu   sync.Mutex
	lock *os.File // nil after Release
}

// TryAcquireOwner takes the owner lock for dir without waiting. dir is the
// directory that holds nebi-client.db; it is created if missing, and a
// relative dir
// is made absolute, so a later change of working directory does not move
// owner.json. It returns
// ErrOwnerLockHeld when another owner holds the lock, including another handle
// in the same process.
//
// A new owner removes owner.json and any temporary file a crashed owner left
// behind before returning. Until then, and after an owner dies, owner.json
// may still describe the dead owner; see Owner.
func TryAcquireOwner(dir string) (*Owner, error) {
	dir, err := resolveOwnerDir(dir)
	if err != nil {
		return nil, err
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

// resolveOwnerDir makes dir absolute, refusing an empty dir.
func resolveOwnerDir(dir string) (string, error) {
	if dir == "" {
		return "", errEmptyOwnerDir
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving owner lock directory: %w", err)
	}
	return abs, nil
}

// AcquireOwner takes the owner lock for dir, waiting until it is free or ctx
// ends. On ctx ending it returns an error that wraps ctx.Err(). A relative
// dir is resolved once, before waiting, so a change of working directory
// while it waits does not make it lock a different directory.
func AcquireOwner(ctx context.Context, dir string) (*Owner, error) {
	dir, err := resolveOwnerDir(dir)
	if err != nil {
		return nil, err
	}
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

// Dir returns the absolute path of the directory the lock was taken in.
func (o *Owner) Dir() string {
	return o.dir
}

// Publish writes owner.json with info. The PID and Hostname are always the
// calling process's; info.PID and info.Hostname are ignored. Every owner
// calls Publish as soon as it holds the lock, so its Kind is known; an owner
// that serves the client API calls it again once it is listening, to add its
// address and token. The file has mode 0600 and is replaced with an atomic
// rename, so a reader sees either the old content or the new, never part.
//
// Kind, NebiVersion and ClientAPIVersion are required. Address and Token are
// set together or not at all, and Address must be a loopback IP and port.
func (o *Owner) Publish(info OwnerInfo) error {
	info.PID = os.Getpid()
	host, err := localHostname()
	if err != nil {
		return err
	}
	info.Hostname = host
	if err := info.validate(); err != nil {
		return err
	}
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

// Release removes owner.json and releases the lock. Close the job runner
// first; see Owner. It is safe to call more than once; later calls return
// nil.
func (o *Owner) Release() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lock == nil {
		return nil
	}
	// owner.json goes first, while the lock is still held: removed after
	// unlocking, it could be the next owner's file.
	removeErr := removeOwnerInfo(o.dir)
	if testHookBeforeUnlock != nil {
		testHookBeforeUnlock(o.dir)
	}
	return errors.Join(removeErr, o.unlock())
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
// owner: wait and try again.
//
// The content can describe an owner that has died (see Owner), so an Address
// that refuses connections or rejects the token also means wait and try
// again. A file that is not valid owner info, such as one with an address
// that is not loopback, is an error wrapping ErrInvalidOwnerInfo, so a reader
// never sends the token off the machine. With that error ReadOwner still
// returns the PID, NebiVersion and ClientAPIVersion the file gave, any of
// which may be empty, and never Address or Token: a caller can then report
// that the owner speaks a client API version it does not. Compare
// ClientAPIVersion before treating the error as a corrupt file.
//
// A file written on another host fails with an error that wraps
// ErrOwnerOnOtherHost and names that host; the OwnerInfo returned with it
// again holds neither Address nor Token.
func ReadOwner(dir string) (OwnerInfo, error) {
	if dir == "" {
		// Reading the working directory would return ErrNoOwnerInfo, which
		// tells the caller to wait, so a missing dir would wait forever.
		return OwnerInfo{}, errEmptyOwnerDir
	}
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
	if err := info.validate(); err != nil {
		info.Address, info.Token = "", ""
		return info, fmt.Errorf("%w: %w", ErrInvalidOwnerInfo, err)
	}
	host, err := localHostname()
	if err != nil {
		info.Address, info.Token = "", ""
		return info, err
	}
	if !strings.EqualFold(info.Hostname, host) {
		info.Address, info.Token = "", ""
		return info, fmt.Errorf("%w: process %d on host %q owns the data directory %s, and this is host %q; "+
			"stop nebi on %q or use a data directory that is not shared between machines",
			ErrOwnerOnOtherHost, info.PID, info.Hostname, dir, host, info.Hostname)
	}
	return info, nil
}

// hostname returns this machine's host name. Tests replace it.
var hostname = os.Hostname

func localHostname() (string, error) {
	h, err := hostname()
	if err != nil {
		return "", fmt.Errorf("finding this machine's host name: %w", err)
	}
	if h == "" {
		return "", errors.New("finding this machine's host name: it is empty")
	}
	return h, nil
}

func (info OwnerInfo) validate() error {
	if info.PID <= 0 {
		return fmt.Errorf("owner pid %d is not a process id", info.PID)
	}
	if !info.Kind.valid() {
		return fmt.Errorf("owner kind %q is not desktop, web or cli", info.Kind)
	}
	if info.Hostname == "" {
		return errors.New("owner host name is empty")
	}
	if info.NebiVersion == "" {
		return errors.New("owner nebi version is empty")
	}
	if info.ClientAPIVersion == "" {
		return errors.New("owner client API version is empty")
	}
	if (info.Address == "") != (info.Token == "") {
		return errors.New("owner address and token must be set together")
	}
	if info.Address != "" {
		return validateLoopbackAddress(info.Address)
	}
	return nil
}

// validateLoopbackAddress accepts only a loopback IP literal and a non-zero
// port. A host name is refused: it could resolve to a non-loopback address.
func validateLoopbackAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("owner address %q: %w", addr, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return fmt.Errorf("owner address %q is not a loopback IP address", addr)
	}
	if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return fmt.Errorf("owner address %q has no valid port", addr)
	}
	return nil
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
