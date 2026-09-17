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

// Research evidence for #582: exercise the existing service and OCI transport,
// without a worker, external registry, or dependency installation.
func TestWorkspaceNameSpikeService(t *testing.T) {
	for _, local := range []bool{true, false} {
		for _, manifestName := range []string{"", "name = \"manifest-name\"\n"} {
			t.Run(fmt.Sprintf("local=%t/named=%t", local, manifestName != ""), func(t *testing.T) {
				svc, db := testSetup(t, local)
				owner := createTestUser(t, db, "owner")
				other := createTestUser(t, db, "other")
				manifest := "[workspace]\n" + manifestName + "channels = []\nplatforms = [\"linux-64\"]\n"
				ctx := context.Background()
				original, err := svc.Create(ctx, CreateRequest{Name: "analysis", PixiToml: manifest}, owner)
				if err != nil {
					t.Fatal(err)
				}

				srv := httptest.NewServer(registry.New())
				defer srv.Close()
				u, err := url.Parse(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				src := t.TempDir()
				lock := "version: 6\n# transport fixture; not an installation test\n"
				for file, content := range map[string]string{"pixi.toml": manifest, "pixi.lock": lock} {
					if err := os.WriteFile(filepath.Join(src, file), []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := oci.Publish(ctx, src, oci.Registry{Host: u.Host, Namespace: "demo", PlainHTTP: true}, "analysis", "v1"); err != nil {
					t.Fatal(err)
				}
				reg := models.OCIRegistry{Name: "spike", URL: srv.URL, Namespace: "demo", Restricted: true}
				if err := db.Create(&reg).Error; err != nil {
					t.Fatal(err)
				}
				if !local {
					grantRegistryAccessForTest(t, db, other, reg.ID, "read")
				}
				imported, err := svc.ImportFromRegistry(ctx, reg.ID.String(), ImportFromRegistryRequest{
					Repository: "analysis", Tag: "v1", Name: "analysis",
				}, other)
				if err != nil {
					t.Fatal(err)
				}
				if original.ID == imported.ID || original.Name != imported.Name {
					t.Fatal("same labels must retain separate IDs")
				}
				if svc.GetWorkspacePath(original) == svc.GetWorkspacePath(imported) {
					t.Fatal("same labels must retain separate paths")
				}
				visible, err := svc.List(owner)
				if err != nil {
					t.Fatal(err)
				}
				wantVisible := 1
				if local {
					wantVisible = 2
				}
				if len(visible) != wantVisible {
					t.Fatalf("visibility: got %d, want %d", len(visible), wantVisible)
				}
				var job models.Job
				if err := db.Where("workspace_id = ?", imported.ID).First(&job).Error; err != nil {
					t.Fatal(err)
				}
				staging, ok := job.Metadata["import_staging_dir"].(string)
				if !ok || staging == "" {
					t.Fatal("missing import staging directory")
				}
				for file, want := range map[string]string{"pixi.toml": manifest, "pixi.lock": lock} {
					// Existing PullBundle trims core text in team mode. Record
					// that behavior instead of claiming byte-for-byte transport.
					if !local {
						want = strings.TrimSpace(want)
					}
					got, err := os.ReadFile(filepath.Join(staging, file))
					if err != nil || string(got) != want {
						t.Fatalf("%s was not preserved: %q, %v", file, got, err)
					}
				}
			})
		}
	}
}
