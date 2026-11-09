package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/store"
)

const compareTomlV1 = `[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.12.*"
numpy = ">=1.26"
scipy = "*"
`

const compareTomlV2 = `[workspace]
name = "demo"
channels = ["conda-forge", "bioconda"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.12.*"
numpy = ">=2.0"
pandas = "*"
`

const compareLockV1 = `version: 6
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda
      - conda: https://conda.anaconda.org/conda-forge/linux-64/numpy-1.26.4-py312h0_0.conda
      - conda: https://conda.anaconda.org/conda-forge/linux-64/scipy-1.14.1-py312h0_0.conda
      osx-arm64:
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/python-3.12.6-h739c21a_0_cpython.conda
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/numpy-1.26.3-py312h1_0.conda
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/scipy-1.14.0-py312h1_0.conda
packages: []
`

const compareLockV2 = `version: 6
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda
      - conda: https://conda.anaconda.org/conda-forge/linux-64/numpy-2.1.3-py312h0_0.conda
      - conda: https://conda.anaconda.org/conda-forge/linux-64/pandas-2.2.3-py312h0_0.conda
      osx-arm64:
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/python-3.12.6-h739c21a_0_cpython.conda
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/numpy-2.1.2-py312h1_0.conda
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/pandas-2.2.2-py312h1_0.conda
packages: []
`

type compareFixture struct {
	c       *Client
	project models.Project
}

func newCompareFixture(t *testing.T) *compareFixture {
	t.Helper()
	dir := t.TempDir()
	gdb, err := local.OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	c, err := New(shared.Deps{
		DB:      gdb,
		DataDir: dir,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	installed := 2
	p := models.Project{Name: "demo", OwnerID: c.Deps().LocalUserID, Status: models.ProjectStatusReady, InstalledVersion: &installed}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	f := &compareFixture{c: c, project: p}
	f.addVersion(t, 1, compareTomlV1, compareLockV1)
	f.addVersion(t, 2, compareTomlV2, compareLockV2)
	return f
}

func (f *compareFixture) addVersion(t *testing.T, n int, toml, lock string) {
	t.Helper()
	v := models.ProjectVersion{
		ProjectID: f.project.ID, VersionNumber: n,
		ManifestContent: toml, LockFileContent: lock, PackageMetadata: "[]",
		CreatedBy: f.c.Deps().LocalUserID,
	}
	if err := f.c.Deps().DB.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
}

func (f *compareFixture) linkServer(t *testing.T, serverProjectID string) {
	t.Helper()
	r := models.ProjectRemote{
		ProjectID: f.project.ID, Kind: models.RemoteKindServer, Name: "team-nebi",
		ServerURL: "http://example.invalid", ServerProjectID: serverProjectID, ServerProjectName: "demo",
	}
	if err := f.c.Deps().DB.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
}

func (f *compareFixture) connect(t *testing.T, url string) {
	t.Helper()
	db := f.c.Deps().DB
	if err := db.Save(&store.Config{ID: 1, ServerURL: url, ServerName: "team-nebi"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&store.Credentials{ID: 1, Token: "t", Username: "sam", AuthMode: "token"}).Error; err != nil {
		t.Fatal(err)
	}
}

func pinMachinePlatform(t *testing.T, p string) {
	t.Helper()
	prev := machinePlatform
	machinePlatform = func() string { return p }
	t.Cleanup(func() { machinePlatform = prev })
}

func entry(t *testing.T, rows []artifact.PackageDiffEntry, name string) artifact.PackageDiffEntry {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q in %+v", name, rows)
	return artifact.PackageDiffEntry{}
}

func TestCompareLocalVersions(t *testing.T) {
	pinMachinePlatform(t, "linux-64")
	f := newCompareFixture(t)

	res, err := f.c.Compare(context.Background(), f.project.ID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Base != (shared.CompareVersionRef{Number: 1}) {
		t.Errorf("base ref = %+v", res.Base)
	}
	if res.Target != (shared.CompareVersionRef{Number: 2, Installed: true}) {
		t.Errorf("target ref = %+v", res.Target)
	}
	if res.Platform != "linux-64" {
		t.Errorf("platform = %q", res.Platform)
	}
	if want := (artifact.DiffCounts{Added: 1, Removed: 1, Changed: 1, Same: 1}); res.RequestedCounts != want {
		t.Errorf("requested counts = %+v, want %+v", res.RequestedCounts, want)
	}
	numpy := entry(t, res.Requested, "numpy")
	if numpy.Change != artifact.ChangeChanged || numpy.Base.Version != "1.26.4" || numpy.Compare.Version != "2.1.3" {
		t.Errorf("numpy = %+v base=%+v compare=%+v", numpy, numpy.Base, numpy.Compare)
	}
	if scipy := entry(t, res.Requested, "scipy"); scipy.Change != artifact.ChangeRemoved || scipy.Compare != nil {
		t.Errorf("scipy = %+v", scipy)
	}
	if pandas := entry(t, res.Requested, "pandas"); pandas.Change != artifact.ChangeAdded || pandas.Base != nil {
		t.Errorf("pandas = %+v", pandas)
	}
	if len(res.Channels.Added) != 1 || res.Channels.Added[0] != "bioconda" {
		t.Errorf("channels = %+v", res.Channels)
	}

	// The JSON body flattens the diff next to base/target.
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"base", "target", "platform", "requested", "requested_counts", "resolved", "resolved_counts", "channels", "platforms"} {
		if _, ok := body[k]; !ok {
			t.Errorf("JSON body misses %q: %s", k, raw)
		}
	}
}

func TestComparePrefersMachinePlatform(t *testing.T) {
	f := newCompareFixture(t)

	pinMachinePlatform(t, "osx-arm64")
	res, err := f.c.Compare(context.Background(), f.project.ID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Platform != "osx-arm64" {
		t.Fatalf("platform = %q, want osx-arm64", res.Platform)
	}
	if numpy := entry(t, res.Requested, "numpy"); numpy.Base.Version != "1.26.3" || numpy.Compare.Version != "2.1.2" {
		t.Errorf("numpy = %+v / %+v", numpy.Base, numpy.Compare)
	}

	// A platform no lock has falls back to artifact's choice (first
	// compare-manifest platform present in a lock).
	pinMachinePlatform(t, "win-64")
	res, err = f.c.Compare(context.Background(), f.project.ID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Platform != "linux-64" {
		t.Errorf("fallback platform = %q, want linux-64", res.Platform)
	}
}

func TestCompareSameVersionHasNoChanges(t *testing.T) {
	pinMachinePlatform(t, "linux-64")
	f := newCompareFixture(t)
	res, err := f.c.Compare(context.Background(), f.project.ID, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestedCounts.HasChanges() || res.RequestedCounts.Same != 3 {
		t.Errorf("counts = %+v", res.RequestedCounts)
	}
	if !res.Base.Installed || !res.Target.Installed {
		t.Errorf("both sides are the installed version: %+v %+v", res.Base, res.Target)
	}
}

func TestCompareErrors(t *testing.T) {
	f := newCompareFixture(t)
	ctx := context.Background()

	var verr *shared.ValidationError
	if _, err := f.c.Compare(ctx, f.project.ID, 0, 2); !errors.As(err, &verr) || verr.Field != "base" {
		t.Errorf("base 0: %v", err)
	}
	if _, err := f.c.Compare(ctx, f.project.ID, 1, 0); !errors.As(err, &verr) || verr.Field != "target" {
		t.Errorf("target 0: %v", err)
	}
	if _, err := f.c.Compare(ctx, uuid.New(), 1, 2); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown project: %v", err)
	}
	// Not on this machine and the project is not linked: no network call.
	if _, err := f.c.Compare(ctx, f.project.ID, 1, 9); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("missing version, unlinked: %v", err)
	}
	// Linked but no server connected: still a plain not-found.
	f.linkServer(t, uuid.NewString())
	if _, err := f.c.Compare(ctx, f.project.ID, 9, 1); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("missing version, linked, not connected: %v", err)
	}
	// Corrupt stored files surface as an error, not a silent empty diff.
	f.addVersion(t, 3, "not = [valid", "")
	if _, err := f.c.Compare(ctx, f.project.ID, 1, 3); err == nil {
		t.Error("corrupt pixi.toml: want error")
	}
}

// remoteImplemented reports whether sdk/remote can fetch server version
// files yet (it is a stub until the remote unit lands).
func remoteImplemented(f *compareFixture) bool {
	_, _, err := f.c.Remote.ServerVersionFiles(context.Background(), f.project.ID, 1)
	return !errors.Is(err, shared.ErrNotImplemented)
}

func TestCompareServerOnlyVersion(t *testing.T) {
	pinMachinePlatform(t, "linux-64")
	f := newCompareFixture(t)
	serverProjectID := uuid.NewString()
	f.linkServer(t, serverProjectID)

	const serverToml = `[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.13.*"
numpy = ">=2.0"
pandas = "*"
rich = "*"
`
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/projects/"+serverProjectID+"/sync/versions/3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 3, "description": "server change", "author": "alex",
			"created_at": "2026-10-01T00:00:00Z", "content_hash": "sha-0123456789ab",
			"base_version": 2, "tags": []string{"latest"}, "publications": []any{},
			"pixi_toml": serverToml, "pixi_lock": "",
		})
	})
	mux.HandleFunc("/api/v1/projects/"+serverProjectID+"/sync/versions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		vs := []map[string]any{}
		for n := 1; n <= 3; n++ {
			vs = append(vs, map[string]any{
				"number": n, "description": fmt.Sprintf("v%d", n), "author": "alex",
				"created_at": "2026-10-01T00:00:00Z", "content_hash": fmt.Sprintf("sha-00000000000%d", n),
				"tags": []string{}, "publications": []any{},
			})
		}
		_ = json.NewEncoder(w).Encode(vs)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	f.connect(t, srv.URL)

	if !remoteImplemented(f) {
		// Stub-safe until sdk/remote serves version files: a server-only
		// side is reported as not found instead of failing with 501.
		if _, err := f.c.Compare(context.Background(), f.project.ID, 2, 3); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("stubbed remote: want ErrNotFound, got %v", err)
		}
		t.Skip("sdk/remote.ServerVersionFiles is not implemented yet")
	}

	res, err := f.c.Compare(context.Background(), f.project.ID, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Target.ServerOnly || res.Base.ServerOnly {
		t.Errorf("server_only flags: base %+v target %+v", res.Base, res.Target)
	}
	if !res.Base.Installed {
		t.Errorf("base should be the installed version: %+v", res.Base)
	}
	if res.RequestedCounts.Added != 1 || res.RequestedCounts.Changed < 1 {
		t.Errorf("requested counts = %+v", res.RequestedCounts)
	}
	// rich comes from the server version, which has no lock: the row shows
	// the constraint.
	if rich := entry(t, res.Requested, "rich"); rich.Compare == nil || rich.Compare.Version != "*" {
		t.Errorf("rich = %+v", rich)
	}
}

func TestCompareServerUnreachable(t *testing.T) {
	pinMachinePlatform(t, "linux-64")
	f := newCompareFixture(t)
	f.linkServer(t, uuid.NewString())
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens there any more
	f.connect(t, url)

	if !remoteImplemented(f) {
		t.Skip("sdk/remote.ServerVersionFiles is not implemented yet")
	}
	_, err := f.c.Compare(context.Background(), f.project.ID, 2, 3)
	var rerr *shared.RemoteError
	if !errors.As(err, &rerr) || rerr.Status != 0 {
		t.Fatalf("want unreachable RemoteError, got %v", err)
	}

	// Local-only compares keep working while the server is down.
	if _, err := f.c.Compare(context.Background(), f.project.ID, 1, 2); err != nil {
		t.Fatalf("local compare with server down: %v", err)
	}
}
