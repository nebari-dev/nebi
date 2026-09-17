package store

import (
	"testing"
)

func TestRenameWorkspaceGuards(t *testing.T) {
	for _, tc := range []struct {
		name, path, status, job string
		fail                    bool
	}{
		{"ready", "absolute", "ready", "", false},
		{"failed", "absolute", "failed", "", false},
		{"empty-path", "", "ready", "", true},
		{"relative-path", "relative/path", "ready", "", true},
		{"creating", "absolute", "creating", "", true},
		{"pending-job", "absolute", "ready", "pending", true},
		{"running-job", "absolute", "ready", "running", true},
		{"completed-job", "absolute", "ready", "completed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			ws := &LocalWorkspace{Name: "before", Path: t.TempDir(), Status: tc.status}
			if err := s.CreateWorkspace(ws); err != nil {
				t.Fatal(err)
			}
			if tc.path != "absolute" {
				if err := s.DB().Model(ws).Update("path", tc.path).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.job != "" {
				if err := s.DB().Exec("CREATE TABLE jobs (workspace_id TEXT, status TEXT)").Error; err != nil {
					t.Fatal(err)
				}
				if err := s.DB().Exec("INSERT INTO jobs VALUES (?, ?)", ws.ID, tc.job).Error; err != nil {
					t.Fatal(err)
				}
			}
			err := s.RenameWorkspace(ws.ID, "after")
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected rename result: %v", err)
			}
			got, err := s.GetWorkspace(ws.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "after"
			if tc.fail {
				want = "before"
			}
			if got.Name != want || got.Status != tc.status {
				t.Fatalf("unexpected record: %+v", got)
			}
		})
	}
}
