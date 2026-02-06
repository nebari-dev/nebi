package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var pushServer string
var pushForce bool

var pushCmd = &cobra.Command{
	Use:   "push [<workspace>:]<tag>",
	Short: "Push workspace spec files to a nebi server",
	Long: `Push pixi.toml and pixi.lock from the current directory to a nebi server.

If the workspace doesn't exist on the server, it will be created automatically.

If the workspace name is omitted (e.g. "nebi push :v2"), the name from the
last push/pull origin for the target server is used.

Examples:
  nebi push myworkspace:v1.0 -s work
  nebi push :v2.0                          # reuse workspace name from origin
  nebi push myworkspace:v2.0 --force`,
	Args: cobra.ExactArgs(1),
	RunE: runPush,
}

func init() {
	pushCmd.Flags().StringVarP(&pushServer, "server", "s", "", "Server name or URL (uses default if not set)")
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Overwrite existing tag on server")
}

func runPush(cmd *cobra.Command, args []string) error {
	wsName, tag := parseWsRef(args[0])
	if tag == "" {
		return fmt.Errorf("tag is required; usage: nebi push [<workspace>:]<tag>")
	}

	server, err := resolveServerFlag(pushServer)
	if err != nil {
		return err
	}

	// If workspace name omitted, resolve from origin
	if wsName == "" {
		origin, err := lookupOrigin(server)
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no origin set for server %q; specify a workspace name: nebi push <workspace>:<tag>", server)
		}
		wsName = origin.Name
		fmt.Fprintf(os.Stderr, "Using workspace %q from origin\n", wsName)
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

	client, err := getAuthenticatedClient(server)
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

	fmt.Fprintf(os.Stderr, "Pushing %s:%s...\n", wsName, tag)
	resp, err := client.PushVersion(ctx, ws.ID, req)
	if err != nil {
		return fmt.Errorf("failed to push %s:%s: %w", wsName, tag, err)
	}

	fmt.Fprintf(os.Stderr, "Pushed %s:%s (version %d)\n", wsName, tag, resp.VersionNumber)

	// Auto-track the workspace so status and origin tracking work
	if err := ensureInit("."); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
	}

	// Save origin
	if saveErr := saveOrigin(server, wsName, tag, "push", string(pixiToml), string(pixiLock)); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save origin: %v\n", saveErr)
	}

	return nil
}
