package worker

import (
	"reflect"
	"testing"

	"github.com/nebari-dev/nebi/internal/executor"
)

func TestBuildCreateWorkspaceOptions(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     executor.CreateWorkspaceOptions
	}{
		{
			name:     "empty metadata",
			metadata: map[string]interface{}{},
			want:     executor.CreateWorkspaceOptions{},
		},
		{
			name: "pixi_toml only",
			metadata: map[string]interface{}{
				"pixi_toml": "[project]\nname = \"x\"\n",
			},
			want: executor.CreateWorkspaceOptions{
				PixiToml: "[project]\nname = \"x\"\n",
			},
		},
		{
			name: "import_staging_dir only",
			metadata: map[string]interface{}{
				"import_staging_dir": "/tmp/staging-abc",
			},
			want: executor.CreateWorkspaceOptions{
				SeedDir: "/tmp/staging-abc",
			},
		},
		{
			name: "both pixi_toml and import_staging_dir",
			metadata: map[string]interface{}{
				"pixi_toml":          "[project]\nname = \"x\"\n",
				"import_staging_dir": "/tmp/staging-abc",
			},
			want: executor.CreateWorkspaceOptions{
				PixiToml: "[project]\nname = \"x\"\n",
				SeedDir:  "/tmp/staging-abc",
			},
		},
		{
			name: "ignores non-string values",
			metadata: map[string]interface{}{
				"pixi_toml":          123,
				"import_staging_dir": true,
			},
			want: executor.CreateWorkspaceOptions{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCreateWorkspaceOptions(tc.metadata)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildCreateWorkspaceOptions() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
