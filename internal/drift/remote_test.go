package drift

import (
	"testing"

	"github.com/aktech/darb/internal/nebifile"
)

func TestRemoteStatus_TagHasMoved(t *testing.T) {
	rs := &RemoteStatus{
		OriginDigest:     "sha256:aaa",
		CurrentTagDigest: "sha256:bbb",
		TagHasMoved:      true,
	}

	if !rs.TagHasMoved {
		t.Error("TagHasMoved should be true")
	}
}

func TestRemoteStatus_TagUnchanged(t *testing.T) {
	rs := &RemoteStatus{
		OriginDigest:     "sha256:aaa",
		CurrentTagDigest: "sha256:aaa",
		TagHasMoved:      false,
	}

	if rs.TagHasMoved {
		t.Error("TagHasMoved should be false when digests match")
	}
}

func TestGenerateSummary_CleanLocalCleanRemote(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{Overall: StatusClean},
		Remote: &RemoteStatus{
			TagHasMoved: false,
		},
	}

	summary := generateSummary(ts)
	if summary != "clean (local matches origin, tag unchanged)" {
		t.Errorf("summary = %q, unexpected", summary)
	}
}

func TestGenerateSummary_ModifiedLocalCleanRemote(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{Overall: StatusModified},
		Remote: &RemoteStatus{
			TagHasMoved: false,
		},
	}

	summary := generateSummary(ts)
	if summary != "modified locally, remote unchanged" {
		t.Errorf("summary = %q, unexpected", summary)
	}
}

func TestGenerateSummary_CleanLocalMovedRemote(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{Overall: StatusClean},
		Remote: &RemoteStatus{
			TagHasMoved: true,
		},
	}

	summary := generateSummary(ts)
	if summary != "clean locally, but remote tag has moved" {
		t.Errorf("summary = %q, unexpected", summary)
	}
}

func TestGenerateSummary_ModifiedLocalMovedRemote(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{Overall: StatusModified},
		Remote: &RemoteStatus{
			TagHasMoved: true,
		},
	}

	summary := generateSummary(ts)
	if summary != "modified locally AND remote tag has moved" {
		t.Errorf("summary = %q, unexpected", summary)
	}
}

func TestGenerateSummary_RemoteError(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{Overall: StatusClean},
		Remote: &RemoteStatus{
			Error: "network error",
		},
	}

	summary := generateSummary(ts)
	if summary != "clean locally (remote check failed)" {
		t.Errorf("summary = %q, unexpected", summary)
	}
}

func TestGenerateSummary_RemoteErrorWithLocalModified(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{Overall: StatusModified},
		Remote: &RemoteStatus{
			Error: "network error",
		},
	}

	summary := generateSummary(ts)
	if summary != "modified locally (remote check failed)" {
		t.Errorf("summary = %q, unexpected", summary)
	}
}

func TestThreeWayStatus_Structure(t *testing.T) {
	ts := &ThreeWayStatus{
		Local: &RepoStatus{
			Overall: StatusModified,
			Files: []FileStatus{
				{Filename: "pixi.toml", Status: StatusModified},
				{Filename: "pixi.lock", Status: StatusClean},
			},
		},
		Remote: &RemoteStatus{
			OriginDigest:     "sha256:aaa",
			CurrentTagDigest: "sha256:bbb",
			TagHasMoved:      true,
			CurrentVersionID: 5,
		},
		Summary: "modified locally AND remote tag has moved",
	}

	if ts.Local.Overall != StatusModified {
		t.Errorf("Local.Overall = %q, want %q", ts.Local.Overall, StatusModified)
	}
	if !ts.Remote.TagHasMoved {
		t.Error("Remote.TagHasMoved should be true")
	}
	if ts.Remote.CurrentVersionID != 5 {
		t.Errorf("Remote.CurrentVersionID = %d, want 5", ts.Remote.CurrentVersionID)
	}
}

func TestVersionContent_Structure(t *testing.T) {
	vc := &VersionContent{
		PixiToml:  "[workspace]\nname = \"test\"\n",
		PixiLock:  "version: 1\n",
		VersionID: 42,
	}

	if vc.PixiToml == "" {
		t.Error("PixiToml should not be empty")
	}
	if vc.PixiLock == "" {
		t.Error("PixiLock should not be empty")
	}
	if vc.VersionID != 42 {
		t.Errorf("VersionID = %d, want 42", vc.VersionID)
	}
}

func TestCheckWithNebiFile_EmptyLayers(t *testing.T) {
	nf := &nebifile.NebiFile{
		Layers: map[string]nebifile.Layer{},
	}

	ws := CheckWithNebiFile("/tmp", nf)
	if ws.Overall != StatusClean {
		t.Errorf("Overall = %q, want %q (empty layers should be clean)", ws.Overall, StatusClean)
	}
	if len(ws.Files) != 0 {
		t.Errorf("Files length = %d, want 0", len(ws.Files))
	}
}
