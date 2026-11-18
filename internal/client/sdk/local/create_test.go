package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

func TestCreateFromSpec(t *testing.T) {
	h := newHarness(t)
	req := specRequest("my-analysis")
	req.Description = "Q3 sales analysis"
	req.Tags = []string{"stable", " stable ", "", "q3"}

	p, job, err := h.svc.CreateProject(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if job == nil || job.Type != jobs.TypeCreate || job.ProjectID != p.ID {
		t.Fatalf("want a create job for the project, got %+v", job)
	}
	wantDir := h.resolved(filepath.Join(h.projectsDir, "my-analysis"))
	if p.Path != wantDir || !p.ManagedDir || p.Name != "my-analysis" || p.InstalledVersionID != nil {
		t.Fatalf("unexpected project %+v (want path %s, managed)", p, wantDir)
	}

	manifest := readFile(t, filepath.Join(wantDir, artifact.ManifestFile))
	parsed, err := artifact.ParseManifest(manifest)
	if err != nil {
		t.Fatalf("written pixi.toml does not parse: %v\n%s", err, manifest)
	}
	if parsed.Name != "my-analysis" || parsed.Description != "Q3 sales analysis" {
		t.Fatalf("[workspace] name/description not written: %+v", parsed)
	}
	if !strings.Contains(manifest, `numpy = ">=1.26"`) || !strings.Contains(manifest, `platforms = ["linux-64", "osx-arm64"]`) {
		t.Fatalf("spec not rendered:\n%s", manifest)
	}

	done := h.wait(job)
	if done.Status != jobs.StatusCompleted {
		t.Fatalf("create job failed: %+v", done)
	}
	lock, install := h.pixi.calls()
	if len(lock) != 1 || lock[0] != wantDir || len(install) != 0 {
		t.Fatalf("pixi calls: lock=%v install=%v; want one lock in %s and no install", lock, install, wantDir)
	}

	versions := h.versions.versionsOf(t, p.ID)
	if len(versions) != 1 {
		t.Fatalf("want version 1 recorded, got %d versions", len(versions))
	}
	v := versions[0]
	if v.Number != 1 || v.Origin != OriginCreated || v.Base != nil || v.Tags != "stable,q3" {
		t.Fatalf("version 1 = %+v; want origin created, no base, tags stable,q3", v)
	}
	if v.Author != "tester" || v.Description != "" {
		t.Fatalf("version 1 author/description = %q/%q; want the service author and no description", v.Author, v.Description)
	}
	if v.Manifest != manifest || v.Lock != fakeLockContent {
		t.Fatal("version 1 must hold the pixi.toml and pixi.lock contents")
	}
	if done.VersionID == nil || *done.VersionID != v.ID {
		t.Fatalf("the job must carry version 1's ID: VersionID = %v, want %s", done.VersionID, v.ID)
	}
	if h.versions.recordTx[0] == nil || h.versions.recordTx[0].Statement.ConnPool == h.db.Statement.ConnPool {
		t.Fatal("version 1 must be recorded inside a transaction")
	}

	d := h.detail(p.ID)
	if d.State != StateNotInstalled || d.LatestVersion == nil || *d.LatestVersion != 1 || d.InstalledVersion != nil {
		t.Fatalf("after create: %+v", d.ProjectSummary)
	}
}

func TestCreateFromManifest(t *testing.T) {
	h := newHarness(t)
	raw := `# keep this comment
[workspace]
name = "whatever"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.12.*"
`
	p, job := h.mustCreate(CreateProjectRequest{Name: "raw-env", Manifest: raw, Description: "from a paste"})
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("job: %+v", job)
	}
	want := `# keep this comment
[workspace]
name = "raw-env"
description = "from a paste"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.12.*"
`
	if got := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)); got != want {
		t.Fatalf("manifest must be written as given with only name/description set\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
		t.Fatalf("want 1 version, got %d", n)
	}
}

// TestCreateLeavesPlatformsAndChannelsToPixi covers raw manifests that pixi
// locks but a fixed platform list or a top-level channel check would
// refuse: newer platform names, and no channels in [workspace] because a
// feature sets its own.
func TestCreateLeavesPlatformsAndChannelsToPixi(t *testing.T) {
	h := newHarness(t)
	for name, raw := range map[string]string{
		"newer-platforms": "[workspace]\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\", \"linux-loongarch64\", \"freebsd-64\"]\n",
		"feature-channels": "[workspace]\nchannels = []\nplatforms = [\"linux-64\"]\n\n" +
			"[feature.py.dependencies]\npython = \"*\"\n\n[feature.py]\nchannels = [\"conda-forge\"]\n\n" +
			"[environments]\npy = [\"py\"]\n",
	} {
		p, job := h.mustCreate(CreateProjectRequest{Name: name, Manifest: raw})
		if job.Status != jobs.StatusCompleted {
			t.Fatalf("%s: job: %+v", name, job)
		}
		if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
			t.Fatalf("%s: want 1 version, got %d", name, n)
		}
	}
}

func TestCreateAndInstall(t *testing.T) {
	h := newHarness(t)
	req := specRequest("installed-env")
	req.Install = true
	p, job := h.mustCreate(req)
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("job: %+v", job)
	}
	lock, install := h.pixi.calls()
	if len(lock) != 1 || len(install) != 1 {
		t.Fatalf("want lock then install, got lock=%v install=%v", lock, install)
	}
	v := h.versions.versionsOf(t, p.ID)
	d := h.detail(p.ID)
	if d.InstalledVersionID == nil || *d.InstalledVersionID != v[0].ID {
		t.Fatalf("InstalledVersionID = %v, want version 1 (%s)", d.InstalledVersionID, v[0].ID)
	}
	if d.State != StateInstalled || d.InstalledVersion == nil || *d.InstalledVersion != 1 {
		t.Fatalf("want installed v1, got %+v", d.ProjectSummary)
	}
}

func TestCreateLockFailureKeepsProject(t *testing.T) {
	h := newHarness(t)
	h.pixi.lockErr = errors.New("cannot solve numpy ==99")
	p, job, err := h.svc.CreateProject(context.Background(), specRequest("broken"))
	if err != nil {
		t.Fatalf("CreateProject itself must succeed; the job fails: %v", err)
	}
	done := h.wait(job)
	if done.Status != jobs.StatusFailed || !strings.Contains(done.Error, "cannot solve") {
		t.Fatalf("want failed job, got %+v", done)
	}

	d := h.detail(p.ID)
	if d.State != StateLockFailed {
		t.Fatalf("state = %s, want lock_failed", d.State)
	}
	if d.JobID == nil || *d.JobID != job.ID {
		t.Fatalf("lock_failed must link to the job: JobID = %v, want %s", d.JobID, job.ID)
	}
	if d.VersionCount != 0 || d.LatestVersion != nil {
		t.Fatalf("no version may be recorded: %+v", d)
	}
	if !exists(filepath.Join(p.Path, artifact.ManifestFile)) {
		t.Fatal("the user's pixi.toml must be kept after a failed lock")
	}
	if h.projectCount() != 1 {
		t.Fatal("the project row must be kept after a failed lock")
	}
}

func TestCreateInstallFailureAfterLock(t *testing.T) {
	h := newHarness(t)
	h.pixi.installErr = errors.New("download failed")
	h.pixi.partialEnv = true // pixi can leave .pixi/envs behind before failing
	req := specRequest("half-installed")
	req.Install = true
	p, job := h.mustCreate(req)
	if job.Status != jobs.StatusFailed || job.Stage != StageInstall {
		t.Fatalf("want job failed at the install stage, got %+v", job)
	}
	d := h.detail(p.ID)
	if d.VersionCount != 1 {
		t.Fatalf("version 1 must be recorded before the install: %d versions", d.VersionCount)
	}
	if d.State != StateInstallFailed || d.JobID == nil || *d.JobID != job.ID {
		t.Fatalf("want install_failed linked to the job, got %+v", d.ProjectSummary)
	}
	if d.InstalledVersionID != nil {
		t.Fatal("InstalledVersionID must stay nil when the install failed")
	}
}

func TestCreateRecordVersionFailure(t *testing.T) {
	h := newHarness(t)
	h.versions.recordErr = errors.New("db locked")
	p, job := h.mustCreate(specRequest("unrecorded"))
	if job.Status != jobs.StatusFailed {
		t.Fatalf("job: %+v", job)
	}
	if d := h.detail(p.ID); d.State != StateLockFailed {
		t.Fatalf("no version recorded means lock_failed, got %s", d.State)
	}
	if job.VersionID != nil {
		t.Fatalf("a job that recorded no version must not report one, got %s", job.VersionID)
	}
}

func TestCreateValidation(t *testing.T) {
	spec := specRequest("x").Spec
	noPlatform := &artifact.Spec{Channels: []string{"conda-forge"}}
	cases := []struct {
		name string
		req  CreateProjectRequest
		want error
	}{
		{"invalid name", CreateProjectRequest{Name: "Bad Name", Spec: spec}, ErrInvalidName},
		{"empty name", CreateProjectRequest{Name: "", Spec: spec}, ErrInvalidName},
		{"both spec and manifest", CreateProjectRequest{Name: "x", Spec: spec, Manifest: "[workspace]\nplatforms=[\"linux-64\"]\n"}, ErrInvalidManifest},
		{"neither spec nor manifest", CreateProjectRequest{Name: "x"}, ErrInvalidManifest},
		{"blank manifest", CreateProjectRequest{Name: "x", Manifest: "  \n"}, ErrInvalidManifest},
		{"spec without platform", CreateProjectRequest{Name: "x", Spec: noPlatform}, ErrNoPlatform},
		{"spec with blank platform", CreateProjectRequest{Name: "x", Spec: &artifact.Spec{Platforms: []string{" "}}}, ErrNoPlatform},
		{"manifest without platform", CreateProjectRequest{Name: "x", Manifest: "[workspace]\nchannels = [\"conda-forge\"]\n"}, ErrNoPlatform},
		{"manifest does not parse", CreateProjectRequest{Name: "x", Manifest: "[workspace\nname="}, ErrInvalidManifest},
		{"manifest without workspace", CreateProjectRequest{Name: "x", Manifest: "[dependencies]\nnumpy = \"*\"\n"}, ErrInvalidManifest},
		{"bad package in spec", CreateProjectRequest{Name: "x", Spec: &artifact.Spec{Platforms: []string{"linux-64"}, Channels: []string{"conda-forge"}, Packages: []artifact.Package{{Name: "bad name"}}}}, ErrInvalidManifest},
		{"spec without channel", CreateProjectRequest{Name: "x", Spec: &artifact.Spec{Platforms: []string{"linux-64"}}}, ErrInvalidManifest},
		{"spec with operator twice", CreateProjectRequest{Name: "x", Spec: &artifact.Spec{Platforms: []string{"linux-64"}, Channels: []string{"conda-forge"}, Packages: []artifact.Package{{Name: "numpy", Operator: ">=", Constraint: ">=1"}}}}, ErrInvalidManifest},
		{"manifest with duplicate platform", CreateProjectRequest{Name: "x", Manifest: "[workspace]\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\", \"linux-64\"]\n"}, ErrInvalidManifest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			p, job, err := h.svc.CreateProject(context.Background(), c.req)
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
			if p != nil || job != nil {
				t.Fatal("a rejected create must return no project and no job")
			}
			if h.projectCount() != 0 {
				t.Fatal("a rejected create must not insert a row")
			}
			if exists(h.projectsDir) {
				t.Fatal("a rejected create must not create any directory")
			}
			if lock, _ := h.pixi.calls(); len(lock) != 0 {
				t.Fatal("a rejected create must not run pixi")
			}
		})
	}
}

func TestCreateNameTakenIgnoringCase(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(specRequest("demo"))
	h.insertRow(&Project{Name: "Legacy", Path: "/legacy"})

	for _, name := range []string{"demo", "legacy"} {
		if _, _, err := h.svc.CreateProject(context.Background(), specRequest(name)); !errors.Is(err, ErrNameTaken) {
			t.Fatalf("%s: want ErrNameTaken, got %v", name, err)
		}
	}
}

func TestCreatePathTakenAndNotEmpty(t *testing.T) {
	h := newHarness(t)
	userDir := filepath.Join(t.TempDir(), "work")
	req := specRequest("first")
	req.Path = userDir
	first, _ := h.mustCreate(req)

	t.Run("path already tracked", func(t *testing.T) {
		req := specRequest("second")
		req.Path = userDir
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrPathTaken) {
			t.Fatalf("want ErrPathTaken, got %v", err)
		}
	})
	t.Run("same directory through a symlink", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(first.Path, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		req := specRequest("third")
		req.Path = link
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrPathTaken) {
			t.Fatalf("want ErrPathTaken for a symlink to a tracked directory, got %v", err)
		}
	})
	t.Run("untracked directory with a pixi.toml", func(t *testing.T) {
		dir := t.TempDir()
		original := "[workspace]\nname = \"mine\"\nplatforms = [\"linux-64\"]\n"
		if err := os.WriteFile(filepath.Join(dir, artifact.ManifestFile), []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		req := specRequest("fourth")
		req.Path = dir
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrPathNotEmpty) {
			t.Fatalf("want ErrPathNotEmpty, got %v", err)
		}
		if got := readFile(t, filepath.Join(dir, artifact.ManifestFile)); got != original {
			t.Fatal("an existing pixi.toml must never be overwritten")
		}
	})
	t.Run("path is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		req := specRequest("fifth")
		req.Path = file
		if _, _, err := h.svc.CreateProject(context.Background(), req); err == nil {
			t.Fatal("a path that is a file must be rejected")
		}
	})
}

func TestCreateInExistingUserDirectory(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := specRequest("in-place")
	req.Path = dir
	p, _ := h.mustCreate(req)
	if p.ManagedDir {
		t.Fatal("a directory the user chose must not be managed")
	}
	if p.Path != h.resolved(dir) {
		t.Fatalf("path = %s, want %s", p.Path, h.resolved(dir))
	}
	if readFile(t, other) != "keep" {
		t.Fatal("other files in the directory must be left alone")
	}
}

func TestCreateManagedDirRules(t *testing.T) {
	t.Run("new directory outside the projects dir is not managed", func(t *testing.T) {
		h := newHarness(t)
		req := specRequest("outside")
		req.Path = filepath.Join(t.TempDir(), "new", "outside")
		p, _ := h.mustCreate(req)
		if p.ManagedDir {
			t.Fatal("only directories under the projects dir can be managed")
		}
		if !exists(filepath.Join(p.Path, artifact.ManifestFile)) {
			t.Fatal("the directory and pixi.toml must be created")
		}
	})
	t.Run("existing directory at the default location is not managed", func(t *testing.T) {
		h := newHarness(t)
		leftover := filepath.Join(h.projectsDir, "leftover")
		if err := os.MkdirAll(leftover, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(leftover, "data.csv"), []byte("1,2"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, _ := h.mustCreate(specRequest("leftover"))
		if p.ManagedDir {
			t.Fatal("a directory nebi did not create must not be managed, even at the default location")
		}
	})
	t.Run("new directory given explicitly under the projects dir is managed", func(t *testing.T) {
		h := newHarness(t)
		req := specRequest("explicit")
		req.Path = filepath.Join(h.projectsDir, "explicit")
		p, _ := h.mustCreate(req)
		if !p.ManagedDir {
			t.Fatal("nebi created it under the projects dir, so it is managed")
		}
	})
}

func TestCreateExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	h := newHarness(t)
	req := specRequest("tilde")
	req.Path = "~/code/tilde"
	p, _ := h.mustCreate(req)
	if want := h.resolved(filepath.Join(home, "code", "tilde")); p.Path != want {
		t.Fatalf("path = %s, want %s", p.Path, want)
	}
}

// failInsert makes every insert into client_projects fail.
func failInsert(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Callback().Create().Before("gorm:create").Register("test:fail_insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "client_projects" {
			tx.AddError(errors.New("disk full"))
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateInsertFailureCleansUp(t *testing.T) {
	t.Run("directory nebi created is removed", func(t *testing.T) {
		h := newHarness(t)
		failInsert(t, h.db)
		_, _, err := h.svc.CreateProject(context.Background(), specRequest("doomed"))
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("want the insert error, got %v", err)
		}
		if exists(filepath.Join(h.projectsDir, "doomed")) {
			t.Fatal("the directory nebi created must be removed when the insert fails")
		}
	})
	t.Run("user directory is kept, only our pixi.toml is removed", func(t *testing.T) {
		h := newHarness(t)
		failInsert(t, h.db)
		dir := t.TempDir()
		other := filepath.Join(dir, "notes.txt")
		if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		req := specRequest("doomed")
		req.Path = dir
		if _, _, err := h.svc.CreateProject(context.Background(), req); err == nil {
			t.Fatal("want an error")
		}
		if !exists(dir) || readFile(t, other) != "keep" {
			t.Fatal("a directory nebi did not create must be kept")
		}
		if exists(filepath.Join(dir, artifact.ManifestFile)) {
			t.Fatal("the pixi.toml nebi wrote must be removed so a retry works")
		}
	})
}

func TestCreateSubmitFailureUndoesEverything(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.Jobs = failingSubmitter{jobs.NewMemory(jobs.Options{})} })
	_, _, err := h.svc.CreateProject(context.Background(), specRequest("unsubmitted"))
	if err == nil || !strings.Contains(err.Error(), "queue unavailable") {
		t.Fatalf("want the submit error, got %v", err)
	}
	if h.projectCount() != 0 {
		t.Fatal("the row must be removed when the job cannot be submitted")
	}
	if exists(filepath.Join(h.projectsDir, "unsubmitted")) {
		t.Fatal("the directory must be removed when the job cannot be submitted")
	}
}

func TestConcurrentCreatesSameName(t *testing.T) {
	h := newHarness(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	created := make([]*jobs.Job, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := specRequest("race")
			req.Path = filepath.Join(t.TempDir(), "dir")
			_, created[i], errs[i] = h.svc.CreateProject(context.Background(), req)
		}(i)
	}
	wg.Wait()
	ok := 0
	for i, err := range errs {
		switch {
		case err == nil:
			ok++
			h.wait(created[i])
		case !errors.Is(err, ErrNameTaken):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || h.projectCount() != 1 {
		t.Fatalf("exactly one create must win: %d won, %d rows", ok, h.projectCount())
	}
}

func TestWriteNewFileNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixi.toml")
	if err := writeNewFile(path, "first"); err != nil {
		t.Fatal(err)
	}
	if err := writeNewFile(path, "second"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("want ErrExist, got %v", err)
	}
	if readFile(t, path) != "first" {
		t.Fatal("an existing file must not be overwritten")
	}
}

func TestNameIsReusableAfterDelete(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("reuse"))
	job, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	h.wait(job)
	if _, job := h.mustCreate(specRequest("reuse")); job.Status != jobs.StatusCompleted {
		t.Fatalf("recreate: %+v", job)
	}
}

func TestCreateRejectsNestedPaths(t *testing.T) {
	h := newHarness(t)
	outer, _ := h.mustCreate(specRequest("outer"))

	t.Run("inside a tracked project", func(t *testing.T) {
		req := specRequest("inner")
		req.Path = filepath.Join(outer.Path, "inner")
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrPathTaken) {
			t.Fatalf("want ErrPathTaken, got %v", err)
		}
		if exists(req.Path) {
			t.Fatal("a refused create must not create the directory")
		}
	})
	t.Run("containing a tracked project", func(t *testing.T) {
		req := specRequest("parent")
		req.Path = filepath.Dir(outer.Path)
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrPathTaken) {
			t.Fatalf("want ErrPathTaken, got %v", err)
		}
	})
	t.Run("sibling with a shared prefix is fine", func(t *testing.T) {
		req := specRequest("outer-2")
		req.Path = outer.Path + "-2"
		if _, job, err := h.svc.CreateProject(context.Background(), req); err != nil {
			t.Fatalf("CreateProject: %v", err)
		} else {
			h.wait(job)
		}
	})
	if h.projectCount() != 2 {
		t.Fatalf("want 2 projects, got %d", h.projectCount())
	}
}

func TestVersionAuthorDefaults(t *testing.T) {
	if got := versionAuthor("  alice "); got != "alice" {
		t.Fatalf("explicit author: got %q", got)
	}
	if got := versionAuthor(""); strings.TrimSpace(got) == "" {
		t.Fatal("the default author must not be empty")
	}
}

func TestCreateRejectsCaseVariantOfTrackedPath(t *testing.T) {
	h := newHarness(t)
	outer, _ := h.mustCreate(specRequest("outer"))
	projects := filepath.Dir(outer.Path)
	if !caseInsensitive(t, projects) {
		t.Skip("the filesystem is case-sensitive")
	}
	variant := filepath.Join(filepath.Dir(projects), strings.ToUpper(filepath.Base(projects)))

	for name, path := range map[string]string{
		"inside":   filepath.Join(variant, "outer", "inner"),
		"same":     filepath.Join(variant, "OUTER"),
		"contains": variant,
	} {
		req := specRequest("case-" + name)
		req.Path = path
		if _, _, err := h.svc.CreateProject(context.Background(), req); !errors.Is(err, ErrPathTaken) {
			t.Errorf("%s: want ErrPathTaken, got %v", name, err)
		}
	}
	if h.projectCount() != 1 || exists(filepath.Join(outer.Path, "inner")) {
		t.Fatal("a refused create must change nothing")
	}
}

func TestCreateJobLogsPixiOutput(t *testing.T) {
	h := newHarness(t)
	req := specRequest("logged")
	req.Install = true
	p, job := h.mustCreate(req)
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("job: %+v", job)
	}
	out, err := h.runner.Logs(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := fakeLockOutput(p.Path) + fakeInstallOutput(p.Path); string(out) != want {
		t.Fatalf("job log = %q, want pixi's lock then install output %q", out, want)
	}
}

func TestCreateFailureOutputIsLogged(t *testing.T) {
	h := newHarness(t)
	h.pixi.lockErr = errors.New("cannot solve numpy ==99")
	p, job := h.mustCreate(specRequest("unsolvable"))
	out, _ := h.runner.Logs(job.ID)
	if want := fakeLockOutput(p.Path) + "error: cannot solve numpy ==99\n"; string(out) != want {
		t.Fatalf("job log = %q, want %q", out, want)
	}
}

func TestCancelRunningCreateLeavesLockFailed(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.lockGate, h.pixi.lockStarted = gate, started
	defer close(gate)

	p, job, err := h.svc.CreateProject(context.Background(), specRequest("stopped"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := h.runner.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	done := h.wait(job)
	if done.Status != jobs.StatusCancelled || done.Error != "" {
		t.Fatalf("want a cancelled job, got %+v", done)
	}
	d := h.detail(p.ID)
	if d.State != StateLockFailed || d.JobID == nil || *d.JobID != job.ID {
		t.Fatalf("a cancelled create is lock_failed and links to its job, got %+v", d.ProjectSummary)
	}
	if d.VersionCount != 0 || exists(filepath.Join(p.Path, artifact.LockFile)) {
		t.Fatal("a cancelled lock must record no version and write no pixi.lock")
	}
	if !exists(filepath.Join(p.Path, artifact.ManifestFile)) || h.projectCount() != 1 {
		t.Fatal("the project and its pixi.toml are kept, as after a failed lock")
	}
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("a cancelled job must not keep the project busy: %v", err)
	}
}

func TestCancelPendingCreateLeavesLockFailed(t *testing.T) {
	h := newHarnessWithRunner(t, jobs.Options{MaxRunning: 1})
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.lockGate, h.pixi.lockStarted = gate, started

	first, firstJob, err := h.svc.CreateProject(context.Background(), specRequest("first"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	second, secondJob, err := h.svc.CreateProject(context.Background(), specRequest("second"))
	if err != nil {
		t.Fatal(err)
	}
	if secondJob.Status != jobs.StatusPending {
		t.Fatalf("with one slot the second create must wait, got %+v", secondJob)
	}
	if err := h.runner.Cancel(context.Background(), secondJob.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if done := h.wait(secondJob); done.Status != jobs.StatusCancelled || done.StartedAt != nil {
		t.Fatalf("want cancelled without starting, got %+v", done)
	}
	close(gate)
	if done := h.wait(firstJob); done.Status != jobs.StatusCompleted {
		t.Fatalf("first create: %+v", done)
	}

	if d := h.detail(second.ID); d.State != StateLockFailed || d.JobID == nil || *d.JobID != secondJob.ID {
		t.Fatalf("second: want lock_failed linked to its job, got %+v", d.ProjectSummary)
	}
	if d := h.detail(first.ID); d.State != StateNotInstalled {
		t.Fatalf("first: %s", d.State)
	}
	if lock, _ := h.pixi.calls(); len(lock) != 1 || lock[0] != first.Path {
		t.Fatalf("pixi lock must run only for the first project, ran for %v", lock)
	}
}

func TestCancelCreateDuringInstall(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.installGate, h.pixi.installStarted = gate, started
	defer close(gate)

	req := specRequest("half-way")
	req.Install = true
	p, job, err := h.svc.CreateProject(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := h.runner.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	done := h.wait(job)
	if done.Status != jobs.StatusCancelled || done.Stage != StageInstall || done.VersionID == nil {
		t.Fatalf("want cancelled at the install stage after recording version 1, got %+v", done)
	}
	d := h.detail(p.ID)
	if d.VersionCount != 1 || d.InstalledVersionID != nil {
		t.Fatalf("version 1 is kept but not installed: %+v", d)
	}
	// The fake leaves a partial .pixi/envs behind, as pixi can.
	if d.State != StateInstallFailed || d.JobID == nil || *d.JobID != job.ID {
		t.Fatalf("a cancelled install must not look installed, got %+v", d.ProjectSummary)
	}
}
