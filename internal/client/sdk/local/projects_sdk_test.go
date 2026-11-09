package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

func formRequest(name string) shared.CreateProjectRequest {
	return shared.CreateProjectRequest{
		Name: name,
		Form: &shared.ManifestForm{
			Platforms: []string{"linux-64"},
			Requested: []shared.RequestedPackage{{Name: "python", Constraint: ">=3.11"}},
		},
		Description: "Initial project creation",
		Tags:        []string{"latest", "baseline"},
	}
}

func wantValidation(t *testing.T, err error, field string) {
	t.Helper()
	var verr *shared.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError on %q, got %v", field, err)
	}
	if verr.Field != field {
		t.Fatalf("want field %q, got %q (%s)", field, verr.Field, verr.Message)
	}
}

func TestCheckName(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	l := stack.SDK.Local

	cases := map[string]string{
		"":          "Enter a project name.",
		"has space": "Use letters, numbers, hyphens, underscores or dots.",
		"..":        "",
	}
	for name, want := range cases {
		res, err := l.CheckName(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		if res.Available {
			t.Fatalf("%q: want unavailable", name)
		}
		if want != "" && res.Error != want {
			t.Fatalf("%q: want %q, got %q", name, want, res.Error)
		}
	}
	res, err := l.CheckName(ctx, "my-data.project_1")
	if err != nil || !res.Available || res.Error != "" {
		t.Fatalf("valid name: %+v %v", res, err)
	}

	if _, err := l.CreateProject(ctx, formRequest("Taken")); err != nil {
		t.Fatal(err)
	}
	res, _ = l.CheckName(ctx, "taken")
	if res.Available || res.Error != "A project with that name already exists." {
		t.Fatalf("duplicate (case-insensitive): %+v", res)
	}
}

func TestCreateProjectFromForm(t *testing.T) {
	stack, _, projectsDir := newTestStack(t)
	ctx := context.Background()
	l := stack.SDK.Local

	res, err := l.CreateProject(ctx, formRequest("my-data"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Job.Type != models.JobTypeCreate || res.Job.Status != models.JobStatusCompleted {
		t.Fatalf("job: %+v", res.Job)
	}
	if res.Job.VersionNumber == nil || *res.Job.VersionNumber != 1 {
		t.Fatalf("job version: %v", res.Job.VersionNumber)
	}
	wantPath := filepath.Join(projectsDir, "my-data")
	if res.Project.Path != wantPath || res.Project.Status != models.ProjectStatusReady {
		t.Fatalf("project: %+v", res.Project)
	}
	toml, err := os.ReadFile(filepath.Join(wantPath, "pixi.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name = "my-data"`, `"conda-forge"`, `"linux-64"`, `python = ">=3.11"`} {
		if !strings.Contains(string(toml), want) {
			t.Fatalf("pixi.toml missing %q:\n%s", want, toml)
		}
	}

	v, err := l.LoadVersion(ctx, res.Project.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Description != "Initial project creation" || v.ContentHash == "" || v.LockFileContent == "" || v.AuthorName == "" {
		t.Fatalf("version 1: %+v", v)
	}
	var tags []models.ProjectTag
	stack.SDK.Deps().DB.Where("project_id = ?", res.Project.ID).Order("tag").Find(&tags)
	if len(tags) != 2 || tags[0].Tag != "baseline" || tags[1].Tag != "latest" || tags[1].VersionNumber != 1 {
		t.Fatalf("tags: %+v", tags)
	}

	items, err := l.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("list: %+v", items)
	}
	it := items[0]
	if it.LatestVersion == nil || *it.LatestVersion != 1 || it.InstallStatus != models.InstallStatusNotInstalled ||
		it.InstalledVersion != nil || it.SizeBytes != nil || it.ActiveJobID != nil || it.Remotes == nil {
		t.Fatalf("list item: %+v", it)
	}
}

func TestCreateProjectFromToml(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	toml := "[workspace]\nname = \"tomlproj\"\nchannels = [\"conda-forge\"]\nplatforms = [\"osx-arm64\"]\n\n[tasks]\nhello = \"echo hi\"\n"
	res, err := stack.SDK.Local.CreateProject(ctx, shared.CreateProjectRequest{
		Name: "tomlproj", PixiToml: toml, Description: "From a file",
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := stack.SDK.Local.LoadVersion(ctx, res.Project.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.ManifestContent != toml {
		t.Fatalf("pixi.toml must be stored verbatim, got:\n%s", v.ManifestContent)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	l := stack.SDK.Local

	req := formRequest("bad name")
	_, err := l.CreateProject(ctx, req)
	wantValidation(t, err, "name")

	req = formRequest("ok")
	req.Form.Platforms = nil
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "platforms")

	req = formRequest("ok")
	req.Description = "  "
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "description")

	req = formRequest("ok")
	req.Tags = []string{"1bad"}
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "tags")

	req = formRequest("ok")
	req.Form.Requested = append(req.Form.Requested, shared.RequestedPackage{Name: "bad name!"})
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "packages")

	req = formRequest("ok")
	req.PixiToml = "[workspace]\n"
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "manifest")

	_, err = l.CreateProject(ctx, shared.CreateProjectRequest{Name: "ok", PixiToml: "[dependencies]\npython = \"*\"\n", Description: "x"})
	wantValidation(t, err, "manifest")

	_, err = l.CreateProject(ctx, shared.CreateProjectRequest{Name: "ok", PixiToml: "[workspace]\nname = \"ok\"\nplatforms = []\n", Description: "x"})
	wantValidation(t, err, "manifest")

	_, err = l.CreateProject(ctx, shared.CreateProjectRequest{Name: "ok", PixiToml: "not toml = = =", Description: "x"})
	wantValidation(t, err, "manifest")

	req = formRequest("ok")
	req.Path = "relative/dir"
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "path")

	if _, err := l.CreateProject(ctx, formRequest("dup")); err != nil {
		t.Fatal(err)
	}
	_, err = l.CreateProject(ctx, formRequest("DUP"))
	var cerr *shared.ConflictError
	if !errors.As(err, &cerr) || cerr.Code != "name_taken" {
		t.Fatalf("duplicate: want name_taken conflict, got %v", err)
	}

	var count int64
	stack.SDK.Deps().DB.Model(&models.Project{}).Count(&count)
	if count != 1 {
		t.Fatalf("rejected requests must not leave projects behind, have %d", count)
	}
}

func TestCreateProjectCustomPath(t *testing.T) {
	stack, fake, _ := newTestStack(t)
	ctx := context.Background()
	l := stack.SDK.Local

	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "pixi.toml"), []byte("[workspace]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := formRequest("occupied")
	req.Path = occupied
	_, err := l.CreateProject(ctx, req)
	wantValidation(t, err, "path")

	// An existing empty folder belongs to the user: used, never deleted.
	existing := t.TempDir()
	req = formRequest("existing")
	req.Path = existing
	res, err := l.CreateProject(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project.Path != existing {
		t.Fatalf("path: %s", res.Project.Path)
	}
	req = formRequest("same-folder")
	req.Path = existing
	_, err = l.CreateProject(ctx, req)
	wantValidation(t, err, "path")

	if _, err := l.DeleteProject(ctx, res.Project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(existing, "pixi.toml")); err != nil {
		t.Fatalf("user folder must be kept: %v", err)
	}
	if len(fake.deleted) != 0 {
		t.Fatalf("executor deleted %v", fake.deleted)
	}

	// A new folder under a custom parent is Nebi's and goes away on delete.
	fresh := filepath.Join(t.TempDir(), "nested", "fresh")
	req = formRequest("fresh")
	req.Path = fresh
	res, err = l.CreateProject(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.DeleteProject(ctx, res.Project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatalf("managed folder must be removed, stat err %v", err)
	}
}

func TestCreateProjectFailure(t *testing.T) {
	stack, fake, projectsDir := newTestStack(t)
	ctx := context.Background()
	fake.failSolve = true

	res, err := stack.SDK.Local.CreateProject(ctx, formRequest("broken"))
	if err == nil {
		t.Fatal("want the inline job error")
	}
	if res == nil || res.Job.Status != models.JobStatusFailed || res.Project.Status != models.ProjectStatusFailed {
		t.Fatalf("result: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(projectsDir, "broken")); !os.IsNotExist(err) {
		t.Fatalf("partial folder must be removed, stat err %v", err)
	}
	items, _ := stack.SDK.Local.ListProjects(ctx)
	if len(items) != 1 || items[0].Status != models.ProjectStatusFailed || items[0].LatestVersion != nil ||
		items[0].InstallStatus != models.InstallStatusNotInstalled {
		t.Fatalf("failed project in list: %+v", items)
	}
}

func TestInstallStatusAndSize(t *testing.T) {
	stack, fake, _ := newTestStack(t)
	ctx := context.Background()
	l := stack.SDK.Local
	db := stack.SDK.Deps().DB

	res, err := l.CreateProject(ctx, formRequest("envs"))
	if err != nil {
		t.Fatal(err)
	}
	id := res.Project.ID
	var p models.Project
	db.First(&p, "id = ?", id)
	if err := fake.InstallEnvironment(ctx, &p, nil); err != nil {
		t.Fatal(err)
	}
	one := 1
	db.Model(&models.Project{}).Where("id = ?", id).Updates(map[string]any{"installed_version": one, "last_installed_version": one, "size_bytes": 0})

	item, err := l.GetProject(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.InstallStatus != models.InstallStatusInstalled || item.InstalledVersion == nil || *item.InstalledVersion != 1 {
		t.Fatalf("installed: %+v", item)
	}
	if item.SizeBytes == nil || *item.SizeBytes < 4096 || item.Size == "" {
		t.Fatalf("size: %v %q", item.SizeBytes, item.Size)
	}
	size, err := l.ProjectSize(ctx, id)
	if err != nil || size != *item.SizeBytes {
		t.Fatalf("ProjectSize: %d %v (list %d)", size, err, *item.SizeBytes)
	}

	// Active install job: installing.
	job := models.Job{ProjectID: id, Type: models.JobTypeInstallVersion, Status: models.JobStatusRunning}
	db.Create(&job)
	item, _ = l.GetProject(ctx, id)
	if item.InstallStatus != models.InstallStatusInstalling || item.ActiveJobID == nil || *item.ActiveJobID != job.ID {
		t.Fatalf("installing: %+v", item)
	}

	// The install failed and the env is gone: install_failed.
	db.Model(&job).Update("status", models.JobStatusFailed)
	if err := fake.UninstallEnvironment(ctx, &p, nil); err != nil {
		t.Fatal(err)
	}
	item, _ = l.GetProject(ctx, id)
	if item.InstallStatus != models.InstallStatusFailed || item.InstalledVersion != nil || item.SizeBytes != nil {
		t.Fatalf("install failed: %+v", item)
	}
	if item.LastInstalledVersion == nil || *item.LastInstalledVersion != 1 {
		t.Fatalf("last installed must stay: %+v", item)
	}

	// A later uninstall supersedes the failure.
	db.Create(&models.Job{ProjectID: id, Type: models.JobTypeUninstall, Status: models.JobStatusCompleted})
	item, _ = l.GetProject(ctx, id)
	if item.InstallStatus != models.InstallStatusNotInstalled {
		t.Fatalf("after uninstall: %+v", item)
	}

	// Active uninstall: uninstalling.
	db.Create(&models.Job{ProjectID: id, Type: models.JobTypeUninstall, Status: models.JobStatusPending})
	item, _ = l.GetProject(ctx, id)
	if item.InstallStatus != models.InstallStatusUninstalling {
		t.Fatalf("uninstalling: %+v", item)
	}
}

func TestListRegistryRemotesAndDecorators(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	res, err := stack.SDK.Local.CreateProject(ctx, formRequest("remotes"))
	if err != nil {
		t.Fatal(err)
	}
	db := stack.SDK.Deps().DB
	db.Create(&models.ProjectRemote{ProjectID: res.Project.ID, Kind: models.RemoteKindRegistry, Name: "ghcr.io/example-org", RegistryHost: "ghcr.io", Repository: "example-org/remotes"})
	db.Create(&models.ProjectRemote{ProjectID: res.Project.ID, Kind: models.RemoteKindServer, Name: "team-nebi", ServerProjectID: "s-1"})

	called := false
	stack.SDK.Runtime().AddProjectDecorator(decoratorFunc(func(ctx context.Context, items []shared.ProjectListItem) error {
		called = true
		for i := range items {
			items[i].UpdateAvailable = true
		}
		return nil
	}))
	items, err := stack.SDK.Local.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !called || !items[0].UpdateAvailable {
		t.Fatal("decorators must run on the list")
	}
	// sdk/local emits the registry remotes; the server remote, when present,
	// is added by the remote decorator registered in the stack.
	var registryRefs []shared.RemoteRef
	for _, ref := range items[0].Remotes {
		if ref.Kind == models.RemoteKindRegistry {
			registryRefs = append(registryRefs, ref)
		}
	}
	if len(registryRefs) != 1 || registryRefs[0].Name != "ghcr.io/example-org" ||
		registryRefs[0].URL != "ghcr.io/example-org/remotes" {
		t.Fatalf("registry remotes: %+v", items[0].Remotes)
	}
	got, err := stack.SDK.Local.GetProject(ctx, res.Project.ID)
	if err != nil || !got.UpdateAvailable {
		t.Fatalf("GetProject decorated: %+v %v", got, err)
	}
	if _, err := stack.SDK.Local.GetProject(ctx, uuid.New()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

type decoratorFunc func(context.Context, []shared.ProjectListItem) error

func (f decoratorFunc) DecorateProjects(ctx context.Context, items []shared.ProjectListItem) error {
	return f(ctx, items)
}

func TestDeleteProject(t *testing.T) {
	stack, _, projectsDir := newTestStack(t)
	ctx := context.Background()
	l := stack.SDK.Local
	db := stack.SDK.Deps().DB

	res, err := l.CreateProject(ctx, formRequest("doomed"))
	if err != nil {
		t.Fatal(err)
	}
	id := res.Project.ID

	active := models.Job{ProjectID: id, Type: models.JobTypeInstallVersion, Status: models.JobStatusRunning}
	db.Create(&active)
	_, err = l.DeleteProject(ctx, id)
	var cerr *shared.ConflictError
	if !errors.As(err, &cerr) || cerr.Code != "job_active" {
		t.Fatalf("delete during a job: %v", err)
	}
	db.Model(&active).Update("status", models.JobStatusCompleted)

	job, err := l.DeleteProject(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Type != models.JobTypeDelete || job.Status != models.JobStatusCompleted || job.ProjectName != "doomed" {
		t.Fatalf("delete job: %+v", job)
	}
	if _, err := os.Stat(filepath.Join(projectsDir, "doomed")); !os.IsNotExist(err) {
		t.Fatalf("folder must be gone: %v", err)
	}
	items, _ := l.ListProjects(ctx)
	if len(items) != 0 {
		t.Fatalf("list after delete: %+v", items)
	}
	var versions, tags int64
	db.Model(&models.ProjectVersion{}).Where("project_id = ?", id).Count(&versions)
	db.Model(&models.ProjectTag{}).Where("project_id = ?", id).Count(&tags)
	if versions != 0 || tags != 0 {
		t.Fatalf("versions %d tags %d left", versions, tags)
	}
	// The name is free again.
	if _, err := l.CreateProject(ctx, formRequest("doomed")); err != nil {
		t.Fatalf("re-create after delete: %v", err)
	}
	if _, err := l.DeleteProject(ctx, uuid.New()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestCreateAndInstallChainsInstall(t *testing.T) {
	stack, _, _ := newTestStack(t)
	ctx := context.Background()
	req := formRequest("withenv")
	req.Install = true
	res, err := stack.SDK.Local.CreateProject(ctx, req)
	if res == nil {
		t.Fatalf("no result: %v", err)
	}
	// Version 1 is stored whatever the install step does.
	if _, verr := stack.SDK.Local.LoadVersion(ctx, res.Project.ID, 1); verr != nil {
		t.Fatalf("version 1: %v", verr)
	}
	if err != nil {
		t.Fatalf("create and install: %v (job error %q)", err, res.Job.Error)
	}
	if res.Job.VersionNumber == nil || *res.Job.VersionNumber != 1 {
		t.Fatalf("job version = %v, want 1", res.Job.VersionNumber)
	}
	item, _ := stack.SDK.Local.GetProject(ctx, res.Project.ID)
	if item.InstallStatus != models.InstallStatusInstalled {
		t.Fatalf("installed after create and install: %+v", item)
	}
}
