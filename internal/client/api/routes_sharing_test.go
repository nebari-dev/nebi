package api_test

// Client REST sharing routes (local mode) proxied to a stub team server:
// status and error-code mapping, and the not-connected path.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/localext"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/store"
)

func newSharingLocalRouter(t *testing.T) (http.Handler, *client.Extension) {
	t.Helper()
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Auth.JWTSecret = "test-secret-for-client-sharing"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "client-sharing.db")
	cfg.Storage.ProjectsDir = t.TempDir()
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	database, err := db.New(cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(database, false); err != nil {
		t.Fatal(err)
	}
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	q := queue.NewMemoryQueue(8)
	t.Cleanup(func() { q.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ext := client.NewExtension(desktop.Capabilities{})
	if err := ext.Init(localext.Deps{Config: cfg, DB: database, Queue: q, Executor: exec, Logger: logger}); err != nil {
		t.Fatal(err)
	}
	return api.NewRouterWithOptions(cfg, database, q, exec, nil, logger, api.RouterOptions{Local: ext}), ext
}

type recordedCall struct {
	method, path, query, auth, body string
}

// stubServer answers team-server sharing calls with fixed responses.
func stubServer(t *testing.T, status int, body string) (*httptest.Server, *[]recordedCall) {
	t.Helper()
	var mu sync.Mutex
	calls := []recordedCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, recordedCall{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func connect(t *testing.T, ext *client.Extension, url string) {
	t.Helper()
	gdb := ext.Stack().SDK.Deps().DB
	if err := gdb.Save(&store.Config{ID: 1, ServerURL: url}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Save(&store.Credentials{ID: 1, Token: "tok", Username: "sam", AuthMode: "token"}).Error; err != nil {
		t.Fatal(err)
	}
}

func call(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Code
}

const accessBody = `{"entries":[{"principal":{"kind":"user","id":"u1","name":"sam","email":"sam@example.com"},"role":"owner"},{"principal":{"kind":"group","id":"g1","name":"team-jatic","members":6},"role":"edit"}],"can_manage":true}`

func TestClientSharingRoutesProxyToServer(t *testing.T) {
	h, ext := newSharingLocalRouter(t)
	const base = "/api/v1/client/remote"

	rec := call(h, http.MethodGet, base+"/projects/sp1/access", "")
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "not_connected" {
		t.Fatalf("not connected: %d %s", rec.Code, rec.Body)
	}

	srv, calls := stubServer(t, http.StatusOK, accessBody)
	connect(t, ext, srv.URL)

	rec = call(h, http.MethodGet, base+"/projects/sp1/access", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var list shared.AccessList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Entries) != 2 || list.Entries[1].Principal.Members != 6 || !list.CanManage {
		t.Fatalf("list body: %v %s", err, rec.Body)
	}

	if rec := call(h, http.MethodPost, base+"/projects/sp1/access", `{"principals":[{"kind":"group","id":"g1"}],"role":"edit"}`); rec.Code != http.StatusOK {
		t.Fatalf("grant: %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, http.MethodPut, base+"/projects/sp1/access/user/u2", `{"role":"view"}`); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, http.MethodDelete, base+"/projects/sp1/access/user/u2", ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
	}
	srvPrincipals, pcalls := stubServer(t, http.StatusOK, `[{"kind":"user","id":"u2","name":"alex","email":"alex@example.com"}]`)
	connect(t, ext, srvPrincipals.URL)
	rec = call(h, http.MethodGet, base+"/principals?q=al%20ex", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"alex"`) {
		t.Fatalf("principals: %d %s", rec.Code, rec.Body)
	}
	// An empty query never reaches the server.
	rec = call(h, http.MethodGet, base+"/principals?q=", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty principals: %d %s", rec.Code, rec.Body)
	}

	want := []recordedCall{
		{http.MethodGet, "/api/v1/projects/sp1/access", "", "Bearer tok", ""},
		{http.MethodPost, "/api/v1/projects/sp1/access", "", "Bearer tok", `{"principals":[{"kind":"group","id":"g1"}],"role":"edit"}`},
		{http.MethodPut, "/api/v1/projects/sp1/access/user/u2", "", "Bearer tok", `{"role":"view"}`},
		{http.MethodDelete, "/api/v1/projects/sp1/access/user/u2", "", "Bearer tok", ""},
	}
	if len(*calls) != len(want) {
		t.Fatalf("server calls: %+v", *calls)
	}
	for i, c := range *calls {
		if c != want[i] {
			t.Errorf("call %d: got %+v want %+v", i, c, want[i])
		}
	}
	if len(*pcalls) != 1 || (*pcalls)[0].path != "/api/v1/principals" || (*pcalls)[0].query != "q=al+ex" {
		t.Fatalf("principal calls: %+v", *pcalls)
	}
}

func TestClientSharingRoutesMapServerErrors(t *testing.T) {
	h, ext := newSharingLocalRouter(t)
	cases := []struct {
		status   int
		wantCode int
		code     string
	}{
		{http.StatusForbidden, http.StatusForbidden, "server_forbidden"},
		{http.StatusConflict, http.StatusConflict, "server_conflict"},
		{http.StatusNotFound, http.StatusNotFound, "server_not_found"},
		{http.StatusUnauthorized, http.StatusBadGateway, "server_auth"},
	}
	for _, tc := range cases {
		srv, _ := stubServer(t, tc.status, `{"error":"nope","code":"x"}`)
		connect(t, ext, srv.URL)
		rec := call(h, http.MethodDelete, "/api/v1/client/remote/projects/sp1/access/user/u1", "")
		if rec.Code != tc.wantCode || errorCode(t, rec) != tc.code || !strings.Contains(rec.Body.String(), "nope") {
			t.Errorf("server %d: got %d %s", tc.status, rec.Code, rec.Body)
		}
	}

	// Input the server would reject never leaves the machine.
	srv, calls := stubServer(t, http.StatusOK, accessBody)
	connect(t, ext, srv.URL)
	rec := call(h, http.MethodPost, "/api/v1/client/remote/projects/sp1/access", `{"principals":[{"kind":"user","id":"u1"}],"role":"owner"}`)
	if rec.Code != http.StatusUnprocessableEntity || errorCode(t, rec) != "invalid" {
		t.Fatalf("bad role: %d %s", rec.Code, rec.Body)
	}
	if len(*calls) != 0 {
		t.Fatalf("server was called: %+v", *calls)
	}

	// An unreachable server is a 502.
	srv.Close()
	rec = call(h, http.MethodGet, "/api/v1/client/remote/projects/sp1/access", "")
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "server_unreachable" {
		t.Fatalf("unreachable: %d %s", rec.Code, rec.Body)
	}
}
