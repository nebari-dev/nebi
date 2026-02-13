# Registry CLI Commands Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `nebi registry create` and `nebi registry delete` CLI commands for managing OCI registries.

**Architecture:** Extend existing `cmd/nebi/registry.go` with two new subcommands. Use existing `cliclient.CreateRegistry()` and `cliclient.DeleteRegistry()` methods. Follow patterns from `login.go` for secure password handling.

**Tech Stack:** Go, Cobra CLI, golang.org/x/term for password prompts

---

### Task 1: Add `registry create` command structure

**Files:**
- Modify: `cmd/nebi/registry.go`

**Step 1: Add command variables and flags**

Add after the existing `registryListCmd` variable:

```go
var (
	registryCreateName     string
	registryCreateURL      string
	registryCreateUsername string
	registryCreatePassword string
	registryCreateDefault  bool
	registryCreatePwdStdin bool
)

var registryCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new OCI registry",
	Long: `Create a new OCI registry configuration on the server.

Examples:
  # Interactive - prompts for password
  nebi registry create --name ghcr --url ghcr.io --username myuser

  # Programmatic - read password from stdin
  echo "$TOKEN" | nebi registry create --name ghcr --url ghcr.io --username myuser --password-stdin

  # Public registry (no auth)
  nebi registry create --name dockerhub --url docker.io --default`,
	Args: cobra.NoArgs,
	RunE: runRegistryCreate,
}
```

**Step 2: Register flags and command in init()**

Add to the existing `init()` function:

```go
func init() {
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryCreateCmd)

	registryCreateCmd.Flags().StringVar(&registryCreateName, "name", "", "Registry name (required)")
	registryCreateCmd.Flags().StringVar(&registryCreateURL, "url", "", "Registry URL (required)")
	registryCreateCmd.Flags().StringVar(&registryCreateUsername, "username", "", "Username for authentication")
	registryCreateCmd.Flags().BoolVar(&registryCreatePwdStdin, "password-stdin", false, "Read password from stdin")
	registryCreateCmd.Flags().BoolVar(&registryCreateDefault, "default", false, "Set as default registry")

	registryCreateCmd.MarkFlagRequired("name")
	registryCreateCmd.MarkFlagRequired("url")
}
```

**Step 3: Add stub runRegistryCreate function**

```go
func runRegistryCreate(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("not implemented")
}
```

**Step 4: Verify it compiles**

Run: `go build -o /dev/null ./cmd/nebi`
Expected: Compiles without errors

**Step 5: Commit**

```bash
git add cmd/nebi/registry.go
git commit -m "feat(cli): add registry create command structure"
```

---

### Task 2: Write E2E test for registry create

**Files:**
- Modify: `cmd/nebi/e2e_test.go`

**Step 1: Add test for basic registry creation**

Add after `TestE2E_RegistryListEmpty`:

```go
func TestE2E_RegistryCreate(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Create a registry
	res := runCLI(t, dir, "registry", "create", "--name", "test-registry", "--url", "ghcr.io")
	if res.ExitCode != 0 {
		t.Fatalf("registry create failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Created registry") {
		t.Errorf("expected 'Created registry' message, got stderr: %s", res.Stderr)
	}

	// Verify it shows up in list
	res = runCLI(t, dir, "registry", "list")
	if res.ExitCode != 0 {
		t.Fatalf("registry list failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "test-registry") {
		t.Errorf("expected registry in list, got stdout: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "ghcr.io") {
		t.Errorf("expected URL in list, got stdout: %s", res.Stdout)
	}
}
```

**Step 2: Add test for duplicate name error**

```go
func TestE2E_RegistryCreateDuplicate(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Create first registry
	res := runCLI(t, dir, "registry", "create", "--name", "dup-test", "--url", "ghcr.io")
	if res.ExitCode != 0 {
		t.Fatalf("first create failed: %s", res.Stderr)
	}

	// Try to create with same name - should fail
	res = runCLI(t, dir, "registry", "create", "--name", "dup-test", "--url", "quay.io")
	if res.ExitCode == 0 {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(res.Stderr, "already exists") && !strings.Contains(res.Stderr, "UNIQUE constraint") {
		t.Errorf("expected duplicate error message, got: %s", res.Stderr)
	}
}
```

**Step 3: Add test for password-stdin**

```go
func TestE2E_RegistryCreateWithPasswordStdin(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Create with password via stdin
	res := runCLIWithStdin(t, dir, "secret-password\n", "registry", "create",
		"--name", "auth-registry",
		"--url", "private.registry.io",
		"--username", "testuser",
		"--password-stdin")
	if res.ExitCode != 0 {
		t.Fatalf("registry create with password failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	// Verify it shows up in list
	res = runCLI(t, dir, "registry", "list")
	if !strings.Contains(res.Stdout, "auth-registry") {
		t.Errorf("expected registry in list, got stdout: %s", res.Stdout)
	}
}
```

**Step 4: Add runCLIWithStdin helper function**

Add after the existing `runCLI` function:

```go
// runCLIWithStdin runs the CLI with the given args and stdin input.
func runCLIWithStdin(t *testing.T, cwd, stdin string, args ...string) cliResult {
	t.Helper()
	origStdin := os.Stdin

	// Create a pipe for stdin
	stdinR, stdinW, _ := os.Pipe()
	os.Stdin = stdinR
	go func() {
		stdinW.WriteString(stdin)
		stdinW.Close()
	}()

	result := runCLI(t, cwd, args...)

	os.Stdin = origStdin
	stdinR.Close()

	return result
}
```

**Step 5: Run tests to verify they fail**

Run: `go test -tags e2e ./cmd/nebi/... -run TestE2E_RegistryCreate -v`
Expected: Tests fail with "not implemented"

**Step 6: Commit**

```bash
git add cmd/nebi/e2e_test.go
git commit -m "test(cli): add E2E tests for registry create"
```

---

### Task 3: Implement registry create

**Files:**
- Modify: `cmd/nebi/registry.go`

**Step 1: Add required imports**

Update imports at top of file:

```go
import (
	"bufio"
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)
```

**Step 2: Implement runRegistryCreate**

Replace the stub with:

```go
func runRegistryCreate(cmd *cobra.Command, args []string) error {
	var password string

	// Handle password input
	if registryCreateUsername != "" {
		if registryCreatePwdStdin {
			// Read password from stdin
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				password = scanner.Text()
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("reading password from stdin: %w", err)
			}
		} else if term.IsTerminal(int(os.Stdin.Fd())) {
			// Interactive prompt
			fmt.Fprint(os.Stderr, "Password: ")
			passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			password = string(passBytes)
		}
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	req := cliclient.CreateRegistryRequest{
		Name: registryCreateName,
		URL:  registryCreateURL,
	}
	if registryCreateUsername != "" {
		req.Username = &registryCreateUsername
	}
	if password != "" {
		req.Password = &password
	}
	if registryCreateDefault {
		req.IsDefault = &registryCreateDefault
	}

	ctx := context.Background()
	registry, err := client.CreateRegistry(ctx, req)
	if err != nil {
		return fmt.Errorf("creating registry: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created registry '%s' (%s)\n", registry.Name, registry.URL)
	return nil
}
```

**Step 3: Run tests**

Run: `go test -tags e2e ./cmd/nebi/... -run TestE2E_RegistryCreate -v`
Expected: All registry create tests pass

**Step 4: Commit**

```bash
git add cmd/nebi/registry.go
git commit -m "feat(cli): implement registry create command"
```

---

### Task 4: Add `registry delete` command structure

**Files:**
- Modify: `cmd/nebi/registry.go`

**Step 1: Add command variable and flags**

Add after `registryCreateCmd`:

```go
var registryDeleteForce bool

var registryDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an OCI registry",
	Long: `Delete an OCI registry configuration from the server.

Examples:
  # Interactive - prompts for confirmation
  nebi registry delete ghcr

  # Skip confirmation
  nebi registry delete ghcr --force`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryDelete,
}
```

**Step 2: Register command in init()**

Update `init()`:

```go
func init() {
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryCreateCmd)
	registryCmd.AddCommand(registryDeleteCmd)

	// ... existing create flags ...

	registryDeleteCmd.Flags().BoolVarP(&registryDeleteForce, "force", "f", false, "Skip confirmation prompt")
}
```

**Step 3: Add stub runRegistryDelete function**

```go
func runRegistryDelete(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("not implemented")
}
```

**Step 4: Verify it compiles**

Run: `go build -o /dev/null ./cmd/nebi`
Expected: Compiles without errors

**Step 5: Commit**

```bash
git add cmd/nebi/registry.go
git commit -m "feat(cli): add registry delete command structure"
```

---

### Task 5: Write E2E tests for registry delete

**Files:**
- Modify: `cmd/nebi/e2e_test.go`

**Step 1: Add test for registry delete with --force**

```go
func TestE2E_RegistryDelete(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Create a registry first
	res := runCLI(t, dir, "registry", "create", "--name", "delete-me", "--url", "example.io")
	if res.ExitCode != 0 {
		t.Fatalf("create failed: %s", res.Stderr)
	}

	// Delete it with --force
	res = runCLI(t, dir, "registry", "delete", "delete-me", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("registry delete failed (exit %d):\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Deleted registry") {
		t.Errorf("expected 'Deleted registry' message, got stderr: %s", res.Stderr)
	}

	// Verify it's gone from list
	res = runCLI(t, dir, "registry", "list")
	if strings.Contains(res.Stdout, "delete-me") {
		t.Errorf("registry should be deleted, but still in list: %s", res.Stdout)
	}
}
```

**Step 2: Add test for delete not found**

```go
func TestE2E_RegistryDeleteNotFound(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	res := runCLI(t, dir, "registry", "delete", "nonexistent", "--force")
	if res.ExitCode == 0 {
		t.Fatal("expected error for nonexistent registry")
	}
	if !strings.Contains(res.Stderr, "not found") {
		t.Errorf("expected 'not found' error, got: %s", res.Stderr)
	}
}
```

**Step 3: Add test for confirmation prompt (defaults to no)**

```go
func TestE2E_RegistryDeleteConfirmNo(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Create a registry first
	res := runCLI(t, dir, "registry", "create", "--name", "keep-me", "--url", "example.io")
	if res.ExitCode != 0 {
		t.Fatalf("create failed: %s", res.Stderr)
	}

	// Try delete without --force, stdin provides empty (defaults to no)
	res = runCLIWithStdin(t, dir, "\n", "registry", "delete", "keep-me")
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0 when declining, got %d: %s", res.ExitCode, res.Stderr)
	}

	// Registry should still exist
	res = runCLI(t, dir, "registry", "list")
	if !strings.Contains(res.Stdout, "keep-me") {
		t.Errorf("registry should still exist after declining delete: %s", res.Stdout)
	}
}
```

**Step 4: Add test for confirmation prompt (yes)**

```go
func TestE2E_RegistryDeleteConfirmYes(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	// Create a registry first
	res := runCLI(t, dir, "registry", "create", "--name", "confirm-delete", "--url", "example.io")
	if res.ExitCode != 0 {
		t.Fatalf("create failed: %s", res.Stderr)
	}

	// Delete with confirmation
	res = runCLIWithStdin(t, dir, "y\n", "registry", "delete", "confirm-delete")
	if res.ExitCode != 0 {
		t.Fatalf("registry delete failed (exit %d):\nstderr: %s", res.ExitCode, res.Stderr)
	}

	// Registry should be gone
	res = runCLI(t, dir, "registry", "list")
	if strings.Contains(res.Stdout, "confirm-delete") {
		t.Errorf("registry should be deleted: %s", res.Stdout)
	}
}
```

**Step 5: Run tests to verify they fail**

Run: `go test -tags e2e ./cmd/nebi/... -run TestE2E_RegistryDelete -v`
Expected: Tests fail with "not implemented"

**Step 6: Commit**

```bash
git add cmd/nebi/e2e_test.go
git commit -m "test(cli): add E2E tests for registry delete"
```

---

### Task 6: Implement registry delete

**Files:**
- Modify: `cmd/nebi/registry.go`

**Step 1: Add helper function to find registry by name**

```go
// findRegistryByName finds a registry by name and returns its ID.
func findRegistryByName(client *cliclient.Client, ctx context.Context, name string) (string, error) {
	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return "", fmt.Errorf("listing registries: %w", err)
	}

	for _, r := range registries {
		if r.Name == name {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("registry '%s' not found", name)
}
```

**Step 2: Implement runRegistryDelete**

Replace the stub with:

```go
func runRegistryDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find the registry by name
	registryID, err := findRegistryByName(client, ctx, name)
	if err != nil {
		return err
	}

	// Confirm unless --force
	if !registryDeleteForce {
		fmt.Fprintf(os.Stderr, "Delete registry '%s'? [y/N] ", name)

		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	if err := client.DeleteRegistry(ctx, registryID); err != nil {
		return fmt.Errorf("deleting registry: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Deleted registry '%s'\n", name)
	return nil
}
```

**Step 3: Run tests**

Run: `go test -tags e2e ./cmd/nebi/... -run TestE2E_RegistryDelete -v`
Expected: All registry delete tests pass

**Step 4: Commit**

```bash
git add cmd/nebi/registry.go
git commit -m "feat(cli): implement registry delete command"
```

---

### Task 7: Run full test suite and verify

**Step 1: Run all E2E tests**

Run: `go test -tags e2e ./cmd/nebi/... -v`
Expected: All tests pass

**Step 2: Run go vet**

Run: `go vet ./cmd/nebi/...`
Expected: No issues

**Step 3: Manual verification**

```bash
# Build the CLI
go build -o ./nebi ./cmd/nebi

# Test the commands (if you have a server running)
./nebi registry list
./nebi registry create --name test --url example.com
./nebi registry list
./nebi registry delete test --force
./nebi registry list
```

**Step 4: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix(cli): address any issues from testing"
```

---

### Task 8: Update CLI documentation

**Files:**
- Modify: `docs/docs/cli-overview.md` (if exists and documents commands)

**Step 1: Check if CLI docs exist**

Run: `cat docs/docs/cli-overview.md | head -50`

**Step 2: Add registry create/delete to documentation if needed**

Add to the appropriate section:

```markdown
### Registry Commands

#### `nebi registry list`
List all configured OCI registries.

#### `nebi registry create`
Create a new OCI registry.

```bash
nebi registry create --name <name> --url <url> [--username <user>] [--password-stdin] [--default]
```

Flags:
- `--name` (required): Display name for the registry
- `--url` (required): Registry URL (e.g., "ghcr.io")
- `--username`: Username for authentication
- `--password-stdin`: Read password from stdin (for scripting)
- `--default`: Set as the default registry

#### `nebi registry delete`
Delete an OCI registry.

```bash
nebi registry delete <name> [--force]
```

Flags:
- `--force`, `-f`: Skip confirmation prompt
```

**Step 3: Commit**

```bash
git add docs/
git commit -m "docs: add registry create/delete to CLI documentation"
```
