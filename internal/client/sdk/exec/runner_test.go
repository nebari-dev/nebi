package exec

import (
	"context"
	"errors"
	"testing"
)

// TestResult checks how run reports a finished pixi command. A command
// that succeeded is a success even when the context was cancelled after
// it finished, because pixi has already done its work; a failure counts
// as a cancel only when the context is done.
func TestResult(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithTimeout(context.Background(), 0)
	defer cancelExpired()
	exitErr := errors.New("exit status 1")
	const stderr = "Error:   × something broke\n"

	if err := result(context.Background(), "pixi lock", stderr, nil); err != nil {
		t.Errorf("success: got %v", err)
	}
	if err := result(cancelled, "pixi install --frozen", stderr, nil); err != nil {
		t.Errorf("success, then cancelled: want nil, got %v", err)
	}

	err := result(cancelled, "pixi lock", stderr, exitErr)
	var cmdErr *CommandError
	if !errors.Is(err, context.Canceled) || errors.As(err, &cmdErr) {
		t.Errorf("failure while cancelled: want the context's error, got %T %v", err, err)
	}
	if err := result(expired, "pixi lock", stderr, exitErr); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("failure after the deadline: want the context's error, got %v", err)
	}

	err = result(context.Background(), "pixi lock", stderr, exitErr)
	if !errors.As(err, &cmdErr) || cmdErr.Line != "something broke" || !errors.Is(err, exitErr) {
		t.Errorf("failure: want a *CommandError carrying pixi's error, got %T %v", err, err)
	}
}
