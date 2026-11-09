package local_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

type versionsFixture struct {
	svc     *local.Service
	db      *gorm.DB
	runtime *shared.Runtime
	user    uuid.UUID
	project models.Project
	toml    string
	lock    string
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "artifact", "testdata", "v7", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// newVersionsFixture creates a project with versions 1..3 (v3 installed),
// tags stable->1, latest->3 and a content-hash tag on 2.
func newVersionsFixture(t *testing.T) *versionsFixture {
	t.Helper()
	gdb, err := local.OpenDB(t.TempDir())
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
	la, err := auth.NewLocalAuthenticator(gdb)
	if err != nil {
		t.Fatal(err)
	}
	f := &versionsFixture{
		db:      gdb,
		runtime: &shared.Runtime{},
		user:    la.User().ID,
		toml:    readTestdata(t, "pixi.toml"),
		lock:    readTestdata(t, "pixi.lock"),
	}
	f.svc = local.New(shared.Deps{DB: gdb, DataDir: t.TempDir(), LocalUserID: f.user, Runtime: f.runtime})

	three := 3
	f.project = models.Project{Name: "artlock", OwnerID: f.user, Status: models.ProjectStatusReady, InstalledVersion: &three, LastInstalledVersion: &three}
	if err := gdb.Create(&f.project).Error; err != nil {
		t.Fatal(err)
	}
	other := models.User{Username: "alex", Email: "alex@example.com", PasswordHash: "x"}
	if err := gdb.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	one := 1
	rows := []models.ProjectVersion{
		{VersionNumber: 1, Description: "First version", CreatedBy: f.user},
		{VersionNumber: 2, Description: "Bumped Python", CreatedBy: other.ID, BaseVersion: &one, ContentHash: "abc"},
		{VersionNumber: 3, Description: "Pulled from the server", CreatedBy: f.user, AuthorName: "Sam Garcia"},
	}
	for _, r := range rows {
		r.ProjectID = f.project.ID
		r.ManifestContent = f.toml
		r.LockFileContent = f.lock
		r.PackageMetadata = "[]"
		if err := gdb.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := local.ApplyTags(gdb, f.project.ID, 1, []string{"stable"}, f.user); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&models.ProjectTag{ProjectID: f.project.ID, Tag: "sha-0123456789ab", VersionNumber: 2, CreatedBy: f.user}).Error; err != nil {
		t.Fatal(err)
	}
	if err := local.ApplyTags(gdb, f.project.ID, 3, []string{"prod"}, f.user); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestListVersionsNewestFirstWithTagsAuthorsAndInstalled(t *testing.T) {
	f := newVersionsFixture(t)
	items, err := f.svc.ListVersions(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int{items[0].Number, items[1].Number, items[2].Number}; !slices.Equal(got, []int{3, 2, 1}) {
		t.Fatalf("order = %v", got)
	}
	if !slices.Equal(items[0].Tags, []string{"latest", "prod"}) {
		t.Fatalf("v3 tags = %v (latest first)", items[0].Tags)
	}
	if len(items[1].Tags) != 0 || items[1].Tags == nil {
		t.Fatalf("v2 tags = %#v: content-hash tags are hidden, never null", items[1].Tags)
	}
	if !slices.Equal(items[2].Tags, []string{"stable"}) {
		t.Fatalf("v1 tags = %v", items[2].Tags)
	}
	if !items[0].Installed || items[1].Installed || items[2].Installed {
		t.Fatal("only v3 is installed")
	}
	if items[0].Author.Name != "Sam Garcia" || items[0].Author.Username != auth.LocalUsername() {
		t.Fatalf("v3 author = %+v", items[0].Author)
	}
	if items[1].Author.Name != "alex" || items[1].Author.Username != "alex" {
		t.Fatalf("v2 author = %+v", items[1].Author)
	}
	if items[2].Author.Username != auth.LocalUsername() || items[2].Author.Name == "" {
		t.Fatalf("v1 author = %+v", items[2].Author)
	}
	if items[1].BaseVersion == nil || *items[1].BaseVersion != 1 {
		t.Fatalf("v2 base = %v", items[1].BaseVersion)
	}
	if items[0].Published == nil {
		t.Fatal("published must never be null")
	}
}

type fakeDecorator struct{}

func (fakeDecorator) DecorateVersions(_ context.Context, _ uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	here := shared.SyncNewestHere
	onServer := shared.SyncNewestOnServer
	for i := range items {
		if items[i].Number == 3 {
			items[i].Sync = &here
		}
	}
	return append(items, shared.VersionItem{Number: 4, ServerOnly: true, Sync: &onServer}), nil
}

type publishDecorator struct{}

func (publishDecorator) DecorateVersions(_ context.Context, _ uuid.UUID, items []shared.VersionItem) ([]shared.VersionItem, error) {
	for i := range items {
		if items[i].Number == 1 {
			items[i].Published = append(items[i].Published, shared.PublishedRef{Reference: "ghcr.io/org/artlock:1", Source: "local"})
		}
	}
	return items, nil
}

func TestListVersionsRunsDecoratorsInOrder(t *testing.T) {
	f := newVersionsFixture(t)
	f.runtime.AddVersionDecorator(publishDecorator{})
	f.runtime.AddVersionDecorator(fakeDecorator{})
	items, err := f.svc.ListVersions(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].Number != 4 || !items[0].ServerOnly {
		t.Fatalf("server-only v4 must be first: %+v", items)
	}
	if items[0].Tags == nil || items[0].Published == nil {
		t.Fatal("decorator items get non-null slices")
	}
	if items[1].Sync == nil || *items[1].Sync != shared.SyncNewestHere {
		t.Fatalf("v3 sync = %v", items[1].Sync)
	}
	if len(items[3].Published) != 1 {
		t.Fatalf("v1 published = %+v", items[3].Published)
	}

	d, err := f.svc.GetVersion(context.Background(), f.project.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if d.Sync == nil || *d.Sync != shared.SyncNewestHere {
		t.Fatal("GetVersion carries the decorator output")
	}
	if _, err := f.svc.GetVersion(context.Background(), f.project.ID, 4); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("server-only version is not local: %v", err)
	}
}

func TestGetVersionSpec(t *testing.T) {
	f := newVersionsFixture(t)
	d, err := f.svc.GetVersion(context.Background(), f.project.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Spec.Channels, []string{"conda-forge"}) || !slices.Equal(d.Spec.Platforms, []string{"linux-64", "osx-arm64"}) {
		t.Fatalf("spec = %+v", d.Spec)
	}
	if len(d.Spec.Requested) != 3 {
		t.Fatalf("requested = %+v", d.Spec.Requested)
	}
	if d.Number != 2 || d.Description != "Bumped Python" {
		t.Fatalf("item = %+v", d.VersionItem)
	}
	if _, err := f.svc.GetVersion(context.Background(), f.project.ID, 9); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}
	if _, err := f.svc.ListVersions(context.Background(), uuid.New()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

func TestGetVersionFile(t *testing.T) {
	f := newVersionsFixture(t)
	ctx := context.Background()
	toml, err := f.svc.GetVersionFile(ctx, f.project.ID, 1, "pixi.toml")
	if err != nil || toml.Content != f.toml || toml.Name != "pixi.toml" {
		t.Fatalf("pixi.toml = %+v, %v", toml, err)
	}
	lock, err := f.svc.GetVersionFile(ctx, f.project.ID, 1, "pixi.lock")
	if err != nil || lock.Content != f.lock {
		t.Fatalf("pixi.lock: %v", err)
	}
	var verr *shared.ValidationError
	if _, err := f.svc.GetVersionFile(ctx, f.project.ID, 1, "../etc/passwd"); !errors.As(err, &verr) {
		t.Fatalf("bad name: %v", err)
	}
	if _, err := f.svc.GetVersionFile(ctx, f.project.ID, 7, "pixi.toml"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}
}

func TestResolvedPackages(t *testing.T) {
	f := newVersionsFixture(t)
	p, err := f.svc.ResolvedPackages(context.Background(), f.project.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Platforms, []string{"linux-64", "osx-arm64"}) {
		t.Fatalf("platforms = %v", p.Platforms)
	}
	linux := p.Packages["linux-64"]
	var python *shared.ResolvedPackage
	for i := range linux {
		if linux[i].Name == "python" {
			python = &linux[i]
		}
	}
	if python == nil || python.Version != "3.12.16" || python.Channel != "conda-forge" {
		t.Fatalf("python = %+v", python)
	}
	if p.TotalSize["linux-64"] != artifact.TotalSize(linux) || p.TotalSize["linux-64"] <= 0 {
		t.Fatalf("total size = %d", p.TotalSize["linux-64"])
	}

	empty, err := local.ResolvedPackagesFromFiles([]byte(f.toml), nil)
	if err != nil || len(empty.Platforms) != 0 || empty.Packages == nil || empty.TotalSize == nil {
		t.Fatalf("no lock = %+v, %v", empty, err)
	}
	if _, err := local.ResolvedPackagesFromFiles([]byte(f.toml), []byte("version: 1\n")); err == nil {
		t.Fatal("unsupported lock must fail")
	}
}

func TestWriteVersionArchive(t *testing.T) {
	f := newVersionsFixture(t)
	var buf bytes.Buffer
	name, err := f.svc.WriteVersionArchive(context.Background(), f.project.ID, 3, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if name != "artlock-v3.tar.gz" {
		t.Fatalf("name = %q", name)
	}
	got, err := artifact.ReadArchive(&buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Toml) != f.toml || string(got.Lock) != f.lock {
		t.Fatal("archive content differs from the version")
	}
	if got.Metadata.Version != 3 || got.Metadata.Name != "artlock" || got.Metadata.Description != "Pulled from the server" {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	if !slices.Equal(got.Metadata.Tags, []string{"latest", "prod"}) {
		t.Fatalf("tags = %v", got.Metadata.Tags)
	}
	if _, err := f.svc.WriteVersionArchive(context.Background(), f.project.ID, 8, &buf); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}
}
