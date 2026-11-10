package logstream

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	// subscriberBufferSize is the number of log lines buffered per subscriber.
	// pixi emits output in bursts of many lines; a small buffer caused lines
	// to be dropped before the SSE writer could drain them.
	subscriberBufferSize = 4096
)

// subscriber holds a subscriber's channel and how many lines have been
// dropped for it since the last delivered drop marker.
type subscriber struct {
	ch      chan string
	mu      sync.Mutex // serializes Publish per subscriber so marker + line ordering and dropped are consistent
	dropped int
}

// jobHistory is everything published for a tracked job so far, so a late
// subscriber can catch up without a gap or duplicate lines.
type jobHistory struct {
	mu  sync.Mutex
	buf strings.Builder
}

// LogBroker manages log streams for jobs
type LogBroker struct {
	subscribers map[uuid.UUID]map[chan string]*subscriber // jobID -> set of subscribers
	history     map[uuid.UUID]*jobHistory                 // jobs opened with Open
	mu          sync.RWMutex
	closed      bool
}

// NewBroker creates a new log broker
func NewBroker() *LogBroker {
	return &LogBroker{
		subscribers: make(map[uuid.UUID]map[chan string]*subscriber),
		history:     make(map[uuid.UUID]*jobHistory),
	}
}

// Open starts recording the log history of a job that is about to run in
// this process. SubscribeWithHistory only succeeds for opened jobs; Close
// drops the history. The worker opens every job it runs.
func (b *LogBroker) Open(jobID uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	if _, ok := b.history[jobID]; !ok {
		b.history[jobID] = &jobHistory{}
	}
}

// SubscribeWithHistory atomically returns everything published so far for
// an opened job and a channel for what follows, so the caller sees every
// byte exactly once. ok is false (and ch nil) when the job is not running
// in this process: it was never opened, or it already finished.
func (b *LogBroker) SubscribeWithHistory(jobID uuid.UUID) (history string, ch chan string, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h, tracked := b.history[jobID]
	if !tracked || b.closed {
		return "", nil, false
	}
	h.mu.Lock()
	history = h.buf.String()
	h.mu.Unlock()
	ch = make(chan string, subscriberBufferSize)
	if b.subscribers[jobID] == nil {
		b.subscribers[jobID] = make(map[chan string]*subscriber)
	}
	b.subscribers[jobID][ch] = &subscriber{ch: ch}
	return history, ch, true
}

// Subscribe creates a new subscription for a job's logs
func (b *LogBroker) Subscribe(jobID uuid.UUID) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan string, subscriberBufferSize)
	if b.closed {
		close(ch)
		return ch
	}

	if b.subscribers[jobID] == nil {
		b.subscribers[jobID] = make(map[chan string]*subscriber)
	}
	b.subscribers[jobID][ch] = &subscriber{ch: ch}

	return ch
}

// Unsubscribe removes a subscription
func (b *LogBroker) Unsubscribe(jobID uuid.UUID, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, exists := b.subscribers[jobID]; exists {
		if _, ok := subs[ch]; ok {
			delete(subs, ch)
			close(ch)
		}

		// Clean up if no more subscribers for this job
		if len(subs) == 0 {
			delete(b.subscribers, jobID)
		}
	}
}

// Publish sends a log line to all subscribers of a job.
//
// Publish never blocks. If a subscriber's buffer is full, the line is
// dropped for that subscriber and counted; the next successful delivery to
// that subscriber is preceded by a marker line reporting how many lines
// were lost, so the gap is visible to the client instead of silent. Publish
// runs synchronously in the job's stdout writer, so a subscriber that has
// fallen subscriberBufferSize lines behind is not worth stalling the job for.
func (b *LogBroker) Publish(jobID uuid.UUID, line string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if h, tracked := b.history[jobID]; tracked {
		h.mu.Lock()
		h.buf.WriteString(line)
		h.mu.Unlock()
	}

	subs, exists := b.subscribers[jobID]
	if !exists {
		return
	}

	for _, sub := range subs {
		b.publishTo(sub, line)
	}
}

// publishTo delivers a line to one subscriber, emitting a drop marker first
// if earlier lines were lost.
func (b *LogBroker) publishTo(sub *subscriber, line string) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.dropped > 0 {
		marker := fmt.Sprintf("\n[nebi] %d log lines dropped because the client could not keep up; refresh to see the full log\n", sub.dropped)
		if !trySend(sub.ch, marker) {
			// Still no room; drop this line too and keep the counter growing.
			sub.dropped++
			return
		}
		sub.dropped = 0
	}
	if !trySend(sub.ch, line) {
		sub.dropped++
	}
}

// trySend attempts a non-blocking send and returns false if the channel
// buffer is full.
func trySend(ch chan string, line string) bool {
	select {
	case ch <- line:
		return true
	default:
		return false
	}
}

// Close closes all subscriptions for a job and drops its history.
func (b *LogBroker) Close(jobID uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.history, jobID)

	if subs, exists := b.subscribers[jobID]; exists {
		for ch := range subs {
			close(ch)
		}
		delete(b.subscribers, jobID)
	}
}

// Shutdown ends every stream, including pending jobs that will not run. New
// subscriptions are closed immediately so in-flight HTTP requests can finish.
func (b *LogBroker) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	clear(b.history)
	for _, subs := range b.subscribers {
		for ch := range subs {
			close(ch)
		}
	}
	clear(b.subscribers)
}

// HasSubscribers returns true if there are active subscribers for a job
func (b *LogBroker) HasSubscribers(jobID uuid.UUID) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.subscribers[jobID]
	return exists && len(subs) > 0
}
