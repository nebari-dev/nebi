package local

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMigrateCreatesClientProjectsTable(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate is not idempotent: %v", err)
	}
	if !db.Migrator().HasTable("client_projects") {
		t.Fatal("client_projects table missing")
	}
	if !db.Migrator().HasTable("legacy_project_origins") {
		t.Fatal("legacy_project_origins table missing")
	}
	if !db.Migrator().HasTable("client_pending_removals") {
		t.Fatal("client_pending_removals table missing")
	}
	if db.Migrator().HasTable("projects") {
		t.Fatal("Migrate must not create or share the server's projects table")
	}

	cols, err := db.Migrator().ColumnTypes(&Project{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range cols {
		names = append(names, c.Name())
	}
	sort.Strings(names)
	want := []string{"created_at", "id", "installed_version_id", "managed_dir", "name", "path"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("columns = %v, want exactly %v (no owner, status, source, size, origin or soft-delete columns)", names, want)
	}
}

func TestNameIsUniqueIgnoringCaseInTheDatabase(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Project{Name: "demo", Path: "/a"}).Error; err != nil {
		t.Fatal(err)
	}
	dup := &Project{Name: "DEMO", Path: "/b"}
	err := db.Create(dup).Error
	if err == nil {
		t.Fatal("inserting DEMO next to demo should violate the case-insensitive unique index")
	}
	if mapped := uniqueViolation(err, dup); !errors.Is(mapped, ErrNameTaken) {
		t.Fatalf("uniqueViolation = %v, want ErrNameTaken", mapped)
	}
}

func TestPathIsUniqueInTheDatabase(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Project{Name: "a", Path: "/same"}).Error; err != nil {
		t.Fatal(err)
	}
	dup := &Project{Name: "b", Path: "/same"}
	err := db.Create(dup).Error
	if mapped := uniqueViolation(err, dup); !errors.Is(mapped, ErrPathTaken) {
		t.Fatalf("uniqueViolation = %v, want ErrPathTaken", mapped)
	}
}

func TestBeforeCreateAssignsID(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	p := &Project{Name: "x", Path: "/x"}
	if err := db.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	if p.ID == uuid.Nil {
		t.Fatal("ID not assigned")
	}
	keep := uuid.New()
	q := &Project{ID: keep, Name: "y", Path: "/y"}
	if err := db.Create(q).Error; err != nil {
		t.Fatal(err)
	}
	if q.ID != keep {
		t.Fatal("an explicit ID must be kept")
	}
}

// TestClientDoesNotImportServerModels enforces the data model acceptance
// criterion: nothing in internal/client imports internal/models.
func TestClientDoesNotImportServerModels(t *testing.T) {
	root := filepath.Join("..", "..") // internal/client
	const forbidden = "github.com/nebari-dev/nebi/internal/models"
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == forbidden {
				t.Errorf("%s imports %s", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no Go files found under internal/client")
	}
}
