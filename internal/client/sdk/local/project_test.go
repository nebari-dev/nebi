package local

import (
	"errors"
	"os/exec"
	"sort"
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

// TestMigrateNextToLegacyProjectsIndex checks that the name index does not
// clash with an index on the legacy projects table. SQLite index names are
// database-wide, and idx_projects_name is the name GORM gives an index on
// projects.name.
func TestMigrateNextToLegacyProjectsIndex(t *testing.T) {
	db := openTestDB(t)
	for _, stmt := range []string{
		"CREATE TABLE `projects` (`id` text PRIMARY KEY, `name` text)",
		"CREATE INDEX `idx_projects_name` ON `projects`(`name`)",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate next to a legacy projects index: %v", err)
	}
	if !db.Migrator().HasIndex(&Project{}, "idx_client_projects_name") {
		t.Fatal("client_projects must have idx_client_projects_name")
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

// serverPackages are the server-side packages internal/client must not
// depend on, directly or through another package.
var serverPackages = []string{
	"github.com/nebari-dev/nebi/internal/models",
	"github.com/nebari-dev/nebi/internal/store",
	"github.com/nebari-dev/nebi/internal/service",
	"github.com/nebari-dev/nebi/internal/db",
	"github.com/nebari-dev/nebi/internal/queue",
	"github.com/nebari-dev/nebi/internal/worker",
	"github.com/nebari-dev/nebi/internal/executor",
	"github.com/nebari-dev/nebi/internal/api",
}

// TestClientDoesNotDependOnServerPackages enforces the data model acceptance
// criterion: nothing in internal/client depends on internal/models or the
// other server packages. It reads the full dependency graph from go list
// -deps, test files included, so an import through another package (for
// example internal/store, which reaches internal/models via internal/auth)
// is caught too.
func TestClientDoesNotDependOnServerPackages(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go list needs the go command on PATH: %v", err)
	}
	cmd := exec.Command(goBin, "list", "-deps", "-test", "-tags=e2e",
		"-f", "{{.ImportPath}}|{{join .Imports \",\"}}",
		"github.com/nebari-dev/nebi/internal/client/...")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}

	// importers maps every package in the graph to the packages that
	// import it, so a violation can name where it comes from.
	importers := map[string]map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, "|")
		pkg = stripTestVariant(pkg)
		if importers[pkg] == nil {
			importers[pkg] = map[string]bool{}
		}
		for _, imp := range strings.Split(imports, ",") {
			if imp = stripTestVariant(imp); imp != "" {
				if importers[imp] == nil {
					importers[imp] = map[string]bool{}
				}
				importers[imp][pkg] = true
			}
		}
	}
	if _, ok := importers["github.com/nebari-dev/nebi/internal/client/sdk/local"]; !ok {
		t.Fatalf("go list did not report internal/client/sdk/local:\n%s", out)
	}
	for pkg, set := range importers {
		for _, forbidden := range serverPackages {
			if pkg == forbidden || strings.HasPrefix(pkg, forbidden+"/") {
				var by []string
				for b := range set {
					by = append(by, b)
				}
				sort.Strings(by)
				t.Errorf("internal/client depends on %s (imported by %s)", pkg, strings.Join(by, ", "))
			}
		}
	}
}

// stripTestVariant drops the " [pkg.test]" suffix go list adds to packages
// recompiled for a test.
func stripTestVariant(pkg string) string {
	if i := strings.Index(pkg, " ["); i >= 0 {
		return pkg[:i]
	}
	return pkg
}
