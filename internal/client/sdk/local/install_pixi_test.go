package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	sdkexec "github.com/nebari-dev/nebi/internal/client/sdk/exec"
	"gorm.io/gorm"
)

// TestRealPixiInstallUninstall creates, installs, replaces and uninstalls a
// project with the real pixi runner, and checks that an install is frozen:
// it installs the version's pixi.lock as it is, without solving. Like the
// runner's own tests it runs only when NEBI_TEST_PIXI lists the pixi
// binaries to test against (separated like PATH), and needs the network to
// solve against conda-forge.
func TestRealPixiInstallUninstall(t *testing.T) {
	var bins []string
	for _, p := range filepath.SplitList(os.Getenv("NEBI_TEST_PIXI")) {
		if p = strings.TrimSpace(p); p != "" {
			bins = append(bins, p)
		}
	}
	if len(bins) == 0 {
		t.Skip("set NEBI_TEST_PIXI to the pixi binaries to test against")
	}
	platform, ok := map[string]string{
		"darwin/arm64":  "osx-arm64",
		"darwin/amd64":  "osx-64",
		"linux/amd64":   "linux-64",
		"linux/arm64":   "linux-aarch64",
		"windows/amd64": "win-64",
		"windows/arm64": "win-arm64",
	}[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		t.Skipf("no conda platform for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	for _, bin := range bins {
		r, err := sdkexec.New(context.Background(), sdkexec.Options{PixiPath: bin})
		if err != nil {
			t.Fatalf("sdkexec.New(%s): %v", bin, err)
		}
		t.Run(r.Version(), func(t *testing.T) { testRealPixiInstallUninstall(t, r, platform) })
	}
}

func testRealPixiInstallUninstall(t *testing.T, r *sdkexec.Runner, platform string) {
	h := newHarness(t, func(o *Options) { o.Pixi = r })
	ctx := context.Background()
	wait := func(job *jobs.Job) jobs.Job {
		t.Helper()
		wctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		done, err := h.runner.Wait(wctx, job.ID)
		if err != nil {
			t.Fatalf("waiting for %s job: %v", job.Type, err)
		}
		if done.Status != jobs.StatusCompleted {
			logs, _ := h.runner.Logs(job.ID)
			t.Fatalf("%s job: %+v\n%s", done.Type, done, logs)
		}
		return done
	}

	manifest := fmt.Sprintf("[workspace]\nchannels = [\"conda-forge\"]\nplatforms = [%q]\n\n[dependencies]\nzlib = \"*\"\n", platform)
	p, job, err := h.svc.CreateProject(ctx, CreateProjectRequest{Name: "real-pixi", Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	wait(job)
	versions := h.versions.versionsOf(t, p.ID)
	if len(versions) != 1 {
		t.Fatalf("want version 1, got %d versions", len(versions))
	}
	v1 := versions[0]
	if exists(envsDir(p.Path)) {
		t.Fatal("create without install must not install")
	}

	installed := func(pkg string) bool {
		t.Helper()
		found, err := filepath.Glob(filepath.Join(envsDir(p.Path), "default", "conda-meta", pkg+"-[0-9]*.json"))
		if err != nil {
			t.Fatal(err)
		}
		return len(found) > 0
	}
	install := func(req InstallRequest) {
		t.Helper()
		job, err := h.svc.InstallVersion(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		done := wait(job)
		if logs, _ := h.runner.Logs(done.ID); len(logs) == 0 {
			t.Error("pixi's output must reach the job's log")
		}
	}

	// Install with no version given: the latest, version 1.
	install(InstallRequest{ProjectID: p.ID})
	if !installed("zlib") {
		t.Fatal("zlib must be installed in .pixi/envs/default")
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.LockFile)); got != v1.Lock {
		t.Fatal("pixi install must not change the version's pixi.lock")
	}
	h.wantInstallState(p, v1)

	// Frozen: version 2 adds bzip2 to pixi.toml but keeps version 1's
	// pixi.lock, which does not have it. Installing it must install exactly
	// that lock: no solve, so no bzip2, and pixi.lock unchanged.
	manifest2 := strings.Replace(v1.Manifest, `zlib = "*"`, "zlib = \"*\"\nbzip2 = \"*\"", 1)
	if manifest2 == v1.Manifest {
		t.Fatalf("could not add bzip2 to:\n%s", v1.Manifest)
	}
	var v2 *ProjectVersion
	if err := h.svc.versionTx(ctx, func(tx *gorm.DB) error {
		var err error
		v2, err = h.svc.recordVersion(tx, recordVersionRequest{
			ProjectID:                p.ID,
			BaseVersionID:            &v1.ID,
			EnvironmentSpecification: EnvironmentSpecification{Manifest: manifest2, Lock: v1.Lock},
			Origin:                   OriginResolved,
			Author:                   "tester",
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	install(InstallRequest{ProjectID: p.ID, VersionID: &v2.ID})
	if got := readFile(t, filepath.Join(p.Path, artifact.LockFile)); got != v1.Lock {
		t.Fatal("a frozen install must leave pixi.lock exactly as the version has it")
	}
	if installed("bzip2") {
		t.Fatal("bzip2 is not in the version's pixi.lock, so a frozen install must not install it")
	}
	if !installed("zlib") {
		t.Fatal("zlib must stay installed")
	}
	h.wantInstallState(p, *v2)
	for _, v := range h.versions.versionsOf(t, p.ID) {
		if v.Lock != v1.Lock {
			t.Fatalf("version %d's pixi.lock changed", v.Number)
		}
	}

	// Installing version 1 again replaces version 2.
	install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	h.wantInstallState(p, v1)

	job, err = h.svc.Uninstall(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	wait(job)
	if exists(envsDir(p.Path)) {
		t.Fatal("uninstall must remove .pixi/envs")
	}
	if readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) != v1.Manifest || readFile(t, filepath.Join(p.Path, artifact.LockFile)) != v1.Lock {
		t.Fatal("uninstall must keep pixi.toml and pixi.lock")
	}
	d := h.detail(p.ID)
	if d.State != StateNotInstalled || d.InstalledVersionID != nil || deref(d.LastInstalledVersion) != 1 {
		t.Fatalf("after uninstall want not_installed, last installed 1, got %+v", d)
	}
}
