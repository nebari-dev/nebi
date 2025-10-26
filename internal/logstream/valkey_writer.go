package logstream

import (
	"context"
	"fmt"

	"github.com/valkey-io/valkey-go"
)

// ValkeyLogWriter publishes log lines to Valkey pub/sub channel
// Implements io.Writer interface for use with log streaming
type ValkeyLogWriter struct {
	client valkey.Client
	jobID  string
	ctx    context.Context
}

// NewValkeyLogWriter creates a new Valkey log publisher
func NewValkeyLogWriter(client valkey.Client, jobID string) *ValkeyLogWriter {
	return &ValkeyLogWriter{
		client: client,
		jobID:  jobID,
		ctx:    context.Background(),
	}
}

// Write implements io.Writer - publishes log line to Valkey
func (w *ValkeyLogWriter) Write(p []byte) (n int, err error) {
	channel := fmt.Sprintf("logs:%s", w.jobID)
	cmd := w.client.B().Publish().Channel(channel).Message(string(p)).Build()

	if err := w.client.Do(w.ctx, cmd).Error(); err != nil {
		// Don't fail on log publish errors, just log them
		// This ensures job execution continues even if Valkey is down
		fmt.Printf("Warning: Failed to publish log to Valkey: %v\n", err)
	}

	return len(p), nil
}

// Publish sends a message to the log channel (for completion/error messages)
func (w *ValkeyLogWriter) Publish(message string) error {
	channel := fmt.Sprintf("logs:%s", w.jobID)
	cmd := w.client.B().Publish().Channel(channel).Message(message).Build()
	return w.client.Do(w.ctx, cmd).Error()
}

// SetTTL sets the TTL on the log channel for cleanup
func (w *ValkeyLogWriter) SetTTL(seconds int64) error {
	channel := fmt.Sprintf("logs:%s", w.jobID)
	cmd := w.client.B().Expire().Key(channel).Seconds(seconds).Build()
	return w.client.Do(w.ctx, cmd).Error()
}
