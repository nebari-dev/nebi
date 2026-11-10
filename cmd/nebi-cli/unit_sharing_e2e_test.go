//go:build e2e

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func TestE2E_ServerShareCommands(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()
	ctx := context.Background()
	admin := cliclient.New(e2eEnv.serverURL, e2eEnv.token)

	project, err := admin.CreateProject(ctx, cliclient.CreateProjectRequest{Name: "share-cli-e2e"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := admin.Post(ctx, "/admin/users", map[string]any{
		"username": "sharee", "email": "sharee@example.com", "password": "sharee-password",
	}, nil); err != nil {
		t.Fatalf("create user: %v", err)
	}

	res := runCLI(t, dir, "server", "principals", "sharee", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("principals: %s %s", res.Stdout, res.Stderr)
	}
	var found []shared.Principal
	if err := json.Unmarshal([]byte(res.Stdout), &found); err != nil || len(found) != 1 || found[0].Email != "sharee@example.com" {
		t.Fatalf("principals output: %v %q", err, res.Stdout)
	}

	// Share by username; the table shows the new row.
	res = runCLI(t, dir, "server", "share", project.ID, "user", "sharee", "--role", "edit", "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Shared with sharee") || !strings.Contains(res.Stdout, "Can edit") {
		t.Fatalf("share: %d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "server", "set-role", project.ID, "user", "sharee@example.com", "--role", "view", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("set-role: %s %s", res.Stdout, res.Stderr)
	}
	var list shared.AccessList
	if err := json.Unmarshal([]byte(res.Stdout), &list); err != nil || len(list.Entries) != 2 || list.Entries[1].Role != "view" || !list.CanManage {
		t.Fatalf("set-role output: %v %q", err, res.Stdout)
	}

	res = runCLI(t, dir, "server", "access", project.ID, "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "WHO HAS ACCESS") || !strings.Contains(res.Stdout, "Can view") || !strings.Contains(res.Stdout, "Owner") {
		t.Fatalf("access: %q %q", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "server", "unshare", project.ID, "user", "sharee", "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Access removed") || strings.Contains(res.Stdout, "sharee@example.com") {
		t.Fatalf("unshare: %q %q", res.Stdout, res.Stderr)
	}

	// A principal given by ID is reported by name.
	shareeID := list.Entries[1].Principal.ID
	res = runCLI(t, dir, "server", "share", project.ID, "user", shareeID, "--role", "view", "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Shared with sharee: can view.") {
		t.Fatalf("share by ID: %d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	res = runCLI(t, dir, "server", "unshare", project.ID, "user", shareeID, "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Access removed: sharee can no longer open this project.") {
		t.Fatalf("unshare by ID: %d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}

	// A server refusal is an error without the usage text.
	ownerID := list.Entries[0].Principal.ID
	res = runCLI(t, dir, "server", "unshare", project.ID, "user", ownerID)
	if res.ExitCode == 0 || !strings.Contains(res.Stderr, "owner") || strings.Contains(res.Stdout+res.Stderr, "Usage:") {
		t.Fatalf("unshare owner: %d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Input errors are explained without a server round trip.
	if res := runCLI(t, dir, "server", "share", project.ID, "user", "sharee", "--role", "owner"); res.ExitCode == 0 {
		t.Fatal("share with --role owner must fail")
	}
	if res := runCLI(t, dir, "server", "share", project.ID, "user", "nobody-here", "--role", "view"); res.ExitCode == 0 {
		t.Fatal("share with an unknown user must fail")
	}
	if res := runCLI(t, dir, "server", "access", "not-a-project"); res.ExitCode == 0 {
		t.Fatal("access with an unknown project must fail")
	}
}
