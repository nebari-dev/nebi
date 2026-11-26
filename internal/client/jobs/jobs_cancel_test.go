package jobs

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// readPIDs waits for the process a cancel test starts to record its PID
// and its child's, as "<pid> <child pid>".
func readPIDs(t *testing.T, path string) []int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		b, err := os.ReadFile(path)
		if err == nil {
			var pids []int
			for _, f := range strings.Fields(string(b)) {
				pid, err := strconv.Atoi(f)
				if err != nil {
					t.Fatalf("bad pid file %q", b)
				}
				pids = append(pids, pid)
			}
			if len(pids) != 2 {
				t.Fatalf("bad pid file %q", b)
			}
			return pids
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("the process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
