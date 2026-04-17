package server

import "testing"

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
