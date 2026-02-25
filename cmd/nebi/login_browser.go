package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	browserPollInterval = 2 * time.Second
	browserLoginTimeout = 5 * time.Minute
)

// browserLogin performs the browser-based CLI login flow:
//  1. Generate a random session code
//  2. Register it on the server
//  3. Print/open the browser URL for the user
//  4. Poll for the token until the user completes auth in the browser
func browserLogin(ctx context.Context, serverURL string) (token, username string, err error) {
	code, err := randomHex(16)
	if err != nil {
		return "", "", fmt.Errorf("generating session code: %w", err)
	}

	// Register the session code on the server
	apiBase := serverURL + "/api/v1"
	if err := registerCLISession(ctx, apiBase, code); err != nil {
		return "", "", err
	}

	// Build the browser login URL
	loginURL := serverURL + "/auth/cli/login?code=" + code

	// Try to open the browser (best-effort)
	if openBrowser(loginURL) {
		fmt.Fprintln(os.Stderr, "Opening browser to authenticate...")
		fmt.Fprintln(os.Stderr, "If the browser doesn't open, visit this URL:")
	} else {
		fmt.Fprintln(os.Stderr, "Open this URL in a browser to authenticate:")
	}
	fmt.Fprintf(os.Stderr, "\n  %s\n\n", loginURL)
	fmt.Fprintln(os.Stderr, "Waiting for authentication...")

	// Poll for completion
	ctx, cancel := context.WithTimeout(ctx, browserLoginTimeout)
	defer cancel()

	ticker := time.NewTicker(browserPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", "", fmt.Errorf("login timed out — no browser authentication within %s", browserLoginTimeout)
		case <-ticker.C:
			t, u, done, err := pollCLIToken(ctx, apiBase, code)
			if err != nil {
				return "", "", err
			}
			if done {
				return t, u, nil
			}
		}
	}
}

func registerCLISession(ctx context.Context, apiBase, code string) error {
	body := `{"code":"` + code + `"}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/auth/cli/session", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("registering CLI session: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned %d when registering CLI session", resp.StatusCode)
	}
	return nil
}

func pollCLIToken(ctx context.Context, apiBase, code string) (token, username string, done bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/auth/cli/token?code="+code, nil)
	if err != nil {
		return "", "", false, fmt.Errorf("creating poll request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", false, fmt.Errorf("polling for token: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", false, fmt.Errorf("reading poll response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var result struct {
			Token    string `json:"token"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", "", false, fmt.Errorf("decoding token response: %w", err)
		}
		return result.Token, result.Username, true, nil
	case http.StatusAccepted:
		return "", "", false, nil
	case http.StatusNotFound:
		return "", "", false, fmt.Errorf("session expired or not found — please run nebi login again")
	default:
		return "", "", false, fmt.Errorf("unexpected poll response: %d %s", resp.StatusCode, string(respBody))
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openBrowser(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return false
	}
	return cmd.Start() == nil
}
