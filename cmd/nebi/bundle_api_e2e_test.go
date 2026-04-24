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

// TestE2E_BundlePublishImportViaAPI_LocalMode verifies that importing a bundle
// via the Nebi HTTP API in local mode correctly extracts all asset layers to
// the workspace directory on disk. It:
//
//  1. Starts a fresh Nebi server in local mode (independent of the shared TestMain server).
//  2. Seeds an in-memory OCI registry with a bundle containing pixi.toml,
//     pixi.lock, and notebook.ipynb using oci.Publish directly.
//  3. Creates the registry in Nebi via POST /admin/registries.
//  4. Imports via POST /registries/:id/import.
//  5. Polls GET /workspaces/:id until status == "ready".
//  6. Asserts notebook.ipynb and pixi.lock exist on disk with correct content.
func TestE2E_BundlePublishImportViaAPI_LocalMode(t *testing.T) {
	// Snapshot and restore env vars that overlap with the global TestMain setup.
	// We need to override them for this test's private server without poisoning
	// the shared server that TestMain already started on a different port.
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

	// ---- Temp directories ----
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "bundle-api-e2e.db")
	wsDir := t.TempDir()

	// ---- Port allocation ----
	port, err := findFreePort()
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}

	// ---- Set env vars for local-mode server ----
	os.Setenv("NEBI_MODE", "local")
	os.Setenv("NEBI_DATABASE_DSN", dbPath)
	os.Setenv("NEBI_STORAGE_WORKSPACES_DIR", wsDir)
	os.Setenv("NEBI_SERVER_PORT", fmt.Sprintf("%d", port))
	// Point pixi to a no-op binary so "pixi install" succeeds without
	// actually installing any packages. The workspace only needs files on
	// disk for this test to pass. Use /usr/bin/true (macOS) with /bin/true
	// as a fallback for Linux.
	noopBinary := "/usr/bin/true"
	if _, err := os.Stat(noopBinary); err != nil {
		noopBinary = "/bin/true"
	}
	os.Setenv("NEBI_PACKAGE_MANAGER_PIXI_PATH", noopBinary)

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// ---- Start local-mode server ----
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

	// ---- Login ----
	unauthClient := cliclient.NewWithoutAuth(serverURL)
	loginResp, err := unauthClient.Login(ctx, "admin", "adminpass")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	token := loginResp.Token

	// ---- In-memory OCI registry ----
	ociSrv := httptest.NewServer(registry.New())
	t.Cleanup(ociSrv.Close)
	ociURL, _ := url.Parse(ociSrv.URL)
	ociHost := ociURL.Host

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
		"repository": repoName,
		"tag":        bundleTag,
		"name":       "notebook-imported",
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
