package handlers

import (
	"bytes"
	"testing"
)

func TestWriteSSEEventFormatsMultilineData(t *testing.T) {
	var out bytes.Buffer

	writeSSEData(&out, "line one\nline two\n")

	const want = "data: line one\ndata: line two\ndata: \n\n"
	if got := out.String(); got != want {
		t.Fatalf("unexpected SSE data frame:\nwant %q\ngot  %q", want, got)
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
