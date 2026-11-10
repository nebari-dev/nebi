package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/sdk"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

const compareRouteToml = `[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.12.*"
%s
`

func newCompareRouter(t *testing.T) (http.Handler, *sdk.Client, models.Project) {
	t.Helper()
	gin.SetMode(gin.TestMode)
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
	c, err := sdk.New(shared.Deps{DB: gdb, DataDir: dir, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	installed := 1
	p := models.Project{Name: "demo", OwnerID: c.Deps().LocalUserID, Status: models.ProjectStatusReady, InstalledVersion: &installed}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for n, extra := range map[int]string{1: `numpy = ">=1.26"`, 2: `pandas = "*"`} {
		v := models.ProjectVersion{
			ProjectID: p.ID, VersionNumber: n, PackageMetadata: "[]", CreatedBy: c.Deps().LocalUserID,
			ManifestContent: fmt.Sprintf(compareRouteToml, extra),
		}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
	r := gin.New()
	New(c, nil, desktop.Capabilities{}).registerCompareRoutes(r.Group("/api/v1/client"))
	return r, c, p
}

func getCompare(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestCompareRoute(t *testing.T) {
	h, _, p := newCompareRouter(t)
	rec := getCompare(t, h, "/api/v1/client/projects/"+p.ID.String()+"/compare?base=1&target=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var res shared.CompareResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Base.Number != 1 || !res.Base.Installed || res.Target.Number != 2 || res.Target.Installed {
		t.Errorf("refs = %+v / %+v", res.Base, res.Target)
	}
	if res.RequestedCounts.Added != 1 || res.RequestedCounts.Removed != 1 || res.RequestedCounts.Same != 1 {
		t.Errorf("counts = %+v", res.RequestedCounts)
	}
}

func TestCompareRouteErrors(t *testing.T) {
	h, _, p := newCompareRouter(t)
	base := "/api/v1/client/projects/" + p.ID.String() + "/compare"
	cases := []struct {
		name, path string
		status     int
		code       string
	}{
		{"missing params", base, http.StatusBadRequest, "bad_request"},
		{"non-numeric", base + "?base=a&target=2", http.StatusBadRequest, "bad_request"},
		{"zero", base + "?base=0&target=2", http.StatusBadRequest, "bad_request"},
		{"bad project id", "/api/v1/client/projects/nope/compare?base=1&target=2", http.StatusBadRequest, "bad_request"},
		{"unknown project", "/api/v1/client/projects/" + uuid.NewString() + "/compare?base=1&target=2", http.StatusNotFound, "not_found"},
		{"unknown version", base + "?base=1&target=7", http.StatusNotFound, "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := getCompare(t, h, tc.path)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			var body ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != tc.code || body.Error == "" {
				t.Errorf("body = %+v, want code %q", body, tc.code)
			}
		})
	}
}
