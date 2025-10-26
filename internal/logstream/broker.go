package logstream

import (
	"sync"

	"github.com/google/uuid"
)

// LogBroker manages log streams for jobs
type LogBroker struct {
	subscribers map[uuid.UUID]map[chan string]bool // jobID -> set of subscriber channels
	mu          sync.RWMutex
}

// NewBroker creates a new log broker
func NewBroker() *LogBroker {
	return &LogBroker{
		subscribers: make(map[uuid.UUID]map[chan string]bool),
	}
}

// Subscribe creates a new subscription for a job's logs
func (b *LogBroker) Subscribe(jobID uuid.UUID) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan string, 100) // Buffered channel to prevent blocking

	if b.subscribers[jobID] == nil {
		b.subscribers[jobID] = make(map[chan string]bool)
	}
	b.subscribers[jobID][ch] = true

	return ch
}

// Unsubscribe removes a subscription
func (b *LogBroker) Unsubscribe(jobID uuid.UUID, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, exists := b.subscribers[jobID]; exists {
		delete(subs, ch)
		close(ch)

		// Clean up if no more subscribers for this job
		if len(subs) == 0 {
			delete(b.subscribers, jobID)
		}
	}
}

// Publish sends a log line to all subscribers of a job
func (b *LogBroker) Publish(jobID uuid.UUID, line string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if subs, exists := b.subscribers[jobID]; exists {
		for ch := range subs {
			// Non-blocking send - drop if channel is full
			select {
			case ch <- line:
			default:
				// Channel full, log is dropped
			}
		}
	}
}

// Close closes all subscriptions for a job
func (b *LogBroker) Close(jobID uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, exists := b.subscribers[jobID]; exists {
		for ch := range subs {
			close(ch)
		}
		delete(b.subscribers, jobID)
	}
}

// HasSubscribers returns true if there are active subscribers for a job
func (b *LogBroker) HasSubscribers(jobID uuid.UUID) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.subscribers[jobID]
	return exists && len(subs) > 0
}
