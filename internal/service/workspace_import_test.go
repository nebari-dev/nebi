package service

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
		Repository: "team-import", Tag: "v1", Name: "imported-team",
	}, userID)
	if err != nil {
		t.Fatalf("ImportFromRegistry: %v", err)
	}

	// Expect a JobTypeCreate job with pixi_toml set and NO import_staging_dir.
	var job models.Job
	if err := db.Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeCreate).First(&job).Error; err != nil {
		t.Fatalf("find create job: %v", err)
	}
	if _, ok := job.Metadata["import_staging_dir"]; ok {
		t.Errorf("team mode should not set import_staging_dir in metadata; got %+v", job.Metadata)
	}
	got, _ := job.Metadata["pixi_toml"].(string)
	if got == "" {
		t.Errorf("team mode should stamp pixi_toml from registry; got empty, metadata=%+v", job.Metadata)
	}
}
