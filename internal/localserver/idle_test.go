package localserver

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleTimer_ExpiresAfterTimeout(t *testing.T) {
	var expired atomic.Bool

	timer := NewIdleTimer(50*time.Millisecond, func() {
		expired.Store(true)
	})
	defer timer.Stop()

	// Wait for expiry.
	time.Sleep(100 * time.Millisecond)

	if !expired.Load() {
		t.Error("Timer should have expired")
	}
}

func TestIdleTimer_ResetPreventsExpiry(t *testing.T) {
	var expired atomic.Bool

	timer := NewIdleTimer(80*time.Millisecond, func() {
		expired.Store(true)
	})
	defer timer.Stop()

	// Reset before it expires.
	time.Sleep(50 * time.Millisecond)
	timer.Reset()

	// Wait past the original expiry time but not the reset one.
	time.Sleep(50 * time.Millisecond)

	if expired.Load() {
		t.Error("Timer should not have expired after reset")
	}

	// Now wait for it to actually expire.
	time.Sleep(50 * time.Millisecond)

	if !expired.Load() {
		t.Error("Timer should have expired after reset timeout")
	}
}

func TestIdleTimer_StopPreventsExpiry(t *testing.T) {
	var expired atomic.Bool

	timer := NewIdleTimer(50*time.Millisecond, func() {
		expired.Store(true)
	})

	timer.Stop()

	time.Sleep(100 * time.Millisecond)

	if expired.Load() {
		t.Error("Timer should not expire after Stop()")
	}
}
