package cliclient

import (
	"context"
)

// ListJobs returns all jobs for the authenticated user.
func (c *Client) ListJobs(ctx context.Context) ([]Job, error) {
	var jobs []Job
	_, err := c.Get(ctx, "/jobs", &jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}
