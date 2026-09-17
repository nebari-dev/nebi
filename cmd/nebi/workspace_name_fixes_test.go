package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
)

func TestWorkspaceSelectorsKeepUUIDNames(t *testing.T) {
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	s, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &store.LocalWorkspace{Name: "ordinary", Path: t.TempDir()}
	if err := s.CreateWorkspace(a); err != nil {
		t.Fatal(err)
	}
	b := &store.LocalWorkspace{Name: a.ID.String(), Path: t.TempDir()}
	if err := s.CreateWorkspace(b); err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]*store.LocalWorkspace{a.ID.String(): b, "id::" + a.ID.String(): a} {
		got, err := resolveLocalWorkspace(s, ref)
		if err != nil || got.ID != want.ID {
			t.Fatalf("%s: got %+v, %v", ref, got, err)
		}
		dir, rest, byManifest, err := resolveWorkspaceArgs([]string{ref, "-e", "dev"})
		if err != nil || dir != want.Path || !byManifest || len(rest) != 2 {
			t.Fatalf("shell/run selector %s: %s %v %t %v", ref, dir, rest, byManifest, err)
		}
	}
	if err := workspaceRenameCmd.RunE(workspaceRenameCmd, []string{a.ID.String(), "renamed-b"}); err != nil {
		t.Fatal(err)
	}
	gotA, _ := s.GetWorkspace(a.ID)
	gotB, _ := s.GetWorkspace(b.ID)
	if gotA.Name != "ordinary" || gotB.Name != "renamed-b" {
		t.Fatalf("wrong rename: %s %s", gotA.Name, gotB.Name)
	}
	if err := runWorkspaceRemoveLocal("id::" + a.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetWorkspace(b.ID); err != nil {
		t.Fatal("ID removal affected B", err)
	}
	if _, err := resolveLocalWorkspace(s, "id::not-a-uuid"); err == nil {
		t.Fatal("invalid selector accepted")
	}
}

func TestRemoteSelectorsKeepUUIDNames(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspaces":
			_ = json.NewEncoder(w).Encode([]cliclient.Workspace{{ID: a, Name: "ordinary"}, {ID: b, Name: a}})
		case "/api/v1/workspaces/" + a:
			_ = json.NewEncoder(w).Encode(cliclient.Workspace{ID: a, Name: "ordinary"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client := cliclient.New(srv.URL, "test")
	for ref, want := range map[string]string{a: b, "id::" + a: a} {
		got, err := findWsByName(client, context.Background(), ref)
		if err != nil || got.ID != want {
			t.Fatalf("%s: %+v %v", ref, got, err)
		}
	}
	for _, tc := range []struct{ ref, name, tag string }{{"id:v1", "id", "v1"}, {"id:" + a, "id", a}, {a, a, ""}, {"id::" + a, "id::" + a, ""}, {"id::" + a + ":v1", "id::" + a, "v1"}, {a + ":v1", a, "v1"}} {
		name, tag := parseWsRef(tc.ref)
		if name != tc.name || tag != tc.tag {
			t.Fatalf("parse %s: %s %s", tc.ref, name, tag)
		}
	}
}

func TestRegistrationRejectsMalformedManifest(t *testing.T) {
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("pixi.toml", []byte("[workspace\ninvalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runInit(initCmd, nil); err == nil {
		t.Fatal("init accepted malformed TOML")
	}
	if err := ensureInitWithName(dir, "explicit-label"); err == nil {
		t.Fatal("auto-init accepted malformed TOML with explicit name")
	}
	s, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.ListWorkspaces()
	if err != nil || len(rows) != 0 {
		t.Fatalf("created rows: %+v %v", rows, err)
	}
}

func TestPullImportRejectNameBeforeTransfer(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	for _, cmd := range []*struct {
		set func(string) error
		run func() error
	}{
		{func(s string) error { return pullCmd.Flags().Set("name", s) }, func() error { return runPull(pullCmd, []string{"analysis:v1"}) }},
		{func(s string) error { return importCmd.Flags().Set("name", s) }, func() error { return runImport(importCmd, []string{"127.0.0.1:1/demo:v1"}) }},
	} {
		if err := cmd.set("bad/name"); err != nil {
			t.Fatal(err)
		}
		err := cmd.run()
		_ = cmd.set("")
		if err == nil || !strings.Contains(err.Error(), "must not contain") {
			t.Fatalf("did not reject input first: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pixi.toml")); !os.IsNotExist(err) {
		t.Fatal("manifest was written", err)
	}
}

func TestPullRejectsInvalidSourceNameBeforeTransfer(t *testing.T) {
	id := uuid.NewString()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspaces/"+id {
			t.Errorf("unexpected transfer request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(cliclient.Workspace{ID: id, Name: "invalid/label"})
	}))
	defer srv.Close()
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	t.Setenv("NEBI_REMOTE_URL", srv.URL)
	t.Setenv("NEBI_AUTH_TOKEN", "test-token")
	t.Chdir(t.TempDir())
	err := runPull(pullCmd, []string{"id::" + id + ":v1"})
	if err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("invalid source label was not rejected: %v", err)
	}
	if _, err := os.Stat("pixi.toml"); !os.IsNotExist(err) {
		t.Fatal("manifest was written", err)
	}
}
