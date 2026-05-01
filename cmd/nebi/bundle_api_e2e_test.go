//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/server"
)

// localModeEnv holds everything a local-mode HTTP test needs.
type localModeEnv struct {
	serverURL string
	token     string
	ctx       context.Context
	wsDir     string
	ociHost   string
}

// startLocalModeServer spins up a private Nebi server in NEBI_MODE=local
// (independent of the shared TestMain team-mode server) plus an in-memory
// OCI registry. Env vars are snapshotted/restored via t.Cleanup so the
// shared server is unaffected.
func startLocalModeServer(t *testing.T) *localModeEnv {
	t.Helper()
	envVars := []string{
		"NEBI_MODE",
		"NEBI_DATABASE_DSN",
		"NEBI_STORAGE_WORKSPACES_DIR",
		"NEBI_SERVER_PORT",
		"NEBI_PACKAGE_MANAGER_PIXI_PATH",
	}
	saved := make(map[string]string, len(envVars))
	for _, k := range envVars {
		saved[k] = os.Getenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "bundle-api-e2e.db")
	wsDir := t.TempDir()

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}

	os.Setenv("NEBI_MODE", "local")
	os.Setenv("NEBI_DATABASE_DSN", dbPath)
	os.Setenv("NEBI_STORAGE_WORKSPACES_DIR", wsDir)
	os.Setenv("NEBI_SERVER_PORT", fmt.Sprintf("%d", port))
	noopBinary := "/usr/bin/true"
	if _, err := os.Stat(noopBinary); err != nil {
		noopBinary = "/bin/true"
	}
	os.Setenv("NEBI_PACKAGE_MANAGER_PIXI_PATH", noopBinary)

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx, server.Config{
			Port:    port,
			Mode:    "both",
			Version: "e2e-bundle-api-test",
		})
	}()
	waitForHealth(serverURL+"/api/v1/health", serverErr, io.Discard)

	unauthClient := cliclient.NewWithoutAuth(serverURL)
	loginResp, err := unauthClient.Login(ctx, "admin", "adminpass")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	ociSrv := httptest.NewServer(registry.New())
	t.Cleanup(ociSrv.Close)
	ociURL, _ := url.Parse(ociSrv.URL)

	return &localModeEnv{
		serverURL: serverURL,
		token:     loginResp.Token,
		ctx:       ctx,
		wsDir:     wsDir,
		ociHost:   ociURL.Host,
	}
}

// TestE2E_BundlePublishImportViaAPI_LocalMode verifies that importing a bundle
// via the Nebi HTTP API in local mode correctly extracts all asset layers to
// the workspace directory on disk.
func TestE2E_BundlePublishImportViaAPI_LocalMode(t *testing.T) {
	env := startLocalModeServer(t)
	serverURL := env.serverURL
	token := env.token
	ctx := env.ctx
	wsDir := env.wsDir
	ociHost := env.ociHost

	// ---- Seed the OCI registry via oci.Publish ----
	const (
		repoName     = "notebook-env"
		bundleTag    = "v1"
		ociNS        = "demo"
		notebookBody = `{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`
		pixiTomlBody = "[project]\nname = \"notebook-env\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
		pixiLockBody = "version: 6\n# seeded-by-test\n"
	)

	srcDir := t.TempDir()
	writeFile := func(rel, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(srcDir, rel)), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, rel), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeFile("pixi.toml", pixiTomlBody)
	writeFile("pixi.lock", pixiLockBody)
	writeFile("notebook.ipynb", notebookBody)

	reg := oci.Registry{Host: ociHost, Namespace: ociNS, PlainHTTP: true}
	if _, err := oci.Publish(ctx, srcDir, reg, repoName, bundleTag); err != nil {
		t.Fatalf("seed oci.Publish: %v", err)
	}

	// ---- Create registry in Nebi via POST /admin/registries ----
	registryID := createRegistryViaAPI(t, serverURL, token, map[string]interface{}{
		"name":       "bundle-api-e2e-reg",
		"url":        "http://" + ociHost,
		"namespace":  ociNS,
		"is_default": true,
	})

	// ---- Import via POST /registries/:id/import ----
	wsID := importViaAPI(t, serverURL, token, registryID, map[string]interface{}{
		"repository_path": ociNS + "/" + repoName,
		"tag":             bundleTag,
		"name":            "notebook-imported",
	})

	// ---- Poll workspace until ready (or fail after 30s) ----
	pollWorkspaceReady(t, serverURL, token, wsID, 30*time.Second)

	// ---- Derive workspace directory path ----
	// LocalExecutor.GetWorkspacePath: {wsDir}/{normalized-name}-{uuid}
	// normalized("notebook-imported") → "notebook-imported"
	wsPath := filepath.Join(wsDir, fmt.Sprintf("notebook-imported-%s", wsID))

	// ---- Assertions ----
	assertFileContent(t, wsPath, "notebook.ipynb", notebookBody)
	assertFileContent(t, wsPath, "pixi.lock", pixiLockBody)
	assertFileContains(t, wsPath, "pixi.toml", "notebook-env")
}

// TestE2E_BundlePublishViaAPI_LocalMode verifies that publishing a workspace
// via POST /workspaces/:id/publish in local mode uploads the full bundle —
// pixi.toml, pixi.lock, AND every asset file — to the registry.
func TestE2E_BundlePublishViaAPI_LocalMode(t *testing.T) {
	env := startLocalModeServer(t)

	// ---- Create + populate a workspace ----
	const (
		notebookBody = `{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`
		pixiTomlBody = "[project]\nname = \"publish-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
		pixiLockBody = "version: 6\n# server-published\n"
	)
	wsID := createWorkspaceViaAPI(t, env.serverURL, env.token, "publish-test", pixiTomlBody)
	pollWorkspaceReady(t, env.serverURL, env.token, wsID, 30*time.Second)

	// Drop a real lockfile + asset on disk where the server expects them
	// (the worker only wrote pixi.toml; we add the rest by hand to
	// simulate what the user would have done via push or the editor).
	wsPath := filepath.Join(env.wsDir, fmt.Sprintf("publish-test-%s", wsID))
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.lock"), []byte(pixiLockBody), 0o644); err != nil {
		t.Fatalf("seed pixi.lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "notebook.ipynb"), []byte(notebookBody), 0o644); err != nil {
		t.Fatalf("seed notebook: %v", err)
	}

	// ---- Register the OCI destination ----
	registryID := createRegistryViaAPI(t, env.serverURL, env.token, map[string]interface{}{
		"name":       "publish-target-reg",
		"url":        "http://" + env.ociHost,
		"namespace":  "demo",
		"is_default": true,
	})

	// ---- Publish via POST /workspaces/:id/publish ----
	publishViaAPI(t, env.serverURL, env.token, wsID, map[string]interface{}{
		"registry_id": registryID,
		"repository":  "publish-test-repo",
		"tag":         "v1",
	})

	// ---- Pull back via oci.PullBundle and verify the asset layer is present ----
	repoRef := fmt.Sprintf("%s/demo/publish-test-repo", env.ociHost)
	pull, err := oci.PullBundle(env.ctx, repoRef, "v1", oci.PullOptions{PlainHTTP: true})
	if err != nil {
		t.Fatalf("PullBundle: %v", err)
	}
	if len(pull.Assets) != 1 || pull.Assets[0].Path != "notebook.ipynb" {
		t.Fatalf("expected one asset notebook.ipynb in published bundle, got %+v", pull.Assets)
	}
	if !contains(pull.PixiToml, "publish-test") {
		t.Errorf("published pixi.toml missing project name; got %q", pull.PixiToml)
	}
	if !contains(pull.PixiLock, "server-published") {
		t.Errorf("published pixi.lock did not round-trip; got %q", pull.PixiLock)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

// createWorkspaceViaAPI POSTs to /workspaces and returns the created workspace ID.
func createWorkspaceViaAPI(t *testing.T, serverURL, token, name, pixiToml string) string {
	t.Helper()
	body := map[string]interface{}{
		"name":            name,
		"package_manager": "pixi",
		"pixi_toml":       pixiToml,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/workspaces", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /workspaces: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /workspaces: status %d, body: %s", resp.StatusCode, raw)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode workspace response: %v (body: %s)", err, raw)
	}
	if result.ID == "" {
		t.Fatalf("workspace response missing id: %s", raw)
	}
	return result.ID
}

// publishViaAPI POSTs to /workspaces/:id/publish and asserts a 2xx response.
func publishViaAPI(t *testing.T, serverURL, token, wsID string, body map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/publish", serverURL, wsID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /workspaces/%s/publish: %v", wsID, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("POST /workspaces/%s/publish: status %d, body: %s", wsID, resp.StatusCode, raw)
	}
}

// createRegistryViaAPI POSTs to /admin/registries and returns the created registry ID.
func createRegistryViaAPI(t *testing.T, serverURL, token string, body map[string]interface{}) string {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/admin/registries", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/registries: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /admin/registries: status %d, body: %s", resp.StatusCode, raw)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode registry response: %v (body: %s)", err, raw)
	}
	if result.ID == "" {
		t.Fatalf("registry response missing id: %s", raw)
	}
	return result.ID
}

// importViaAPI POSTs to /registries/:id/import and returns the created workspace ID.
func importViaAPI(t *testing.T, serverURL, token, registryID string, body map[string]interface{}) string {
	t.Helper()
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/api/v1/registries/%s/import", serverURL, registryID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /registries/%s/import: %v", registryID, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /registries/%s/import: status %d, body: %s", registryID, resp.StatusCode, raw)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode import response: %v (body: %s)", err, raw)
	}
	if result.ID == "" {
		t.Fatalf("import response missing id: %s", raw)
	}
	return result.ID
}

// pollWorkspaceReady polls GET /workspaces/:id until status == "ready" or times out.
func pollWorkspaceReady(t *testing.T, serverURL, token, wsID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("%s/api/v1/workspaces/%s", serverURL, wsID)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var ws struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &ws); err != nil {
			t.Fatalf("decode workspace response: %v (body: %s)", err, raw)
		}
		switch ws.Status {
		case "ready":
			return
		case "failed":
			t.Fatalf("workspace %s entered 'failed' state", wsID)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("workspace %s did not become ready within %s", wsID, timeout)
}

// assertFileContent reads rel inside wsDir and checks it equals want exactly.
func assertFileContent(t *testing.T, wsDir, rel, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(wsDir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if string(got) != want {
		t.Fatalf("content mismatch for %s:\n got: %q\nwant: %q", rel, got, want)
	}
}

// assertFileContains reads rel inside wsDir and checks it contains substr.
func assertFileContains(t *testing.T, wsDir, rel, substr string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(wsDir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if !bytes.Contains(got, []byte(substr)) {
		t.Fatalf("%s does not contain %q:\n%s", rel, substr, got)
	}
}
