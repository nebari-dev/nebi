package localserver

import (
	"net/http"
	"sync"
	"time"
)

// DefaultIdleTimeout is the duration of inactivity before the local server shuts down.
const DefaultIdleTimeout = 15 * time.Minute

// IdleTimer tracks API activity and triggers shutdown after a period of inactivity.
type IdleTimer struct {
	timeout  time.Duration
	timer    *time.Timer
	mu       sync.Mutex
	onExpire func()
}

// NewIdleTimer creates a new idle timer that calls onExpire after the timeout elapses
// without any activity.
func NewIdleTimer(timeout time.Duration, onExpire func()) *IdleTimer {
	it := &IdleTimer{
		timeout:  timeout,
		onExpire: onExpire,
	}
	it.timer = time.AfterFunc(timeout, onExpire)
	return it
}

// Reset resets the idle timer, restarting the countdown.
func (it *IdleTimer) Reset() {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.timer.Reset(it.timeout)
}

// Stop stops the idle timer.
func (it *IdleTimer) Stop() {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.timer.Stop()
}

// IdleTimeoutMiddleware returns an HTTP middleware that resets the idle timer on every request.
func IdleTimeoutMiddleware(idleTimer *IdleTimer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idleTimer.Reset()
		next.ServeHTTP(w, r)
	})
}
