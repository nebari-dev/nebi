package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
)

var environmentsCmd = &cobra.Command{
	Use:     "env",
	Aliases: []string{"envs"},
	Short:   "Manage environments",
	Long:    `Create, list, and manage Pixi environments.`,
}

func init() {
	rootCmd.AddCommand(environmentsCmd)
}

// List environments
var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all environments",
	Long:  `List all environments you have access to.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		envs, httpResp, err := apiClient.EnvironmentsAPI.EnvironmentsGet(ctx).Execute()
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == 401 {
				return fmt.Errorf("not logged in. Run 'darb login' first")
			}
			return fmt.Errorf("failed to list environments: %w", err)
		}

		if len(envs) == 0 {
			fmt.Println("No environments found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tPACKAGE MANAGER\tCREATED")
		for _, env := range envs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				env.Id,
				env.Name,
				env.Status,
				env.PackageManager,
				env.CreatedAt,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	environmentsCmd.AddCommand(envListCmd)
}

// Create environment
var (
	createEnvName           string
	createEnvPackageManager string
	createEnvPixiToml       string
)

var envCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new environment",
	Long: `Create a new Pixi environment.

Examples:
  # Create a basic environment
  darb env create --name myenv

  # Create with a pixi.toml file
  darb env create --name myenv --pixi-toml ./pixi.toml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		req := client.HandlersCreateEnvironmentRequest{
			Name: createEnvName,
		}

		if createEnvPackageManager != "" {
			req.PackageManager = &createEnvPackageManager
		}

		if createEnvPixiToml != "" {
			content, err := os.ReadFile(createEnvPixiToml)
			if err != nil {
				return fmt.Errorf("failed to read pixi.toml: %w", err)
			}
			pixiToml := string(content)
			req.PixiToml = &pixiToml
		}

		env, httpResp, err := apiClient.EnvironmentsAPI.EnvironmentsPost(ctx).
			Environment(req).
			Execute()
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == 401 {
				return fmt.Errorf("not logged in. Run 'darb login' first")
			}
			return fmt.Errorf("failed to create environment: %w", err)
		}

		fmt.Printf("Environment created successfully!\n")
		fmt.Printf("  ID:   %s\n", env.Id)
		fmt.Printf("  Name: %s\n", env.Name)

		return nil
	},
}

func init() {
	environmentsCmd.AddCommand(envCreateCmd)

	envCreateCmd.Flags().StringVar(&createEnvName, "name", "", "Environment name (required)")
	envCreateCmd.Flags().StringVar(&createEnvPackageManager, "package-manager", "pixi", "Package manager (pixi or uv)")
	envCreateCmd.Flags().StringVar(&createEnvPixiToml, "pixi-toml", "", "Path to pixi.toml file")

	envCreateCmd.MarkFlagRequired("name")
}

// Get environment
var envGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get environment details",
	Long:  `Get detailed information about a specific environment.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		env, httpResp, err := apiClient.EnvironmentsAPI.EnvironmentsIdGet(ctx, args[0]).Execute()
		if err != nil {
			if httpResp != nil {
				switch httpResp.StatusCode {
				case 401:
					return fmt.Errorf("not logged in. Run 'darb login' first")
				case 404:
					return fmt.Errorf("environment not found: %s", args[0])
				}
			}
			return fmt.Errorf("failed to get environment: %w", err)
		}

		fmt.Printf("ID:              %s\n", env.Id)
		fmt.Printf("Name:            %s\n", env.Name)
		fmt.Printf("Status:          %s\n", env.Status)
		fmt.Printf("Package Manager: %s\n", env.PackageManager)
		fmt.Printf("Created:         %s\n", env.CreatedAt)

		return nil
	},
}

func init() {
	environmentsCmd.AddCommand(envGetCmd)
}

// Delete environment
var envDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete an environment",
	Long:  `Delete a specific environment.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		httpResp, err := apiClient.EnvironmentsAPI.EnvironmentsIdDelete(ctx, args[0]).Execute()
		if err != nil {
			if httpResp != nil {
				switch httpResp.StatusCode {
				case 401:
					return fmt.Errorf("not logged in. Run 'darb login' first")
				case 404:
					return fmt.Errorf("environment not found: %s", args[0])
				}
			}
			return fmt.Errorf("failed to delete environment: %w", err)
		}

		fmt.Printf("Environment %s deleted successfully\n", args[0])
		return nil
	},
}

func init() {
	environmentsCmd.AddCommand(envDeleteCmd)
}

// Publish environment
var (
	publishRegistryID string
	publishRepository string
	publishTag        string
)

var envPublishCmd = &cobra.Command{
	Use:   "publish [id]",
	Short: "Publish environment to OCI registry",
	Long: `Publish an environment's pixi.toml and pixi.lock to an OCI registry.

Examples:
  darb env publish <env-id> --registry <registry-id> --repository myorg/myenv --tag v1.0.0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		req := client.HandlersPublishRequest{
			RegistryId: publishRegistryID,
			Repository: publishRepository,
			Tag:        publishTag,
		}

		pub, httpResp, err := apiClient.EnvironmentsAPI.EnvironmentsIdPublishPost(ctx, args[0]).
			Request(req).
			Execute()
		if err != nil {
			if httpResp != nil {
				switch httpResp.StatusCode {
				case 401:
					return fmt.Errorf("not logged in. Run 'darb login' first")
				case 404:
					return fmt.Errorf("environment not found: %s", args[0])
				}
			}
			return fmt.Errorf("failed to publish environment: %w", err)
		}

		fmt.Printf("Environment published successfully!\n")
		fmt.Printf("  Registry:   %s\n", pub.RegistryName)
		fmt.Printf("  Repository: %s\n", pub.Repository)
		fmt.Printf("  Tag:        %s\n", pub.Tag)
		fmt.Printf("  Digest:     %s\n", pub.Digest)

		return nil
	},
}

func init() {
	environmentsCmd.AddCommand(envPublishCmd)

	envPublishCmd.Flags().StringVar(&publishRegistryID, "registry", "", "Registry ID (required)")
	envPublishCmd.Flags().StringVar(&publishRepository, "repository", "", "Repository path (required)")
	envPublishCmd.Flags().StringVar(&publishTag, "tag", "latest", "Tag for the publication")

	envPublishCmd.MarkFlagRequired("registry")
	envPublishCmd.MarkFlagRequired("repository")
}

// List packages in environment
var envPackagesCmd = &cobra.Command{
	Use:   "packages [id]",
	Short: "List packages in an environment",
	Long:  `List all packages installed in a specific environment.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		packages, httpResp, err := apiClient.EnvironmentsAPI.EnvironmentsIdPackagesGet(ctx, args[0]).Execute()
		if err != nil {
			if httpResp != nil {
				switch httpResp.StatusCode {
				case 401:
					return fmt.Errorf("not logged in. Run 'darb login' first")
				case 404:
					return fmt.Errorf("environment not found: %s", args[0])
				}
			}
			return fmt.Errorf("failed to list packages: %w", err)
		}

		if len(packages) == 0 {
			fmt.Println("No packages installed.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION")
		for _, pkg := range packages {
			version := ""
			if pkg.Version != nil {
				version = *pkg.Version
			}
			fmt.Fprintf(w, "%s\t%s\n", pkg.Name, version)
		}
		w.Flush()

		return nil
	},
}

func init() {
	environmentsCmd.AddCommand(envPackagesCmd)
}
