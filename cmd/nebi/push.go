package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
)

var pushServer string
var pushForce bool

var pushCmd = &cobra.Command{
	Use:   "push <workspace>:<tag>",
	Short: "Push workspace spec files to a server",
	Long: `Push pixi.toml and pixi.lock from the current directory to a nebi server.

If the workspace doesn't exist on the server, it will be created automatically.

Examples:
  nebi push myworkspace:v1.0 -s work
  nebi push myworkspace:v2.0 -s https://nebi.company.com --force`,
	Args: cobra.ExactArgs(1),
	RunE: runPush,
}

func init() {
	pushCmd.Flags().StringVarP(&pushServer, "server", "s", "", "Server name or URL (required)")
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Overwrite existing tag on server")
	_ = pushCmd.MarkFlagRequired("server")
}

func runPush(cmd *cobra.Command, args []string) error {
	envName, tag := parseEnvRef(args[0])
	if tag == "" {
		return fmt.Errorf("tag is required; usage: nebi push <workspace>:<tag>")
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

	client, err := getAuthenticatedClient(pushServer)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find or create environment
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		// Environment doesn't exist — create it
		fmt.Fprintf(os.Stderr, "Creating environment %q...\n", envName)
		pixiTomlStr := string(pixiToml)
		pkgMgr := "pixi"
		newEnv, createErr := client.CreateEnvironment(ctx, cliclient.CreateEnvironmentRequest{
			Name:           envName,
			PackageManager: &pkgMgr,
			PixiToml:       &pixiTomlStr,
		})
		if createErr != nil {
			return fmt.Errorf("failed to create environment %q: %w", envName, createErr)
		}
		env = newEnv
		fmt.Fprintf(os.Stderr, "Created environment %q\n", envName)
	}

	// Push version
	req := cliclient.PushRequest{
		Tag:      tag,
		PixiToml: string(pixiToml),
		PixiLock: string(pixiLock),
		Force:    pushForce,
	}

	fmt.Fprintf(os.Stderr, "Pushing %s:%s...\n", envName, tag)
	resp, err := client.PushVersion(ctx, env.ID, req)
	if err != nil {
		return fmt.Errorf("failed to push %s:%s: %w", envName, tag, err)
	}

	fmt.Fprintf(os.Stderr, "Pushed %s:%s (version %d)\n", envName, tag, resp.VersionNumber)
	return nil
}
