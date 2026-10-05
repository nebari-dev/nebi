package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
)

func TestDesktopShutdownWaitsForWorkerCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := NewApp()
		workerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		a.workerCancel, a.workerDone = cancel, done
		a.server = &http.Server{}
		a.jobQueue = queue.NewMemoryQueue(10)
		result := make(chan struct{})
		go func() {
			// Wails may supply an already-cancelled context on exit.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			a.shutdown(ctx)
			close(result)
		}()
		synctest.Wait()
		if workerCtx.Err() != context.Canceled {
			t.Fatal("desktop worker was not cancelled")
		}
		if err := a.jobQueue.Enqueue(context.Background(), &models.Job{ID: uuid.New()}); err == nil {
			t.Fatal("shutdown still accepts jobs")
		}
		select {
		case <-result:
			t.Fatal("desktop exited before worker cleanup finished")
		default:
		}
		close(done) // Worker has finished cleanup and its final database writes.
		<-result
		if err := a.stop(context.Background()); err != nil {
			t.Fatalf("repeated shutdown: %v", err)
		}
	})
}

func TestDesktopShutdownTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := NewApp()
		a.workerCancel = func() {}
		a.workerDone = make(chan struct{})
		a.server = &http.Server{}
		a.jobQueue = queue.NewMemoryQueue(10)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := a.stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected bounded cleanup wait, got %v", err)
		}
	})
}

func TestDesktopShutdownBeforeWorkerStartup(t *testing.T) {
	a := NewApp()
	if err := a.stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !a.stopping {
		t.Fatal("initialization can still start a worker after shutdown")
	}
}
