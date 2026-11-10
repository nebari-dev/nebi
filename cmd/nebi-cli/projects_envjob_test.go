//go:build e2e

package main

import (
	"testing"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

func TestEnvJobStatusLine(t *testing.T) {
	two := 2
	cases := []struct {
		job  shared.JobSummary
		want string
	}{
		{
			shared.JobSummary{TypeLabel: "Install version", Status: models.JobStatusCompleted, VersionNumber: &two},
			"Install version (my-data, version 2): Succeeded",
		},
		{
			shared.JobSummary{TypeLabel: "Uninstall", Status: models.JobStatusFailed},
			"Uninstall (my-data): Failed",
		},
	}
	for _, c := range cases {
		if got := envJobStatusLine(&c.job, "my-data"); got != c.want {
			t.Errorf("envJobStatusLine = %q, want %q", got, c.want)
		}
	}
}
