package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/worker"
)

func TestShutdownWaitsForWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		defer cancelWorker()
		workerDone := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			result <- shutdown(context.Background(), &http.Server{}, cancelWorker, workerDone)
		}()
		synctest.Wait()
		if workerCtx.Err() != context.Canceled {
			t.Fatal("worker was not cancelled")
		}
		select {
		case err := <-result:
			t.Fatalf("shutdown returned before worker cleanup: %v", err)
		default:
		}
		close(workerDone)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	})
}

func TestShutdownBoundsWorkerWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := shutdown(ctx, &http.Server{}, func() {}, make(chan struct{}))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected a bounded worker wait, got %v", err)
		}
	})
}

func TestResolveBindHost(t *testing.T) {
	tests := []struct {
		name      string
		localMode bool
		host      string
		want      string
	}{
		{name: "local mode defaults to loopback", localMode: true, host: "", want: "127.0.0.1"},
		{name: "local mode whitespace defaults to loopback", localMode: true, host: "  ", want: "127.0.0.1"},
		{name: "local mode explicit host respected", localMode: true, host: "0.0.0.0", want: "0.0.0.0"},
		{name: "team mode keeps all-interface default", localMode: false, host: "", want: ""},
		{name: "team mode explicit host respected", localMode: false, host: "10.0.0.5", want: "10.0.0.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBindHost(tt.localMode, tt.host)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestListenAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "empty host uses all interfaces", host: "", port: 8460, want: ":8460"},
		{name: "whitespace host uses all interfaces", host: "   ", port: 9000, want: ":9000"},
		{name: "ipv4 host", host: "127.0.0.1", port: 8460, want: "127.0.0.1:8460"},
		{name: "ipv6 host", host: "::1", port: 8460, want: "[::1]:8460"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listenAddress(tt.host, tt.port)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDisplayHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "empty host", host: "", want: "localhost"},
		{name: "whitespace host", host: "  ", want: "localhost"},
		{name: "all interfaces ipv4", host: "0.0.0.0", want: "localhost"},
		{name: "all interfaces ipv6", host: "::", want: "localhost"},
		{name: "loopback ipv4", host: "127.0.0.1", want: "127.0.0.1"},
		{name: "loopback ipv6", host: "::1", want: "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayHost(tt.host)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestServerURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		basePath string
		want     string
	}{
		{name: "empty host defaults to localhost display", host: "", port: 8460, want: "http://localhost:8460"},
		{name: "ipv4 host", host: "127.0.0.1", port: 8460, want: "http://127.0.0.1:8460"},
		{name: "ipv6 host is bracketed", host: "::1", port: 8460, want: "http://[::1]:8460"},
		{name: "all interfaces display localhost", host: "0.0.0.0", port: 9000, basePath: "/api/v1", want: "http://localhost:9000/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serverURL(tt.host, tt.port, tt.basePath)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// A pending job has no worker output to end its HTTP stream. Worker shutdown
// must close that subscription so http.Server.Shutdown can drain the request.
func TestShutdownEndsPendingJobStream(t *testing.T) {
	q := queue.NewMemoryQueue(1)
	defer q.Close()
	w := worker.New(q, nil, nil, nil, slog.Default(), limits.Defaults())
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); _ = w.Start(workerCtx) }()
	subscribed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		stream := w.GetBroker().Subscribe(uuid.New())
		close(subscribed)
		select {
		case <-stream:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	requestCtx, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRequest()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		response, err := srv.Client().Do(request)
		if err == nil {
			_, err = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		result <- err
	}()
	select {
	case <-subscribed:
	case <-requestCtx.Done():
		t.Fatal("log request did not subscribe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdown(ctx, srv.Config, cancelWorker, workerDone); err != nil {
		t.Fatalf("shutdown with pending log stream: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("log request: %v", err)
	}
}
