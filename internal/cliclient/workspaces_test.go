package cliclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPublishDefaultsSendsExplicitRegistryID(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"registry_id":"00000000-0000-0000-0000-000000000001"}`))
	}))
	defer server.Close()

	client := New(server.URL, "token")
	_, err := client.GetPublishDefaults(context.Background(), "workspace-id", "registry-id")
	if err != nil {
		t.Fatalf("GetPublishDefaults: %v", err)
	}

	wantPath := "/api/v1/workspaces/workspace-id/publish-defaults?registry_id=registry-id"
	if gotPath != wantPath {
		t.Fatalf("path mismatch: got %q want %q", gotPath, wantPath)
	}
}
