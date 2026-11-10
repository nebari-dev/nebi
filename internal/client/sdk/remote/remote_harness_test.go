package remote_test

// Test harness: a team server made of the real internal/server/core and
// internal/server/api routes (with a token -> user auth stub), and client
// stacks ("machines") with their own local databases.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/server/api"
	"github.com/nebari-dev/nebi/internal/server/core"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type allowRBAC struct{}

func (allowRBAC) CanReadProject(uuid.UUID, uuid.UUID) (bool, error)           { return true, nil }
func (allowRBAC) CanWriteProject(uuid.UUID, uuid.UUID) (bool, error)          { return true, nil }
func (allowRBAC) CanReadRegistry(uuid.UUID, uuid.UUID) (bool, error)          { return true, nil }
func (allowRBAC) CanWriteRegistry(uuid.UUID, uuid.UUID) (bool, error)         { return true, nil }
func (allowRBAC) IsAdmin(uuid.UUID) (bool, error)                             { return true, nil }
func (allowRBAC) GrantProjectAccess(uuid.UUID, uuid.UUID, string) error       { return nil }
func (allowRBAC) RevokeProjectAccess(uuid.UUID, uuid.UUID) error              { return nil }
func (allowRBAC) MakeAdmin(uuid.UUID) error                                   { return nil }
func (allowRBAC) RevokeAdmin(uuid.UUID) error                                 { return nil }
func (allowRBAC) GetAllAdminUserIDs() (map[uuid.UUID]bool, error)             { return nil, nil }
func (allowRBAC) AddUserToGroup(uuid.UUID, uuid.UUID) error                   { return nil }
func (allowRBAC) RemoveUserFromGroup(uuid.UUID, uuid.UUID) error              { return nil }
func (allowRBAC) GetUserGroups(uuid.UUID) ([]uuid.UUID, error)                { return nil, nil }
func (allowRBAC) GrantGroupProjectAccess(uuid.UUID, uuid.UUID, string) error  { return nil }
func (allowRBAC) RevokeGroupProjectAccess(uuid.UUID, uuid.UUID) error         { return nil }
func (allowRBAC) GrantGroupRegistryAccess(uuid.UUID, uuid.UUID, string) error { return nil }
func (allowRBAC) RevokeGroupRegistryAccess(uuid.UUID, uuid.UUID) error        { return nil }
func (allowRBAC) MakeGroupAdmin(uuid.UUID) error                              { return nil }
func (allowRBAC) RevokeGroupAdmin(uuid.UUID) error                            { return nil }
func (allowRBAC) RemoveAllGroupPolicies(uuid.UUID) error                      { return nil }

type teamServer struct {
	URL    string
	DB     *gorm.DB
	tokens map[string]*models.User
	srv    *httptest.Server
}

func openSQLite(t *testing.T, name string) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	gdb, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := gdb.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

func newTeamServer(t *testing.T) *teamServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb := openSQLite(t, "server.db")
	if err := db.Migrate(gdb, false); err != nil {
		t.Fatal(err)
	}
	ts := &teamServer{DB: gdb, tokens: map[string]*models.User{}}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/version", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"version": "test", "mode": "team"}) })
	protected := v1.Group("")
	protected.Use(func(c *gin.Context) {
		tok := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		u, ok := ts.tokens[tok]
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(auth.UserContextKey, u)
		c.Next()
	})
	protected.GET("/auth/me", func(c *gin.Context) {
		u, _ := c.Get(auth.UserContextKey)
		c.JSON(http.StatusOK, u)
	})
	api.Register(protected, api.Deps{
		Core: core.New(core.Deps{DB: gdb, RBAC: allowRBAC{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}),
		RBAC: allowRBAC{},
	})
	ts.srv = httptest.NewServer(r)
	t.Cleanup(ts.srv.Close)
	ts.URL = ts.srv.URL
	return ts
}

// addUser creates a server user and returns its token.
func (ts *teamServer) addUser(t *testing.T, name string) string {
	t.Helper()
	u := &models.User{ID: uuid.New(), Username: name, Email: name + "@example.com"}
	if err := ts.DB.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	tok := "tok-" + name
	ts.tokens[tok] = u
	return tok
}

// machine is one client (its own local database and projects dir).
type machine struct {
	*client.Stack
	DB  *gorm.DB
	Dir string
}

func newMachine(t *testing.T, name string) *machine {
	t.Helper()
	gdb := openSQLite(t, name+".db")
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	stack, err := client.NewStack(client.StackOptions{
		DB:          gdb,
		DataDir:     dir,
		ProjectsDir: filepath.Join(dir, "projects"),
		InlineLog:   io.Discard,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &machine{Stack: stack, DB: gdb, Dir: dir}
}

func (m *machine) connect(t *testing.T, ts *teamServer, token string) *shared.ServerConnection {
	t.Helper()
	conn, err := m.SDK.Remote.Connect(context.Background(), shared.ConnectRequest{URL: ts.URL, Auth: "token", Token: token})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return conn
}

// addProject creates a ready local project with the given versions
// (pixi.toml contents, in order).
func (m *machine) addProject(t *testing.T, name string, tomls ...string) models.Project {
	t.Helper()
	p := models.Project{
		Name:    name,
		OwnerID: m.SDK.Deps().LocalUserID,
		Status:  models.ProjectStatusReady,
		Source:  "local",
		Path:    filepath.Join(m.Dir, "projects", name),
	}
	if err := m.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for _, toml := range tomls {
		m.addVersion(t, p.ID, toml)
	}
	return p
}

// addVersion stores the next local version (as create_version would).
func (m *machine) addVersion(t *testing.T, projectID uuid.UUID, toml string, tags ...string) int {
	t.Helper()
	v := models.ProjectVersion{
		ProjectID:       projectID,
		ManifestContent: toml,
		LockFileContent: "lock of " + toml,
		PackageMetadata: "[]",
		CreatedBy:       m.SDK.Deps().LocalUserID,
		Description:     "made " + toml,
		AuthorName:      "Sam Garcia",
		CreatedAt:       time.Now(),
	}
	err := m.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		return local.ApplyTags(tx, projectID, v.VersionNumber, tags, m.SDK.Deps().LocalUserID)
	})
	if err != nil {
		t.Fatal(err)
	}
	return v.VersionNumber
}

// baseItems is the undecorated version list (what sdk/local builds).
func (m *machine) baseItems(t *testing.T, projectID uuid.UUID) []shared.VersionItem {
	t.Helper()
	var rows []models.ProjectVersion
	if err := m.DB.Where("project_id = ?", projectID).Order("version_number DESC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	var tags []models.ProjectTag
	if err := m.DB.Where("project_id = ?", projectID).Order("tag ASC").Find(&tags).Error; err != nil {
		t.Fatal(err)
	}
	out := make([]shared.VersionItem, len(rows))
	for i, r := range rows {
		out[i] = shared.VersionItem{Number: r.VersionNumber, Description: r.Description, Tags: []string{}, Published: []shared.PublishedRef{}}
		for _, tag := range tags {
			if tag.VersionNumber == r.VersionNumber && !local.IsContentHashTag(tag.Tag) {
				out[i].Tags = append(out[i].Tags, tag.Tag)
			}
		}
	}
	return out
}

func numbers(items []shared.VersionItem) []int {
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = it.Number
	}
	return out
}

func marker(items []shared.VersionItem, n int) string {
	for _, it := range items {
		if it.Number == n {
			if it.Sync == nil {
				return ""
			}
			return string(*it.Sync)
		}
	}
	return "missing"
}
