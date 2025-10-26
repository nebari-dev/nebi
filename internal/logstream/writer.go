package logstream

import (
	"io"

	"github.com/google/uuid"
)

// StreamWriter is an io.Writer that broadcasts to both a buffer and the broker
type StreamWriter struct {
	jobID  uuid.UUID
	broker *LogBroker
	buffer io.Writer
}

// NewStreamWriter creates a writer that broadcasts to the broker and writes to a buffer
func NewStreamWriter(jobID uuid.UUID, broker *LogBroker, buffer io.Writer) *StreamWriter {
	return &StreamWriter{
		jobID:  jobID,
		broker: broker,
		buffer: buffer,
	}
}

// Write implements io.Writer interface
func (w *StreamWriter) Write(p []byte) (n int, err error) {
	// Write to buffer first
	n, err = w.buffer.Write(p)
	if err != nil {
		return n, err
	}

	// Broadcast to subscribers
	line := string(p)
	w.broker.Publish(w.jobID, line)

	return n, nil
}
