package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nebari-dev/nebi/internal/cliclient"
)

// Exercise the CLI client against the real router and database so that a
// matching rename in client mocks cannot hide a broken API or SQL relation.
func TestProjectClientContract(t *testing.T) {
	for _, mode := range []string{"local", "team"} {
		t.Run(mode, func(t *testing.T) {
			var handler http.Handler
			var token string
			if mode == "local" {
				handler = buildTestRouter(t, "")
			} else {
				handler, token = buildTeamTestRouter(t, nil)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			client := cliclient.New(server.URL, token)
			ctx := context.Background()
			manifest := "[workspace]\nname = \"pixi-workspace\"\nchannels = [\"conda-forge\"]\n"

			project, err := client.CreateProject(ctx, cliclient.CreateProjectRequest{
				Name:     "nebi-project",
				PixiToml: &manifest,
			})
			if err != nil {
				t.Fatalf("create project: %v", err)
			}
			got, err := client.GetProject(ctx, project.ID)
			if err != nil {
				t.Fatalf("get project: %v", err)
			}
			if got.Name != "nebi-project" {
				t.Fatalf("project name = %q", got.Name)
			}
			projects, err := client.ListProjects(ctx)
			if err != nil {
				t.Fatalf("list projects: %v", err)
			}
			if len(projects) != 1 || projects[0].ID != project.ID {
				t.Fatalf("created project missing from list: %+v", projects)
			}

			var jobs []struct {
				ProjectID string `json:"project_id"`
				Metadata  struct {
					PixiToml string `json:"pixi_toml"`
				} `json:"metadata"`
			}
			if _, err := client.Get(ctx, "/jobs", &jobs); err != nil {
				t.Fatalf("list jobs: %v", err)
			}
			if len(jobs) != 1 || jobs[0].ProjectID != project.ID {
				t.Fatalf("creation job missing its project_id: %+v", jobs)
			}
			if jobs[0].Metadata.PixiToml != manifest {
				t.Fatalf("Pixi workspace manifest changed: %q", jobs[0].Metadata.PixiToml)
			}
		})
	}
}
