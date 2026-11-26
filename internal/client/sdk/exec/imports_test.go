package exec

import (
	osexec "os/exec"
	"strings"
	"testing"
)

// TestImports checks the package boundary from #682: sdk/exec runs pixi
// and nothing else, so it imports no database package and nothing from
// the SDK or the client's API and entrypoints.
func TestImports(t *testing.T) {
	// go test puts its own GOROOT/bin first in PATH.
	goBin, err := osexec.LookPath("go")
	if err != nil {
		t.Fatalf("go command: %v", err)
	}
	out, err := osexec.Command(goBin, "list", "-deps", "github.com/nebari-dev/nebi/internal/client/sdk/exec").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	forbidden := []string{
		"database/sql",
		"gorm.io/",
		"github.com/glebarez/",
		"modernc.org/sqlite",
		"github.com/nebari-dev/nebi/internal/db",
		"github.com/nebari-dev/nebi/internal/models",
		"github.com/nebari-dev/nebi/internal/store",
		"github.com/nebari-dev/nebi/internal/client/sdk/local",
		"github.com/nebari-dev/nebi/internal/client/api",
		"github.com/nebari-dev/nebi/internal/client/cli",
		"github.com/nebari-dev/nebi/internal/client/web",
		"github.com/nebari-dev/nebi/internal/client/desktop",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == strings.TrimSuffix(f, "/") || strings.HasPrefix(dep, strings.TrimSuffix(f, "/")+"/") {
				t.Errorf("sdk/exec must not import %s", dep)
			}
		}
	}
}
