package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/process"
)

const storageLimitPollInterval = 5 * time.Second

type storageLimitExceededError struct {
	Kind  string
	path  string
	limit int64
	size  int64
	err   error
}

func (e *storageLimitExceededError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("workspace %s limit check failed for %s: %v", e.Kind, e.path, e.err)
	}
	return fmt.Sprintf("workspace %s limit exceeded for %s: %d bytes used, limit is %d bytes", e.Kind, e.path, e.size, e.limit)
}

// IsResourceLimitError reports errors produced by executor-owned resource
// guards, such as workspace storage budgets.
func IsResourceLimitError(err error) bool {
	var storageErr *storageLimitExceededError
	return errors.As(err, &storageErr)
}

type processLimitLogWriter struct {
	target io.Writer
	limits limits.ProcessLimits
	mu     sync.Mutex
	seen   bool
}

func (w *processLimitLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	output := string(p)
	if process.HasResourceLimitOutput(output) || process.HasLikelyResourceLimitOutput(w.limits, output) {
		w.seen = true
	}
	return w.target.Write(p)
}

func (w *processLimitLogWriter) SeenResourceLimitOutput() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seen
}

func (e *LocalExecutor) withStorageLimit(ctx context.Context, path string, logWriter io.Writer, fn func(context.Context) error) error {
	if e.limits.JobStorageBytes <= 0 {
		return fn(ctx)
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var exceeded atomic.Value
	var recordOnce sync.Once
	recordExceeded := func(size int64) {
		recordOnce.Do(func() {
			err := &storageLimitExceededError{Kind: "storage", path: path, limit: e.limits.JobStorageBytes, size: size}
			exceeded.Store(err)
			fmt.Fprintf(logWriter, "Workspace storage limit exceeded: %d bytes used, limit is %d bytes\n", size, e.limits.JobStorageBytes)
			cancel()
		})
	}
	recordCheckError := func(err error) {
		recordOnce.Do(func() {
			limitErr := &storageLimitExceededError{Kind: "storage", path: path, limit: e.limits.JobStorageBytes, err: err}
			exceeded.Store(limitErr)
			fmt.Fprintf(logWriter, "Workspace storage limit check failed: %v\n", err)
			cancel()
		})
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(storageLimitPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				size, err := directorySize(path)
				if err != nil {
					recordCheckError(err)
				} else if size > e.limits.JobStorageBytes {
					recordExceeded(size)
				}
			case <-done:
				return
			case <-jobCtx.Done():
				return
			}
		}
	}()

	err := fn(jobCtx)
	close(done)

	if size, sizeErr := directorySize(path); sizeErr != nil {
		recordCheckError(sizeErr)
	} else if size > e.limits.JobStorageBytes {
		recordExceeded(size)
	}
	if stored := exceeded.Load(); stored != nil {
		return stored.(error)
	}
	return err
}

func directorySize(root string) (int64, error) {
	var total int64
	tracker := newDiskUsageTracker()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		total += tracker.size(info)
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}
