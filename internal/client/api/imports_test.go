package api

import (
	osexec "os/exec"
	"strings"
	"testing"
)

// TestImports checks the package boundary from #680: the client API is
// built by the composition roots and must not depend on them, on the CLI
// or on Wails.
func TestImports(t *testing.T) {
	// go test puts its own GOROOT/bin first in PATH.
	goBin, err := osexec.LookPath("go")
	if err != nil {
		t.Fatalf("go command: %v", err)
	}
	out, err := osexec.Command(goBin, "list", "-deps", "github.com/nebari-dev/nebi/internal/client/api").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	forbidden := []string{
		"github.com/nebari-dev/nebi/internal/client/web",
		"github.com/nebari-dev/nebi/internal/client/desktop",
		"github.com/nebari-dev/nebi/internal/client/cli",
		"github.com/nebari-dev/nebi/internal/client/boot",
		"github.com/nebari-dev/nebi/internal/client/ui",
		"github.com/nebari-dev/nebi/cmd",
		"github.com/wailsapp/",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == strings.TrimSuffix(f, "/") || strings.HasPrefix(dep, strings.TrimSuffix(f, "/")+"/") {
				t.Errorf("internal/client/api must not import %s", dep)
			}
		}
	}
}
