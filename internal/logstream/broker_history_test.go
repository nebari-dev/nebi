package logstream

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestSubscribeWithHistoryUntrackedJob(t *testing.T) {
	b := NewBroker()
	if _, ch, ok := b.SubscribeWithHistory(uuid.New()); ok || ch != nil {
		t.Fatal("untracked job must not be subscribable with history")
	}
}

// TestSubscribeWithHistoryNoGapNoDuplicate publishes concurrently with a
// late subscription: history + live chunks equal the full stream exactly.
func TestSubscribeWithHistoryNoGapNoDuplicate(t *testing.T) {
	b := NewBroker()
	id := uuid.New()
	b.Open(id)

	var want strings.Builder
	var wg sync.WaitGroup
	subscribed := make(chan struct{})
	var history string
	var ch chan string
	wg.Go(func() {
		for i := range 2000 {
			line := string(rune('a'+i%26)) + "\n"
			want.WriteString(line)
			b.Publish(id, line)
			if i == 500 {
				<-subscribed
			}
		}
	})
	// Subscribe mid-stream.
	for {
		b.mu.RLock()
		h := b.history[id]
		b.mu.RUnlock()
		h.mu.Lock()
		n := h.buf.Len()
		h.mu.Unlock()
		if n > 100 {
			break
		}
	}
	var ok bool
	history, ch, ok = b.SubscribeWithHistory(id)
	close(subscribed)
	if !ok {
		t.Fatal("tracked job not subscribable")
	}
	var got strings.Builder
	got.WriteString(history)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for c := range ch {
			got.WriteString(c)
		}
	}()
	wg.Wait()
	b.Close(id)
	<-done
	if got.String() != want.String() {
		t.Fatalf("history+live (%d bytes) != published (%d bytes)", got.Len(), want.Len())
	}
	if _, _, ok := b.SubscribeWithHistory(id); ok {
		t.Fatal("closed job still tracked")
	}
}

func TestShutdownDropsHistory(t *testing.T) {
	b := NewBroker()
	id := uuid.New()
	b.Open(id)
	b.Publish(id, "x")
	b.Shutdown()
	if _, _, ok := b.SubscribeWithHistory(id); ok {
		t.Fatal("history survived shutdown")
	}
	b.Open(id) // no-op after shutdown
	if _, _, ok := b.SubscribeWithHistory(id); ok {
		t.Fatal("Open after shutdown tracked a job")
	}
}
