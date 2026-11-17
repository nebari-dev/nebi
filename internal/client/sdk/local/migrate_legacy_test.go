package local

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// legacyCLISchema is the old shared "projects" table as the CLI store and
// the server models together left it in a client nebi.db.
const legacyCLISchema = "CREATE TABLE `projects` (" +
	"`id` text PRIMARY KEY, `name` text NOT NULL, `owner_id` text, `status` text NOT NULL DEFAULT 'ready', " +
	"`path` text, `source` text DEFAULT 'managed', `size_bytes` integer DEFAULT 0, " +
	"`origin_id` text, `origin_name` text, `origin_tag` text, `origin_action` text, " +
	"`origin_toml_hash` text, `origin_lock_hash` text, " +
	"`created_at` datetime, `updated_at` datetime, `deleted_at` datetime)"

// legacyServerSchema is the table as models.Project alone creates it: no
// Origin* columns.
const legacyServerSchema = "CREATE TABLE `projects` (" +
	"`id` text PRIMARY KEY, `name` text NOT NULL, `owner_id` text, `status` text NOT NULL DEFAULT 'pending', " +
	"`source` text DEFAULT 'managed', `path` text, `size_bytes` integer DEFAULT 0, " +
	"`created_at` datetime, `updated_at` datetime, `deleted_at` datetime)"

type legacyFixture struct {
	id      uuid.UUID
	name    string
	path    string
	created time.Time
	deleted bool
	origin  string // origin_name; empty for none
}

func insertLegacy(t *testing.T, db *gorm.DB, withOrigins bool, rows ...legacyFixture) {
	t.Helper()
	for _, r := range rows {
		var deleted any
		if r.deleted {
			deleted = r.created.Add(time.Hour)
		}
		var err error
		if withOrigins {
			err = db.Exec("INSERT INTO projects (id, name, status, path, source, size_bytes, origin_name, origin_tag, created_at, updated_at, deleted_at) VALUES (?, ?, 'ready', ?, 'local', 123, ?, ?, ?, ?, ?)",
				r.id.String(), r.name, r.path, r.origin, originTag(r.origin), r.created, r.created, deleted).Error
		} else {
			err = db.Exec("INSERT INTO projects (id, name, status, path, created_at, updated_at, deleted_at) VALUES (?, ?, 'ready', ?, ?, ?, ?)",
				r.id.String(), r.name, r.path, r.created, r.created, deleted).Error
		}
		if err != nil {
			t.Fatalf("insert legacy row %q: %v", r.name, err)
		}
	}
}

func originTag(origin string) string {
	if origin == "" {
		return ""
	}
	return "v1"
}

func clientProjects(t *testing.T, db *gorm.DB) map[uuid.UUID]Project {
	t.Helper()
	var list []Project
	if err := db.Find(&list).Error; err != nil {
		t.Fatal(err)
	}
	out := make(map[uuid.UUID]Project, len(list))
	for _, p := range list {
		out[p.ID] = p
	}
	return out
}

func TestMigrateLegacyProjects(t *testing.T) {
	db := openTestDB(t)
	if err := db.Exec(legacyCLISchema).Error; err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Join(t.TempDir(), "data", "projects")
	userDir := t.TempDir()
	t0 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	a := legacyFixture{id: uuid.New(), name: "demo", created: t0}
	a.path = filepath.Join(oldDir, "demo-"+a.id.String())
	b := legacyFixture{id: uuid.New(), name: "demo", path: filepath.Join(userDir, "demo"), created: t0.Add(1 * time.Hour)}
	c := legacyFixture{id: uuid.New(), name: "Demo", path: filepath.Join(userDir, "demo-upper"), created: t0.Add(2 * time.Hour)}
	d := legacyFixture{id: uuid.New(), name: "old", path: filepath.Join(userDir, "old"), created: t0.Add(3 * time.Hour), deleted: true}
	e := legacyFixture{id: uuid.New(), name: "My Env", path: "", created: t0.Add(4 * time.Hour)}
	f := legacyFixture{id: uuid.New(), name: "copy", path: b.path, created: t0.Add(5 * time.Hour)}
	g := legacyFixture{id: uuid.New(), name: "synced", path: filepath.Join(userDir, "synced"), created: t0.Add(6 * time.Hour), origin: "team/synced"}
	h := legacyFixture{id: uuid.New(), name: "Rel Path!", path: "rel/dir", created: t0.Add(7 * time.Hour)}
	// Inserted out of order: the migration must sort by created_at.
	insertLegacy(t, db, true, g, c, a, f, d, b, e, h)

	var logs []string
	report, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{
		OldProjectsDir: oldDir,
		Logf:           func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatalf("MigrateLegacyProjects: %v", err)
	}

	got := clientProjects(t, db)
	resolvedOld, _ := resolveSymlinks(oldDir)
	resolvedUser, _ := resolveSymlinks(userDir)
	want := map[uuid.UUID]struct {
		name    string
		path    string
		managed bool
	}{
		a.id: {"demo", filepath.Join(resolvedOld, "demo-"+a.id.String()), true},
		b.id: {"demo-2", filepath.Join(resolvedUser, "demo"), false},
		c.id: {"Demo-3", filepath.Join(resolvedUser, "demo-upper"), false},
		e.id: {"My Env", filepath.Join(resolvedOld, "my-env-"+e.id.String()), true},
		g.id: {"synced", filepath.Join(resolvedUser, "synced"), false},
		h.id: {"Rel Path!", filepath.Join(resolvedOld, "rel-path-"+h.id.String()), true},
	}
	if len(got) != len(want) {
		t.Fatalf("migrated %d projects, want %d: %+v", len(got), len(want), got)
	}
	for id, w := range want {
		p, ok := got[id]
		if !ok {
			t.Errorf("project %s (%s) missing", id, w.name)
			continue
		}
		if p.Name != w.name || p.Path != w.path || p.ManagedDir != w.managed {
			t.Errorf("project %s: got name=%q path=%s managed=%v; want name=%q path=%s managed=%v",
				id, p.Name, p.Path, p.ManagedDir, w.name, w.path, w.managed)
		}
		if p.InstalledVersionID != nil {
			t.Errorf("project %s: InstalledVersionID must start empty", id)
		}
	}
	if _, ok := got[d.id]; ok {
		t.Error("soft-deleted row must be dropped")
	}
	if _, ok := got[f.id]; ok {
		t.Error("row with an already used path must be dropped")
	}
	if !got[b.id].CreatedAt.Equal(b.created) {
		t.Errorf("created_at must be kept: got %v, want %v", got[b.id].CreatedAt, b.created)
	}

	if report.Migrated != 6 || report.SkippedDeleted != 1 || report.OriginsKept != 1 || report.AlreadyApplied {
		t.Errorf("unexpected report %+v", report)
	}
	if len(report.Renamed) != 2 ||
		report.Renamed[0] != (LegacyRename{ProjectID: b.id, From: "demo", To: "demo-2"}) ||
		report.Renamed[1] != (LegacyRename{ProjectID: c.id, From: "Demo", To: "Demo-3"}) {
		t.Errorf("renames = %+v", report.Renamed)
	}
	if len(report.Dropped) != 1 || report.Dropped[0].ProjectID != f.id {
		t.Errorf("dropped = %+v", report.Dropped)
	}
	renameLogs := 0
	for _, l := range logs {
		if strings.HasPrefix(l, "renamed project") {
			renameLogs++
		}
	}
	if renameLogs != 2 {
		t.Errorf("each rename must be logged; logs: %v", logs)
	}

	var origins []LegacyProjectOrigin
	if err := db.Find(&origins).Error; err != nil {
		t.Fatal(err)
	}
	if len(origins) != 1 || origins[0].ProjectID != g.id || origins[0].OriginName != "team/synced" || origins[0].OriginTag != "v1" {
		t.Errorf("origins = %+v", origins)
	}

	// The old table is left in place.
	var oldCount int64
	if err := db.Table("projects").Count(&oldCount).Error; err != nil || oldCount != 8 {
		t.Errorf("old table must be untouched: %d rows, %v", oldCount, err)
	}

	// Running it again does nothing.
	again, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{OldProjectsDir: oldDir})
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyApplied || again.Migrated != 0 {
		t.Errorf("second run: %+v", again)
	}
	if n := len(clientProjects(t, db)); n != 6 {
		t.Errorf("second run changed client_projects: %d rows", n)
	}
}

func TestMigrateLegacyProjectsWithoutOldTable(t *testing.T) {
	db := openTestDB(t)
	report, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.AlreadyApplied || report.Migrated != 0 {
		t.Fatalf("report = %+v", report)
	}
	if !db.Migrator().HasTable("client_projects") {
		t.Fatal("client_projects must exist afterwards")
	}
	again, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{})
	if err != nil || !again.AlreadyApplied {
		t.Fatalf("second run: %+v, %v", again, err)
	}
}

func TestMigrateLegacyProjectsNeedsOldDirForRowsWithoutPath(t *testing.T) {
	db := openTestDB(t)
	if err := db.Exec(legacyServerSchema).Error; err != nil {
		t.Fatal(err)
	}
	ok := legacyFixture{id: uuid.New(), name: "fine", path: filepath.Join(t.TempDir(), "fine"), created: time.Now()}
	noPath := legacyFixture{id: uuid.New(), name: "desktop-made", created: time.Now().Add(time.Second)}
	insertLegacy(t, db, false, ok, noPath)

	_, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{})
	if err == nil || !strings.Contains(err.Error(), "OldProjectsDir") {
		t.Fatalf("want an error asking for OldProjectsDir, got %v", err)
	}
	if n := len(clientProjects(t, db)); n != 0 {
		t.Fatalf("a failed migration must roll back completely, %d rows copied", n)
	}

	// Once configured, it runs: nothing was marked applied by the failure.
	report, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{OldProjectsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if report.AlreadyApplied || report.Migrated != 2 || report.OriginsKept != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMigrateLegacyProjectsAvoidsExistingClientProjects(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	taken := filepath.Join(t.TempDir(), "taken")
	resolvedTaken, _ := resolveSymlinks(taken)
	existing := &Project{Name: "demo", Path: resolvedTaken}
	if err := db.Create(existing).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(legacyCLISchema).Error; err != nil {
		t.Fatal(err)
	}
	sameName := legacyFixture{id: uuid.New(), name: "DEMO", path: filepath.Join(t.TempDir(), "other"), created: time.Now()}
	samePath := legacyFixture{id: uuid.New(), name: "elsewhere", path: taken, created: time.Now().Add(time.Second)}
	sameID := legacyFixture{id: existing.ID, name: "dup-id", path: filepath.Join(t.TempDir(), "dup"), created: time.Now().Add(2 * time.Second)}
	insertLegacy(t, db, true, sameName, samePath, sameID)

	report, err := MigrateLegacyProjects(context.Background(), db, LegacyMigrationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := clientProjects(t, db)
	if got[sameName.id].Name != "DEMO-2" {
		t.Fatalf("name must not collide with an existing client project, got %q", got[sameName.id].Name)
	}
	if _, ok := got[samePath.id]; ok {
		t.Fatal("a row whose path an existing client project uses must be dropped")
	}
	if got[existing.ID].Name != "demo" {
		t.Fatal("an existing client project must not be overwritten")
	}
	if report.Migrated != 1 || len(report.Dropped) != 2 {
		t.Fatalf("report = %+v", report)
	}
}

func TestLegacyDirNameMatchesOldExecutor(t *testing.T) {
	cases := map[string]string{
		"demo":                    "demo",
		"My Env":                  "my-env",
		"--Weird__Name!!":         "weird-name",
		strings.Repeat("ab", 40):  strings.Repeat("ab", 25),
		"data.science v2":         "data-science-v2",
		"UPPER-lower_123":         "upper-lower-123",
		"trailing-hyphen-cut-at-": "trailing-hyphen-cut-at",
	}
	for in, want := range cases {
		if got := legacyDirName(in); got != want {
			t.Errorf("legacyDirName(%q) = %q, want %q", in, got, want)
		}
	}
}
