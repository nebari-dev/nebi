package main

// Owner: U8 (jobs).

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/spf13/cobra"
)

func registerJobsCommands(root *cobra.Command) {
	var status, jobType, project, sort string
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List jobs (filter by status, type, project)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := shared.JobListFilter{Sort: sort}
			for _, s := range splitList(status) {
				filter.Statuses = append(filter.Statuses, models.JobStatus(s))
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
				return writeJSON(res)
			})
		},
	}
	list.Flags().StringVar(&status, "status", "", "Comma-separated statuses (pending,running,completed,failed,cancelled)")
	list.Flags().StringVar(&jobType, "type", "", "Comma-separated job types")
	list.Flags().StringVar(&project, "project", "", "Project name or id")
	list.Flags().StringVar(&sort, "sort", "-started_at", "started_at or -started_at")

	show := &cobra.Command{
		Use:   "show <job-id>",
		Short: "Show a job with its log",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid job id %q", args[0])
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.Jobs.Get(ctx, id)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}

	cancel := &cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Cancel a pending or running job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid job id %q", args[0])
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				res, err := s.Jobs.Cancel(ctx, id)
				if err != nil {
					return err
				}
				return writeJSON(res)
			})
		},
	}

	jobCmd.AddCommand(list, show, cancel)
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
