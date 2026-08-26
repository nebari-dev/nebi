package logstream

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPublishDeliversBurstLargerThanOldBuffer(t *testing.T) {
	b := NewBroker()
	jobID := uuid.New()
	ch := b.Subscribe(jobID)

	const n = 1000
	for i := 0; i < n; i++ {
		b.Publish(jobID, fmt.Sprintf("line %d", i))
	}
	b.Close(jobID)

	var got []string
	for line := range ch {
		got = append(got, line)
	}
	if len(got) != n {
		t.Fatalf("got %d lines, want %d", len(got), n)
	}
}

func TestPublishDropsWhenFullAndEmitsMarker(t *testing.T) {
	b := NewBroker()
	jobID := uuid.New()
	ch := b.Subscribe(jobID)

	for i := 0; i < subscriberBufferSize; i++ {
		b.Publish(jobID, fmt.Sprintf("fill %d", i))
	}

	// Nobody is reading: these must be dropped immediately, and Publish
	// must return promptly rather than blocking the producer.
	const dropped = 3
	start := time.Now()
	for i := 0; i < dropped; i++ {
		b.Publish(jobID, fmt.Sprintf("lost %d", i))
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Publish blocked for %v with a full buffer", elapsed)
	}

	// Free two slots, then publish another line. The marker must be
	// delivered before that line.
	<-ch
	<-ch
	b.Publish(jobID, "after")
	b.Close(jobID)

	var got []string
	for line := range ch {
		got = append(got, line)
	}
	if len(got) < 2 {
		t.Fatalf("got %d lines after drain, want at least marker + after: %q", len(got), got)
	}
	marker, after := got[len(got)-2], got[len(got)-1]
	if !strings.Contains(marker, fmt.Sprintf("%d log lines dropped", dropped)) {
		t.Fatalf("expected drop marker for %d lines, got %q", dropped, marker)
	}
	if after != "after" {
		t.Fatalf("line following marker = %q, want %q", after, "after")
	}
	for _, l := range got {
		if strings.HasPrefix(l, "lost") {
			t.Fatalf("dropped line %q was unexpectedly delivered", l)
		}
	}
}

func TestDropMarkerNotRepeatedOnceDelivered(t *testing.T) {
	b := NewBroker()
	jobID := uuid.New()
	ch := b.Subscribe(jobID)

	for i := 0; i < subscriberBufferSize; i++ {
		b.Publish(jobID, "fill")
	}
	b.Publish(jobID, "lost")

	// Drain everything so both the marker and subsequent lines fit.
	for i := 0; i < subscriberBufferSize; i++ {
		<-ch
	}
	b.Publish(jobID, "a")
	b.Publish(jobID, "b")
	b.Close(jobID)

	var got []string
	for line := range ch {
		got = append(got, line)
	}
	markers := 0
	for _, l := range got {
		if strings.Contains(l, "dropped") {
			markers++
		}
	}
	if markers != 1 {
		t.Fatalf("got %d drop markers, want exactly 1: %q", markers, got)
	}
	if len(got) != 3 || got[1] != "a" || got[2] != "b" {
		t.Fatalf("unexpected sequence after drop: %q", got)
	}
}

func TestPublishIsolatesSlowSubscriberFromFastOne(t *testing.T) {
	b := NewBroker()
	jobID := uuid.New()
	slow := b.Subscribe(jobID)
	fast := b.Subscribe(jobID)

	const n = subscriberBufferSize + 5
	fastGot := make(chan int)
	go func() {
		count := 0
		for range fast {
			count++
		}
		fastGot <- count
	}()

	for i := 0; i < n; i++ {
		b.Publish(jobID, "x")
	}
	b.Close(jobID)

	if c := <-fastGot; c != n {
		t.Fatalf("fast subscriber got %d lines, want %d", c, n)
	}
	slowCount := 0
	for range slow {
		slowCount++
	}
	if slowCount != subscriberBufferSize {
		t.Fatalf("slow subscriber got %d lines, want %d buffered (rest dropped)", slowCount, subscriberBufferSize)
	}
}

func TestConcurrentPublishDoesNotRace(t *testing.T) {
	b := NewBroker()
	jobID := uuid.New()
	ch := b.Subscribe(jobID)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Enough total to overflow the buffer a few hundred times.
			for i := 0; i < subscriberBufferSize/8+100; i++ {
				b.Publish(jobID, "x")
			}
		}()
	}
	wg.Wait()
	b.Close(jobID)
	n := 0
	for range ch {
		n++
	}
	if n == 0 || n > subscriberBufferSize {
		t.Fatalf("delivered %d lines, want 1..%d", n, subscriberBufferSize)
	}
}
