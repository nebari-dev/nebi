package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// conformanceTOML/v2 are two distinct, server-parseable manifests so a
// re-push of the same ref carries different content (same content would
// exercise dedup, not conflict, on the server).
const (
	conformanceTOML  = "[workspace]\nname = \"conf-demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"osx-arm64\"]\n\n[dependencies]\npython = \"3.12.*\"\n"
	conformanceTOML2 = "[workspace]\nname = \"conf-demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"osx-arm64\"]\n\n[dependencies]\npython = \"3.13.*\"\n"
	conformanceLock  = "version: 6\n"
	conformanceLock2 = "version: 6\n# v2\n"
)

// runRemoteConformance drives one Remote implementation through the
// full client lifecycle from the issue: discover auth schemes,
// authenticate (bad then good), push a new workspace, list it, pull it
// back (latest and by ref), hit a push conflict, push a second version,
// and check typed not-found errors. This single script running
// unmodified against both OCIRemote and ServerRemote is the spike's
// "are they peers?" demonstration.
func runRemoteConformance(t *testing.T, r Remote, good, bad AuthenticationCredential, missingID string) {
	t.Helper()
	ctx := context.Background()

	schemes := r.SupportedAuthentication()
	if len(schemes) > 0 {
		if _, err := r.List(ctx); !errors.Is(err, ErrNotAuthenticated) {
			t.Fatalf("List before auth: want ErrNotAuthenticated, got %v", err)
		}
		if err := r.Authenticate(ctx, bad); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("Authenticate with bad creds: want ErrUnauthorized, got %v", err)
		}
		if err := r.Authenticate(ctx, good); err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
	}

	// Push a brand-new workspace at ref v1.
	err := r.Push(ctx, Workspace{
		WorkspacePreview: WorkspacePreview{Name: "conf-demo"},
		Ref:              "v1",
		PixiToml:         conformanceTOML,
		PixiLock:         conformanceLock,
	})
	if err != nil {
		t.Fatalf("Push new workspace: %v", err)
	}

	// It shows up in the listing with the pushed ref.
	previews, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var preview *WorkspacePreview
	for i := range previews {
		if previews[i].Name == "conf-demo" {
			preview = &previews[i]
		}
	}
	if preview == nil {
		t.Fatalf("List: workspace conf-demo missing from %+v", previews)
	}
	if !containsRef(preview.Refs, "v1") {
		t.Fatalf("List: ref v1 missing from %v", preview.Refs)
	}

	// Pull latest and pull by ref return the pushed content.
	for _, id := range []string{preview.ID, preview.ID + "@v1"} {
		ws, err := r.Pull(ctx, id)
		if err != nil {
			t.Fatalf("Pull(%s): %v", id, err)
		}
		if strings.TrimSpace(ws.PixiToml) != strings.TrimSpace(conformanceTOML) {
			t.Fatalf("Pull(%s): pixi.toml mismatch:\n%s", id, ws.PixiToml)
		}
		if strings.TrimSpace(ws.PixiLock) != strings.TrimSpace(conformanceLock) {
			t.Fatalf("Pull(%s): pixi.lock mismatch:\n%s", id, ws.PixiLock)
		}
	}

	// Re-pushing an existing ref with different content is a conflict.
	err = r.Push(ctx, Workspace{
		WorkspacePreview: *preview,
		Ref:              "v1",
		PixiToml:         conformanceTOML2,
		PixiLock:         conformanceLock2,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Push existing ref: want ErrConflict, got %v", err)
	}

	// A new ref goes through and becomes the new latest.
	err = r.Push(ctx, Workspace{
		WorkspacePreview: *preview,
		Ref:              "v2",
		PixiToml:         conformanceTOML2,
		PixiLock:         conformanceLock2,
	})
	if err != nil {
		t.Fatalf("Push v2: %v", err)
	}
	ws, err := r.Pull(ctx, preview.ID)
	if err != nil {
		t.Fatalf("Pull latest after v2: %v", err)
	}
	if strings.TrimSpace(ws.PixiToml) != strings.TrimSpace(conformanceTOML2) {
		t.Fatalf("Pull latest after v2: got v1 content:\n%s", ws.PixiToml)
	}

	// Typed not-found errors: unknown workspace, unknown ref. The
	// server answers 403 for an unknown workspace ID (RBAC denies
	// before checking existence, so forbidden and not-found are
	// indistinguishable to a client — a spike finding), hence both
	// sentinels are accepted here.
	if _, err := r.Pull(ctx, missingID); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrForbidden) {
		t.Fatalf("Pull(%s): want ErrNotFound or ErrForbidden, got %v", missingID, err)
	}
	if _, err := r.Pull(ctx, preview.ID+"@no-such-ref"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Pull(@no-such-ref): want ErrNotFound, got %v", err)
	}
}

func containsRef(refs []string, want string) bool {
	for _, r := range refs {
		if r == want {
			return true
		}
	}
	return false
}

// nonexistentUUID is a syntactically valid workspace id that no server
// will have; OCI remotes treat it as just another missing repo name.
var nonexistentUUID = fmt.Sprintf("%08d-0000-0000-0000-%012d", 0, 0)
