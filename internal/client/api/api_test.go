package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakePixi is a PixiRunner for a Service whose projects these tests never
// lock or install.
type fakePixi struct{}

func (fakePixi) Lock(context.Context, string, io.Writer) error      { return nil }
func (fakePixi) Install(context.Context, string, io.Writer) error   { return nil }
func (fakePixi) CheckLock(context.Context, string, io.Writer) error { return nil }

// testOptions returns Options with a real Service over a fresh database.
func testOptions(t *testing.T) Options {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "nebi-client.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := local.Migrate(db); err != nil {
		t.Fatal(err)
	}
	runner := jobs.NewMemory(jobs.Options{})
	t.Cleanup(runner.Close)
	svc, err := local.New(local.Options{DB: db, Jobs: runner, Pixi: fakePixi{}, ProjectsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return Options{Service: svc, Jobs: runner, Logger: testLogger(), Version: VersionInfo{NebiVersion: "1.2.3", Commit: "abc123"}}
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newHandler(t *testing.T, opts Options) http.Handler {
	t.Helper()
	h, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, path string, edit ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for _, e := range edit {
		e(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorBody {
	t.Helper()
	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error body %q is not an ErrorResponse: %v", rec.Body, err)
	}
	if resp.Error.Code == "" || resp.Error.Message == "" || resp.Error.Details == nil {
		t.Fatalf("error body %q lacks code, message or details", rec.Body)
	}
	return resp.Error
}

func TestNewRequiresDependencies(t *testing.T) {
	full := testOptions(t)
	for name, edit := range map[string]func(*Options){
		"service": func(o *Options) { o.Service = nil },
		"jobs":    func(o *Options) { o.Jobs = nil },
		"version": func(o *Options) { o.Version.NebiVersion = "" },
	} {
		opts := full
		edit(&opts)
		if _, err := New(opts); err == nil {
			t.Errorf("New without %s succeeded", name)
		}
	}
}

func TestVersion(t *testing.T) {
	h := newHandler(t, testOptions(t))
	rec := do(h, http.MethodGet, "/api/v1/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /version = %d %s", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"nebi_version": "1.2.3", "commit": "abc123", "client_api_version": ClientAPIVersion, "mode": "local"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("GET /version = %v, want %v", got, want)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestClientAPIVersionIsMajorMinor(t *testing.T) {
	var major, minor int
	if n, err := fmt.Sscanf(ClientAPIVersion, "%d.%d", &major, &minor); n != 2 || err != nil ||
		fmt.Sprintf("%d.%d", major, minor) != ClientAPIVersion {
		t.Fatalf("ClientAPIVersion %q is not major.minor", ClientAPIVersion)
	}
}

type fakePicker struct {
	path string
	err  error
}

func (p fakePicker) PickFolder(context.Context) (string, error) { return p.path, p.err }

type fakeWindow struct{ shown int }

func (w *fakeWindow) Show(context.Context) error { w.shown++; return nil }

func TestCapabilitiesAndPickFolder(t *testing.T) {
	t.Run("no picker", func(t *testing.T) {
		h := newHandler(t, testOptions(t))
		rec := do(h, http.MethodGet, "/api/v1/capabilities")
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"folder_picker":false}` {
			t.Fatalf("GET /capabilities = %d %s", rec.Code, rec.Body)
		}
		rec = do(h, http.MethodPost, "/api/v1/desktop/pick-folder")
		if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != CodeFolderPickerUnavailable {
			t.Fatalf("POST /desktop/pick-folder without a picker = %d %s", rec.Code, rec.Body)
		}
		rec = do(h, http.MethodPost, "/api/v1/desktop/show-window")
		if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != CodeWindowUnavailable {
			t.Fatalf("POST /desktop/show-window without a window = %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("picker", func(t *testing.T) {
		opts := testOptions(t)
		opts.FolderPicker = fakePicker{path: "/home/me/projects"}
		h := newHandler(t, opts)
		rec := do(h, http.MethodGet, "/api/v1/capabilities")
		if strings.TrimSpace(rec.Body.String()) != `{"folder_picker":true}` {
			t.Fatalf("GET /capabilities = %s", rec.Body)
		}
		rec = do(h, http.MethodPost, "/api/v1/desktop/pick-folder")
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"path":"/home/me/projects"}` {
			t.Fatalf("POST /desktop/pick-folder = %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		opts := testOptions(t)
		opts.FolderPicker = fakePicker{}
		rec := do(newHandler(t, opts), http.MethodPost, "/api/v1/desktop/pick-folder")
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("POST /desktop/pick-folder after cancel = %d %q", rec.Code, rec.Body)
		}
	})

	t.Run("picker fails", func(t *testing.T) {
		opts := testOptions(t)
		opts.FolderPicker = fakePicker{err: errors.New("secret internal detail")}
		rec := do(newHandler(t, opts), http.MethodPost, "/api/v1/desktop/pick-folder")
		body := decodeError(t, rec)
		if rec.Code != http.StatusInternalServerError || body.Code != CodeInternal || strings.Contains(rec.Body.String(), "secret") {
			t.Fatalf("POST /desktop/pick-folder with a failing picker = %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("window", func(t *testing.T) {
		opts := testOptions(t)
		w := &fakeWindow{}
		opts.Window = w
		rec := do(newHandler(t, opts), http.MethodPost, "/api/v1/desktop/show-window")
		if rec.Code != http.StatusNoContent || w.shown != 1 {
			t.Fatalf("POST /desktop/show-window = %d, shown %d times", rec.Code, w.shown)
		}
	})
}

// Unknown routes and methods answer in the error format, not net/http's
// plain text.
func TestRouterErrors(t *testing.T) {
	h := newHandler(t, testOptions(t))
	rec := do(h, http.MethodGet, "/api/v1/nothing-here")
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != CodeNotFound {
		t.Fatalf("unknown route = %d %s", rec.Code, rec.Body)
	}
	rec = do(h, http.MethodDelete, "/api/v1/version")
	if rec.Code != http.StatusMethodNotAllowed || decodeError(t, rec).Code != CodeMethodNotAllowed {
		t.Fatalf("wrong method = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Allow") != "GET" {
		t.Errorf("Allow = %q, want GET", rec.Header().Get("Allow"))
	}
	if rec := do(h, http.MethodHead, "/api/v1/version"); rec.Code != http.StatusOK {
		t.Errorf("HEAD /version = %d", rec.Code)
	}
}

func TestRouterRecoversPanics(t *testing.T) {
	r := newRouter(testLogger())
	r.handle(http.MethodGet, "/boom", func(http.ResponseWriter, *http.Request) error { panic("boom") })
	rec := do(r, http.MethodGet, "/api/v1/boom")
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != CodeInternal {
		t.Fatalf("panicking handler = %d %s", rec.Code, rec.Body)
	}
}

func TestRouterRefusesDuplicateRoute(t *testing.T) {
	r := newRouter(testLogger())
	h := func(http.ResponseWriter, *http.Request) error { return nil }
	r.handle(http.MethodGet, "/x", h)
	r.handle(http.MethodPost, "/x", h) // another method on the same path is fine
	defer func() {
		if recover() == nil {
			t.Fatal("registering GET /x twice did not panic")
		}
	}()
	r.handle(http.MethodGet, "/x", h)
}

// A resource adds a file with a route group, which registers its routes
// and its errors, and one line in routeGroups. This is what such a file
// does, with an SDK error and its details.
func TestRouteGroupRegistersRoutesAndErrors(t *testing.T) {
	busy := &local.BusyError{ProjectID: uuid.New(), JobIDs: []uuid.UUID{uuid.New(), uuid.New()}}
	group := func(r *router, _ *Options) {
		r.errors.Register(
			ErrorMapping{Err: local.ErrNameTaken, Status: http.StatusConflict, Code: "name_taken"},
			ErrorMapping{Err: local.ErrProjectBusy, Status: http.StatusConflict, Code: "project_busy",
				Details: func(err error) map[string]any {
					var b *local.BusyError
					if errors.As(err, &b) {
						return map[string]any{"job_ids": b.JobIDs}
					}
					return nil
				}},
		)
		r.handle(http.MethodPost, "/things", func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("creating thing: %w", local.ErrNameTaken)
		})
		r.handle(http.MethodDelete, "/things/{id}", func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("deleting: %w", busy)
		})
	}
	orig := routeGroups
	routeGroups = append(slices.Clone(orig), group)
	t.Cleanup(func() { routeGroups = orig })

	h := newHandler(t, testOptions(t))
	rec := do(h, http.MethodPost, "/api/v1/things")
	body := decodeError(t, rec)
	if rec.Code != http.StatusConflict || body.Code != "name_taken" || body.Message != "creating thing: project name is already taken" ||
		len(body.Details) != 0 {
		t.Fatalf("name taken = %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"details":{}`) {
		t.Errorf("empty details are not {} in %s", rec.Body)
	}

	rec = do(h, http.MethodDelete, "/api/v1/things/"+uuid.NewString())
	body = decodeError(t, rec)
	if rec.Code != http.StatusConflict || body.Code != "project_busy" {
		t.Fatalf("busy = %d %s", rec.Code, rec.Body)
	}
	ids, _ := body.Details["job_ids"].([]any)
	if len(ids) != 2 || ids[0] != busy.JobIDs[0].String() || ids[1] != busy.JobIDs[1].String() {
		t.Fatalf("busy details = %v, want the job IDs %v", body.Details, busy.JobIDs)
	}
}

func TestErrorMapResolve(t *testing.T) {
	var m ErrorMap
	specific := errors.New("specific")
	general := fmt.Errorf("general: %w", specific)
	m.Register(
		ErrorMapping{Err: general, Status: http.StatusConflict, Code: "general"},
		ErrorMapping{Err: specific, Status: http.StatusUnprocessableEntity, Code: "specific"},
	)
	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
		msg    string
	}{
		"exact":   {specific, http.StatusUnprocessableEntity, "specific", "specific"},
		"wrapped": {fmt.Errorf("op: %w", specific), http.StatusUnprocessableEntity, "specific", "op: specific"},
		// The first mapping that matches wins: general wraps specific, so
		// general's error matches general first.
		"first match": {general, http.StatusConflict, "general", "general: specific"},
		"joined":      {errors.Join(errors.New("other"), specific), http.StatusUnprocessableEntity, "specific", "other\nspecific"},
		"api error": {&Error{Status: http.StatusTeapot, Code: "teapot", Message: "short and stout"},
			http.StatusTeapot, "teapot", "short and stout"},
		"unknown": {errors.New("disk on fire at /secret/path"), http.StatusInternalServerError, CodeInternal, "internal error"},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := m.Resolve(tc.err)
			if status != tc.status || body.Code != tc.code || body.Message != tc.msg || body.Details == nil {
				t.Fatalf("Resolve(%v) = %d %+v, want %d %s %q", tc.err, status, body, tc.status, tc.code, tc.msg)
			}
		})
	}
}

func TestErrorMapRegisterRefusesInvalidMappings(t *testing.T) {
	sentinel := errors.New("x")
	for name, mp := range map[string]ErrorMapping{
		"nil error":     {Status: 400, Code: "x"},
		"status 200":    {Err: sentinel, Status: 200, Code: "x"},
		"status 600":    {Err: sentinel, Status: 600, Code: "x"},
		"empty code":    {Err: sentinel, Status: 400},
		"code case":     {Err: sentinel, Status: 400, Code: "NameTaken"},
		"code dash":     {Err: sentinel, Status: 400, Code: "name-taken"},
		"code trailing": {Err: sentinel, Status: 400, Code: "name_"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("Register(%+v) did not panic", mp)
				}
			}()
			var m ErrorMap
			m.Register(mp)
		})
	}
	t.Run("twice", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("registering an error twice did not panic")
			}
		}()
		var m ErrorMap
		m.Register(ErrorMapping{Err: sentinel, Status: 400, Code: "a"})
		m.Register(ErrorMapping{Err: sentinel, Status: 409, Code: "b"})
	})
}
