package service

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
)

// --- GetPublishDefaults tests ---

func TestGetPublishDefaults_ReturnsDefaults(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "publish-test", userID)

	// Migrate OCIRegistry and create a default registry
	// OCIRegistry already migrated in testSetup
	registry := models.OCIRegistry{
		Name:      "test-registry",
		URL:       "https://quay.io",
		Namespace: "myorg",
		IsDefault: true,
	}
	db.Create(&registry)
	grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	defaults, err := svc.GetPublishDefaults(project.ID.String(), userID, uuid.Nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if defaults.RegistryID != registry.ID {
		t.Errorf("expected registry ID %s, got %s", registry.ID, defaults.RegistryID)
	}
	if defaults.RegistryName != "test-registry" {
		t.Errorf("expected registry name %q, got %q", "test-registry", defaults.RegistryName)
	}
	if defaults.Namespace != "myorg" {
		t.Errorf("expected namespace %q, got %q", "myorg", defaults.Namespace)
	}

	// Repo should be name-first8charsOfID
	expectedRepo := "publish-test-" + project.ID.String()[:8]
	if defaults.Repository != expectedRepo {
		t.Errorf("expected repository %q, got %q", expectedRepo, defaults.Repository)
	}

	// No versions pushed, so tag should default to "latest"
	if defaults.Tag != "latest" {
		t.Errorf("expected tag %q, got %q", "latest", defaults.Tag)
	}
}

func TestGetPublishDefaults_UsesContentHashTag(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "publish-hash", userID)

	// OCIRegistry already migrated in testSetup
	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", IsDefault: true}
	db.Create(&registry)
	grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	// Push a version so there's a content hash
	svc.PushVersion(context.Background(), project.ID.String(), PushRequest{
		PixiToml: "[project]\nname = \"test\"",
		PixiLock: "version: 6",
	}, userID)

	defaults, err := svc.GetPublishDefaults(project.ID.String(), userID, uuid.Nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tag should be the content hash, not "latest"
	if defaults.Tag == "latest" {
		t.Error("expected content hash tag, got 'latest'")
	}
	if len(defaults.Tag) < 10 {
		t.Errorf("expected sha-prefixed hash tag, got %q", defaults.Tag)
	}
}

func TestGetPublishDefaults_NoDefaultRegistry(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "no-reg", userID)

	// OCIRegistry already migrated in testSetup
	// No default registry exists

	_, err := svc.GetPublishDefaults(project.ID.String(), userID, uuid.Nil)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPublishDefaults_MasksDefaultRegistryWithoutReadAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "private-default", userID)

	db.Create(&models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", IsDefault: true, Restricted: true})

	_, err := svc.GetPublishDefaults(project.ID.String(), userID, uuid.Nil)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %T: %v", err, err)
	}
}

func TestGetPublishDefaults_ProjectNotFound(t *testing.T) {
	svc, _ := testSetup(t, false)

	_, err := svc.GetPublishDefaults(uuid.New().String(), uuid.New(), uuid.Nil)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPublishDefaults_UsesExplicitRegistryAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "explicit-reg", userID)

	defaultRegistry := models.OCIRegistry{Name: "default", URL: "https://quay.io", IsDefault: true}
	explicitRegistry := models.OCIRegistry{Name: "team-b", URL: "https://ghcr.io", Namespace: "team"}
	db.Create(&defaultRegistry)
	db.Create(&explicitRegistry)
	grantRegistryAccessForTest(t, db, userID, explicitRegistry.ID, "read")

	defaults, err := svc.GetPublishDefaults(project.ID.String(), userID, explicitRegistry.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if defaults.RegistryID != explicitRegistry.ID {
		t.Fatalf("expected explicit registry %s, got %s", explicitRegistry.ID, defaults.RegistryID)
	}
	if defaults.RegistryName != explicitRegistry.Name {
		t.Fatalf("expected registry name %q, got %q", explicitRegistry.Name, defaults.RegistryName)
	}
	if defaults.Namespace != explicitRegistry.Namespace {
		t.Fatalf("expected namespace %q, got %q", explicitRegistry.Namespace, defaults.Namespace)
	}
}

func TestGetPublishDefaults_MasksExplicitRegistryWithoutReadAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "explicit-private", userID)

	registry := models.OCIRegistry{Name: "private", URL: "https://ghcr.io", Restricted: true}
	db.Create(&registry)

	_, err := svc.GetPublishDefaults(project.ID.String(), userID, registry.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %T: %v", err, err)
	}
}

// --- ListPublications tests ---

func TestListPublications_Empty(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "no-pubs", userID)

	// OCIRegistry and Publication already migrated in testSetup

	pubs, err := svc.ListPublications(project.ID.String(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pubs) != 0 {
		t.Errorf("expected 0 publications, got %d", len(pubs))
	}
}

func TestListPublications_ReturnsRecords(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "with-pubs", userID)

	// OCIRegistry and Publication already migrated in testSetup

	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", Namespace: "myorg"}
	db.Create(&registry)
	grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	pub := models.Publication{
		ProjectID:     project.ID,
		VersionNumber: 1,
		RegistryID:    registry.ID,
		Repository:    "my-env",
		Tag:           "v1.0.0",
		Digest:        "sha256:abc123",
		PublishedBy:   userID,
	}
	db.Create(&pub)

	pubs, err := svc.ListPublications(project.ID.String(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication, got %d", len(pubs))
	}

	if pubs[0].Repository != "my-env" {
		t.Errorf("expected repository %q, got %q", "my-env", pubs[0].Repository)
	}
	if pubs[0].Tag != "v1.0.0" {
		t.Errorf("expected tag %q, got %q", "v1.0.0", pubs[0].Tag)
	}
	if pubs[0].RegistryName != "reg" {
		t.Errorf("expected registry name %q, got %q", "reg", pubs[0].RegistryName)
	}
}

func TestListPublications_NotFound(t *testing.T) {
	svc, _ := testSetup(t, false)

	_, err := svc.ListPublications(uuid.New().String(), uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListPublications_FiltersUnreadableRegistries(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "filtered-pubs", userID)

	readable := models.OCIRegistry{Name: "readable", URL: "https://ghcr.io", Restricted: true}
	hidden := models.OCIRegistry{Name: "hidden", URL: "https://quay.io", Restricted: true}
	db.Create(&readable)
	db.Create(&hidden)
	grantRegistryAccessForTest(t, db, userID, readable.ID, "read")

	db.Create(&models.Publication{
		ProjectID:     project.ID,
		VersionNumber: 1,
		RegistryID:    readable.ID,
		Repository:    "visible-env",
		Tag:           "v1",
		PublishedBy:   userID,
	})
	db.Create(&models.Publication{
		ProjectID:     project.ID,
		VersionNumber: 1,
		RegistryID:    hidden.ID,
		Repository:    "hidden-env",
		Tag:           "v1",
		PublishedBy:   userID,
	})

	pubs, err := svc.ListPublications(project.ID.String(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pubs) != 1 {
		t.Fatalf("expected 1 readable publication, got %d: %+v", len(pubs), pubs)
	}
	if pubs[0].RegistryName != readable.Name || pubs[0].Repository != "visible-env" {
		t.Fatalf("unexpected publication returned: %+v", pubs[0])
	}
}

// --- UpdatePublication tests ---

func TestUpdatePublication_TogglesVisibility(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "toggle-vis", userID)

	// OCIRegistry and Publication already migrated in testSetup

	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", Restricted: true}
	db.Create(&registry)
	grantRegistryAccessForTest(t, db, userID, registry.ID, "write")

	pub := models.Publication{
		ProjectID:     project.ID,
		VersionNumber: 1,
		RegistryID:    registry.ID,
		Repository:    "my-env",
		Tag:           "v1",
		PublishedBy:   userID,
		IsPublic:      false,
	}
	db.Create(&pub)

	result, err := svc.UpdatePublication(context.Background(), project.ID.String(), pub.ID.String(), true, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsPublic {
		t.Error("expected IsPublic=true after update")
	}

	// Verify in DB
	var updated models.Publication
	db.First(&updated, pub.ID)
	if !updated.IsPublic {
		t.Error("expected IsPublic=true in DB")
	}
}

func TestUpdatePublication_NotFound(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "pub-nf", userID)

	// OCIRegistry and Publication already migrated in testSetup

	_, err := svc.UpdatePublication(context.Background(), project.ID.String(), uuid.New().String(), true, userID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdatePublication_RequiresRegistryWriteAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "private-vis", userID)

	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", Restricted: true}
	db.Create(&registry)
	grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	pub := models.Publication{
		ProjectID:     project.ID,
		VersionNumber: 1,
		RegistryID:    registry.ID,
		Repository:    "my-env",
		Tag:           "v1",
		PublishedBy:   userID,
		IsPublic:      false,
	}
	db.Create(&pub)

	_, err := svc.UpdatePublication(context.Background(), project.ID.String(), pub.ID.String(), true, userID)
	if err == nil {
		t.Fatal("expected forbidden error with read-only registry grant")
	}
	if !isForbiddenError(err, nil) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}

	var updated models.Publication
	db.First(&updated, pub.ID)
	if updated.IsPublic {
		t.Fatal("publication visibility changed despite missing registry write grant")
	}
}

func TestPublishProject_RequiresRegistryWriteAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "private-pub", userID)

	db.Create(&models.ProjectVersion{
		ProjectID:     project.ID,
		VersionNumber: 1,
		ContentHash:   "sha-private",
	})
	registry := models.OCIRegistry{Name: "reg", URL: "https://ghcr.io", Restricted: true}
	db.Create(&registry)
	grantRegistryAccessForTest(t, db, userID, registry.ID, "read")

	_, err := svc.PublishProject(context.Background(), project.ID.String(), PublishProjectRequest{
		RegistryID: registry.ID,
		Repository: "private-pub",
		Tag:        "v1",
	}, userID)
	if err == nil {
		t.Fatal("expected forbidden error with read-only registry grant")
	}
	if !isForbiddenError(err, nil) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}

	var count int64
	db.Model(&models.Publication{}).Where("project_id = ?", project.ID).Count(&count)
	if count != 0 {
		t.Fatalf("expected no publication record without registry write grant, got %d", count)
	}
}

func TestPublishProject_LocalMode_UploadsAssets(t *testing.T) {
	svc, db := testSetup(t, true) // isLocal=true
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "bundle-pub", userID)

	// Seed the project on disk with pixi files + one asset.
	projectPath := svc.executor.GetProjectPath(project)
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(projectPath, "pixi.toml"),
		[]byte("[project]\nname = \"bundle-pub\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"), 0o644)
	os.WriteFile(filepath.Join(projectPath, "pixi.lock"), []byte("version: 6\n"), 0o644)
	os.WriteFile(filepath.Join(projectPath, "notebook.ipynb"), []byte(`{"cells":[]}`), 0o644)

	// Seed a version row so PublishProject finds a latest version.
	db.Create(&models.ProjectVersion{
		ProjectID:     project.ID,
		VersionNumber: 1,
		ContentHash:   "sha-abcdef",
	})

	// Spin up the in-memory registry.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	regHost := mustParseHost(t, srv.URL)

	reg := models.OCIRegistry{
		Name:      "test-reg",
		URL:       "http://" + regHost,
		Namespace: "demo",
		IsDefault: true,
	}
	db.Create(&reg)

	_, err := svc.PublishProject(context.Background(), project.ID.String(), PublishProjectRequest{
		RegistryID: reg.ID,
		Repository: "bundle-pub",
		Tag:        "v1",
	}, userID)
	if err != nil {
		t.Fatalf("PublishProject: %v", err)
	}

	// Pull back via oci.PullBundle and verify the asset layer exists.
	result, err := oci.PullBundle(context.Background(),
		regHost+"/demo/bundle-pub", "v1",
		oci.PullOptions{PlainHTTP: true},
	)
	if err != nil {
		t.Fatalf("PullBundle: %v", err)
	}
	if len(result.Assets) != 1 || result.Assets[0].Path != "notebook.ipynb" {
		t.Errorf("expected one asset notebook.ipynb, got %+v", result.Assets)
	}
}

func mustParseHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestGetPublishDefaults_LocalMode_UsesBundleHash(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "bundle-defaults", userID)

	// Project on disk with pixi + one asset.
	projectPath := svc.executor.GetProjectPath(project)
	os.MkdirAll(projectPath, 0o755)
	pixiToml := "[project]\nname = \"bundle-defaults\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	pixiLock := "version: 6\n"
	assetBody := `{"cells":[]}`
	os.WriteFile(filepath.Join(projectPath, "pixi.toml"), []byte(pixiToml), 0o644)
	os.WriteFile(filepath.Join(projectPath, "pixi.lock"), []byte(pixiLock), 0o644)
	os.WriteFile(filepath.Join(projectPath, "notebook.ipynb"), []byte(assetBody), 0o644)

	db.Create(&models.OCIRegistry{Name: "r", URL: "https://ghcr.io", IsDefault: true})
	// Seed a version so GetPublishDefaults doesn't short-circuit to "latest".
	db.Create(&models.ProjectVersion{ProjectID: project.ID, VersionNumber: 1, ContentHash: "sha-oldstyle"})

	got, err := svc.GetPublishDefaults(project.ID.String(), userID, uuid.Nil)
	if err != nil {
		t.Fatalf("GetPublishDefaults: %v", err)
	}

	refs, err := oci.PreviewAssetRefs(projectPath)
	if err != nil {
		t.Fatalf("PreviewAssetRefs: %v", err)
	}
	want := contenthash.HashBundle(pixiToml, pixiLock, refs)
	if got.Tag != want {
		t.Errorf("tag mismatch: got %q want %q", got.Tag, want)
	}
}
