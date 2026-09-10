package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
)

func TestImportFromRegistry_LocalMode_ExtractsBundleAndEnqueuesSeedJob(t *testing.T) {
	svc, db := testSetup(t, true) // isLocal=true
	userID := createTestUser(t, db, "alice")

	// Stand up an in-memory registry and publish a bundle into it.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "pixi.toml"),
		[]byte(`[project]
name = "bundle-import"
channels = ["conda-forge"]
platforms = ["linux-64"]
`), 0o644)
	os.WriteFile(filepath.Join(srcDir, "pixi.lock"), []byte("version: 6\n# published\n"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "notebook.ipynb"), []byte(`{"cells":[]}`), 0o644)

	reg := oci.Registry{Host: u.Host, Namespace: "demo", PlainHTTP: true}
	published, err := oci.Publish(context.Background(), srcDir, reg, "bundle-import", "v1")
	if err != nil {
		t.Fatalf("seed publish: %v", err)
	}

	dbReg := models.OCIRegistry{
		Name:      "import-src",
		URL:       "http://" + u.Host,
		Namespace: "demo",
		IsDefault: true,
	}
	db.Create(&dbReg)

	ws, err := svc.ImportFromRegistry(context.Background(), dbReg.ID.String(), ImportFromRegistryRequest{
		Repository: "bundle-import",
		Tag:        "v1",
		Name:       "imported-env",
	}, userID)
	if err != nil {
		t.Fatalf("ImportFromRegistry: %v", err)
	}
	if ws.Name != "imported-env" {
		t.Errorf("workspace name: got %q want %q", ws.Name, "imported-env")
	}
	if ws.ImportRepository != published.Repository {
		t.Errorf("import repository: got %q want %q", ws.ImportRepository, published.Repository)
	}
	if ws.ImportTag != "v1" {
		t.Errorf("import tag: got %q want %q", ws.ImportTag, "v1")
	}
	if ws.ImportDigest != published.Digest {
		t.Errorf("import digest: got %q want %q", ws.ImportDigest, published.Digest)
	}

	// Expect a JobTypeCreate job with import_staging_dir pointing at an
	// existing directory that contains the extracted bundle.
	var job models.Job
	if err := db.Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeCreate).First(&job).Error; err != nil {
		t.Fatalf("find create job: %v", err)
	}
	stagingDir, _ := job.Metadata["import_staging_dir"].(string)
	if stagingDir == "" {
		t.Fatalf("expected import_staging_dir in job metadata, got %+v", job.Metadata)
	}

	for _, rel := range []string{"pixi.toml", "pixi.lock", "notebook.ipynb"} {
		if _, err := os.Stat(filepath.Join(stagingDir, rel)); err != nil {
			t.Errorf("expected %s in staging dir %q, got %v", rel, stagingDir, err)
		}
	}

	lockBytes, _ := os.ReadFile(filepath.Join(stagingDir, "pixi.lock"))
	if string(lockBytes) != "version: 6\n# published\n" {
		t.Errorf("published pixi.lock was not preserved in staging: %q", string(lockBytes))
	}

	// Audit row should record the import with the registry/repo/tag and
	// manifest digest so IR can reconstruct what was pulled.
	var audits []models.AuditLog
	if err := db.Where("user_id = ? AND action = ?", userID, "import_workspace").Find(&audits).Error; err != nil {
		t.Fatalf("query audit logs: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected exactly one import_workspace audit row, got %d", len(audits))
	}
	for _, want := range []string{`"registry":"import-src"`, `"repository":"bundle-import"`, `"tag":"v1"`, `"digest":"sha256:`} {
		if !strings.Contains(audits[0].DetailsJSON, want) {
			t.Errorf("audit details missing %s; got %s", want, audits[0].DetailsJSON)
		}
	}
}

func TestImportFromRegistry_DigestOverridesMovedTag(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	reg := oci.Registry{Host: u.Host, Namespace: "demo", PlainHTTP: true}

	originalDir := t.TempDir()
	writeImportTestBundle(t, originalDir, "reviewed")
	original, err := oci.Publish(context.Background(), originalDir, reg, "digest-import", "v1")
	if err != nil {
		t.Fatalf("publish reviewed bundle: %v", err)
	}

	movedDir := t.TempDir()
	writeImportTestBundle(t, movedDir, "moved")
	moved, err := oci.Publish(context.Background(), movedDir, reg, "digest-import", "v1")
	if err != nil {
		t.Fatalf("move tag to replacement bundle: %v", err)
	}
	if moved.Digest == original.Digest {
		t.Fatal("test setup produced identical manifests")
	}

	dbReg := models.OCIRegistry{
		Name: "digest-source", URL: "http://" + u.Host, Namespace: "demo", IsDefault: true,
	}
	if err := db.Create(&dbReg).Error; err != nil {
		t.Fatalf("create registry: %v", err)
	}

	ws, err := svc.ImportFromRegistry(context.Background(), dbReg.ID.String(), ImportFromRegistryRequest{
		Repository: "digest-import",
		Tag:        "v1",
		Digest:     original.Digest,
		Name:       "digest-pinned",
	}, userID)
	if err != nil {
		t.Fatalf("ImportFromRegistry: %v", err)
	}
	if ws.ImportRepository != original.Repository || ws.ImportTag != "v1" || ws.ImportDigest != original.Digest {
		t.Fatalf("unexpected import metadata: repository=%q tag=%q digest=%q", ws.ImportRepository, ws.ImportTag, ws.ImportDigest)
	}

	var job models.Job
	if err := db.Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeCreate).First(&job).Error; err != nil {
		t.Fatalf("find create job: %v", err)
	}
	stagingDir, _ := job.Metadata["import_staging_dir"].(string)
	tomlBytes, err := os.ReadFile(filepath.Join(stagingDir, "pixi.toml"))
	if err != nil {
		t.Fatalf("read staged pixi.toml: %v", err)
	}
	if !strings.Contains(string(tomlBytes), "name = \"reviewed\"") {
		t.Fatalf("import followed the moved tag instead of the requested digest: %s", tomlBytes)
	}
}

func TestImportSelector(t *testing.T) {
	validDigest := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name          string
		tag           string
		digest        string
		wantSelector  string
		wantRequested string
		wantErr       string
	}{
		{name: "tag", tag: "v1", wantSelector: "v1"},
		{name: "digest", digest: validDigest, wantSelector: validDigest, wantRequested: validDigest},
		{name: "digest takes precedence", tag: "v1", digest: validDigest, wantSelector: validDigest, wantRequested: validDigest},
		{name: "missing selector", wantErr: "tag or digest is required"},
		{name: "invalid digest", digest: "sha256:abc", wantErr: "invalid digest"},
		{name: "invalid tag with digest", tag: "bad tag", digest: validDigest, wantErr: "invalid tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, requested, err := importSelector(tt.tag, tt.digest)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("importSelector: %v", err)
			}
			if selector != tt.wantSelector || requested != tt.wantRequested {
				t.Fatalf("selector=%q requested=%q, want selector=%q requested=%q", selector, requested, tt.wantSelector, tt.wantRequested)
			}
		})
	}
}

func writeImportTestBundle(t *testing.T, dir, name string) {
	t.Helper()
	toml := fmt.Sprintf("[project]\nname = %q\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n", name)
	if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml), 0o644); err != nil {
		t.Fatalf("write pixi.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte("version: 6\n# "+name+"\n"), 0o644); err != nil {
		t.Fatalf("write pixi.lock: %v", err)
	}
}

func TestImportFromRegistry_TeamMode_PixiOnly(t *testing.T) {
	svc, db := testSetup(t, false) // isLocal=false
	userID := createTestUser(t, db, "alice")

	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "pixi.toml"),
		[]byte(`[project]
name = "team-import"
channels = ["conda-forge"]
platforms = ["linux-64"]
`), 0o644)
	os.WriteFile(filepath.Join(srcDir, "pixi.lock"), []byte("version: 6\n"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "asset.txt"), []byte("should-be-dropped\n"), 0o644)

	reg := oci.Registry{Host: u.Host, Namespace: "demo", PlainHTTP: true}
	if _, err := oci.Publish(context.Background(), srcDir, reg, "team-import", "v1"); err != nil {
		t.Fatalf("seed publish: %v", err)
	}
	dbReg := models.OCIRegistry{
		Name: "team-src", URL: "http://" + u.Host, Namespace: "demo", IsDefault: true,
	}
	db.Create(&dbReg)
	grantRegistryAccessForTest(t, db, userID, dbReg.ID, "read")

	ws, err := svc.ImportFromRegistry(context.Background(), dbReg.ID.String(), ImportFromRegistryRequest{
		RepositoryPath: "demo/team-import", Tag: "v1", Name: "imported-team",
	}, userID)
	if err != nil {
		t.Fatalf("ImportFromRegistry: %v", err)
	}

	// Both modes stage to disk so the worker honours the published
	// pixi.lock; team mode just stages fewer files (no asset layers).
	var job models.Job
	if err := db.Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeCreate).First(&job).Error; err != nil {
		t.Fatalf("find create job: %v", err)
	}
	stagingDir, _ := job.Metadata["import_staging_dir"].(string)
	if stagingDir == "" {
		t.Fatalf("team mode should stage pixi files for the worker, got %+v", job.Metadata)
	}
	for _, rel := range []string{"pixi.toml", "pixi.lock"} {
		if _, err := os.Stat(filepath.Join(stagingDir, rel)); err != nil {
			t.Errorf("expected %s in team-mode staging dir, got %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "asset.txt")); !os.IsNotExist(err) {
		t.Errorf("team mode must drop asset layers; asset.txt present in staging dir: %v", err)
	}
	// PullBundle whitespace-trims core layer bytes; the staged file
	// must still carry the published lock content even after that
	// transform (pixi tolerates the difference; both forms parse).
	lockBytes, _ := os.ReadFile(filepath.Join(stagingDir, "pixi.lock"))
	if !strings.Contains(string(lockBytes), "version: 6") {
		t.Errorf("team mode should preserve published pixi.lock content, got %q", string(lockBytes))
	}
}

func TestImportFromRegistry_RequiresRegistryReadAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	registry := models.OCIRegistry{Name: "private", URL: "https://ghcr.io", Namespace: "demo", Restricted: true}
	db.Create(&registry)

	_, err := svc.ImportFromRegistry(context.Background(), registry.ID.String(), ImportFromRegistryRequest{
		Repository: "bundle-import",
		Tag:        "v1",
		Name:       "imported-env",
	}, userID)
	if err == nil {
		t.Fatal("expected forbidden error without registry read grant")
	}
	if !isForbiddenError(err, nil) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}

	var count int64
	db.Model(&models.Workspace{}).Where("name = ?", "imported-env").Count(&count)
	if count != 0 {
		t.Fatalf("expected no workspace to be created without registry read grant, got %d", count)
	}
}
