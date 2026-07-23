package cliclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamJobLogs_WritesDataLinesUntilDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs/j-1/logs/stream" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("missing bearer token, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: line one\n\n")
		fmt.Fprintf(w, "data: line two\n\n")
		fmt.Fprintf(w, "event: done\ndata: Job completed\n\n")
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var out bytes.Buffer
	if err := c.StreamJobLogs(context.Background(), "j-1", &out); err != nil {
		t.Fatalf("StreamJobLogs: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Errorf("missing log lines in output: %q", got)
	}
	if strings.Contains(got, "Job completed") {
		t.Errorf("done-event payload should not appear in log output: %q", got)
	}
}

func TestStreamJobLogs_ReturnsErrorOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"nope"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var out bytes.Buffer
	if err := c.StreamJobLogs(context.Background(), "j-1", &out); err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
	}
}
