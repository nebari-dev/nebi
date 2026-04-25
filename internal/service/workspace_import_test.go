package service

import (
	"context"
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
	if _, err := oci.Publish(context.Background(), srcDir, reg, "bundle-import", "v1"); err != nil {
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
