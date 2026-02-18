package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var pushForce bool

var pushCmd = &cobra.Command{
	Use:   "push [<workspace>][:<tag>]",
	Short: "Push workspace spec files to a nebi server",
	Long: `Push pixi.toml and pixi.lock from the current directory to a nebi server.

If the workspace doesn't exist on the server, it will be created automatically.

Every push automatically creates a content-addressed tag (sha-<hash>) and
updates the "latest" tag. If a user tag is specified, it is added as well.

If no tag is specified, only the content hash and "latest" tags are created.
If the content hasn't changed since the last push, the version is deduplicated.

If the workspace name is omitted, the name from the last push/pull origin is used.

Examples:
  nebi push myworkspace                    # auto-tag with content hash + latest
  nebi push myworkspace:v1.0               # also add user tag v1.0
  nebi push                                # reuse workspace name from origin
  nebi push :v2.0                          # reuse workspace name, add tag v2.0
  nebi push myworkspace:v2.0 --force       # overwrite existing user tag`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPush,
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Overwrite existing tag on server")
}

func runPush(cmd *cobra.Command, args []string) error {
	var wsName, tag string
	if len(args) == 1 {
		wsName, tag = parseWsRef(args[0])
	}

	// If workspace name omitted, resolve from origin
	if wsName == "" {
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no origin set; specify a workspace name: nebi push <workspace>[:<tag>]")
		}
		wsName = origin.OriginName
		fmt.Fprintf(os.Stderr, "Using workspace %q from origin\n", wsName)
	}

	if err := validateWorkspaceName(wsName); err != nil {
		return fmt.Errorf("invalid workspace name: %w", err)
	}

	// Read local spec files
	pixiToml, err := os.ReadFile("pixi.toml")
	if err != nil {
		return fmt.Errorf("pixi.toml not found in current directory; run 'pixi init' first")
	}

	pixiLock, _ := os.ReadFile("pixi.lock")
	if len(pixiLock) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: pixi.lock not found. Run 'pixi install' to generate it.")
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find or create workspace
	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		// Workspace doesn't exist — create it
		fmt.Fprintf(os.Stderr, "Creating workspace %q...\n", wsName)
		pixiTomlStr := string(pixiToml)
		pkgMgr := "pixi"
		newWs, createErr := client.CreateWorkspace(ctx, cliclient.CreateWorkspaceRequest{
			Name:           wsName,
			PackageManager: &pkgMgr,
			PixiToml:       &pixiTomlStr,
		})
		if createErr != nil {
			return fmt.Errorf("failed to create workspace %q: %w", wsName, createErr)
		}
		// Wait for workspace to be ready (server runs pixi install)
		ws, err = waitForWsReady(client, ctx, newWs.ID, 60*time.Second)
		if err != nil {
			return fmt.Errorf("workspace %q failed to become ready: %w", wsName, err)
		}
		fmt.Fprintf(os.Stderr, "Created workspace %q\n", wsName)
	}

	// Push version
	req := cliclient.PushRequest{
		Tag:      tag,
		PixiToml: string(pixiToml),
		PixiLock: string(pixiLock),
		Force:    pushForce,
	}

	pushLabel := wsName
	if tag != "" {
		pushLabel = fmt.Sprintf("%s:%s", wsName, tag)
	}
	fmt.Fprintf(os.Stderr, "Pushing %s...\n", pushLabel)
	resp, err := client.PushVersion(ctx, ws.ID, req)
	if err != nil {
		return fmt.Errorf("failed to push %s: %w", pushLabel, err)
	}

	if resp.Deduplicated {
		fmt.Fprintf(os.Stderr, "Content unchanged — %s (version %d, tags: %s)\n",
			wsName, resp.VersionNumber, strings.Join(resp.Tags, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "Pushed %s (version %d, tags: %s)\n",
			wsName, resp.VersionNumber, strings.Join(resp.Tags, ", "))
	}

	// Auto-track the workspace so status and origin tracking work
	if err := ensureInit("."); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
	}

	// Save origin — use content hash as tag if no user tag was specified
	originTag := tag
	if originTag == "" {
		originTag = resp.ContentHash
	}
	if saveErr := saveOrigin(wsName, originTag, "push", string(pixiToml), string(pixiLock)); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save origin: %v\n", saveErr)
	}

	return nil
}
