package local

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
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
	if !db.Migrator().HasTable("client_pending_removals") {
		t.Fatal("client_pending_removals table missing")
	}
	for _, table := range []string{"projects", "legacy_project_origins", "client_migrations"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("Migrate must not create %s", table)
		}
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
	want := []string{"created_at", "id", "installed_version_id", "installing_version_id", "last_installed_version_id", "managed_dir", "name", "path"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("columns = %v, want exactly %v (no owner, status, source, size, origin or soft-delete columns)", names, want)
	}
}

// TestMigrateAddsInstallColumns checks that a database made before #682
// gains last_installed_version_id and installing_version_id, and that its
// rows are kept.
func TestMigrateAddsInstallColumns(t *testing.T) {
	db := openTestDB(t)
	if err := db.Exec("CREATE TABLE `client_projects` (`id` text PRIMARY KEY, `name` text NOT NULL, `path` text NOT NULL, `managed_dir` numeric NOT NULL DEFAULT false, `installed_version_id` text, `created_at` datetime)").Error; err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := db.Exec("INSERT INTO client_projects (id, name, path, managed_dir, created_at) VALUES (?, 'old', '/old', false, CURRENT_TIMESTAMP)", id).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, col := range []string{"last_installed_version_id", "installing_version_id"} {
		if !db.Migrator().HasColumn(&Project{}, col) {
			t.Fatalf("Migrate must add %s", col)
		}
	}
	var p Project
	if err := db.First(&p, "id = ?", id).Error; err != nil {
		t.Fatalf("the existing row must be kept: %v", err)
	}
	if p.Name != "old" || p.LastInstalledVersionID != nil || p.InstallingVersionID != nil {
		t.Fatalf("existing row = %+v, want name old and no last installed or installing version", p)
	}
}

// TestMigrateNextToServerProjectsIndex checks that the name index does not
// clash with an index on the server's projects table. SQLite index names are
// database-wide, and idx_projects_name is the name GORM gives an index on
// projects.name.
func TestMigrateNextToServerProjectsIndex(t *testing.T) {
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
		t.Fatalf("Migrate next to a server projects index: %v", err)
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

// isServerPackage reports whether pkg is one of serverPackages or inside one.
func isServerPackage(pkg string) bool {
	for _, forbidden := range serverPackages {
		if pkg == forbidden || strings.HasPrefix(pkg, forbidden+"/") {
			return true
		}
	}
	return false
}

// TestClientDoesNotImportServerPackages checks the direct imports of every
// .go file under internal/client, whatever its build constraints. go list
// in TestClientDoesNotDependOnServerPackages only sees the files that build
// for the operating systems it lists with -tags=e2e, so it misses a server
// import in a file behind another build tag.
func TestClientDoesNotImportServerPackages(t *testing.T) {
	root := filepath.Join("..", "..") // internal/client
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
			if p, _ := strconv.Unquote(imp.Path.Value); isServerPackage(p) {
				t.Errorf("%s imports %s", path, p)
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

// clientTargetOSes are the operating systems the client is built for.
var clientTargetOSes = []string{"linux", "darwin", "windows"}

// TestClientDoesNotDependOnServerPackages enforces the data model acceptance
// criterion: nothing in internal/client depends on internal/models or the
// other server packages. It reads the full dependency graph from go list
// -deps, test files included, so an import through another package (for
// example internal/store, which reaches internal/models via internal/auth)
// is caught too. go list only sees the files that build for one GOOS and
// cgo setting, so it runs for each OS the client targets, with cgo off and
// on: an OS-specific file (such as a _windows.go file) or a cgo or !cgo
// file that reaches a server package through another package is caught
// too. TestClientDoesNotImportServerPackages covers the direct
// imports of files behind other build tags.
func TestClientDoesNotDependOnServerPackages(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go list needs the go command on PATH: %v", err)
	}
	for _, goos := range clientTargetOSes {
		// Release builds set CGO_ENABLED=0 (.goreleaser.yaml, Dockerfile),
		// and a cgo build keeps the cgo files instead of the !cgo ones
		// (go list drops cgo files by default when GOOS is not the
		// host's), so check both graphs.
		for _, cgo := range []string{"0", "1"} {
			t.Run(goos+"/cgo="+cgo, func(t *testing.T) {
				checkServerDeps(t, goBin, goos, cgo)
			})
		}
	}
}

// checkServerDeps fails t when internal/client, built for goos with
// CGO_ENABLED=cgo, depends on a server package.
func checkServerDeps(t *testing.T, goBin, goos, cgo string) {
	cmd := exec.Command(goBin, "list", "-deps", "-test", "-tags=e2e",
		"-f", "{{.ImportPath}}|{{join .Imports \",\"}}",
		"github.com/nebari-dev/nebi/internal/client/...")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED="+cgo)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("GOOS=%s CGO_ENABLED=%s go list: %v\n%s", goos, cgo, err, stderr.String())
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
		if isServerPackage(pkg) {
			var by []string
			for b := range set {
				by = append(by, b)
			}
			sort.Strings(by)
			t.Errorf("internal/client depends on %s for GOOS=%s CGO_ENABLED=%s (imported by %s)", pkg, goos, cgo, strings.Join(by, ", "))
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
