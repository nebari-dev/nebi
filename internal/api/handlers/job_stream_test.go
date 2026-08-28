package handlers

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/logstream"
)

// parseSSE decodes an SSE byte stream the way a browser EventSource does:
// consecutive "data:" lines are joined with "\n" into one message, and a
// blank line terminates the message.
func parseSSE(t *testing.T, body string) (messages []string, events []string) {
	t.Helper()
	var data []string
	var event string
	flush := func() {
		if len(data) == 0 && event == "" {
			return
		}
		messages = append(messages, strings.Join(data, "\n"))
		events = append(events, event)
		data, event = nil, ""
	}
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "data: "):
			data = append(data, strings.TrimPrefix(line, "data: "))
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		default:
			t.Fatalf("malformed SSE line (would be discarded by the client): %q", line)
		}
	}
	flush()
	return messages, events
}

// TestStreamLogsFromBrokerPreservesMultilineBurst reproduces the bug in
// nebari-dev/nebi#419: pixi emits log chunks containing embedded newlines,
// in bursts. Every line must reach the client with a data: prefix, and a
// burst larger than the old 100-entry buffer must not drop lines.
func TestStreamLogsFromBrokerPreservesMultilineBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broker := logstream.NewBroker()
	jobID := uuid.New()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/jobs/"+jobID.String()+"/logs/stream", nil)

	const burst = 1000
	want := make([]string, 0, burst)
	for i := 0; i < burst; i++ {
		want = append(want, fmt.Sprintf("chunk %d line a\nchunk %d line b\r\n  chunk %d line c", i, i, i))
	}

	// Subscribe synchronously so Publish has a subscriber, then publish the
	// whole burst before the consumer goroutine drains anything, which is
	// the worst case for the broker's non-blocking send.
	ch := broker.Subscribe(jobID)
	for _, line := range want {
		broker.Publish(jobID, line)
	}
	broker.Close(jobID)

	// Drive the handler's per-line writer over the pre-filled channel.
	for line := range ch {
		writeSSEData(c.Writer, line)
	}
	writeSSEEvent(c.Writer, "done", "Stream ended")

	messages, events := parseSSE(t, rec.Body.String())
	if len(messages) != burst+1 {
		t.Fatalf("got %d SSE messages, want %d log messages + 1 done event (lines dropped?)", len(messages), burst+1)
	}
	for i, w := range want {
		wantNorm := strings.ReplaceAll(w, "\r\n", "\n")
		if messages[i] != wantNorm {
			t.Fatalf("message %d mismatch:\nwant %q\ngot  %q", i, wantNorm, messages[i])
		}
	}
	if events[burst] != "done" {
		t.Fatalf("last event = %q, want done", events[burst])
	}
}

// TestStreamLogsFromBrokerEndToEnd runs the real handler loop against a
// recorder with a producer publishing multiline chunks concurrently.
func TestStreamLogsFromBrokerEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broker := logstream.NewBroker()
	h := &JobHandler{broker: broker}
	jobID := uuid.New()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/jobs/"+jobID.String()+"/logs/stream", nil)

	const n = 500
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.streamLogsFromBroker(c, jobID)
	}()

	// Wait for the handler to subscribe before publishing.
	for !broker.HasSubscribers(jobID) {
		runtime.Gosched()
	}
	for i := 0; i < n; i++ {
		broker.Publish(jobID, fmt.Sprintf("Installing package %d\n  - dependency resolved", i))
	}
	broker.Close(jobID)
	<-done

	messages, events := parseSSE(t, rec.Body.String())
	if len(messages) != n+1 {
		t.Fatalf("got %d SSE messages, want %d + done", len(messages), n+1)
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("Installing package %d\n  - dependency resolved", i)
		if messages[i] != want {
			t.Fatalf("message %d:\nwant %q\ngot  %q", i, want, messages[i])
		}
	}
	if events[n] != "done" {
		t.Fatalf("final event = %q, want done", events[n])
	}
}
