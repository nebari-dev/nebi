package remote_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nebari-dev/nebi/internal/client/sdk/remote"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

func TestNormalizeServerURL(t *testing.T) {
	cases := map[string]string{
		"nebi.example.com":             "https://nebi.example.com",
		"https://nebi.example.com/":    "https://nebi.example.com",
		"http://localhost:8460":        "http://localhost:8460",
		" HTTPS://Nebi.Example.com/x/": "https://Nebi.Example.com/x",
	}
	for in, want := range cases {
		got, err := remote.NormalizeServerURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeServerURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "not a url", "ftp://x.y", "nebi example.com"} {
		_, err := remote.NormalizeServerURL(bad)
		var verr *shared.ValidationError
		if !errors.As(err, &verr) || verr.Field != "url" {
			t.Errorf("NormalizeServerURL(%q) want url validation error, got %v", bad, err)
		}
	}
}

func TestConnectManageDisconnect(t *testing.T) {
	ctx := context.Background()
	ts := newTeamServer(t)
	tok := ts.addUser(t, "sam")
	m := newMachine(t, "a")

	conn, err := m.SDK.Remote.GetConnection(ctx)
	if err != nil || conn.Connected {
		t.Fatalf("fresh machine should be disconnected: %+v %v", conn, err)
	}

	var verr *shared.ValidationError
	if _, err := m.SDK.Remote.Connect(ctx, shared.ConnectRequest{URL: ts.URL, Auth: "token"}); !errors.As(err, &verr) || verr.Field != "token" {
		t.Fatalf("missing token: %v", err)
	}
	if _, err := m.SDK.Remote.Connect(ctx, shared.ConnectRequest{URL: ts.URL, Auth: "token", Token: "wrong"}); !errors.As(err, &verr) || verr.Field != "token" {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := m.SDK.Remote.Connect(ctx, shared.ConnectRequest{URL: "http://127.0.0.1:1", Auth: "none"}); !errors.As(err, &verr) || verr.Field != "url" {
		t.Fatalf("unreachable: %v", err)
	}

	conn = m.connect(t, ts, tok)
	if !conn.Connected || conn.Username != "sam" || conn.Auth != "token" || conn.URL != ts.URL || conn.Name == "" {
		t.Fatalf("unexpected connection %+v", conn)
	}
	// Manage connection: saving without retyping the token keeps it.
	conn, err = m.SDK.Remote.Connect(ctx, shared.ConnectRequest{URL: ts.URL + "/", Auth: "token"})
	if err != nil || conn.Username != "sam" {
		t.Fatalf("manage without token: %+v %v", conn, err)
	}
	// None: the server does not identify us.
	conn, err = m.SDK.Remote.Connect(ctx, shared.ConnectRequest{URL: ts.URL, Auth: "none"})
	if err != nil || conn.Username != remote.AnonymousUser || conn.Auth != "none" {
		t.Fatalf("auth none: %+v %v", conn, err)
	}
	m.connect(t, ts, tok)

	p := m.addProject(t, "proj", "a")
	if _, err := m.SDK.Remote.Push(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.SDK.Remote.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	conn, _ = m.SDK.Remote.GetConnection(ctx)
	if conn.Connected {
		t.Fatal("still connected after disconnect")
	}
	var remotes int64
	m.DB.Model(&models.ProjectRemote{}).Where("project_id = ?", p.ID).Count(&remotes)
	if remotes != 1 {
		t.Fatal("disconnect must keep project remotes")
	}
	if _, err := m.SDK.Remote.ListServerProjects(ctx, "", false); !errors.Is(err, shared.ErrNotConnected) {
		t.Fatalf("list while disconnected: %v", err)
	}
	// Decorators never fail while disconnected.
	items := m.baseItems(t, p.ID)
	got, err := m.SDK.Remote.DecorateVersions(ctx, p.ID, items)
	if err != nil || len(got) != 1 || got[0].Sync != nil {
		t.Fatalf("decorate while disconnected: %+v %v", got, err)
	}
}

func TestPushPullAndSyncMarkers(t *testing.T) {
	ctx := context.Background()
	ts := newTeamServer(t)
	samTok := ts.addUser(t, "sam")
	alexTok := ts.addUser(t, "alex")

	a := newMachine(t, "a")
	a.connect(t, ts, samTok)
	p := a.addProject(t, "ml-baseline", "v1")
	a.addVersion(t, p.ID, "v2", "stable")

	// Before the first push the project is not linked: no markers.
	items, _ := a.SDK.Remote.DecorateVersions(ctx, p.ID, a.baseItems(t, p.ID))
	if marker(items, 2) != "" {
		t.Fatalf("unlinked project has markers: %+v", items)
	}

	// Push to server up to version 1 first, then the rest.
	res, err := a.SDK.Remote.PushUpTo(ctx, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.PushedVersions, []int{1}) || res.ServerLatestVersion != 1 || res.ServerProjectID == "" {
		t.Fatalf("push up to 1: %+v", res)
	}
	items, _ = a.SDK.Remote.DecorateVersions(ctx, p.ID, a.baseItems(t, p.ID))
	if marker(items, 2) != "newest_here" || marker(items, 1) != "" {
		t.Fatalf("after partial push: %+v", items)
	}
	res, err = a.SDK.Remote.Push(ctx, p.ID)
	if err != nil || !slices.Equal(res.PushedVersions, []int{2}) || res.ServerLatestVersion != 2 {
		t.Fatalf("push rest: %+v %v", res, err)
	}
	res, err = a.SDK.Remote.Push(ctx, p.ID)
	if err != nil || len(res.PushedVersions) != 0 {
		t.Fatalf("second push should be a no-op: %+v %v", res, err)
	}
	items, _ = a.SDK.Remote.DecorateVersions(ctx, p.ID, a.baseItems(t, p.ID))
	if marker(items, 2) != "in_sync" {
		t.Fatalf("after push want in_sync: %+v", items)
	}
	var synced int64
	a.DB.Model(&models.ProjectVersion{}).Where("project_id = ? AND synced_at IS NOT NULL", p.ID).Count(&synced)
	if synced != 2 {
		t.Fatalf("pushed versions not marked synced: %d", synced)
	}

	// Machine B lists and pulls the project.
	b := newMachine(t, "b")
	b.connect(t, ts, alexTok)
	list, err := b.SDK.Remote.ListServerProjects(ctx, "ml", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Projects) != 1 || list.LastSyncedAt == nil || list.Projects[0].LocalProjectID != nil {
		t.Fatalf("server list: %+v", list)
	}
	serverID := list.Projects[0].ID
	if *list.Projects[0].LatestVersion != 2 {
		t.Fatalf("latest version: %+v", list.Projects[0])
	}
	conn, _ := b.SDK.Remote.GetConnection(ctx)
	if conn.LastSyncedAt == nil {
		t.Fatal("last synced not recorded")
	}

	b.addProject(t, "ML-Baseline", "unrelated")
	var cerr *shared.ConflictError
	if _, err := b.SDK.Remote.Pull(ctx, serverID, shared.PullRequest{LocalName: "ml-baseline"}); !errors.As(err, &cerr) || cerr.Code != "name_taken" ||
		cerr.Message != "A project called ml-baseline is already on this machine. Pick another name." {
		t.Fatalf("want name_taken, got %v", err)
	}
	var verr *shared.ValidationError
	if _, err := b.SDK.Remote.Pull(ctx, serverID, shared.PullRequest{LocalName: "bad name"}); !errors.As(err, &verr) || verr.Field != "local_name" {
		t.Fatalf("want local_name validation, got %v", err)
	}
	pr, err := b.SDK.Remote.Pull(ctx, serverID, shared.PullRequest{LocalName: "ml-local"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Job.Status != models.JobStatusCompleted || pr.Job.Type != models.JobTypePull || pr.Job.VersionNumber == nil || *pr.Job.VersionNumber != 2 {
		t.Fatalf("pull job: %+v", pr.Job)
	}
	var bp models.Project
	if err := b.DB.Where("id = ?", pr.ProjectID).First(&bp).Error; err != nil || bp.Status != models.ProjectStatusReady {
		t.Fatalf("pulled project: %+v %v", bp, err)
	}
	toml, err := os.ReadFile(filepath.Join(bp.Path, "pixi.toml"))
	if err != nil || string(toml) != "v2" {
		t.Fatalf("pixi.toml on disk: %q %v", toml, err)
	}
	var tags []models.ProjectTag
	b.DB.Where("project_id = ?", bp.ID).Order("tag").Find(&tags)
	if len(tags) != 2 || tags[0].Tag != "latest" || tags[0].VersionNumber != 2 || tags[1].Tag != "stable" || tags[1].VersionNumber != 2 {
		t.Fatalf("mirrored tags: %+v", tags)
	}
	var v1 models.ProjectVersion
	b.DB.Where("project_id = ? AND version_number = 1", bp.ID).First(&v1)
	if v1.AuthorName != "Sam Garcia" || v1.SyncedAt == nil {
		t.Fatalf("pulled version keeps the server author: %+v", v1)
	}
	if _, err := b.SDK.Remote.Pull(ctx, serverID, shared.PullRequest{LocalName: "again"}); !errors.As(err, &cerr) || cerr.Code != "already_pulled" {
		t.Fatalf("want already_pulled, got %v", err)
	}
	list, _ = b.SDK.Remote.ListServerProjects(ctx, "", false)
	if list.Projects[0].LocalProjectID == nil || *list.Projects[0].LocalProjectID != bp.ID {
		t.Fatalf("local project id not linked: %+v", list.Projects[0])
	}

	// A creates version 3 and pushes; B, meanwhile, creates its own version 3.
	a.addVersion(t, p.ID, "a3")
	if _, err := a.SDK.Remote.Push(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	b.addVersion(t, bp.ID, "b3")
	b.DB.Model(&models.Project{}).Where("id = ?", bp.ID).Update("installed_version", 3)

	projItems := []shared.ProjectListItem{{ID: bp.ID, Remotes: []shared.RemoteRef{}}}
	if err := b.SDK.Remote.DecorateProjects(ctx, projItems); err != nil {
		t.Fatal(err)
	}
	if !projItems[0].UpdateAvailable || projItems[0].ServerLatestVersion == nil || *projItems[0].ServerLatestVersion != 3 {
		t.Fatalf("project decorator: %+v", projItems[0])
	}
	if len(projItems[0].Remotes) != 1 || projItems[0].Remotes[0].Kind != models.RemoteKindServer || projItems[0].Remotes[0].ServerProjectID != serverID {
		t.Fatalf("server remote: %+v", projItems[0].Remotes)
	}

	// Decorating B's versions renumbers its local-only 3 to 4.
	items, err = b.SDK.Remote.DecorateVersions(ctx, bp.ID, b.baseItems(t, bp.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(numbers(items), []int{4, 3, 2, 1}) {
		t.Fatalf("after renumber: %v", numbers(items))
	}
	if marker(items, 4) != "newest_here" || marker(items, 3) != "newest_on_server" || !items[1].ServerOnly {
		t.Fatalf("markers: %+v", items)
	}
	b.DB.Where("id = ?", bp.ID).First(&bp)
	if bp.InstalledVersion == nil || *bp.InstalledVersion != 4 {
		t.Fatalf("installed version should follow the renumber: %v", bp.InstalledVersion)
	}

	// Server-only version files (compare) and detail.
	toml3, _, err := b.SDK.Remote.ServerVersionFiles(ctx, bp.ID, 3)
	if err != nil || toml3 != "a3" {
		t.Fatalf("server version files: %q %v", toml3, err)
	}
	detail, err := b.SDK.Remote.ServerVersion(ctx, bp.ID, 3)
	if err != nil || !detail.ServerOnly || detail.Number != 3 {
		t.Fatalf("server version: %+v %v", detail, err)
	}

	// Pull version 3, then push 4: both sides in sync.
	item, err := b.SDK.Remote.PullVersion(ctx, bp.ID, 3)
	if err != nil || item.Number != 3 || item.ServerOnly {
		t.Fatalf("pull version: %+v %v", item, err)
	}
	res, err = b.SDK.Remote.Push(ctx, bp.ID)
	if err != nil || !slices.Equal(res.PushedVersions, []int{4}) || res.ServerLatestVersion != 4 {
		t.Fatalf("push from b: %+v %v", res, err)
	}
	items, _ = b.SDK.Remote.DecorateVersions(ctx, bp.ID, b.baseItems(t, bp.ID))
	if !slices.Equal(numbers(items), []int{4, 3, 2, 1}) || marker(items, 4) != "in_sync" {
		t.Fatalf("b in sync: %+v", items)
	}

	// A now sees version 4 as server-only and an update.
	items, _ = a.SDK.Remote.DecorateVersions(ctx, p.ID, a.baseItems(t, p.ID))
	if marker(items, 4) != "newest_on_server" || marker(items, 3) != "" {
		t.Fatalf("a sees server ahead: %+v", items)
	}

	details, err := a.SDK.Remote.GetRemoteDetails(ctx, serverID)
	if err != nil || len(details.Versions) != 4 || details.Versions[0].Number != 4 || !details.Versions[0].ServerOnly || details.Versions[1].ServerOnly {
		t.Fatalf("remote details: %+v %v", details, err)
	}
}

func TestPullCancelledBeforeVersionsArriveIsDiscarded(t *testing.T) {
	ts := newTeamServer(t)
	tok := ts.addUser(t, "sam")
	a := newMachine(t, "a")
	a.connect(t, ts, tok)
	p := a.addProject(t, "proj", "v1")
	res, err := a.SDK.Remote.Push(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}

	b := newMachine(t, "b")
	conn := b.connect(t, ts, tok)
	proj := models.Project{Name: "proj", OwnerID: b.SDK.Deps().LocalUserID, Status: models.ProjectStatusPending, Source: "local", Path: filepath.Join(b.Dir, "proj")}
	if err := b.DB.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	b.DB.Create(&models.ProjectRemote{ProjectID: proj.ID, Kind: models.RemoteKindServer, Name: conn.Name, ServerURL: conn.URL, ServerProjectID: res.ServerProjectID})
	job := models.Job{ProjectID: proj.ID, Type: models.JobTypePullInstall, Metadata: map[string]any{
		remote.MetaServerProjectID: res.ServerProjectID, remote.MetaServerURL: conn.URL,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.SDK.Remote.RunPull(ctx, &job, io.Discard); err == nil {
		t.Fatal("cancelled pull should fail")
	}
	var count int64
	b.DB.Model(&models.Project{}).Where("name = ?", "proj").Count(&count)
	if count != 0 {
		t.Fatal("half-pulled project should be removed so the name is free")
	}
	if _, err := os.Stat(proj.Path); !os.IsNotExist(err) {
		t.Fatalf("no folder should be left behind: %v", err)
	}
}
