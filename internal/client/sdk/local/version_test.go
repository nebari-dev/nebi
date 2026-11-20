package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"gorm.io/gorm"
)

// newProject creates a project, waits for its create job and returns it
// with version 1.
func (h *harness) newProject(name string, install bool) (*Project, ProjectVersion) {
	h.t.Helper()
	req := specRequest(name)
	req.Install = install
	p, job := h.mustCreate(req)
	if job.Status != jobs.StatusCompleted {
		h.t.Fatalf("create job: %+v", job)
	}
	list := h.versions.versionsOf(h.t, p.ID)
	if len(list) != 1 {
		h.t.Fatalf("want version 1, got %d versions", len(list))
	}
	return p, list[0]
}

// createVersion calls CreateVersion and, when it started a job, waits for
// it and requires it to complete.
func (h *harness) createVersion(req CreateVersionRequest) (*CreateVersionResult, jobs.Job) {
	h.t.Helper()
	res, err := h.svc.CreateVersion(context.Background(), req)
	if err != nil {
		h.t.Fatalf("CreateVersion: %v", err)
	}
	if res.Job == nil {
		return res, jobs.Job{}
	}
	done := h.wait(res.Job)
	if done.Status != jobs.StatusCompleted {
		h.t.Fatalf("job %s: %+v", done.Type, done)
	}
	return res, done
}

func newerSpec() *artifact.Spec {
	return &artifact.Spec{
		Platforms: []string{"linux-64", "osx-arm64"},
		Channels:  []string{"conda-forge"},
		Packages: []artifact.Package{
			{Name: "numpy", Operator: ">=", Constraint: "2"},
			{Name: "pandas"},
		},
	}
}

func TestCreateVersionFromSpec(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("spec-edit", false)
	before := readFile(t, filepath.Join(p.Path, artifact.ManifestFile))

	res, done := h.createVersion(CreateVersionRequest{
		ProjectID:     p.ID,
		BaseVersionID: v1.ID,
		Spec:          newerSpec(),
		Description:   "numpy 2",
		Tags:          []string{"stable"},
	})
	if res.Version != nil || res.Job == nil || res.Job.Type != jobs.TypeCreateVersion {
		t.Fatalf("a changed manifest must be solved in a create_version job, got %+v", res)
	}
	if done.Deduplicated || done.VersionID == nil || done.Stage != StageLock {
		t.Fatalf("job = %+v; want a recorded version, stage lock", done)
	}

	list := h.versions.versionsOf(t, p.ID)
	if len(list) != 2 {
		t.Fatalf("want 2 versions, got %d", len(list))
	}
	v2 := list[1]
	if v2.ID != *done.VersionID || v2.Number != 2 || v2.Origin != OriginCreated || v2.BaseVersionID == nil || *v2.BaseVersionID != v1.ID {
		t.Fatalf("version 2 = %+v", v2)
	}
	if v2.Author != "tester" || v2.Description != "numpy 2" {
		t.Fatalf("author/description = %q/%q", v2.Author, v2.Description)
	}
	if !strings.Contains(v2.Manifest, `numpy = ">=2"`) || !strings.Contains(v2.Manifest, `pandas = "*"`) ||
		!strings.Contains(v2.Manifest, `name = "spec-edit"`) {
		t.Fatalf("the spec must be applied to the base's pixi.toml, keeping its name:\n%s", v2.Manifest)
	}
	if v2.Lock != fakeLockFor(v2.Manifest) || v2.ContentHash != contenthash.Hash(v2.Manifest, v2.Lock) {
		t.Fatal("version 2 must hold the solved lock and its content hash")
	}
	if tags := h.versions.tagsOf(t, p.ID); tags["stable"] != 2 {
		t.Fatalf("tags = %v; want stable on version 2", tags)
	}

	// The solve ran in a temporary directory, starting from the base's
	// lock, and the project directory was not touched.
	h.pixi.mu.Lock()
	lockDir, lockInput := h.pixi.lockCalls[1], h.pixi.lockInputs[1]
	h.pixi.mu.Unlock()
	if lockDir == p.Path || strings.HasPrefix(lockDir, p.Path+string(filepath.Separator)) {
		t.Fatalf("the solve must not run in the project directory: %s", lockDir)
	}
	if exists(lockDir) {
		t.Fatalf("the temporary directory must be removed: %s", lockDir)
	}
	if lockInput != v1.Lock {
		t.Fatalf("the solve must start from the base's pixi.lock, got %q", lockInput)
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)); got != before {
		t.Fatal("the project's pixi.toml must not change until install")
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.LockFile)); got != v1.Lock {
		t.Fatal("the project's pixi.lock must not change until install")
	}
	if d := h.detail(p.ID); d.LatestVersion == nil || *d.LatestVersion != 2 || d.InstalledVersionID != nil {
		t.Fatalf("project after create version: %+v", d.ProjectSummary)
	}
}

func TestCreateVersionSpecKeepsOtherTables(t *testing.T) {
	h := newHarness(t)
	raw := "[workspace]\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\npython = \"3.12.*\"\n\n" +
		"[pypi-dependencies]\nrequests = \">=2\"\n\n[tasks]\nstart = \"python app.py\"\n"
	p, job := h.mustCreate(CreateProjectRequest{Name: "kept-tables", Manifest: raw})
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("create: %+v", job)
	}
	v1 := h.versions.versionsOf(t, p.ID)[0]
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]
	for _, keep := range []string{"[pypi-dependencies]\nrequests = \">=2\"", "[tasks]\nstart = \"python app.py\""} {
		if !strings.Contains(v2.Manifest, keep) {
			t.Fatalf("tables the spec does not cover must be kept (%q):\n%s", keep, v2.Manifest)
		}
	}
	if strings.Contains(v2.Manifest, "python = ") {
		t.Fatalf("[dependencies] must be the spec's:\n%s", v2.Manifest)
	}
}

func TestCreateVersionFromManifest(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("raw-edit", false)
	raw := "# by hand\n[workspace]\nname = \"raw-edit\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nscipy = \"*\"\n"
	_, done := h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: raw})
	v2 := h.versions.versionsOf(t, p.ID)[1]
	if v2.ID != *done.VersionID || v2.Manifest != raw {
		t.Fatalf("the manifest must be recorded as given:\n%s", v2.Manifest)
	}
}

func TestCreateVersionUnchangedCopy(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("copy", false)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	lockCalls, _ := h.pixi.calls()
	locks := len(lockCalls)

	// A copy of the latest version is deduplicated: nothing is written.
	v2 := h.versions.versionsOf(t, p.ID)[1]
	res, _ := h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v2.ID, Tags: []string{"ignored"}})
	if !res.Deduplicated || res.Version == nil || res.Version.ID != v2.ID || res.Job != nil {
		t.Fatalf("copy of the latest: want deduplicated version 2, got %+v", res)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 2 {
		t.Fatalf("a deduplicated copy must write nothing, %d versions", n)
	}
	if _, ok := h.versions.tagsOf(t, p.ID)["ignored"]; ok {
		t.Fatal("a deduplicated copy must not write its tags")
	}

	// A copy of an older version needs the confirmation.
	_, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID})
	if !errors.Is(err, ErrOlderBase) {
		t.Fatalf("older base without confirmation: want ErrOlderBase, got %v", err)
	}
	res, _ = h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, AllowOlderBase: true, Description: "back to 1"})
	if res.Deduplicated || res.Version == nil || res.Job != nil {
		t.Fatalf("copy of an older version: want a new version and no job, got %+v", res)
	}
	v3 := res.Version
	if v3.Number != 3 || v3.Manifest != v1.Manifest || v3.Lock != v1.Lock || *v3.BaseVersionID != v1.ID {
		t.Fatalf("version 3 must be a copy of version 1 with its lock: %+v", v3)
	}
	if v3.ContentHash != v1.ContentHash {
		t.Fatal("a copy has the same content hash as its base")
	}
	if lockCalls, _ = h.pixi.calls(); len(lockCalls) != locks {
		t.Fatal("an unchanged copy must not solve")
	}
	latest, err := h.svc.ResolveTag(context.Background(), p.ID, LatestTag)
	if err != nil || latest.ID != v3.ID {
		t.Fatalf("latest must move to the copy: %v, %v", latest, err)
	}
}

func TestCreateVersionDeduplicatesSolvedResult(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("same-result", false)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	// The same spec from version 1 solves to version 2's content.
	_, done := h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, AllowOlderBase: true, Spec: newerSpec()})
	if !done.Deduplicated || done.VersionID == nil || *done.VersionID != v2.ID {
		t.Fatalf("want the job deduplicated against version 2, got %+v", done)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 2 {
		t.Fatalf("a deduplicated result must write nothing, %d versions", n)
	}
}

func TestCreateVersionValidation(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("checked", false)
	_, otherV1 := h.newProject("other", false)
	if _, err := h.svc.SetTag(context.Background(), SetTagRequest{ProjectID: p.ID, Name: "taken", VersionID: v1.ID}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		req  CreateVersionRequest
		want error
	}{
		{"unknown project", CreateVersionRequest{ProjectID: uuid.New(), BaseVersionID: v1.ID}, ErrNotFound},
		{"no base", CreateVersionRequest{ProjectID: p.ID}, ErrInvalidRequest},
		{"base of another project", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: otherV1.ID}, ErrNotFound},
		{"spec and manifest", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Manifest: "[workspace]\n"}, ErrInvalidManifest},
		{"spec without platform", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: &artifact.Spec{Channels: []string{"conda-forge"}}}, ErrNoPlatform},
		{"manifest without platform", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: "[workspace]\nchannels = []\n"}, ErrNoPlatform},
		{"unparsable manifest", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: "[workspace\n"}, ErrInvalidManifest},
		{"reserved tag", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Tags: []string{"latest"}}, ErrInvalidTag},
		{"sha tag", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Tags: []string{"sha-abc"}}, ErrInvalidTag},
		{"bad tag", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Tags: []string{"a b"}}, ErrInvalidTag},
		{"tag on another version", CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Tags: []string{"taken"}}, ErrTagExists},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := h.svc.CreateVersion(context.Background(), c.req); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
		t.Fatalf("refused creates must record nothing, %d versions", n)
	}
	if list, _ := h.runner.ProjectJobs(context.Background(), p.ID); len(list) != 1 {
		t.Fatalf("refused creates must start no job: %+v", list)
	}
}

func TestCreateVersionLockFailure(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("unsolvable", true)
	h.pixi.mu.Lock()
	h.pixi.lockErr = errors.New("cannot solve numpy ==99")
	h.pixi.mu.Unlock()
	res, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	if err != nil {
		t.Fatal(err)
	}
	done := h.wait(res.Job)
	if done.Status != jobs.StatusFailed || done.VersionID != nil || !strings.Contains(done.Error, "cannot solve") {
		t.Fatalf("want a failed job with no version, got %+v", done)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
		t.Fatalf("a failed solve must record nothing, %d versions", n)
	}
	if d := h.detail(p.ID); d.State != StateInstalled {
		t.Fatalf("a failed solve must leave the installed version alone, state %s", d.State)
	}
}

func TestCreateVersionAndInstall(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("upgrade", true)

	res, done := h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Install: true})
	if done.Stage != StageInstall || done.VersionID == nil {
		t.Fatalf("job = %+v; want it to record and then install", done)
	}
	v2 := h.versions.versionsOf(t, p.ID)[1]
	if got := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)); got != v2.Manifest {
		t.Fatal("install must write the new version's pixi.toml")
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.LockFile)); got != v2.Lock {
		t.Fatal("install must write the new version's pixi.lock")
	}
	d := h.detail(p.ID)
	if d.State != StateInstalled || d.InstalledVersionID == nil || *d.InstalledVersionID != v2.ID || *d.InstalledVersion != 2 {
		t.Fatalf("want version 2 installed, got %+v", d.ProjectSummary)
	}
	if st, _ := h.svc.WorkingCopyStatus(context.Background(), p.ID); st != WorkingCopyClean {
		t.Fatalf("working copy = %s, want clean", st)
	}
	if res.Version != nil {
		t.Fatal("a solved version is reported through the job")
	}

	// An unchanged copy with Install records at once and installs in a job.
	res, done = h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, AllowOlderBase: true, Install: true})
	if res.Version == nil || res.Version.Number != 3 || res.Job == nil || res.Job.Type != jobs.TypeInstall {
		t.Fatalf("want version 3 and an install job, got %+v", res)
	}
	if d := h.detail(p.ID); d.InstalledVersion == nil || *d.InstalledVersion != 3 {
		t.Fatalf("want version 3 installed, got %+v", d.ProjectSummary)
	}
}

func TestCreateVersionInstallFailure(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("broken-install", true)
	h.pixi.mu.Lock()
	h.pixi.installErr = errors.New("download failed")
	h.pixi.mu.Unlock()
	res, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Install: true})
	if err != nil {
		t.Fatal(err)
	}
	done := h.wait(res.Job)
	if done.Status != jobs.StatusFailed || done.Stage != StageInstall || done.VersionID == nil {
		t.Fatalf("want a job that recorded version 2 and failed installing it, got %+v", done)
	}
	d := h.detail(p.ID)
	if d.VersionCount != 2 || d.State != StateInstallFailed || d.JobID == nil || *d.JobID != done.ID || d.InstalledVersionID != nil {
		t.Fatalf("want install_failed linked to the job, got %+v", d)
	}
}

func TestCreateVersionInstallProtectsUnsavedEdits(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("edited", true)
	edited := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) + "\n# my edit\n"
	if err := os.WriteFile(filepath.Join(p.Path, artifact.ManifestFile), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Install: true})
	if !errors.Is(err, ErrWorkingCopyModified) {
		t.Fatalf("want ErrWorkingCopyModified, got %v", err)
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)); got != edited {
		t.Fatal("the edit must be kept")
	}
	// Without Install the working copy is not touched, so it is allowed.
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	if got := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)); got != edited {
		t.Fatal("the edit must be kept")
	}
}

func TestInstallReplacesOnlyRecordedOrMissingFiles(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		manifest func(v1 ProjectVersion) *string // nil: remove pixi.toml
		lock     func(v1 ProjectVersion) *string // nil: remove pixi.lock
		want     error
	}{
		{"both missing", nil, nil, nil},
		{"lock missing, pixi.toml recorded", func(v1 ProjectVersion) *string { return &v1.Manifest }, nil, nil},
		{"lock missing, pixi.toml edited", func(v1 ProjectVersion) *string { s := v1.Manifest + "# edit\n"; return &s }, nil, ErrWorkingCopyModified},
		{"pixi.toml missing, lock in no version", nil, func(ProjectVersion) *string { s := "version: 6\n# by hand\n"; return &s }, ErrWorkingCopyModified},
		{"pixi.toml missing, recorded lock", nil, func(v1 ProjectVersion) *string { return &v1.Lock }, ErrWorkingCopyModified},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			p, v1 := h.newProject("replace", true)
			set := func(name string, content *string) {
				path := filepath.Join(p.Path, name)
				if content == nil {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					return
				}
				if err := os.WriteFile(path, []byte(*content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var manifest, lock *string
			if c.manifest != nil {
				manifest = c.manifest(v1)
			}
			if c.lock != nil {
				lock = c.lock(v1)
			}
			set(artifact.ManifestFile, manifest)
			set(artifact.LockFile, lock)

			res, err := h.svc.CreateVersion(ctx, CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Install: true})
			if c.want != nil {
				if !errors.Is(err, c.want) {
					t.Fatalf("want %v, got %v", c.want, err)
				}
				if lock != nil && readFile(t, filepath.Join(p.Path, artifact.LockFile)) != *lock {
					t.Fatal("a refused install must keep pixi.lock")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateVersion: %v", err)
			}
			if done := h.wait(res.Job); done.Status != jobs.StatusCompleted {
				t.Fatalf("job: %+v", done)
			}
			v2 := h.versions.versionsOf(t, p.ID)[1]
			if readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) != v2.Manifest {
				t.Fatal("install must write the new version's pixi.toml")
			}
		})
	}
}

const pathDepManifest = "[workspace]\nname = \"editable\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n" +
	"[dependencies]\npython = \"3.12.*\"\n\n[pypi-dependencies]\neditable = { path = \".\", editable = true }\n"

func TestCreateVersionWithPathDependenciesLocksInPlace(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("editable", true)
	origManifest := readFile(t, filepath.Join(p.Path, artifact.ManifestFile))

	_, done := h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: pathDepManifest})
	h.pixi.mu.Lock()
	lockDir, lockInput := h.pixi.lockCalls[1], h.pixi.lockInputs[1]
	h.pixi.mu.Unlock()
	if lockDir != p.Path {
		t.Fatalf("a manifest with path dependencies must be locked in the project directory, got %s", lockDir)
	}
	if lockInput != v1.Lock {
		t.Fatal("the in-place solve must start from the base's lock")
	}
	v2 := h.versions.versionsOf(t, p.ID)[1]
	if v2.ID != *done.VersionID || v2.Manifest != pathDepManifest || v2.Lock != fakeLockFor(pathDepManifest) {
		t.Fatalf("version 2 = %+v", v2)
	}
	if readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) != origManifest ||
		readFile(t, filepath.Join(p.Path, artifact.LockFile)) != v1.Lock {
		t.Fatal("the project directory must be put back on the installed version after the solve")
	}
	if st, _ := h.svc.WorkingCopyStatus(context.Background(), p.ID); st != WorkingCopyClean {
		t.Fatalf("working copy = %s, want clean", st)
	}
}

func TestCreateVersionWithRelativeChannelLocksInPlace(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("local-channel", true)
	manifest := "[workspace]\nname = \"local-channel\"\nchannels = [\"./channel\", \"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\nmytool = \"*\"\n"
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: manifest})
	lockCalls, _ := h.pixi.calls()
	if lockCalls[1] != p.Path {
		t.Fatalf("a relative channel must be locked in the project directory, got %s", lockCalls[1])
	}
	if readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) != v1.Manifest {
		t.Fatal("the project directory must be put back after the solve")
	}
}

func TestCreateVersionSolveUsesProjectConfig(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("configured", false)
	config := "[mirrors]\n\"https://conda.anaconda.org/conda-forge\" = [\"https://mirror.example/conda-forge\"]\n"
	if err := os.MkdirAll(filepath.Join(p.Path, ".pixi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, ".pixi", "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	h.pixi.mu.Lock()
	dir, got := h.pixi.lockCalls[1], h.pixi.lockConfigs[1]
	h.pixi.mu.Unlock()
	if dir == p.Path {
		t.Fatal("precondition: the solve runs in a temporary directory")
	}
	if got != config {
		t.Fatalf("the temporary solve must see the project's .pixi/config.toml, got %q", got)
	}
}

func TestCreateVersionInPlaceRestoresFilesOnFailure(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("editable-fail", true)
	h.pixi.mu.Lock()
	h.pixi.lockErr = errors.New("cannot solve")
	h.pixi.mu.Unlock()
	res, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: pathDepManifest})
	if err != nil {
		t.Fatal(err)
	}
	if done := h.wait(res.Job); done.Status != jobs.StatusFailed {
		t.Fatalf("job: %+v", done)
	}
	if readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) != v1.Manifest ||
		readFile(t, filepath.Join(p.Path, artifact.LockFile)) != v1.Lock {
		t.Fatal("a failed in-place solve must put the files back")
	}
}

func TestCreateVersionInPlaceRefusesUnsavedEdits(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("editable-dirty", true)
	if err := os.WriteFile(filepath.Join(p.Path, artifact.ManifestFile), []byte(pathDepManifest+"# wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: pathDepManifest})
	if !errors.Is(err, ErrWorkingCopyModified) {
		t.Fatalf("want ErrWorkingCopyModified, got %v", err)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: pathDepManifest})
	if !errors.Is(err, ErrWorkingCopyMissing) {
		t.Fatalf("want ErrWorkingCopyMissing, got %v", err)
	}
}

func TestConcurrentCreatesFromOneBaseAreBothRecorded(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("parallel", false)
	specs := []*artifact.Spec{newerSpec(), {
		Platforms: []string{"linux-64"},
		Channels:  []string{"conda-forge"},
		Packages:  []artifact.Package{{Name: "polars"}},
	}}
	// Hold every solve until both creates are accepted, so the first
	// cannot record version 2 before the second is checked: both are
	// created from version 1 while it is the latest.
	gate := make(chan struct{})
	h.pixi.mu.Lock()
	h.pixi.lockGate = gate
	h.pixi.mu.Unlock()
	var wg sync.WaitGroup
	results := make([]*CreateVersionResult, len(specs))
	errs := make([]error, len(specs))
	for i, spec := range specs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: spec})
		}()
	}
	wg.Wait()
	close(gate)
	for i := range specs {
		if errs[i] != nil {
			t.Fatalf("create %d: %v", i, errs[i])
		}
		if done := h.wait(results[i].Job); done.Status != jobs.StatusCompleted || done.Deduplicated {
			t.Fatalf("create %d: %+v", i, done)
		}
	}
	list := h.versions.versionsOf(t, p.ID)
	if len(list) != 3 || list[1].Number != 2 || list[2].Number != 3 {
		t.Fatalf("both creates must be recorded as consecutive numbers: %d versions", len(list))
	}
	for _, v := range list[1:] {
		if *v.BaseVersionID != v1.ID {
			t.Fatalf("version %d must keep base 1", v.Number)
		}
	}
}

func TestNumbersAreAssignedInTheInsertAndNeverReused(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("numbers", false)

	const writers = 12
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manifest := fmt.Sprintf("[workspace]\nname = \"numbers\"\nplatforms = [\"linux-64\"]\n# %d\n", i)
			errs <- h.svc.versionTx(context.Background(), func(tx *gorm.DB) error {
				_, _, err := h.svc.recordVersion(tx, recordVersionRequest{
					ProjectID: p.ID, Manifest: manifest, Lock: fakeLockFor(manifest), Origin: OriginPulled, Author: "remote",
				})
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	list := h.versions.versionsOf(t, p.ID)
	if len(list) != writers+1 {
		t.Fatalf("want %d versions, got %d", writers+1, len(list))
	}
	for i, v := range list {
		if v.Number != i+1 {
			t.Fatalf("numbers must be 1..%d without gaps or repeats, got %d at %d", writers+1, v.Number, i)
		}
	}

	// A failed insert does not consume a number.
	h.versions.setRecordErr(errors.New("disk full"))
	err := h.svc.versionTx(context.Background(), func(tx *gorm.DB) error {
		_, _, err := h.svc.recordVersion(tx, recordVersionRequest{ProjectID: p.ID, Manifest: "[workspace]\n# x\n", Lock: "version: 6\n", Origin: OriginPulled})
		return err
	})
	if err == nil {
		t.Fatal("want the insert error")
	}
	h.versions.setRecordErr(nil)
	err = h.svc.versionTx(context.Background(), func(tx *gorm.DB) error {
		v, _, err := h.svc.recordVersion(tx, recordVersionRequest{ProjectID: p.ID, Manifest: "[workspace]\n# y\n", Lock: "version: 6\n", Origin: OriginPulled})
		if err == nil && v.Number != writers+2 {
			return fmt.Errorf("next number = %d, want %d", v.Number, writers+2)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordVersionChecks(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("checks", false)
	_, otherV1 := h.newProject("checks-other", false)
	recordV := func(req recordVersionRequest) (*ProjectVersion, bool, error) {
		var v *ProjectVersion
		var dedup bool
		err := h.svc.versionTx(context.Background(), func(tx *gorm.DB) error {
			var err error
			v, dedup, err = h.svc.recordVersion(tx, req)
			return err
		})
		return v, dedup, err
	}
	record := func(req recordVersionRequest) error {
		_, _, err := recordV(req)
		return err
	}
	m, l := "[workspace]\n# checks\n", "version: 6\n# checks\n"
	if err := record(recordVersionRequest{ProjectID: uuid.New(), Manifest: m, Lock: l, Origin: OriginPulled}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: %v", err)
	}
	if err := record(recordVersionRequest{ProjectID: p.ID, BaseVersionID: &otherV1.ID, Manifest: m, Lock: l, Origin: OriginPulled}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("base of another project: %v", err)
	}
	if err := record(recordVersionRequest{ProjectID: p.ID, Manifest: m, Lock: l, Origin: OriginPulled, Tags: []string{"LATEST"}}); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("reserved tag: %v", err)
	}
	if err := record(recordVersionRequest{ProjectID: p.ID, Manifest: " ", Lock: l, Origin: OriginPulled}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("empty manifest: %v", err)
	}
	for _, lock := range []string{"", " \n"} {
		if err := record(recordVersionRequest{ProjectID: p.ID, Manifest: m, Lock: lock, Origin: OriginPulled}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("empty lock %q: want ErrInvalidRequest, got %v", lock, err)
		}
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
		t.Fatalf("refused records must write nothing, %d versions", n)
	}
	if err := record(recordVersionRequest{ProjectID: p.ID, Manifest: m, Lock: l, Origin: "rolled_back"}); err == nil {
		t.Fatal("unknown origin must be refused")
	}

	// A tag already on another version is refused by recordVersion itself,
	// not only by the callers' up-front checks.
	if _, err := h.svc.SetTag(context.Background(), SetTagRequest{ProjectID: p.ID, Name: "held", VersionID: v1.ID}); err != nil {
		t.Fatal(err)
	}
	if err := record(recordVersionRequest{ProjectID: p.ID, Manifest: m, Lock: l, Origin: OriginPulled, Tags: []string{"held"}}); !errors.Is(err, ErrTagExists) {
		t.Fatalf("tag on another version: want ErrTagExists, got %v", err)
	}

	// Without DedupeLatest, content equal to the latest version is still
	// a new version; with it, the latest is returned and nothing written.
	same := recordVersionRequest{ProjectID: p.ID, Manifest: v1.Manifest, Lock: v1.Lock, Origin: OriginPulled, Author: "remote"}
	v, dedup, err := recordV(same)
	if err != nil || dedup || v.Number != 2 || v.ContentHash != v1.ContentHash {
		t.Fatalf("DedupeLatest false: want a new version 2 with version 1's hash, got %+v, %v, %v", v, dedup, err)
	}
	same.DedupeLatest = true
	got, dedup, err := recordV(same)
	if err != nil || !dedup || got.ID != v.ID || got.Manifest != v1.Manifest || got.Lock != v1.Lock {
		t.Fatalf("DedupeLatest true: want version 2 returned whole and deduplicated, got %+v, %v, %v", got, dedup, err)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 2 {
		t.Fatalf("a deduplicated record must write nothing, %d versions", n)
	}
}

func TestSnapshotWorkingCopy(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("hand-edited", true)
	manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
	lockPath := filepath.Join(p.Path, artifact.LockFile)

	// The user edits pixi.toml by hand.
	edited := readFile(t, manifestPath) + "scipy = \"*\"\n"
	if err := os.WriteFile(manifestPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.svc.WorkingCopyStatus(context.Background(), p.ID); st != WorkingCopyModified {
		t.Fatalf("working copy = %s, want modified", st)
	}
	// Not locked yet: refused.
	if _, err := h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "", nil); !errors.Is(err, ErrLockStale) {
		t.Fatalf("stale lock: want ErrLockStale, got %v", err)
	}
	// The user runs pixi lock.
	if err := os.WriteFile(lockPath, []byte(fakeLockFor(edited)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "add scipy", []string{"wip"})
	if err != nil {
		t.Fatalf("SnapshotWorkingCopy: %v", err)
	}
	v := res.Version
	if res.Deduplicated || res.Job != nil || v == nil || v.Number != 2 || v.Origin != OriginSnapshot ||
		v.BaseVersionID == nil || *v.BaseVersionID != v1.ID || v.Description != "add scipy" {
		t.Fatalf("want snapshot version 2 based on the installed version 1, got %+v", res)
	}
	if v.Manifest != edited || v.Lock != fakeLockFor(edited) {
		t.Fatal("the snapshot must hold the files on disk")
	}
	if tags := h.versions.tagsOf(t, p.ID); tags["wip"] != 2 {
		t.Fatalf("tags = %v", tags)
	}
	d := h.detail(p.ID)
	if d.InstalledVersionID == nil || *d.InstalledVersionID != v1.ID {
		t.Fatal("a snapshot must not change InstalledVersionID")
	}

	// The same files again are deduplicated against the latest version.
	res, err = h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "", nil)
	if err != nil || !res.Deduplicated || res.Version.ID != v.ID {
		t.Fatalf("want deduplicated against version 2, got %+v, %v", res, err)
	}
}

func TestSnapshotRefusals(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("refused", false)
	lockPath := filepath.Join(p.Path, artifact.LockFile)

	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "", nil); !errors.Is(err, ErrLockStale) {
		t.Fatalf("no pixi.lock: want ErrLockStale, got %v", err)
	}
	// A missing lock is refused before pixi runs, so the result does not
	// depend on what pixi's check does without a lock file.
	h.pixi.mu.Lock()
	checks := len(h.pixi.checkCalls)
	h.pixi.mu.Unlock()
	if checks != 0 {
		t.Fatalf("no pixi.lock: CheckLock must not run, ran %d times", checks)
	}
	if err := os.Remove(filepath.Join(p.Path, artifact.ManifestFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "", nil); !errors.Is(err, ErrWorkingCopyMissing) {
		t.Fatalf("no pixi.toml: want ErrWorkingCopyMissing, got %v", err)
	}
	if _, err := h.svc.SnapshotWorkingCopy(context.Background(), uuid.New(), "", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want ErrNotFound, got %v", err)
	}

	// A check that cannot run is an ordinary error, not ErrLockStale.
	q, _ := h.newProject("check-broken", false)
	h.pixi.mu.Lock()
	h.pixi.checkErr = errors.New("pixi not found")
	h.pixi.mu.Unlock()
	if _, err := h.svc.SnapshotWorkingCopy(context.Background(), q.ID, "", nil); err == nil || errors.Is(err, ErrLockStale) {
		t.Fatalf("want a plain error, got %v", err)
	}
	h.pixi.mu.Lock()
	h.pixi.checkErr = nil
	h.pixi.mu.Unlock()
	if _, err := h.svc.SnapshotWorkingCopy(context.Background(), q.ID, "", []string{"sha-1"}); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("reserved tag: want ErrInvalidTag, got %v", err)
	}
}

func TestSnapshotWithoutInstalledVersionHasNoBase(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("not-installed", false)
	manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
	edited := readFile(t, manifestPath) + "# changed\n"
	if err := os.WriteFile(manifestPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, artifact.LockFile), []byte(fakeLockFor(edited)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Version.BaseVersionID != nil {
		t.Fatal("a snapshot with nothing installed has no base")
	}
}

func TestSnapshotRefusedWhileBusy(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.lockGate, h.pixi.lockStarted = gate, started
	p, job, err := h.svc.CreateProject(context.Background(), specRequest("busy-snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	_, err = h.svc.SnapshotWorkingCopy(context.Background(), p.ID, "", nil)
	var busy *BusyError
	if !errors.Is(err, ErrProjectBusy) || !errors.As(err, &busy) || busy.JobIDs[0] != job.ID {
		t.Fatalf("want a BusyError naming the create job, got %v", err)
	}
	close(gate)
	h.wait(job)
}

func TestReadVersions(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("reads", true)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Tags: []string{"beta", "alpha"}})
	v2 := h.versions.versionsOf(t, p.ID)[1]
	if _, err := h.svc.SetTag(context.Background(), SetTagRequest{ProjectID: p.ID, Name: "stable", VersionID: v1.ID}); err != nil {
		t.Fatal(err)
	}

	list, err := h.svc.ListVersions(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Number != 2 || list[1].Number != 1 {
		t.Fatalf("want versions newest first, got %+v", list)
	}
	if got := strings.Join(list[0].Tags, ","); got != "latest,alpha,beta" {
		t.Fatalf("version 2 tags = %s", got)
	}
	if got := strings.Join(list[1].Tags, ","); got != "stable" {
		t.Fatalf("version 1 tags = %s", got)
	}
	if list[0].Installed || !list[1].Installed {
		t.Fatal("only version 1 is installed")
	}
	if list[0].BaseNumber == nil || *list[0].BaseNumber != 1 || list[1].BaseNumber != nil {
		t.Fatal("version 2 is based on 1, version 1 has no base")
	}
	if list[0].Origin != OriginCreated || list[0].Author != "tester" {
		t.Fatalf("origin/author = %s/%s", list[0].Origin, list[0].Author)
	}
	// File contents are not part of the list.
	if list[0].Manifest != "" || list[0].Lock != "" {
		t.Fatal("the list must not load pixi.toml or pixi.lock")
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fake lock") || strings.Contains(string(raw), "[workspace]") {
		t.Fatalf("the list response must not contain the files: %s", raw)
	}

	one, err := h.svc.GetVersion(context.Background(), p.ID, 1)
	if err != nil || one.ID != v1.ID || !one.Installed || strings.Join(one.Tags, ",") != "stable" {
		t.Fatalf("GetVersion(1) = %+v, %v", one, err)
	}
	two, err := h.svc.GetVersion(context.Background(), p.ID, 2)
	if err != nil || two.BaseNumber == nil || *two.BaseNumber != 1 || two.Tags[0] != LatestTag {
		t.Fatalf("GetVersion(2) = %+v, %v", two, err)
	}
	if _, err := h.svc.GetVersion(context.Background(), p.ID, 9); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVersion(9): want ErrNotFound, got %v", err)
	}

	// Each version is complete on its own.
	for _, v := range []ProjectVersion{v1, v2} {
		manifest, err := h.svc.GetVersionFile(context.Background(), p.ID, v.Number, FileManifest)
		if err != nil || manifest != v.Manifest {
			t.Fatalf("pixi.toml of %d: %v", v.Number, err)
		}
		lock, err := h.svc.GetVersionFile(context.Background(), p.ID, v.Number, FileLock)
		if err != nil || lock != v.Lock {
			t.Fatalf("pixi.lock of %d: %v", v.Number, err)
		}
	}
	if _, err := h.svc.GetVersionFile(context.Background(), p.ID, 1, "pixi.json"); err == nil {
		t.Fatal("unknown file must be refused")
	}
	if _, err := h.svc.GetVersionFile(context.Background(), p.ID, 9, FileLock); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown version: want ErrNotFound, got %v", err)
	}
	if _, err := h.svc.ListVersions(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want ErrNotFound, got %v", err)
	}
}

func TestTags(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("tagged", false)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]
	ctx := context.Background()

	for _, bad := range []string{"latest", "Latest", "sha-123", "SHA-x", "", ".hidden", "-dash", "has space", "a/b", strings.Repeat("a", 129)} {
		if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: bad, VersionID: v1.ID}); !errors.Is(err, ErrInvalidTag) {
			t.Errorf("SetTag(%q): want ErrInvalidTag, got %v", bad, err)
		}
	}
	for _, good := range []string{"v1.0", "_x", "RC-1", strings.Repeat("a", 128), "shadow"} {
		if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: good, VersionID: v1.ID}); err != nil {
			t.Errorf("SetTag(%q): %v", good, err)
		}
	}

	tag, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: "prod", VersionID: v1.ID})
	if err != nil || tag.VersionID != v1.ID || tag.Name != "prod" {
		t.Fatalf("SetTag: %+v, %v", tag, err)
	}
	if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: "prod", VersionID: v1.ID}); err != nil {
		t.Fatalf("setting a tag on its own version again must be a no-op: %v", err)
	}
	if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: "prod", VersionID: v2.ID}); !errors.Is(err, ErrTagExists) {
		t.Fatalf("moving without Move: want ErrTagExists, got %v", err)
	}
	if tag, err = h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: "prod", VersionID: v2.ID, Move: true}); err != nil || tag.VersionID != v2.ID {
		t.Fatalf("Move: %+v, %v", tag, err)
	}
	if v, err := h.svc.ResolveTag(ctx, p.ID, "prod"); err != nil || v.ID != v2.ID || v.Manifest != v2.Manifest {
		t.Fatalf("ResolveTag(prod) = %v, %v", v, err)
	}
	if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: "x", VersionID: uuid.New()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown version: want ErrNotFound, got %v", err)
	}
	q, qv1 := h.newProject("tagged-other", false)
	if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: p.ID, Name: "x", VersionID: qv1.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("version of another project: want ErrNotFound, got %v", err)
	}
	// Tags are per project.
	if _, err := h.svc.SetTag(ctx, SetTagRequest{ProjectID: q.ID, Name: "prod", VersionID: qv1.ID}); err != nil {
		t.Fatalf("the same tag name in another project: %v", err)
	}

	// Deleting a tag keeps its version.
	if err := h.svc.DeleteTag(ctx, p.ID, "prod"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.ResolveTag(ctx, p.ID, "prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted tag: want ErrNotFound, got %v", err)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 2 {
		t.Fatalf("deleting a tag must keep the version, %d versions", n)
	}
	if err := h.svc.DeleteTag(ctx, p.ID, "prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a missing tag: want ErrNotFound, got %v", err)
	}
	if err := h.svc.DeleteTag(ctx, p.ID, "latest"); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("deleting latest: want ErrInvalidTag, got %v", err)
	}
	if _, ok := h.versions.tagsOf(t, q.ID)["prod"]; !ok {
		t.Fatal("deleting a tag must not touch other projects")
	}
}

func TestLatestAlwaysResolvesToTheHighestNumber(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("latest", false)
	ctx := context.Background()
	check := func(want int) {
		t.Helper()
		v, err := h.svc.ResolveTag(ctx, p.ID, LatestTag)
		if err != nil || v.Number != want {
			t.Fatalf("latest = %v, %v; want version %d", v, err, want)
		}
		list, _ := h.svc.ListVersions(ctx, p.ID)
		for _, s := range list {
			hasLatest := len(s.Tags) > 0 && s.Tags[0] == LatestTag
			if hasLatest != (s.Number == want) {
				t.Fatalf("version %d tags %v; latest must be on %d only", s.Number, s.Tags, want)
			}
		}
	}
	check(1)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	check(2)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, AllowOlderBase: true})
	check(3)

	var n int64
	if err := h.db.Model(&ProjectTag{}).Where("name = ?", LatestTag).Count(&n).Error; err != nil || n != 0 {
		t.Fatalf("latest must never be stored, %d rows", n)
	}
	empty := newHarness(t)
	empty.pixi.lockErr = errors.New("no solve")
	ep, _ := empty.mustCreate(specRequest("empty"))
	if _, err := empty.svc.ResolveTag(ctx, ep.ID, LatestTag); !errors.Is(err, ErrNotFound) {
		t.Fatalf("latest of a project without versions: want ErrNotFound, got %v", err)
	}
}

func TestWorkingCopyStatus(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	notInstalled, _ := h.newProject("wc-nobase", false)
	if st, err := h.svc.WorkingCopyStatus(ctx, notInstalled.ID); err != nil || st != WorkingCopyNoBase {
		t.Fatalf("not installed: %s, %v", st, err)
	}

	p, _ := h.newProject("wc", true)
	if st, _ := h.svc.WorkingCopyStatus(ctx, p.ID); st != WorkingCopyClean {
		t.Fatalf("after install: %s", st)
	}
	lockPath := filepath.Join(p.Path, artifact.LockFile)
	lock := readFile(t, lockPath)
	if err := os.WriteFile(lockPath, []byte(lock+"# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.svc.WorkingCopyStatus(ctx, p.ID); st != WorkingCopyModified {
		t.Fatalf("edited lock: %s", st)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.svc.WorkingCopyStatus(ctx, p.ID); st != WorkingCopyModified {
		t.Fatalf("missing lock: %s", st)
	}
	if err := os.Remove(filepath.Join(p.Path, artifact.ManifestFile)); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.svc.WorkingCopyStatus(ctx, p.ID); st != WorkingCopyMissing {
		t.Fatalf("missing pixi.toml: %s", st)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.svc.WorkingCopyStatus(ctx, p.ID); st != WorkingCopyMissing {
		t.Fatalf("missing directory: %s", st)
	}
	if _, err := h.svc.WorkingCopyStatus(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want ErrNotFound, got %v", err)
	}
}

func TestDeleteProjectDeletesVersionsAndTags(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("with-tags", false)
	if _, err := h.svc.SetTag(context.Background(), SetTagRequest{ProjectID: p.ID, Name: "keep", VersionID: v1.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	var versions, tags int64
	h.db.Model(&ProjectVersion{}).Where("project_id = ?", p.ID).Count(&versions)
	h.db.Model(&ProjectTag{}).Where("project_id = ?", p.ID).Count(&tags)
	if versions != 0 || tags != 0 {
		t.Fatalf("versions and tags must go with the project: %d versions, %d tags", versions, tags)
	}
}

func TestMigrateNeverRebuildsTables(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("migrated", false)
	if _, err := h.svc.SetTag(context.Background(), SetTagRequest{ProjectID: p.ID, Name: "kept", VersionID: v1.ID}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var stmts []string
	if err := h.db.Callback().Raw().Before("gorm:raw").Register("test:migrate_sql", func(tx *gorm.DB) {
		mu.Lock()
		stmts = append(stmts, tx.Statement.SQL.String())
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(h.db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	mu.Lock()
	if len(stmts) != 0 {
		t.Fatalf("Migrate on an up-to-date database must change nothing, ran: %q", stmts)
	}
	mu.Unlock()

	// A missing column and index are added back without touching the rows.
	if err := h.db.Exec("DROP INDEX idx_client_project_versions_content_hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := h.db.Exec("ALTER TABLE client_project_versions DROP COLUMN description").Error; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	stmts = nil
	mu.Unlock()
	if err := Migrate(h.db); err != nil {
		t.Fatalf("Migrate with a missing column: %v", err)
	}
	m := h.db.Migrator()
	if !m.HasColumn(&ProjectVersion{}, "description") || !m.HasIndex(&ProjectVersion{}, "idx_client_project_versions_content_hash") {
		t.Fatal("Migrate must add the missing column and index")
	}
	mu.Lock()
	for _, s := range stmts {
		if strings.Contains(s, "__temp") || strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s)), "DROP") {
			t.Fatalf("Migrate must not rebuild a table: %q", s)
		}
	}
	mu.Unlock()
	if list := h.versions.versionsOf(t, p.ID); len(list) != 1 || list[0].Manifest != v1.Manifest {
		t.Fatal("rows must be kept")
	}
	if tags := h.versions.tagsOf(t, p.ID); tags["kept"] != 1 {
		t.Fatalf("tags must be kept: %v", tags)
	}
}

func TestVersionTablesHaveForeignKeys(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("fk", false)
	if err := h.db.Delete(&Project{}, "id = ?", p.ID).Error; err == nil {
		t.Fatal("a project with versions must not be deletable on its own")
	}
	bad := ProjectVersion{ProjectID: uuid.New(), Number: 1, Manifest: "x", ContentHash: "h", Origin: OriginPulled, Author: "a"}
	if err := h.db.Create(&bad).Error; err == nil {
		t.Fatal("a version of an unknown project must be refused")
	}
	if err := h.db.Create(&ProjectTag{ProjectID: p.ID, Name: "dangling", VersionID: uuid.New()}).Error; err == nil {
		t.Fatal("a tag on an unknown version must be refused")
	}
	if err := h.db.Create(&ProjectTag{ProjectID: p.ID, Name: "ok", VersionID: v1.ID}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestCreateProjectValidatesTagsUpFront(t *testing.T) {
	h := newHarness(t)
	for _, bad := range []string{"latest", "sha-abc", "two words"} {
		req := specRequest("tag-check")
		req.Tags = []string{"ok", bad}
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrInvalidTag) {
			t.Fatalf("tag %q: want ErrInvalidTag, got %v", bad, err)
		}
	}
	if h.projectCount() != 0 || exists(filepath.Join(h.projectsDir, "tag-check")) {
		t.Fatal("a refused tag must create no row and no directory")
	}
	if lock, _ := h.pixi.calls(); len(lock) != 0 {
		t.Fatal("a refused tag must start no job")
	}
}
