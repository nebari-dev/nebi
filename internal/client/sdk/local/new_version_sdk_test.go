package local_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
)

// nvFakeExecutor solves environments by writing a fake pixi.lock that
// records the pixi.toml it was solved from. It remembers what it saw.
type nvFakeExecutor struct {
	staging string
	fail    error

	mu         sync.Mutex
	solvedDirs []string
	seedLocks  []string
}

func (e *nvFakeExecutor) CreateProject(context.Context, *models.Project, io.Writer, executor.CreateProjectOptions) error {
	return nil
}
func (e *nvFakeExecutor) InstallPackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (e *nvFakeExecutor) RemovePackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (e *nvFakeExecutor) DeleteProject(context.Context, *models.Project, io.Writer) error { return nil }
func (e *nvFakeExecutor) SolveEnvironment(_ context.Context, p *models.Project, log io.Writer) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.solvedDirs = append(e.solvedDirs, p.Path)
	seed, _ := os.ReadFile(filepath.Join(p.Path, "pixi.lock"))
	e.seedLocks = append(e.seedLocks, string(seed))
	if e.fail != nil {
		fmt.Fprintln(log, "solve failed")
		return e.fail
	}
	toml, err := os.ReadFile(filepath.Join(p.Path, "pixi.toml"))
	if err != nil {
		return err
	}
	lock := fmt.Sprintf("version: 6\nenvironments: {}\npackages: []\n# solved from %d bytes of pixi.toml\n", len(toml))
	return os.WriteFile(filepath.Join(p.Path, "pixi.lock"), []byte(lock), 0o644)
}
func (e *nvFakeExecutor) CleanupJobArtifacts(context.Context, *models.Project, models.JobType, io.Writer) error {
	return nil
}
func (e *nvFakeExecutor) InstallEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}
func (e *nvFakeExecutor) UninstallEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}
func (e *nvFakeExecutor) IsEnvInstalled(*models.Project) bool { return false }
func (e *nvFakeExecutor) GetProjectPath(p *models.Project) string {
	return p.Path
}
func (e *nvFakeExecutor) StagingRoot() string { return e.staging }

const nvBaseToml = `[workspace]
name = "ml-baseline"
channels = ["conda-forge"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.12.*"
numpy = ">=2.0"

[tasks]
train = "python train.py"
`

const nvBaseLock = "version: 6\nenvironments: {}\npackages: []\n# base lock\n"

type nvEnv struct {
	stack   *client.Stack
	exec    *nvFakeExecutor
	project models.Project
	log     *strings.Builder
}

// newNVEnv builds an inline (CLI-style) client stack with one project
// that has version 1 (nvBaseToml, tagged "stable") on disk at a path the
// tests check is never written by version creation.
func newNVEnv(t *testing.T) *nvEnv {
	t.Helper()
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	fx := &nvFakeExecutor{staging: t.TempDir()}
	logBuf := &strings.Builder{}
	stack, err := client.NewStack(client.StackOptions{
		DB:        gdb,
		Executor:  fx,
		DataDir:   dataDir,
		InlineLog: logBuf,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	user := stack.SDK.Deps().LocalUserID
	p := models.Project{Name: "ml-baseline", OwnerID: user, Status: models.ProjectStatusReady, Source: "local", Path: filepath.Join(t.TempDir(), "ml-baseline")}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	env := &nvEnv{stack: stack, exec: fx, project: p, log: logBuf}
	env.addVersion(t, nvBaseToml, nvBaseLock, "stable")
	return env
}

func (e *nvEnv) addVersion(t *testing.T, toml, lock string, tags ...string) int {
	t.Helper()
	gdb := e.stack.SDK.Deps().DB
	user := e.stack.SDK.Deps().LocalUserID
	v := models.ProjectVersion{ProjectID: e.project.ID, ManifestContent: toml, LockFileContent: lock, PackageMetadata: "[]", CreatedBy: user, Description: "v"}
	if err := gdb.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	if err := local.ApplyTags(gdb, e.project.ID, v.VersionNumber, tags, user); err != nil {
		t.Fatal(err)
	}
	return v.VersionNumber
}

func (e *nvEnv) tags(t *testing.T) map[string]int {
	t.Helper()
	var rows []models.ProjectTag
	if err := e.stack.SDK.Deps().DB.Where("project_id = ?", e.project.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.Tag] = r.VersionNumber
	}
	return out
}

func nvWantValidation(t *testing.T, err error, field, msgPart string) {
	t.Helper()
	var verr *shared.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError on %q, got %T %v", field, err, err)
	}
	if verr.Field != field || !strings.Contains(verr.Message, msgPart) {
		t.Fatalf("want field %q containing %q, got field %q: %q", field, msgPart, verr.Field, verr.Message)
	}
}

func TestCreateVersionFromFormKeepsUnrepresentableTablesAndMovesTags(t *testing.T) {
	env := newNVEnv(t)
	ctx := context.Background()
	sdk := env.stack.SDK

	draft, err := sdk.Local.GetVersionDraft(ctx, env.project.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	form := draft.Form
	form.Requested = append(form.Requested, artifact.RequestedPackage{Name: "pandas", Constraint: ">=2.2"})

	job, err := sdk.Local.CreateVersion(ctx, env.project.ID, shared.CreateVersionRequest{
		BaseVersion: 1,
		Form:        &form,
		Description: "Added pandas",
		Tags:        []string{"stable", "latest", " reporting "},
	})
	if err != nil {
		t.Fatalf("CreateVersion: %v\nlog:\n%s", err, env.log)
	}
	if job.Status != models.JobStatusCompleted || job.VersionNumber == nil || *job.VersionNumber != 2 {
		t.Fatalf("job = %+v", job)
	}
	if job.Type != models.JobTypeCreateVersion || job.TypeLabel != "Create version" {
		t.Fatalf("job type = %s / %s", job.Type, job.TypeLabel)
	}

	v, err := sdk.Local.LoadVersion(ctx, env.project.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if v.BaseVersion == nil || *v.BaseVersion != 1 {
		t.Fatalf("base version = %v", v.BaseVersion)
	}
	if v.Description != "Added pandas" || v.AuthorName == "" || v.ContentHash == "" {
		t.Fatalf("version = %+v", v)
	}
	if !strings.Contains(v.ManifestContent, "[tasks]") || !strings.Contains(v.ManifestContent, `pandas = ">=2.2"`) {
		t.Fatalf("manifest lost the [tasks] table or the new package:\n%s", v.ManifestContent)
	}
	if !strings.Contains(v.LockFileContent, "solved from") {
		t.Fatalf("lock = %q", v.LockFileContent)
	}
	if v.JobID == nil || *v.JobID != job.ID {
		t.Fatalf("version job = %v, want %s", v.JobID, job.ID)
	}

	// The lock was solved in staging, seeded with the base lock, never in
	// the project directory, and staging was cleaned up.
	if len(env.exec.solvedDirs) != 1 || env.exec.seedLocks[0] != nvBaseLock {
		t.Fatalf("solves = %v seeds = %q", env.exec.solvedDirs, env.exec.seedLocks)
	}
	if !strings.HasPrefix(env.exec.solvedDirs[0], env.exec.staging) {
		t.Fatalf("solved in %s, want under %s", env.exec.solvedDirs[0], env.exec.staging)
	}
	if _, err := os.Stat(env.exec.solvedDirs[0]); !os.IsNotExist(err) {
		t.Fatalf("staging dir not removed: %v", err)
	}
	if _, err := os.Stat(env.project.Path); !os.IsNotExist(err) {
		t.Fatalf("project directory was written: %v", err)
	}

	got := env.tags(t)
	if got["latest"] != 2 || got["stable"] != 2 || got["reporting"] != 2 || len(got) != 3 {
		t.Fatalf("tags = %v", got)
	}
	if !strings.Contains(env.log.String(), "Version 2 created (Started from version 1 · added pandas)") {
		t.Fatalf("log = %s", env.log)
	}
}

func TestCreateVersionFromOlderBaseInTomlMode(t *testing.T) {
	env := newNVEnv(t)
	ctx := context.Background()
	two := strings.Replace(nvBaseToml, `numpy = ">=2.0"`, `numpy = ">=2.1"`, 1)
	env.addVersion(t, two, nvBaseLock)

	edited := strings.Replace(nvBaseToml, `python = "3.12.*"`, `python = "3.13.*"`, 1)
	job, err := env.stack.SDK.Local.CreateVersion(ctx, env.project.ID, shared.CreateVersionRequest{
		BaseVersion: 1,
		PixiToml:    edited,
		Description: "Bumped Python",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.VersionNumber == nil || *job.VersionNumber != 3 {
		t.Fatalf("job = %+v", job)
	}
	v, _ := env.stack.SDK.Local.LoadVersion(ctx, env.project.ID, 3)
	if *v.BaseVersion != 1 || v.ManifestContent != edited {
		t.Fatalf("version 3 = base %v manifest %q", *v.BaseVersion, v.ManifestContent)
	}
	got := env.tags(t)
	if got["latest"] != 3 || got["stable"] != 1 {
		t.Fatalf("tags = %v (stable stays with version 1 until a newer version uses it)", got)
	}
}

func TestCreateVersionValidation(t *testing.T) {
	env := newNVEnv(t)
	ctx := context.Background()
	l := env.stack.SDK.Local
	id := env.project.ID
	edited := strings.Replace(nvBaseToml, `numpy = ">=2.0"`, `numpy = ">=2.1"`, 1)

	_, err := l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: nvBaseToml + "\n", Description: "x"})
	var cerr *shared.ConflictError
	if !errors.As(err, &cerr) || cerr.Code != local.CodeNoChanges || cerr.Message != "Nothing changed in ml-baseline." {
		t.Fatalf("no change: %v", err)
	}

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: edited, Description: "  "})
	nvWantValidation(t, err, "description", "Enter a description.")

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: edited, Description: "x", Tags: []string{"1x"}})
	nvWantValidation(t, err, "tags", "tags must start with a letter")

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: edited, Description: "x", Tags: []string{"sha-0123456789ab"}})
	nvWantValidation(t, err, "tags", "content hashes")

	noPlatforms := strings.Replace(edited, `platforms = ["linux-64", "osx-arm64"]`, `platforms = []`, 1)
	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: noPlatforms, Description: "x"})
	nvWantValidation(t, err, "manifest", "List at least one platform in platforms = [ ].")

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: "[dependencies]\nnumpy = \"*\"\n", Description: "x"})
	nvWantValidation(t, err, "manifest", "pixi.toml needs a [workspace] table.")

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, PixiToml: "[workspace\n", Description: "x"})
	nvWantValidation(t, err, "manifest", "pixi.toml can’t be read")

	draft, _ := l.GetVersionDraft(ctx, id, 1)
	form := draft.Form
	form.Platforms = nil
	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, Form: &form, Description: "x"})
	nvWantValidation(t, err, "platforms", "Add at least one platform.")

	form = draft.Form
	form.Platforms = []string{"linux-65"}
	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, Form: &form, Description: "x"})
	nvWantValidation(t, err, "platforms", "linux-65")

	form = draft.Form
	form.Requested = append(form.Requested, artifact.RequestedPackage{Name: "numpy", Constraint: "*"})
	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, Form: &form, Description: "x"})
	nvWantValidation(t, err, "packages", "numpy is listed twice.")

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 9, PixiToml: edited, Description: "x"})
	nvWantValidation(t, err, "base_version", "Pull version 9 first.")

	_, err = l.CreateVersion(ctx, id, shared.CreateVersionRequest{BaseVersion: 1, Description: "x"})
	nvWantValidation(t, err, "manifest", "[workspace]")

	_, err = l.CreateVersion(ctx, uuid.New(), shared.CreateVersionRequest{BaseVersion: 1, PixiToml: edited, Description: "x"})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("unknown project: %v", err)
	}

	if len(env.exec.solvedDirs) != 0 {
		t.Fatalf("invalid requests must not submit jobs; solves = %v", env.exec.solvedDirs)
	}
}

func TestCreateVersionSolveFailureStoresNothing(t *testing.T) {
	env := newNVEnv(t)
	env.exec.fail = errors.New("pixi lock exploded")
	edited := strings.Replace(nvBaseToml, `numpy = ">=2.0"`, `numpy = ">=2.1"`, 1)
	_, err := env.stack.SDK.Local.CreateVersion(context.Background(), env.project.ID, shared.CreateVersionRequest{
		BaseVersion: 1, PixiToml: edited, Description: "x", Tags: []string{"stable"},
	})
	if err == nil || !strings.Contains(err.Error(), "pixi lock exploded") {
		t.Fatalf("err = %v", err)
	}
	var count int64
	env.stack.SDK.Deps().DB.Model(&models.ProjectVersion{}).Where("project_id = ?", env.project.ID).Count(&count)
	if count != 1 {
		t.Fatalf("versions = %d, want 1", count)
	}
	var job models.Job
	env.stack.SDK.Deps().DB.Where("project_id = ? AND type = ?", env.project.ID, models.JobTypeCreateVersion).First(&job)
	if job.Status != models.JobStatusFailed || job.VersionNumber != nil {
		t.Fatalf("job = %+v", job)
	}
	if got := env.tags(t); got["stable"] != 1 || got["latest"] != 1 {
		t.Fatalf("tags moved on failure: %v", got)
	}
}

func TestRunCreateVersionReadsJSONMetadata(t *testing.T) {
	env := newNVEnv(t)
	edited := strings.Replace(nvBaseToml, `numpy = ">=2.0"`, `numpy = ">=2.1"`, 1)
	// Metadata as the worker sees it after the JSON round trip.
	job := &models.Job{
		UserID:    env.stack.SDK.Deps().LocalUserID,
		Status:    models.JobStatusRunning,
		ID:        uuid.New(),
		ProjectID: env.project.ID,
		Type:      models.JobTypeCreateVersion,
		Metadata: map[string]any{
			"base_version": float64(1),
			"pixi_toml":    edited,
			"description":  "From the queue",
			"tags":         []any{"stable"},
		},
	}
	if err := env.stack.SDK.Deps().DB.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	if err := env.stack.SDK.Local.RunCreateVersion(context.Background(), job, io.Discard); err != nil {
		t.Fatal(err)
	}
	if job.VersionNumber == nil || *job.VersionNumber != 2 {
		t.Fatalf("version number = %v", job.VersionNumber)
	}
	if got := env.tags(t); got["stable"] != 2 {
		t.Fatalf("tags = %v", got)
	}

	bad := &models.Job{ID: uuid.New(), ProjectID: env.project.ID, Metadata: map[string]any{"pixi_toml": edited}}
	if err := env.stack.SDK.Local.RunCreateVersion(context.Background(), bad, io.Discard); err == nil {
		t.Fatal("missing base_version must fail")
	}
}

func TestPreviewChangesAndDraft(t *testing.T) {
	env := newNVEnv(t)
	ctx := context.Background()
	l := env.stack.SDK.Local
	id := env.project.ID
	env.addVersion(t, strings.Replace(nvBaseToml, `numpy = ">=2.0"`, `numpy = ">=2.1"`, 1), nvBaseLock, "sha-0123456789ab")

	draft, err := l.GetVersionDraft(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if draft.BaseVersion != 1 || draft.LatestVersion != 2 || fmt.Sprint(draft.NewerVersions) != "[2]" {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.ProjectName != "ml-baseline" || draft.PixiToml != nvBaseToml || draft.Form.Name != "ml-baseline" {
		t.Fatalf("draft = %+v", draft)
	}
	if fmt.Sprint(draft.ExistingTags) != "[latest stable]" {
		t.Fatalf("existing tags = %v", draft.ExistingTags)
	}
	if fmt.Sprint(draft.Unrepresentable) != "[[tasks]]" {
		t.Fatalf("unrepresentable = %v", draft.Unrepresentable)
	}
	latest, err := l.GetVersionDraft(ctx, id, 0)
	if err != nil || latest.BaseVersion != 2 || len(latest.NewerVersions) != 0 {
		t.Fatalf("latest draft = %+v, %v", latest, err)
	}
	if _, err := l.GetVersionDraft(ctx, id, 7); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing base: %v", err)
	}

	edited := `[workspace]
name = "ml-baseline"
channels = ["conda-forge", "pytorch"]
platforms = ["linux-64"]

[dependencies]
python = "3.13.*"
pandas = ">=2.2"
rich = "*"
`
	d, err := l.PreviewChanges(ctx, id, 1, edited)
	if err != nil {
		t.Fatal(err)
	}
	if d.RequestedCounts.Added != 2 || d.RequestedCounts.Removed != 1 || d.RequestedCounts.Changed != 1 {
		t.Fatalf("counts = %+v rows = %+v", d.RequestedCounts, d.Requested)
	}
	if fmt.Sprint(d.Channels.Added) != "[pytorch]" || fmt.Sprint(d.Platforms.Removed) != "[osx-arm64]" {
		t.Fatalf("channels = %+v platforms = %+v", d.Channels, d.Platforms)
	}
	if d.Resolved == nil || len(d.Resolved) != 0 {
		t.Fatalf("resolved = %v", d.Resolved)
	}
	same, err := l.PreviewChanges(ctx, id, 1, nvBaseToml)
	if err != nil || same.RequestedCounts.HasChanges() || same.Channels.Added == nil {
		t.Fatalf("same = %+v, %v", same, err)
	}
	_, err = l.PreviewChanges(ctx, id, 1, "[workspace")
	nvWantValidation(t, err, "manifest", "pixi.toml can’t be read")

	if got := local.SuggestDescription(nvBaseToml, edited); got != "Added pandas and rich, removed numpy, changed python" {
		t.Fatalf("suggestion = %q", got)
	}
	if got := local.SuggestDescription(nvBaseToml, nvBaseToml); got != "" {
		t.Fatalf("suggestion for no change = %q", got)
	}

	form := draft.Form
	form.Requested = form.Requested[:1]
	toml, err := l.DraftManifest(ctx, id, 1, "", &form)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(toml, "numpy") || !strings.Contains(toml, "[tasks]") {
		t.Fatalf("form draft = %s", toml)
	}
	// A form with a pixi.toml applies onto that text (edits made in the
	// pixi.toml editor before switching to the form survive).
	edited2 := nvBaseToml + "\n[feature.dev.dependencies]\npytest = \"*\"\n"
	toml, err = l.DraftManifest(ctx, id, 1, edited2, &form)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(toml, "numpy") || !strings.Contains(toml, "[feature.dev.dependencies]") {
		t.Fatalf("form onto pixi.toml = %s", toml)
	}
}

func TestNormalizeVersionTags(t *testing.T) {
	got, err := local.NormalizeVersionTags([]string{" stable ", "latest", "", "stable", "v1.2"})
	if err != nil || fmt.Sprint(got) != "[stable v1.2]" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := local.NormalizeVersionTags([]string{"has space"}); err == nil {
		t.Fatal("want error")
	}
	if !local.SameManifest("a = 1  \r\nb = 2\n\n", "a = 1\nb = 2") {
		t.Fatal("SameManifest should ignore trailing whitespace and line endings")
	}
}
