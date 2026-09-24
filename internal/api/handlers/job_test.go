package handlers

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/logstream"
)

func TestWriteSSEEventFormatsMultilineData(t *testing.T) {
	var out bytes.Buffer

	writeSSEData(&out, "line one\nline two\n")

	const want = "data: line one\ndata: line two\ndata: \n\n"
	if got := out.String(); got != want {
		t.Fatalf("unexpected SSE data frame:\nwant %q\ngot  %q", want, got)
	}
}

func TestStreamLogsEndsWhenBrokerShutsDown(t *testing.T) {
	for _, timing := range []string{"subscribed", "arriving after shutdown"} {
		t.Run(timing, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				broker := logstream.NewBroker()
				h := NewJobHandler(nil, broker)
				response := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(response)
				c.Request = httptest.NewRequest("GET", "/logs/stream", nil)
				if timing == "arriving after shutdown" {
					broker.Shutdown()
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					h.streamLogsFromBroker(c, uuid.New())
				}()
				synctest.Wait()
				broker.Shutdown()
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("log request still blocks after broker shutdown")
				}
				if !strings.Contains(response.Body.String(), "event: done\n") {
					t.Fatalf("missing stream termination event: %q", response.Body.String())
				}
			})
		})
	}
}

func TestWriteSSEEventFormatsNamedEvent(t *testing.T) {
	var out bytes.Buffer

	writeSSEEvent(&out, "done", "Job completed")

	const want = "event: done\ndata: Job completed\n\n"
	if got := out.String(); got != want {
		t.Fatalf("unexpected SSE event frame:\nwant %q\ngot  %q", want, got)
	}
}
