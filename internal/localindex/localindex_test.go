package localindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	store := NewStoreWithDir(dir)
	return store, dir
}

func TestNewStore(t *testing.T) {
	store := NewStore()
	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, DefaultIndexDir, IndexFileName)
	if store.IndexPath() != expected {
		t.Errorf("IndexPath() = %q, want %q", store.IndexPath(), expected)
	}
}

func TestNewStoreWithDir(t *testing.T) {
	store := NewStoreWithDir("/tmp/test-nebi")
	expected := "/tmp/test-nebi/index.json"
	if store.IndexPath() != expected {
		t.Errorf("IndexPath() = %q, want %q", store.IndexPath(), expected)
	}
}

func TestLoadEmptyIndex(t *testing.T) {
	store, _ := setupTestStore(t)

	idx, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", idx.Version, CurrentVersion)
	}
	if len(idx.Workspaces) != 0 {
		t.Errorf("Workspaces length = %d, want 0", len(idx.Workspaces))
	}
	if idx.Aliases == nil {
		t.Error("Aliases should not be nil")
	}
}

func TestSaveAndLoad(t *testing.T) {
	store, _ := setupTestStore(t)

	now := time.Now().Truncate(time.Second)
	idx := &Index{
		Version: CurrentVersion,
		Workspaces: []WorkspaceEntry{
			{
				Workspace:       "data-science",
				Tag:             "v1.0",
				Registry:        "ds-team",
				ServerURL:       "https://nebi.example.com",
				ServerVersionID: 42,
				Path:            "/home/user/project-a",
				IsGlobal:        false,
				PulledAt:        now,
				ManifestDigest:  "sha256:abc123",
				Layers: map[string]string{
					"pixi.toml": "sha256:111",
					"pixi.lock": "sha256:222",
				},
			},
		},
		Aliases: map[string]Alias{
			"ds-stable": {UUID: "550e8400-e29b-41d4-a716-446655440000", Tag: "v1.0"},
		},
	}

	if err := store.Save(idx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CurrentVersion)
	}
	if len(loaded.Workspaces) != 1 {
		t.Fatalf("Workspaces length = %d, want 1", len(loaded.Workspaces))
	}

	ws := loaded.Workspaces[0]
	if ws.Workspace != "data-science" {
		t.Errorf("Workspace = %q, want %q", ws.Workspace, "data-science")
	}
	if ws.Tag != "v1.0" {
		t.Errorf("Tag = %q, want %q", ws.Tag, "v1.0")
	}
	if ws.Registry != "ds-team" {
		t.Errorf("Registry = %q, want %q", ws.Registry, "ds-team")
	}
	if ws.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", ws.ServerURL, "https://nebi.example.com")
	}
	if ws.ServerVersionID != 42 {
		t.Errorf("ServerVersionID = %d, want 42", ws.ServerVersionID)
	}
	if ws.Path != "/home/user/project-a" {
		t.Errorf("Path = %q, want %q", ws.Path, "/home/user/project-a")
	}
	if ws.IsGlobal {
		t.Error("IsGlobal should be false")
	}
	if ws.ManifestDigest != "sha256:abc123" {
		t.Errorf("ManifestDigest = %q, want %q", ws.ManifestDigest, "sha256:abc123")
	}
	if ws.Layers["pixi.toml"] != "sha256:111" {
		t.Errorf("Layers[pixi.toml] = %q, want %q", ws.Layers["pixi.toml"], "sha256:111")
	}
	if ws.Layers["pixi.lock"] != "sha256:222" {
		t.Errorf("Layers[pixi.lock] = %q, want %q", ws.Layers["pixi.lock"], "sha256:222")
	}

	// Check alias
	alias, exists := loaded.Aliases["ds-stable"]
	if !exists {
		t.Fatal("Alias 'ds-stable' not found")
	}
	if alias.UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("Alias UUID = %q, want %q", alias.UUID, "550e8400-e29b-41d4-a716-446655440000")
	}
	if alias.Tag != "v1.0" {
		t.Errorf("Alias Tag = %q, want %q", alias.Tag, "v1.0")
	}
}

func TestAddEntry(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	entry := WorkspaceEntry{
		Workspace:       "data-science",
		Tag:             "v1.0",
		ServerURL:       "https://nebi.example.com",
		ServerVersionID: 42,
		Path:            "/home/user/project-a",
		PulledAt:        now,
	}

	if err := store.AddEntry(entry); err != nil {
		t.Fatalf("AddEntry() error = %v", err)
	}

	entries, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListAll() length = %d, want 1", len(entries))
	}
	if entries[0].Workspace != "data-science" {
		t.Errorf("Workspace = %q, want %q", entries[0].Workspace, "data-science")
	}
}

func TestAddEntryReplacesSamePath(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	entry1 := WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v1.0",
		Path:      "/home/user/project-a",
		PulledAt:  now,
	}
	entry2 := WorkspaceEntry{
		Workspace: "data-science",
		Tag:       "v2.0",
		Path:      "/home/user/project-a",
		PulledAt:  now.Add(time.Hour),
	}

	if err := store.AddEntry(entry1); err != nil {
		t.Fatalf("AddEntry(entry1) error = %v", err)
	}
	if err := store.AddEntry(entry2); err != nil {
		t.Fatalf("AddEntry(entry2) error = %v", err)
	}

	entries, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListAll() length = %d, want 1", len(entries))
	}
	if entries[0].Tag != "v2.0" {
		t.Errorf("Tag = %q, want %q (should be replaced)", entries[0].Tag, "v2.0")
	}
}

func TestAddMultipleEntries(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	entries := []WorkspaceEntry{
		{Workspace: "ws1", Tag: "v1.0", Path: "/path/a", PulledAt: now},
		{Workspace: "ws2", Tag: "v1.0", Path: "/path/b", PulledAt: now},
		{Workspace: "ws1", Tag: "v1.0", Path: "/path/c", PulledAt: now},
	}

	for _, e := range entries {
		if err := store.AddEntry(e); err != nil {
			t.Fatalf("AddEntry() error = %v", err)
		}
	}

	all, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListAll() length = %d, want 3", len(all))
	}
}

func TestRemoveByPath(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/path/a", PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws2", Tag: "v1.0", Path: "/path/b", PulledAt: now})

	removed, err := store.RemoveByPath("/path/a")
	if err != nil {
		t.Fatalf("RemoveByPath() error = %v", err)
	}
	if !removed {
		t.Error("RemoveByPath() should return true")
	}

	entries, _ := store.ListAll()
	if len(entries) != 1 {
		t.Fatalf("ListAll() length = %d, want 1", len(entries))
	}
	if entries[0].Path != "/path/b" {
		t.Errorf("Path = %q, want %q", entries[0].Path, "/path/b")
	}
}

func TestRemoveByPathNotFound(t *testing.T) {
	store, _ := setupTestStore(t)

	removed, err := store.RemoveByPath("/nonexistent")
	if err != nil {
		t.Fatalf("RemoveByPath() error = %v", err)
	}
	if removed {
		t.Error("RemoveByPath() should return false for nonexistent path")
	}
}

func TestFindByPath(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/path/a", PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws2", Tag: "v2.0", Path: "/path/b", PulledAt: now})

	entry, err := store.FindByPath("/path/a")
	if err != nil {
		t.Fatalf("FindByPath() error = %v", err)
	}
	if entry == nil {
		t.Fatal("FindByPath() returned nil")
	}
	if entry.Workspace != "ws1" {
		t.Errorf("Workspace = %q, want %q", entry.Workspace, "ws1")
	}
}

func TestFindByPathNotFound(t *testing.T) {
	store, _ := setupTestStore(t)

	entry, err := store.FindByPath("/nonexistent")
	if err != nil {
		t.Fatalf("FindByPath() error = %v", err)
	}
	if entry != nil {
		t.Errorf("FindByPath() = %v, want nil", entry)
	}
}

func TestFindByWorkspaceTag(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/path/a", PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/path/b", PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v2.0", Path: "/path/c", PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws2", Tag: "v1.0", Path: "/path/d", PulledAt: now})

	matches, err := store.FindByWorkspaceTag("ws1", "v1.0")
	if err != nil {
		t.Fatalf("FindByWorkspaceTag() error = %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("FindByWorkspaceTag() length = %d, want 2", len(matches))
	}
}

func TestFindGlobal(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/path/a", IsGlobal: false, PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/global/ws1/v1.0", IsGlobal: true, PulledAt: now})

	entry, err := store.FindGlobal("ws1", "v1.0")
	if err != nil {
		t.Fatalf("FindGlobal() error = %v", err)
	}
	if entry == nil {
		t.Fatal("FindGlobal() returned nil")
	}
	if !entry.IsGlobal {
		t.Error("Expected IsGlobal = true")
	}
	if entry.Path != "/global/ws1/v1.0" {
		t.Errorf("Path = %q, want %q", entry.Path, "/global/ws1/v1.0")
	}
}

func TestFindGlobalNotFound(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: "/path/a", IsGlobal: false, PulledAt: now})

	entry, err := store.FindGlobal("ws1", "v1.0")
	if err != nil {
		t.Fatalf("FindGlobal() error = %v", err)
	}
	if entry != nil {
		t.Errorf("FindGlobal() = %v, want nil", entry)
	}
}

func TestSetAlias(t *testing.T) {
	store, _ := setupTestStore(t)

	alias := Alias{UUID: "test-uuid", Tag: "v1.0"}
	if err := store.SetAlias("my-alias", alias); err != nil {
		t.Fatalf("SetAlias() error = %v", err)
	}

	got, err := store.GetAlias("my-alias")
	if err != nil {
		t.Fatalf("GetAlias() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetAlias() returned nil")
	}
	if got.UUID != "test-uuid" {
		t.Errorf("UUID = %q, want %q", got.UUID, "test-uuid")
	}
	if got.Tag != "v1.0" {
		t.Errorf("Tag = %q, want %q", got.Tag, "v1.0")
	}
}

func TestSetAliasOverwrite(t *testing.T) {
	store, _ := setupTestStore(t)

	store.SetAlias("my-alias", Alias{UUID: "uuid-1", Tag: "v1.0"})
	store.SetAlias("my-alias", Alias{UUID: "uuid-2", Tag: "v2.0"})

	got, err := store.GetAlias("my-alias")
	if err != nil {
		t.Fatalf("GetAlias() error = %v", err)
	}
	if got.UUID != "uuid-2" {
		t.Errorf("UUID = %q, want %q (should be overwritten)", got.UUID, "uuid-2")
	}
}

func TestRemoveAlias(t *testing.T) {
	store, _ := setupTestStore(t)

	store.SetAlias("my-alias", Alias{UUID: "test-uuid", Tag: "v1.0"})

	removed, err := store.RemoveAlias("my-alias")
	if err != nil {
		t.Fatalf("RemoveAlias() error = %v", err)
	}
	if !removed {
		t.Error("RemoveAlias() should return true")
	}

	got, _ := store.GetAlias("my-alias")
	if got != nil {
		t.Errorf("GetAlias() after remove = %v, want nil", got)
	}
}

func TestRemoveAliasNotFound(t *testing.T) {
	store, _ := setupTestStore(t)

	removed, err := store.RemoveAlias("nonexistent")
	if err != nil {
		t.Fatalf("RemoveAlias() error = %v", err)
	}
	if removed {
		t.Error("RemoveAlias() should return false for nonexistent alias")
	}
}

func TestListAliases(t *testing.T) {
	store, _ := setupTestStore(t)

	store.SetAlias("alias1", Alias{UUID: "uuid-1", Tag: "v1.0"})
	store.SetAlias("alias2", Alias{UUID: "uuid-2", Tag: "v2.0"})

	aliases, err := store.ListAliases()
	if err != nil {
		t.Fatalf("ListAliases() error = %v", err)
	}
	if len(aliases) != 2 {
		t.Errorf("ListAliases() length = %d, want 2", len(aliases))
	}
}

func TestPrune(t *testing.T) {
	store, dir := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	// Create a real directory for one entry
	realDir := filepath.Join(dir, "real-workspace")
	os.MkdirAll(realDir, 0755)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: realDir, PulledAt: now})
	store.AddEntry(WorkspaceEntry{Workspace: "ws2", Tag: "v1.0", Path: "/nonexistent/path", PulledAt: now})

	pruned, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("Prune() length = %d, want 1", len(pruned))
	}
	if pruned[0].Path != "/nonexistent/path" {
		t.Errorf("Pruned path = %q, want %q", pruned[0].Path, "/nonexistent/path")
	}

	entries, _ := store.ListAll()
	if len(entries) != 1 {
		t.Fatalf("ListAll() after prune = %d, want 1", len(entries))
	}
	if entries[0].Path != realDir {
		t.Errorf("Remaining path = %q, want %q", entries[0].Path, realDir)
	}
}

func TestPruneNothingToPrune(t *testing.T) {
	store, dir := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	realDir := filepath.Join(dir, "real-workspace")
	os.MkdirAll(realDir, 0755)

	store.AddEntry(WorkspaceEntry{Workspace: "ws1", Tag: "v1.0", Path: realDir, PulledAt: now})

	pruned, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if pruned != nil {
		t.Errorf("Prune() = %v, want nil (nothing to prune)", pruned)
	}
}

func TestGlobalWorkspacePath(t *testing.T) {
	store := NewStoreWithDir("/home/user/.local/share/nebi")
	path := store.GlobalWorkspacePath("550e8400-e29b-41d4-a716-446655440000", "v1.0")
	expected := "/home/user/.local/share/nebi/workspaces/550e8400-e29b-41d4-a716-446655440000/v1.0"
	if path != expected {
		t.Errorf("GlobalWorkspacePath() = %q, want %q", path, expected)
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	store, dir := setupTestStore(t)

	// Write corrupted JSON
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, IndexFileName), []byte("not json{{{"), 0644)

	_, err := store.Load()
	if err == nil {
		t.Fatal("Load() should return error for corrupted file")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep", "dir")
	store := NewStoreWithDir(dir)

	idx := &Index{Version: CurrentVersion, Workspaces: []WorkspaceEntry{}}
	if err := store.Save(idx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(store.IndexPath()); err != nil {
		t.Fatalf("Index file not created: %v", err)
	}
}

func TestIndexJSONFormat(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Date(2024, 1, 20, 10, 30, 0, 0, time.UTC)

	idx := &Index{
		Version: CurrentVersion,
		Workspaces: []WorkspaceEntry{
			{
				Workspace:       "data-science",
				Tag:             "v1.0",
				Registry:        "ds-team",
				ServerURL:       "https://nebi.example.com",
				ServerVersionID: 42,
				Path:            "/home/user/project-a",
				IsGlobal:        false,
				PulledAt:        now,
				ManifestDigest:  "sha256:abc123def456",
				Layers: map[string]string{
					"pixi.toml": "sha256:111aaa",
					"pixi.lock": "sha256:222bbb",
				},
			},
		},
		Aliases: map[string]Alias{
			"ds-stable": {UUID: "550e8400-e29b-41d4-a716-446655440000", Tag: "v1.0"},
		},
	}

	store.Save(idx)

	// Read raw JSON and verify structure
	data, err := os.ReadFile(store.IndexPath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Check version field is number
	version, ok := raw["version"].(float64)
	if !ok || version != 1 {
		t.Errorf("version = %v, want 1", raw["version"])
	}

	// Check workspaces is array
	workspaces, ok := raw["workspaces"].([]interface{})
	if !ok || len(workspaces) != 1 {
		t.Fatalf("workspaces = %v", raw["workspaces"])
	}

	// Check aliases is object
	aliases, ok := raw["aliases"].(map[string]interface{})
	if !ok || len(aliases) != 1 {
		t.Fatalf("aliases = %v", raw["aliases"])
	}
}

func TestLoadNilAliases(t *testing.T) {
	store, dir := setupTestStore(t)

	// Write JSON without aliases field
	os.MkdirAll(dir, 0755)
	data := `{"version": 1, "workspaces": []}`
	os.WriteFile(filepath.Join(dir, IndexFileName), []byte(data), 0644)

	idx, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if idx.Aliases == nil {
		t.Error("Aliases should be initialized to empty map, not nil")
	}
}
