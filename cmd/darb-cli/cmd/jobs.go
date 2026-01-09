package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Manage background jobs",
	Long:  `List and inspect background jobs (package installations, environment operations).`,
}

func init() {
	rootCmd.AddCommand(jobsCmd)
}

// List jobs
var jobsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all jobs",
	Long:  `List all background jobs for your environments.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		jobs, httpResp, err := apiClient.JobsAPI.JobsGet(ctx).Execute()
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == 401 {
				return fmt.Errorf("not logged in. Run 'darb login' first")
			}
			return fmt.Errorf("failed to list jobs: %w", err)
		}

		if len(jobs) == 0 {
			fmt.Println("No jobs found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tSTATUS\tENVIRONMENT\tCREATED")
		for _, job := range jobs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				job.Id,
				job.Type,
				job.Status,
				job.EnvironmentId,
				job.CreatedAt,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	jobsCmd.AddCommand(jobsListCmd)
}

// Get job
var jobsGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get job details",
	Long:  `Get detailed information about a specific job, including logs.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		job, httpResp, err := apiClient.JobsAPI.JobsIdGet(ctx, args[0]).Execute()
		if err != nil {
			if httpResp != nil {
				switch httpResp.StatusCode {
				case 401:
					return fmt.Errorf("not logged in. Run 'darb login' first")
				case 404:
					return fmt.Errorf("job not found: %s", args[0])
				}
			}
			return fmt.Errorf("failed to get job: %w", err)
		}

		fmt.Printf("ID:          %s\n", job.Id)
		fmt.Printf("Type:        %s\n", job.Type)
		fmt.Printf("Status:      %s\n", job.Status)
		fmt.Printf("Environment: %s\n", job.EnvironmentId)
		fmt.Printf("Created:     %s\n", job.CreatedAt)

		if job.Logs != nil && *job.Logs != "" {
			fmt.Printf("\n--- Logs ---\n%s\n", *job.Logs)
		}

		return nil
	},
}

func init() {
	jobsCmd.AddCommand(jobsGetCmd)
}
