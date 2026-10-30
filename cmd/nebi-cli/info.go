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
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var infoJSON bool

func init() {
	infoCmd.Flags().BoolVar(&infoJSON, "json", false, "Output as JSON")
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show Nebi system information",
	Long: `Display comprehensive information about the Nebi CLI, server connection,
authentication status, and current project.

Examples:
  nebi info
  nebi info --json`,
	Args: cobra.NoArgs,
	RunE: runInfo,
}

type infoResult struct {
	// Nebi section
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	DataDir     string `json:"data_dir"`
	ProjectsDir string `json:"projects_dir,omitempty"`

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

	// Project section (empty when not in a tracked project)
	Project     string `json:"project,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	Origin      string `json:"origin,omitempty"`
	LocalEdits  string `json:"local_edits,omitempty"`
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

	// Projects dir — same resolution `nebi-web` uses (config file,
	// NEBI_STORAGE_PROJECTS_DIR, or the built-in default)
	if cfg, err := config.Load(config.WithMode(config.ModeLocal)); err == nil && cfg.Storage.ProjectsDir != "" {
		home, _ := os.UserHomeDir()
		result.ProjectsDir = shortenPath(cfg.Storage.ProjectsDir, home)
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

	// Project section
	fillProjectInfo(&result)

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

func fillProjectInfo(result *infoResult) {
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

	project, err := s.FindProjectByPath(cwd)
	if err != nil || project == nil {
		return
	}

	result.Project = project.Name
	result.ProjectPath = project.Path

	if project.OriginName != "" {
		action := project.OriginAction
		if action == "push" {
			action = "pushed"
		} else if action == "pull" {
			action = "pulled"
		}
		result.Origin = fmt.Sprintf("%s:%s (%s)", project.OriginName, project.OriginTag, action)

		// Check local edits
		var edits []string
		localToml, _ := os.ReadFile(filepath.Join(cwd, "pixi.toml"))
		localLock, _ := os.ReadFile(filepath.Join(cwd, "pixi.lock"))
		tomlHash, hashErr := store.TomlContentHash(string(localToml))
		if hashErr == nil && project.OriginTomlHash != "" && project.OriginTomlHash != tomlHash {
			edits = append(edits, "pixi.toml modified")
		}
		lockHash := store.ContentHash(string(localLock))
		if project.OriginLockHash != "" && project.OriginLockHash != lockHash {
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
	if r.ProjectsDir != "" {
		printField("Projects dir", r.ProjectsDir)
	}

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

	if r.Project != "" {
		fmt.Println()
		fmt.Println("Project")
		fmt.Println("──────────────")
		printField("Name", r.Project)
		printField("Path", r.ProjectPath)
		printField("Origin", r.Origin)
		if r.LocalEdits != "" {
			printField("Local edits", r.LocalEdits)
		}
	}
}

func printField(label, value string) {
	fmt.Fprintf(os.Stdout, "%16s: %s\n", label, value)
}
