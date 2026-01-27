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
	if len(idx.Entries) != 0 {
		t.Errorf("Entries length = %d, want 0", len(idx.Entries))
	}
}

func TestSaveAndLoad(t *testing.T) {
	store, _ := setupTestStore(t)

	now := time.Now().Truncate(time.Second)
	idx := &Index{
		Version: CurrentVersion,
		Entries: []Entry{
			{
				ID:          "test-entry-uuid",
				SpecName:    "data-science",
				SpecID:      "spec-uuid-123",
				VersionName: "v1.0",
				VersionID:   "version-uuid-456",
				ServerURL:   "https://nebi.example.com",
				ServerID:    "server-uuid-789",
				Path:        "/home/user/project-a",
				PulledAt:    now,
				Layers: map[string]string{
					"pixi.toml": "sha256:111",
					"pixi.lock": "sha256:222",
				},
			},
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
	if len(loaded.Entries) != 1 {
		t.Fatalf("Entries length = %d, want 1", len(loaded.Entries))
	}

	e := loaded.Entries[0]
	if e.ID != "test-entry-uuid" {
		t.Errorf("ID = %q, want %q", e.ID, "test-entry-uuid")
	}
	if e.SpecName != "data-science" {
		t.Errorf("SpecName = %q, want %q", e.SpecName, "data-science")
	}
	if e.VersionName != "v1.0" {
		t.Errorf("VersionName = %q, want %q", e.VersionName, "v1.0")
	}
	if e.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", e.ServerURL, "https://nebi.example.com")
	}
	if e.Path != "/home/user/project-a" {
		t.Errorf("Path = %q, want %q", e.Path, "/home/user/project-a")
	}
	if e.Layers["pixi.toml"] != "sha256:111" {
		t.Errorf("Layers[pixi.toml] = %q, want %q", e.Layers["pixi.toml"], "sha256:111")
	}
	if e.Layers["pixi.lock"] != "sha256:222" {
		t.Errorf("Layers[pixi.lock] = %q, want %q", e.Layers["pixi.lock"], "sha256:222")
	}
}

func TestAddEntry(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	entry := Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		ServerURL:   "https://nebi.example.com",
		VersionID:   "42",
		Path:        "/home/user/project-a",
		PulledAt:    now,
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
	if entries[0].SpecName != "data-science" {
		t.Errorf("SpecName = %q, want %q", entries[0].SpecName, "data-science")
	}
	// Check that ID was auto-generated
	if entries[0].ID == "" {
		t.Error("ID should be auto-generated")
	}
}

func TestAddEntryReplacesSamePath(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	entry1 := Entry{
		SpecName:    "data-science",
		VersionName: "v1.0",
		Path:        "/home/user/project-a",
		PulledAt:    now,
	}
	entry2 := Entry{
		SpecName:    "data-science",
		VersionName: "v2.0",
		Path:        "/home/user/project-a",
		PulledAt:    now.Add(time.Hour),
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
	if entries[0].VersionName != "v2.0" {
		t.Errorf("VersionName = %q, want %q (should be replaced)", entries[0].VersionName, "v2.0")
	}
}

func TestAddMultipleEntries(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	entries := []Entry{
		{SpecName: "ws1", VersionName: "v1.0", Path: "/path/a", PulledAt: now},
		{SpecName: "ws2", VersionName: "v1.0", Path: "/path/b", PulledAt: now},
		{SpecName: "ws1", VersionName: "v1.0", Path: "/path/c", PulledAt: now},
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

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: "/path/a", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws2", VersionName: "v1.0", Path: "/path/b", PulledAt: now})

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

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: "/path/a", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws2", VersionName: "v2.0", Path: "/path/b", PulledAt: now})

	entry, err := store.FindByPath("/path/a")
	if err != nil {
		t.Fatalf("FindByPath() error = %v", err)
	}
	if entry == nil {
		t.Fatal("FindByPath() returned nil")
	}
	if entry.SpecName != "ws1" {
		t.Errorf("SpecName = %q, want %q", entry.SpecName, "ws1")
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

func TestFindBySpecVersion(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: "/path/a", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: "/path/b", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v2.0", Path: "/path/c", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws2", VersionName: "v1.0", Path: "/path/d", PulledAt: now})

	matches, err := store.FindBySpecVersion("ws1", "v1.0")
	if err != nil {
		t.Fatalf("FindBySpecVersion() error = %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("FindBySpecVersion() length = %d, want 2", len(matches))
	}
}

func TestFindGlobal(t *testing.T) {
	store, dir := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	// Non-global entry
	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: "/path/a", PulledAt: now})

	// Global entry (path is under store's envs directory)
	globalPath := filepath.Join(dir, "envs", "ws1-uuid", "v1.0")
	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: globalPath, PulledAt: now})

	entry, err := store.FindGlobal("ws1", "v1.0")
	if err != nil {
		t.Fatalf("FindGlobal() error = %v", err)
	}
	if entry == nil {
		t.Fatal("FindGlobal() returned nil")
	}
	if entry.Path != globalPath {
		t.Errorf("Path = %q, want %q", entry.Path, globalPath)
	}
}

func TestFindGlobalNotFound(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: "/path/a", PulledAt: now})

	entry, err := store.FindGlobal("ws1", "v1.0")
	if err != nil {
		t.Fatalf("FindGlobal() error = %v", err)
	}
	if entry != nil {
		t.Errorf("FindGlobal() = %v, want nil", entry)
	}
}

func TestIsGlobal(t *testing.T) {
	store, dir := setupTestStore(t)

	globalPath := filepath.Join(dir, "envs", "ws1-uuid", "v1.0")
	globalEntry := &Entry{Path: globalPath}
	localEntry := &Entry{Path: "/some/local/path"}

	if !store.IsGlobal(globalEntry) {
		t.Error("IsGlobal() should return true for global path")
	}
	if store.IsGlobal(localEntry) {
		t.Error("IsGlobal() should return false for local path")
	}
}

func TestPrune(t *testing.T) {
	store, dir := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	// Create a real directory for one entry
	realDir := filepath.Join(dir, "real-workspace")
	os.MkdirAll(realDir, 0755)

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: realDir, PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws2", VersionName: "v1.0", Path: "/nonexistent/path", PulledAt: now})

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

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", Path: realDir, PulledAt: now})

	pruned, err := store.Prune()
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if pruned != nil {
		t.Errorf("Prune() = %v, want nil (nothing to prune)", pruned)
	}
}

func TestGlobalRepoPath(t *testing.T) {
	store := NewStoreWithDir("/home/user/.local/share/nebi")
	path := store.GlobalRepoPath("550e8400-e29b-41d4-a716-446655440000", "v1.0")
	expected := "/home/user/.local/share/nebi/envs/550e8400-e29b-41d4-a716-446655440000/v1.0"
	if path != expected {
		t.Errorf("GlobalRepoPath() = %q, want %q", path, expected)
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

	idx := &Index{Version: CurrentVersion, Entries: []Entry{}}
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
		Entries: []Entry{
			{
				ID:          "test-uuid",
				SpecName:    "data-science",
				SpecID:      "spec-uuid",
				VersionName: "v1.0",
				VersionID:   "version-uuid",
				ServerURL:   "https://nebi.example.com",
				ServerID:    "server-uuid",
				Path:        "/home/user/project-a",
				PulledAt:    now,
				Layers: map[string]string{
					"pixi.toml": "sha256:111aaa",
					"pixi.lock": "sha256:222bbb",
				},
			},
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
	if !ok || version != float64(CurrentVersion) {
		t.Errorf("version = %v, want %d", raw["version"], CurrentVersion)
	}

	// Check entries is array
	entries, ok := raw["entries"].([]interface{})
	if !ok || len(entries) != 1 {
		t.Fatalf("entries = %v", raw["entries"])
	}
}

func TestMigrationFromV1Format(t *testing.T) {
	store, dir := setupTestStore(t)
	now := time.Date(2024, 1, 20, 10, 30, 0, 0, time.UTC)

	// Write old v1 format with "repos" array
	os.MkdirAll(dir, 0755)
	oldData := `{
		"version": 1,
		"repos": [
			{
				"repo": "data-science",
				"tag": "v1.0",
				"server_url": "https://nebi.example.com",
				"server_version_id": 42,
				"path": "/home/user/project-a",
				"is_global": false,
				"pulled_at": "2024-01-20T10:30:00Z",
				"layers": {
					"pixi.toml": "sha256:111",
					"pixi.lock": "sha256:222"
				}
			}
		]
	}`
	os.WriteFile(filepath.Join(dir, IndexFileName), []byte(oldData), 0644)

	idx, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(idx.Entries) != 1 {
		t.Fatalf("Entries length = %d, want 1", len(idx.Entries))
	}

	e := idx.Entries[0]
	if e.SpecName != "data-science" {
		t.Errorf("SpecName = %q, want %q", e.SpecName, "data-science")
	}
	if e.VersionName != "v1.0" {
		t.Errorf("VersionName = %q, want %q", e.VersionName, "v1.0")
	}
	if e.VersionID != "42" {
		t.Errorf("VersionID = %q, want %q", e.VersionID, "42")
	}
	if e.ServerURL != "https://nebi.example.com" {
		t.Errorf("ServerURL = %q, want %q", e.ServerURL, "https://nebi.example.com")
	}
	if e.Path != "/home/user/project-a" {
		t.Errorf("Path = %q, want %q", e.Path, "/home/user/project-a")
	}
	if !e.PulledAt.Equal(now) {
		t.Errorf("PulledAt = %v, want %v", e.PulledAt, now)
	}
	if e.ID == "" {
		t.Error("ID should be auto-generated during migration")
	}
}

func TestPruneOrphaned(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	// Add entries with different SpecIDs
	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", SpecID: "spec-1", Path: "/path/a", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws2", VersionName: "v1.0", SpecID: "spec-2", Path: "/path/b", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws3", VersionName: "v1.0", SpecID: "spec-3", Path: "/path/c", PulledAt: now})
	store.AddEntry(Entry{SpecName: "ws4", VersionName: "v1.0", SpecID: "", Path: "/path/d", PulledAt: now}) // No SpecID

	// Only spec-1 and spec-3 still exist on server
	validRemoteIDs := map[string]bool{
		"spec-1": true,
		"spec-3": true,
	}

	pruned, err := store.PruneOrphaned(validRemoteIDs)
	if err != nil {
		t.Fatalf("PruneOrphaned() error = %v", err)
	}

	// Should have pruned spec-2
	if len(pruned) != 1 {
		t.Fatalf("PruneOrphaned() pruned %d entries, want 1", len(pruned))
	}
	if pruned[0].SpecID != "spec-2" {
		t.Errorf("PruneOrphaned() pruned SpecID = %q, want %q", pruned[0].SpecID, "spec-2")
	}

	// Verify remaining entries
	entries, _ := store.ListAll()
	if len(entries) != 3 {
		t.Fatalf("ListAll() length = %d, want 3", len(entries))
	}

	// Check that spec-1, spec-3, and empty SpecID entries remain
	specIDs := make(map[string]bool)
	for _, e := range entries {
		specIDs[e.SpecID] = true
	}
	if !specIDs["spec-1"] {
		t.Error("spec-1 should remain")
	}
	if !specIDs["spec-3"] {
		t.Error("spec-3 should remain")
	}
	if !specIDs[""] {
		t.Error("Entry with empty SpecID should remain")
	}
}

func TestPruneOrphanedNothingToPrune(t *testing.T) {
	store, _ := setupTestStore(t)
	now := time.Now().Truncate(time.Second)

	store.AddEntry(Entry{SpecName: "ws1", VersionName: "v1.0", SpecID: "spec-1", Path: "/path/a", PulledAt: now})

	// All entries exist on server
	validRemoteIDs := map[string]bool{
		"spec-1": true,
	}

	pruned, err := store.PruneOrphaned(validRemoteIDs)
	if err != nil {
		t.Fatalf("PruneOrphaned() error = %v", err)
	}
	if pruned != nil {
		t.Errorf("PruneOrphaned() should return nil when nothing to prune, got %v", pruned)
	}
}
