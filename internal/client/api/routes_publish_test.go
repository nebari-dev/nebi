package api_test

import (
	"encoding/json"
	"io"
	stdlog "log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	ggcr "github.com/google/go-containerregistry/pkg/registry"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/zalando/go-keyring"
)

func init() { keyring.MockInit() }

func newPublishAPI(t *testing.T) (http.Handler, *client.Stack) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Storage.ProjectsDir = filepath.Join(dataDir, "projects")
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := client.NewStack(client.StackOptions{
		DB: gdb, Executor: exec, DataDir: dataDir, ProjectsDir: cfg.Storage.ProjectsDir,
		InlineLog: io.Discard, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	stack.API().Register(r.Group("/api/v1"))
	return r, stack
}

func publishCall(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegistryRoutes(t *testing.T) {
	h, _ := newPublishAPI(t)
	const C = "/api/v1/client"

	rec := publishCall(t, h, http.MethodPost, C+"/registries", `{"name":"Quay","url":"quay.io","namespace":"nebari","username":"sam","password":"pw"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "pw\"") {
		t.Fatalf("password echoed: %s", rec.Body)
	}
	var added shared.RegistryItem
	_ = json.Unmarshal(rec.Body.Bytes(), &added)
	if !added.HasCredentials || added.Namespace != "nebari" {
		t.Fatalf("added = %+v", added)
	}

	rec = publishCall(t, h, http.MethodPost, C+"/registries", `{"name":"","url":"quay.io"}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"name"`) {
		t.Fatalf("validation: %d %s", rec.Code, rec.Body)
	}

	rec = publishCall(t, h, http.MethodPut, C+"/registries/"+added.ID.String(), `{"name":"Quay.io","is_default":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Quay.io"`) || !strings.Contains(rec.Body.String(), `"is_default":true`) {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}

	rec = publishCall(t, h, http.MethodGet, C+"/registries", "")
	var items []shared.RegistryItem
	_ = json.Unmarshal(rec.Body.Bytes(), &items)
	if rec.Code != http.StatusOK || len(items) != 2 || items[0].ID != added.ID {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	if rec := publishCall(t, h, http.MethodDelete, C+"/registries/"+added.ID.String(), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := publishCall(t, h, http.MethodDelete, C+"/registries/"+added.ID.String(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d %s", rec.Code, rec.Body)
	}
}

func TestPublishRoutes(t *testing.T) {
	h, stack := newPublishAPI(t)
	const C = "/api/v1/client"
	srv := httptest.NewServer(ggcr.New(ggcr.Logger(stdlog.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)

	rec := publishCall(t, h, http.MethodPost, C+"/registries", `{"name":"Local","url":"http://`+u.Host+`","namespace":"team"}`)
	var reg shared.RegistryItem
	_ = json.Unmarshal(rec.Body.Bytes(), &reg)
	rec = publishCall(t, h, http.MethodPost, C+"/registries", `{"name":"GitHub","url":"ghcr.io","namespace":"org"}`)
	var gh shared.RegistryItem
	_ = json.Unmarshal(rec.Body.Bytes(), &gh)
	if gh.CanUnpublish || gh.UnpublishReason == "" {
		t.Fatalf("ghcr capability = %+v", gh)
	}

	gdb := stack.SDK.Deps().DB
	user := stack.SDK.Deps().LocalUserID
	p := models.Project{Name: "demo", OwnerID: user, Status: models.ProjectStatusReady}
	gdb.Omit("Owner").Create(&p)
	toml := "[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	gdb.Omit("Project", "Job", "CreatedByUser").Create(&models.ProjectVersion{ProjectID: p.ID, VersionNumber: 1, ManifestContent: toml, LockFileContent: "version: 6\n", PackageMetadata: "[]", CreatedBy: user})
	base := C + "/projects/" + p.ID.String()

	rec = publishCall(t, h, http.MethodGet, base+"/versions/1/publish-defaults", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"repository":"team/demo","tag":"1"`) {
		t.Fatalf("defaults: %d %s", rec.Code, rec.Body)
	}

	rec = publishCall(t, h, http.MethodPost, base+"/versions/1/publish", `{"targets":[{"registry_id":"`+reg.ID.String()+`","repository":"team/demo","tag":"bad tag"}]}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Use repository:tag, e.g. nebari/project_1:5.") {
		t.Fatalf("invalid ref: %d %s", rec.Code, rec.Body)
	}

	rec = publishCall(t, h, http.MethodPost, base+"/versions/1/publish", `{"targets":[{"registry_id":"`+reg.ID.String()+`","repository":"team/demo","tag":"1"}]}`)
	var res shared.PublishResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || len(res.Results) != 1 || res.Results[0].Status != "published" {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	if rec := publishCall(t, h, http.MethodDelete, base+"/publications/"+res.Results[0].PublicationID.String(), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("unpublish: %d %s", rec.Code, rec.Body)
	}

	// Unpublish on a registry that cannot delete tags answers 501 unavailable.
	pub := models.Publication{ProjectID: p.ID, VersionNumber: 1, RegistryID: gh.ID, Repository: "org/demo", Tag: "1", RegistryHost: "ghcr.io", PublishedBy: user}
	gdb.Omit("Project", "Registry", "PublishedByUser").Create(&pub)
	rec = publishCall(t, h, http.MethodDelete, base+"/publications/"+pub.ID.String(), "")
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), `"code":"unavailable"`) {
		t.Fatalf("unpublish ghcr: %d %s", rec.Code, rec.Body)
	}

	// Pull from the registry.
	if _, err := oci.PublishFiles(t.Context(), []byte(toml), []byte("version: 6\n"), oci.Registry{Host: u.Host, PlainHTTP: true}, "team/pulled", "3"); err != nil {
		t.Fatal(err)
	}
	rec = publishCall(t, h, http.MethodGet, C+"/registries/"+reg.ID.String()+"/repositories?search=pull", "")
	if rec.Code != http.StatusOK || rec.Body.String() != `["team/pulled"]` {
		t.Fatalf("repositories: %d %s", rec.Code, rec.Body)
	}
	rec = publishCall(t, h, http.MethodGet, C+"/registries/"+reg.ID.String()+"/tags?repository=team/pulled", "")
	if rec.Code != http.StatusOK || rec.Body.String() != `["3"]` {
		t.Fatalf("tags: %d %s", rec.Code, rec.Body)
	}
	rec = publishCall(t, h, http.MethodPost, C+"/registries/"+reg.ID.String()+"/pull", `{"repository":"team/pulled","tag":"3","local_name":"pulled","install":false}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"type":"pull_registry"`) {
		t.Fatalf("pull: %d %s", rec.Code, rec.Body)
	}
	rec = publishCall(t, h, http.MethodPost, C+"/registries/"+reg.ID.String()+"/pull", `{"repository":"team/pulled","tag":"3","local_name":"pulled"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"name_taken"`) {
		t.Fatalf("pull name taken: %d %s", rec.Code, rec.Body)
	}
}
