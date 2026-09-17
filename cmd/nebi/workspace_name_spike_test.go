package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/pixi"
	"github.com/nebari-dev/nebi/internal/store"
)

func TestWorkspaceNameSpikeLocal(t *testing.T) {
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	manifest := "[workspace]\nchannels = []\nplatforms = [\"linux-64\"]\n"
	for _, dir := range []string{"server-copy", "oci-copy", "directory-default"} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		name := "analysis"
		if dir == "directory-default" {
			name = ""
		}
		if err := ensureInitWithName(dir, name); err != nil {
			t.Fatal(err)
		}
	}
	s, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	wss, err := s.FindWorkspacesByName("analysis")
	if err != nil || len(wss) != 2 || wss[0].ID == wss[1].ID {
		t.Fatalf("duplicate names: %+v, %v", wss, err)
	}
	if _, err := s.FindWorkspaceByName("analysis"); err == nil {
		t.Fatal("ambiguous names must not select the first workspace")
	}
	ws, err := resolveLocalWorkspace(s, "./server-copy")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ListVersions(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspaceRenameCmd.RunE(workspaceRenameCmd, []string{"id::" + ws.ID.String(), "my-label"}); err != nil {
		t.Fatal(err)
	}
	gotManifest, err := os.ReadFile(filepath.Join(ws.Path, "pixi.toml"))
	if err != nil || string(gotManifest) != manifest {
		t.Fatalf("rename changed manifest: %q, %v", gotManifest, err)
	}
	// Re-import/tracking and reads must preserve the independent label even
	// after the Pixi author changes its optional name.
	if err := os.WriteFile(filepath.Join(ws.Path, "pixi.toml"), []byte(manifest+"name = \"edited-in-pixi\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureInitWithName(ws.Path, "upstream-label"); err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceListLocal(); err != nil {
		t.Fatal(err)
	}
	renamed, err := s.GetWorkspace(ws.ID)
	if err != nil || renamed.Name != "my-label" || renamed.Path != ws.Path {
		t.Fatalf("label/path changed: %+v, %v", renamed, err)
	}
	after, err := s.ListVersions(ws.ID)
	if err != nil || len(before) != 1 || len(after) != 1 || before[0].ID != after[0].ID {
		t.Fatalf("rename changed version history: before=%+v after=%+v err=%v", before, after, err)
	}
	if ws, err := s.FindWorkspaceByName("directory-default"); err != nil || ws == nil {
		t.Fatalf("directory fallback failed: %+v, %v", ws, err)
	}
}

func TestWorkspaceNameSpikeRemote(t *testing.T) {
	first, second := uuid.NewString(), uuid.NewString()
	mode := "duplicates"
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "unexpected create", http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/api/v1/workspaces/"+second {
			_ = json.NewEncoder(w).Encode(cliclient.Workspace{ID: second, Name: "analysis"})
			return
		}
		if mode == "unreachable" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode([]cliclient.Workspace{{ID: first, Name: "analysis"}, {ID: second, Name: "analysis"}})
	}))
	defer srv.Close()
	client := cliclient.New(srv.URL, "test-token")
	if _, err := findWsByName(client, context.Background(), "analysis"); err == nil || errors.Is(err, ErrWsNotFound) {
		t.Fatalf("expected ambiguity, not absence: %v", err)
	}
	ws, err := findWsByName(client, context.Background(), "id::"+second)
	if err != nil || ws.ID != second {
		t.Fatalf("UUID selection: %+v, %v", ws, err)
	}
	if _, err := findWsByName(client, context.Background(), "missing"); !errors.Is(err, ErrWsNotFound) {
		t.Fatalf("expected not found: %v", err)
	}
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	t.Setenv("NEBI_REMOTE_URL", srv.URL)
	t.Setenv("NEBI_AUTH_TOKEN", "test-token")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("pixi.toml", []byte("[workspace]\nchannels = []\nplatforms = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"duplicates", "unreachable"} {
		mode = scenario
		if err := runPush(pushCmd, []string{"analysis"}); err == nil {
			t.Fatalf("push should reject %s", scenario)
		}
	}
	if posts != 0 {
		t.Fatalf("push created %d workspaces after a lookup error", posts)
	}
}

func TestWorkspaceNameSpikeRejectsInvalidLabel(t *testing.T) {
	_, err := pixi.InitialWorkspaceName("invalid/label", t.TempDir(), "[workspace]\n")
	if err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("expected name validation: %v", err)
	}
}

func TestWorkspaceNameSpikeCLIImport(t *testing.T) {
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	manifest := "[workspace]\nchannels = []\nplatforms = [\"linux-64\"]\n"
	for file, content := range map[string]string{"pixi.toml": manifest, "pixi.lock": "version: 6\n"} {
		if err := os.WriteFile(filepath.Join(src, file), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := oci.Publish(context.Background(), src, oci.Registry{Host: u.Host, Namespace: "demo", PlainHTTP: true}, "analysis", "v1"); err != nil {
		t.Fatal(err)
	}
	oldOutput, oldForce := importOutput, importForce
	t.Cleanup(func() {
		importOutput, importForce = oldOutput, oldForce
		_ = importCmd.Flags().Set("name", "")
	})
	s, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, label := range []string{"", "personal-label"} {
		importOutput, importForce = t.TempDir(), true
		if err := importCmd.Flags().Set("name", label); err != nil {
			t.Fatal(err)
		}
		if err := runImport(importCmd, []string{srv.URL + "/demo/analysis:v1"}); err != nil {
			t.Fatal(err)
		}
		want := label
		if want == "" {
			want = "analysis"
		}
		ws, err := s.FindWorkspaceByPath(importOutput)
		if err != nil || ws == nil || ws.Name != want {
			t.Fatalf("import name: %+v, %v", ws, err)
		}
		got, err := os.ReadFile(filepath.Join(importOutput, "pixi.toml"))
		if err != nil || string(got) != manifest {
			t.Fatalf("import changed manifest: %q, %v", got, err)
		}
	}
}

func TestWorkspaceNameSpikeCLIPull(t *testing.T) {
	id := uuid.NewString()
	manifest := "[workspace]\nname = \"pixi-label\"\nchannels = []\nplatforms = [\"linux-64\"]\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspaces/" + id:
			_ = json.NewEncoder(w).Encode(cliclient.Workspace{ID: id, Name: "server-label"})
		case "/api/v1/workspaces/" + id + "/tags":
			_, _ = w.Write([]byte(`[{"tag":"v1","version_number":1}]`))
		case "/api/v1/workspaces/" + id + "/versions/1/pixi-toml":
			_, _ = w.Write([]byte(manifest))
		case "/api/v1/workspaces/" + id + "/versions/1/pixi-lock":
			_, _ = w.Write([]byte("version: 6\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	t.Setenv("NEBI_REMOTE_URL", srv.URL)
	t.Setenv("NEBI_AUTH_TOKEN", "test-token")
	s, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	oldOutput, oldForce := pullOutput, pullForce
	t.Cleanup(func() {
		pullOutput, pullForce = oldOutput, oldForce
		_ = pullCmd.Flags().Set("name", "")
	})
	for _, label := range []string{"", "personal-label"} {
		pullOutput, pullForce = t.TempDir(), true
		t.Chdir(pullOutput)
		if err := pullCmd.Flags().Set("name", label); err != nil {
			t.Fatal(err)
		}
		if err := runPull(pullCmd, []string{"id::" + id + ":v1"}); err != nil {
			t.Fatal(err)
		}
		want := label
		if want == "" {
			want = "server-label"
		}
		ws, err := s.FindWorkspaceByPath(pullOutput)
		if err != nil || ws == nil || ws.Name != want {
			t.Fatalf("pull name: %+v, %v", ws, err)
		}
		if ws.OriginID != id || ws.OriginName != "server-label" {
			t.Fatalf("origin should retain remote ID and display name: %+v", ws)
		}
		got, err := os.ReadFile(filepath.Join(pullOutput, "pixi.toml"))
		if err != nil || string(got) != manifest {
			t.Fatalf("pull changed manifest: %q, %v", got, err)
		}
	}
}
