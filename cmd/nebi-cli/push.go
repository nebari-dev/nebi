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

var (
	pushForce bool
	pushJSON  bool
)

var pushCmd = &cobra.Command{
	Use:   "push [<project>][:<tag>]",
	Short: "Push project spec files to a nebi server",
	Long: `Push pixi.toml and pixi.lock from the current directory to a nebi server.

If the project doesn't exist on the server, it will be created automatically.

Every push automatically creates a content-addressed tag (sha-<hash>) and
updates the "latest" tag. If a user tag is specified, it is added as well.

If no tag is specified, only the content hash and "latest" tags are created.
If the content hasn't changed since the last push, the version is deduplicated.

If the project name is omitted, the name from the last push/pull origin is used.

Examples:
  nebi push myproject                    # auto-tag with content hash + latest
  nebi push myproject:v1.0               # also add user tag v1.0
  nebi push                                # reuse project name from origin
  nebi push :v2.0                          # reuse project name, add tag v2.0
  nebi push myproject:v2.0 --force       # overwrite existing user tag`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPush,
	// No completion - project name is user-provided, not selected from existing
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Overwrite existing tag on server")
	pushCmd.Flags().BoolVar(&pushJSON, "json", false, "Output as JSON")
}

func runPush(cmd *cobra.Command, args []string) error {
	var projectName, tag string
	if len(args) == 1 {
		projectName, tag = parseProjectRef(args[0])
	}

	// If project name omitted, resolve from origin
	if projectName == "" {
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no origin set; specify a project name: nebi push <project>[:<tag>]")
		}
		projectName = origin.OriginName
		fmt.Fprintf(os.Stderr, "Using project %q from origin\n", projectName)
	}

	if err := validateProjectName(projectName); err != nil {
		return fmt.Errorf("invalid project name: %w", err)
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

	// Find or create project
	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		// Project doesn't exist — create it
		fmt.Fprintf(os.Stderr, "Creating project %q...\n", projectName)
		pixiTomlStr := string(pixiToml)
		newProject, createErr := client.CreateProject(ctx, cliclient.CreateProjectRequest{
			Name:     projectName,
			PixiToml: &pixiTomlStr,
		})
		if createErr != nil {
			return fmt.Errorf("failed to create project %q: %w", projectName, createErr)
		}
		// Wait for project to be ready (server runs pixi install)
		project, err = waitForProjectReady(client, ctx, newProject.ID, 60*time.Second)
		if err != nil {
			return fmt.Errorf("project %q failed to become ready: %w", projectName, err)
		}
		fmt.Fprintf(os.Stderr, "Created project %q\n", projectName)
	}

	// Push version
	req := cliclient.PushRequest{
		Tag:      tag,
		PixiToml: string(pixiToml),
		PixiLock: string(pixiLock),
		Force:    pushForce,
	}

	pushLabel := projectName
	if tag != "" {
		pushLabel = fmt.Sprintf("%s:%s", projectName, tag)
	}
	fmt.Fprintf(os.Stderr, "Pushing %s...\n", pushLabel)
	resp, err := client.PushVersion(ctx, project.ID, req)
	if err != nil {
		return fmt.Errorf("failed to push %s: %w", pushLabel, err)
	}

	if pushJSON {
		if err := writeJSON(resp); err != nil {
			return err
		}
	} else if resp.Deduplicated {
		fmt.Fprintf(os.Stderr, "Content unchanged — %s (version %d, tags: %s)\n",
			projectName, resp.VersionNumber, strings.Join(resp.Tags, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "Pushed %s (version %d, tags: %s)\n",
			projectName, resp.VersionNumber, strings.Join(resp.Tags, ", "))
	}

	// Auto-track the project so status and origin tracking work
	if err := ensureInit("."); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track project: %v\n", err)
	}

	// Save origin — use content hash as tag if no user tag was specified
	originTag := tag
	if originTag == "" {
		originTag = resp.ContentHash
	}
	if saveErr := saveOrigin(project.ID, projectName, originTag, "push", string(pixiToml), string(pixiLock)); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save origin: %v\n", saveErr)
	}

	return nil
}
