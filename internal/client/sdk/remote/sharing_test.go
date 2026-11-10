package remote_test

// The share dialog SDK against a real team-mode server (production router,
// login, RBAC) served over HTTP.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/api"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/remote"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type shareServer struct {
	url     string
	db      *gorm.DB
	project uuid.UUID
	users   map[string]uuid.UUID
	tokens  map[string]string
}

func startShareServer(t *testing.T) *shareServer {
	t.Helper()
	cfg := &config.Config{Mode: config.ModeTeam}
	cfg.Auth.Type = "basic"
	cfg.Auth.JWTSecret = "test-secret-for-remote-sharing"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "team.db")
	cfg.Storage.ProjectsDir = t.TempDir()
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
	q := queue.NewMemoryQueue(16)
	t.Cleanup(func() { q.Close() })
	router := api.NewRouter(cfg, database, q, exec, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	s := &shareServer{url: srv.URL, db: database, users: map[string]uuid.UUID{}, tokens: map[string]string{}}
	for _, name := range []string{"sam", "alex", "jordan"} {
		hash, err := auth.HashPassword("pw")
		if err != nil {
			t.Fatal(err)
		}
		u := models.User{Username: name, Email: name + "@example.com", PasswordHash: hash}
		if err := database.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		s.users[name] = u.ID
		resp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(`{"username":"`+name+`","password":"pw"}`))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if body.Token == "" {
			t.Fatalf("login %s failed: %d", name, resp.StatusCode)
		}
		s.tokens[name] = body.Token
	}
	p := models.Project{Name: "ml-baseline", OwnerID: s.users["sam"], Status: models.ProjectStatusReady}
	if err := database.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := rbac.NewDefaultProvider().GrantProjectAccess(s.users["sam"], p.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	s.project = p.ID
	return s
}

// clientAs builds the remote SDK on a local database connected to the
// server as the given user (empty: not connected).
func clientAs(t *testing.T, s *shareServer, user string) *remote.Service {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "client.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateServerDB(gdb); err != nil {
		t.Fatal(err)
	}
	if user != "" {
		if err := gdb.Save(&store.Config{ID: 1, ServerURL: s.url}).Error; err != nil {
			t.Fatal(err)
		}
		if err := gdb.Save(&store.Credentials{ID: 1, Token: s.tokens[user], Username: user, AuthMode: "token"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return remote.New(shared.Deps{DB: gdb}, nil)
}

func remoteStatus(err error) int {
	var rerr *shared.RemoteError
	if errors.As(err, &rerr) {
		return rerr.Status
	}
	return -1
}

func TestSharingThroughServer(t *testing.T) {
	s := startShareServer(t)
	ctx := context.Background()
	owner := clientAs(t, s, "sam")
	alex := clientAs(t, s, "alex")
	pid := s.project.String()
	alexRef := shared.PrincipalRef{Kind: "user", ID: s.users["alex"].String()}

	found, err := owner.SearchPrincipals(ctx, "ale")
	if err != nil || len(found) != 1 || found[0].Name != "alex" || found[0].Email != "alex@example.com" {
		t.Fatalf("search: %v %+v", err, found)
	}

	// Alex cannot see the project before it is shared.
	if _, err := alex.ListAccess(ctx, pid); remoteStatus(err) != http.StatusForbidden {
		t.Fatalf("alex list before share: want 403, got %v", err)
	}

	list, err := owner.GrantAccess(ctx, pid, shared.GrantAccessRequest{Principals: []shared.PrincipalRef{alexRef}, Role: "view"})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if len(list.Entries) != 2 || list.Entries[0].Role != "owner" || list.Entries[1].Role != "view" || !list.CanManage {
		t.Fatalf("after grant: %+v", list)
	}

	list, err = alex.ListAccess(ctx, pid)
	if err != nil || list.CanManage || len(list.Entries) != 2 {
		t.Fatalf("alex list: %v %+v", err, list)
	}
	// Alex is a viewer: the server refuses access changes.
	jordanRef := shared.PrincipalRef{Kind: "user", ID: s.users["jordan"].String()}
	if _, err := alex.GrantAccess(ctx, pid, shared.GrantAccessRequest{Principals: []shared.PrincipalRef{jordanRef}, Role: "edit"}); remoteStatus(err) != http.StatusForbidden {
		t.Fatalf("viewer grant: want 403, got %v", err)
	}

	list, err = owner.UpdateAccess(ctx, pid, alexRef, shared.UpdateAccessRequest{Role: "edit"})
	if err != nil || list.Entries[1].Role != "edit" {
		t.Fatalf("update: %v %+v", err, list)
	}
	list, err = owner.RevokeAccess(ctx, pid, alexRef)
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("revoke: %v %+v", err, list)
	}
	ownerRef := shared.PrincipalRef{Kind: "user", ID: s.users["sam"].String()}
	if _, err := owner.RevokeAccess(ctx, pid, ownerRef); remoteStatus(err) != http.StatusConflict {
		t.Fatalf("revoke owner: want 409, got %v", err)
	}
	if _, err := owner.ListAccess(ctx, uuid.New().String()); remoteStatus(err) != http.StatusNotFound {
		t.Fatalf("unknown project: want 404, got %v", err)
	}
}

func TestSharingClientValidation(t *testing.T) {
	ctx := context.Background()
	notConnected := remote.New(shared.Deps{DB: func() *gorm.DB {
		gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "c.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.MigrateServerDB(gdb); err != nil {
			t.Fatal(err)
		}
		return gdb
	}()}, nil)

	if _, err := notConnected.ListAccess(ctx, "abc"); !errors.Is(err, shared.ErrNotConnected) {
		t.Fatalf("not connected: %v", err)
	}
	if got, err := notConnected.SearchPrincipals(ctx, "  "); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty search: %v %+v", err, got)
	}
	var verr *shared.ValidationError
	ref := shared.PrincipalRef{Kind: "user", ID: "u1"}
	checks := []error{
		func() error { _, err := notConnected.ListAccess(ctx, " "); return err }(),
		func() error {
			_, err := notConnected.GrantAccess(ctx, "p", shared.GrantAccessRequest{Principals: []shared.PrincipalRef{ref}, Role: "owner"})
			return err
		}(),
		func() error {
			_, err := notConnected.GrantAccess(ctx, "p", shared.GrantAccessRequest{Role: "view"})
			return err
		}(),
		func() error {
			_, err := notConnected.UpdateAccess(ctx, "p", shared.PrincipalRef{Kind: "robot", ID: "x"}, shared.UpdateAccessRequest{Role: "view"})
			return err
		}(),
		func() error {
			_, err := notConnected.RevokeAccess(ctx, "p", shared.PrincipalRef{Kind: "group"})
			return err
		}(),
	}
	for i, err := range checks {
		if !errors.As(err, &verr) {
			t.Errorf("check %d: want validation error, got %v", i, err)
		}
	}
}
