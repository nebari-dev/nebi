//go:build linux

package sandbox

import (
	"fmt"
	"strings"
	"testing"
)

// An empty allowed-port list must produce an empty rule set that is still
// handed to Landlock, not a skipped call. Under a v4 ruleset that handles
// bind and connect, granting nothing denies all TCP, which is what
// "allowed_ports: []" reads as. The bug this guards against is the opposite:
// no rules meaning no restriction at all.
func TestNetRules_EmptyPortListGrantsNothing(t *testing.T) {
	for _, ports := range [][]int{nil, {}} {
		if got := netRules(ports); len(got) != 0 {
			t.Fatalf("netRules(%v) = %v, want no rules", ports, got)
		}
	}
}

func TestNetRules_GrantsConnectOnlyForListedPorts(t *testing.T) {
	rules := netRules([]int{80, 443})
	if len(rules) != 2 {
		t.Fatalf("expected one rule per port, got %d: %v", len(rules), rules)
	}

	// landlock.Rule is an interface with no exported methods; the concrete
	// NetRule implements Stringer, so format through fmt.
	var joined []string
	for _, r := range rules {
		joined = append(joined, fmt.Sprint(r))
	}
	all := strings.Join(joined, "\n")

	for _, want := range []string{"80", "443"} {
		if !strings.Contains(all, want) {
			t.Fatalf("expected a rule for port %s, got:\n%s", want, all)
		}
	}
	if !strings.Contains(all, "connect") {
		t.Fatalf("expected connect rules, got:\n%s", all)
	}
	// Builds have no reason to listen, so bind stays denied even on the
	// allowed ports.
	if strings.Contains(all, "bind") {
		t.Fatalf("bind must not be granted, got:\n%s", all)
	}
}

// Filesystem and network rules must go into one ruleset. Landlock denies
// reparenting by default in every domain and "refer" can only be granted in
// the domain that handles it, so a stacked network domain silently undoes
// the refer grant on the workspace. This asserts the rules are assembled as
// a single list; that the combination actually preserves renames is checked
// by the Linux integration test.
func TestFSRules_ReferIsOptionalAndCombinesWithNetRules(t *testing.T) {
	r := Restrictions{
		RW:      []string{"/ws"},
		RO:      []string{"/usr"},
		ROFiles: []string{"/etc/hosts"},
		RWFiles: []string{"/dev/null"},
	}

	withRefer := fmt.Sprint(fsRules(r, true))
	without := fmt.Sprint(fsRules(r, false))
	if withRefer == without {
		t.Fatalf("expected refer to change the workspace rule, both were:\n%s", without)
	}
	if !strings.Contains(withRefer, "refer") {
		t.Fatalf("expected a refer grant, got:\n%s", withRefer)
	}
	if strings.Contains(without, "refer") {
		t.Fatalf("v1 fallback must not request refer, got:\n%s", without)
	}

	if got := len(fsRules(r, true)); got != 4 {
		t.Fatalf("expected one rule per populated path set, got %d", got)
	}
	combined := append(fsRules(r, true), netRules([]int{443})...)
	if len(combined) != 5 {
		t.Fatalf("expected fs and net rules to combine into one list, got %d", len(combined))
	}
}
