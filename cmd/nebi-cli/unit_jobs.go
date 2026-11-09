package main

// Owner: U8 (jobs). "nebi job" (alias "nebi jobs"): list, show, cancel and
// follow the background jobs on this machine, the CLI side of the Jobs
// page. Jobs started by the desktop app or nebi-web are visible here and
// can be cancelled from here (the app watches the job row).

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/spf13/cobra"
)

// jobStatusLabels match the Jobs page (JOB_STATUS_LABELS in the frontend).
var jobStatusLabels = map[models.JobStatus]string{
	models.JobStatusPending:   "Queued",
	models.JobStatusRunning:   "Running",
	models.JobStatusCompleted: "Succeeded",
	models.JobStatusFailed:    "Failed",
	models.JobStatusCancelled: "Cancelled",
}

// parseJobStatus accepts stored statuses and the UI labels.
func parseJobStatus(v string) models.JobStatus {
	switch strings.ToLower(v) {
	case "queued":
		return models.JobStatusPending
	case "succeeded", "success":
		return models.JobStatusCompleted
	case "canceled":
		return models.JobStatusCancelled
	}
	return models.JobStatus(strings.ToLower(v))
}

func registerJobsCommands(root *cobra.Command) {
	jobCmd.Aliases = append(jobCmd.Aliases, "jobs")

	var status, jobType, project, sort string
	var limit int
	var listJSON bool
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List jobs (filter by status, type, project)",
		Example: `  nebi job list
  nebi job list --status running,queued
  nebi job list --type install_version --project my-project --sort started_at`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := shared.JobListFilter{Sort: sort, Limit: limit}
			for _, s := range splitList(status) {
				filter.Statuses = append(filter.Statuses, parseJobStatus(s))
			}
			for _, t := range splitList(jobType) {
				filter.Types = append(filter.Types, models.JobType(t))
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				if project != "" {
					id, err := resolveProjectID(ctx, s, project)
					if err != nil {
						return err
					}
					filter.ProjectID = &id
				}
				res, err := s.Jobs.List(ctx, filter)
				if err != nil {
					return err
				}
				if listJSON {
					return writeJSON(res)
				}
				printJobTable(cmd.OutOrStdout(), res, time.Now())
				return nil
			})
		},
	}
	list.Flags().StringVar(&status, "status", "", "Comma-separated statuses: queued, running, succeeded, failed, cancelled")
	list.Flags().StringVar(&jobType, "type", "", "Comma-separated job types (e.g. install_version,create_version)")
	list.Flags().StringVar(&project, "project", "", "Project name or id")
	list.Flags().StringVar(&sort, "sort", "-started_at", "started_at (oldest first) or -started_at (newest first)")
	list.Flags().IntVar(&limit, "limit", 0, "Show at most this many jobs (0 = all)")
	list.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	var showJSON bool
	show := &cobra.Command{
		Use:   "show <job-id>",
		Short: "Show a job with its log",
		Long:  "Show a job's status, timing and log. The id can be the full id or its first characters, as shown by 'nebi job list'.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := s.Jobs.ResolveJobID(ctx, args[0])
				if err != nil {
					return err
				}
				res, err := s.Jobs.Get(ctx, id)
				if err != nil {
					return err
				}
				if showJSON {
					return writeJSON(res)
				}
				printJobDetail(cmd.OutOrStdout(), res, time.Now())
				return nil
			})
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, "Output as JSON")

	var cancelJSON bool
	cancel := &cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Cancel a queued or running job",
		Long: `Cancel a queued or running job. A running job's pixi process is stopped
and the job ends as Cancelled. Jobs started by the desktop app or nebi-web
are stopped by the app within a few seconds.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := s.Jobs.ResolveJobID(ctx, args[0])
				if err != nil {
					return err
				}
				res, err := s.Jobs.Cancel(ctx, id)
				if err != nil {
					return err
				}
				if cancelJSON {
					return writeJSON(res)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s on %s: %s\n", res.TypeLabel, projectLabel(*res), jobStatusLabel(res.Status))
				return nil
			})
		},
	}
	cancel.Flags().BoolVar(&cancelJSON, "json", false, "Output as JSON")

	var follow bool
	logs := &cobra.Command{
		Use:   "logs <job-id>",
		Short: "Print a job's log (--follow streams it until the job ends)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				id, err := s.Jobs.ResolveJobID(ctx, args[0])
				if err != nil {
					return err
				}
				if !follow {
					res, err := s.Jobs.Get(ctx, id)
					if err != nil {
						return err
					}
					_, err = io.WriteString(out, res.Logs)
					return err
				}
				final, err := s.Jobs.FollowLogs(ctx, id, func(chunk string) error {
					_, err := io.WriteString(out, chunk)
					return err
				})
				if err != nil {
					if ctx.Err() != nil {
						return nil // Ctrl-C stops following, not the job.
					}
					return err
				}
				fmt.Fprintf(os.Stderr, "\nJob %s\n", strings.ToLower(jobStatusLabel(final.Status)))
				return nil
			})
		},
	}
	logs.Flags().BoolVarP(&follow, "follow", "f", false, "Stream the log until the job ends")

	jobCmd.AddCommand(list, show, cancel, logs)
}

func jobStatusLabel(st models.JobStatus) string {
	if l, ok := jobStatusLabels[st]; ok {
		return l
	}
	return string(st)
}

func projectLabel(j shared.JobSummary) string {
	if j.ProjectName != "" {
		return j.ProjectName
	}
	return j.ProjectID.String()
}

func shortJobID(j shared.JobSummary) string {
	return j.ID.String()[:8]
}

func printJobTable(w io.Writer, jobs []shared.JobSummary, now time.Time) {
	if len(jobs) == 0 {
		fmt.Fprintln(w, "No jobs match these filters.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tJOB\tPROJECT\tSTATUS\tSTARTED\tDURATION")
	for _, j := range jobs {
		started := j.CreatedAt
		if j.StartedAt != nil {
			started = *j.StartedAt
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			shortJobID(j), jobTitle(j), projectLabel(j), jobStatusLabel(j.Status),
			started.Local().Format("2006-01-02 15:04"), jobDuration(j, now))
	}
	_ = tw.Flush()
}

// jobTitle is the type label plus the version it acted on, if any.
func jobTitle(j shared.JobSummary) string {
	if j.VersionNumber != nil {
		return fmt.Sprintf("%s (v%d)", j.TypeLabel, *j.VersionNumber)
	}
	return j.TypeLabel
}

func printJobDetail(w io.Writer, j *shared.JobDetail, now time.Time) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Job:\t%s\n", jobTitle(j.JobSummary))
	fmt.Fprintf(tw, "ID:\t%s\n", j.ID)
	fmt.Fprintf(tw, "Status:\t%s\n", jobStatusLabel(j.Status))
	fmt.Fprintf(tw, "Project:\t%s\n", projectLabel(j.JobSummary))
	fmt.Fprintf(tw, "Started:\t%s\n", fmtJobTime(j.StartedAt))
	fmt.Fprintf(tw, "Ended:\t%s\n", fmtJobTime(j.CompletedAt))
	fmt.Fprintf(tw, "Duration:\t%s\n", jobDuration(j.JobSummary, now))
	if j.Error != "" && j.Status == models.JobStatusFailed {
		fmt.Fprintf(tw, "Error:\t%s\n", j.Error)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	if j.Logs == "" {
		fmt.Fprintln(w, "Waiting to start. The log appears once the job runs.")
		return
	}
	fmt.Fprint(w, j.Logs)
	if !strings.HasSuffix(j.Logs, "\n") {
		fmt.Fprintln(w)
	}
}

func fmtJobTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// jobDuration formats like the Jobs page: 42s, 2m 05s, 1h 03m. Running
// jobs count up to now; jobs that never started show "-".
func jobDuration(j shared.JobSummary, now time.Time) string {
	if j.StartedAt == nil {
		return "-"
	}
	end := now
	if j.CompletedAt != nil {
		end = *j.CompletedAt
	}
	return formatJobDuration(end.Sub(*j.StartedAt))
}

func formatJobDuration(d time.Duration) string {
	secs := int(d.Round(time.Second) / time.Second)
	if secs < 0 {
		secs = 0
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm %02ds", mins, secs%60)
	}
	return fmt.Sprintf("%dh %02dm", mins/60, mins%60)
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
