package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var infoJSON bool

func init() {
	infoCmd.Flags().BoolVar(&infoJSON, "json", false, "Output as JSON")
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show nebi system information",
	Long: `Display comprehensive information about the nebi CLI, server connection,
authentication status, and current workspace.

Examples:
  nebi info
  nebi info --json`,
	Args: cobra.NoArgs,
	RunE: runInfo,
}

type infoResult struct {
	// Nebi section
	Version  string `json:"version"`
	Platform string `json:"platform"`
	DataDir  string `json:"data_dir"`

	// Server section
	ServerURL      string `json:"server_url"`
	ServerStatus   string `json:"server_status,omitempty"`
	ServerVersion  string `json:"server_version,omitempty"`
	ServerMode     string `json:"server_mode,omitempty"`
	ServerFeatures string `json:"server_features,omitempty"`

	// Auth section
	LoggedIn   bool   `json:"logged_in"`
	Username   string `json:"username,omitempty"`
	AuthSource string `json:"auth_source"`

	// Workspace section (empty when not in a tracked workspace)
	Workspace      string `json:"workspace,omitempty"`
	WorkspacePath  string `json:"workspace_path,omitempty"`
	PackageManager string `json:"package_manager,omitempty"`
	Origin         string `json:"origin,omitempty"`
	LocalEdits     string `json:"local_edits,omitempty"`
}

func runInfo(cmd *cobra.Command, args []string) error {
	result := infoResult{
		Version:  Version,
		Platform: runtime.GOOS + "-" + runtime.GOARCH,
	}

	// Data dir
	dataDir, err := store.DefaultDataDir()
	if err == nil {
		home, _ := os.UserHomeDir()
		result.DataDir = shortenPath(dataDir, home)
	}

	// Resolve server URL, token, username from local sources (no API calls)
	serverURL, token, username, authSource := resolveInfoAuth()
	result.AuthSource = authSource
	if token != "" {
		result.LoggedIn = true
		result.Username = username
	}

	if serverURL != "" {
		result.ServerURL = serverURL
	} else {
		result.ServerURL = "not configured"
	}

	// Check server version (short timeout, public endpoint) — only if configured
	if serverURL != "" {
		result.ServerStatus = "unreachable"
		client := cliclient.NewWithoutAuth(serverURL)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		sv, err := client.GetServerVersion(ctx)
		if err == nil {
			result.ServerStatus = "reachable"
			vStr := sv.Version
			if sv.Commit != "" && !strings.Contains(sv.Version, sv.Commit) {
				vStr += " (" + sv.Commit + ")"
			}
			result.ServerVersion = vStr
			result.ServerMode = sv.Mode
			result.ServerFeatures = formatFeatures(sv.Features)
		}
	}

	// Workspace section
	fillWorkspaceInfo(&result)

	if infoJSON {
		return writeJSON(result)
	}

	printInfo(result)
	return nil
}

func resolveInfoAuth() (serverURL, token, username, source string) {
	if envToken := os.Getenv("NEBI_AUTH_TOKEN"); envToken != "" {
		if envURL := os.Getenv("NEBI_REMOTE_URL"); envURL != "" {
			return envURL, envToken, "", "environment variable"
		}
	}

	s, err := store.New()
	if err != nil {
		return "", "", "", "none"
	}
	defer s.Close()

	url, _ := s.LoadServerURL()
	creds, _ := s.LoadCredentials()

	if url == "" && (creds == nil || creds.Token == "") {
		return "", "", "", "none"
	}
	if creds != nil && creds.Token != "" {
		return url, creds.Token, creds.Username, "stored credentials"
	}
	return url, "", "", "none"
}

func fillWorkspaceInfo(result *infoResult) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	// Check for pixi.toml in current directory
	pixiPath := filepath.Join(cwd, "pixi.toml")
	if _, err := os.Stat(pixiPath); err != nil {
		return
	}

	s, err := store.New()
	if err != nil {
		return
	}
	defer s.Close()

	ws, err := s.FindWorkspaceByPath(cwd)
	if err != nil || ws == nil {
		return
	}

	result.Workspace = ws.Name
	result.WorkspacePath = ws.Path
	result.PackageManager = ws.PackageManager

	if ws.OriginName != "" {
		action := ws.OriginAction
		if action == "push" {
			action = "pushed"
		} else if action == "pull" {
			action = "pulled"
		}
		result.Origin = fmt.Sprintf("%s:%s (%s)", ws.OriginName, ws.OriginTag, action)

		// Check local edits
		var edits []string
		localToml, _ := os.ReadFile(filepath.Join(cwd, "pixi.toml"))
		localLock, _ := os.ReadFile(filepath.Join(cwd, "pixi.lock"))
		tomlHash, hashErr := store.TomlContentHash(string(localToml))
		if hashErr == nil && ws.OriginTomlHash != "" && ws.OriginTomlHash != tomlHash {
			edits = append(edits, "pixi.toml modified")
		}
		lockHash := store.ContentHash(string(localLock))
		if ws.OriginLockHash != "" && ws.OriginLockHash != lockHash {
			edits = append(edits, "pixi.lock modified")
		}
		if len(edits) > 0 {
			result.LocalEdits = strings.Join(edits, ", ")
		} else {
			result.LocalEdits = "none"
		}
	} else {
		result.Origin = "none"
	}
}

func formatFeatures(features map[string]bool) string {
	var enabled []string
	for k, v := range features {
		if v {
			enabled = append(enabled, k)
		}
	}
	if len(enabled) == 0 {
		return "none"
	}
	sort.Strings(enabled)
	return strings.Join(enabled, ", ")
}

func shortenPath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func printInfo(r infoResult) {
	fmt.Println("Nebi")
	fmt.Println("──────────────")
	printField("Version", r.Version)
	printField("Platform", r.Platform)
	printField("Data dir", r.DataDir)

	fmt.Println()
	fmt.Println("Server")
	fmt.Println("──────────────")
	printField("URL", r.ServerURL)
	if r.ServerURL != "not configured" {
		printField("Status", r.ServerStatus)
		if r.ServerVersion != "" {
			printField("Server version", r.ServerVersion)
		}
		if r.ServerMode != "" {
			printField("Mode", r.ServerMode)
		}
		if r.ServerFeatures != "" {
			printField("Features", r.ServerFeatures)
		}
	}

	fmt.Println()
	fmt.Println("Auth")
	fmt.Println("──────────────")
	if r.LoggedIn {
		printField("Logged in", "yes")
		printField("Username", r.Username)
	} else {
		printField("Logged in", "no")
	}
	printField("Auth source", r.AuthSource)

	if r.Workspace != "" {
		fmt.Println()
		fmt.Println("Workspace")
		fmt.Println("──────────────")
		printField("Name", r.Workspace)
		printField("Path", r.WorkspacePath)
		printField("Package manager", r.PackageManager)
		printField("Origin", r.Origin)
		if r.LocalEdits != "" {
			printField("Local edits", r.LocalEdits)
		}
	}
}

func printField(label, value string) {
	fmt.Fprintf(os.Stdout, "%16s: %s\n", label, value)
}
