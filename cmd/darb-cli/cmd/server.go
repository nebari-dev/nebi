package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage server connections",
	Long: `Configure and manage connections to local and remote Nebi servers.

The CLI can connect to either a local auto-spawned server or remote servers.
By default, it uses a local server that starts automatically when needed.

Examples:
  nebi server list                    # List all configured servers
  nebi server status                  # Show current server status
  nebi server login https://nebi.io   # Login to a remote server
  nebi server login local             # Switch to local server
  nebi server logout                  # Logout and switch to local`,
}

var serverListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured servers",
	Long:    `List all configured servers with their connection status.`,
	Args:    cobra.NoArgs,
	Run:     runServerList,
}

var serverLoginCmd = &cobra.Command{
	Use:   "login <url|local>",
	Short: "Login to a server",
	Long: `Login to a remote Nebi server or switch to local server.

For remote servers, you will be prompted for credentials.
For local server, no authentication is needed.

Examples:
  nebi server login https://nebi.company.com   # Login to remote server
  nebi server login local                      # Switch to local server`,
	Args: cobra.ExactArgs(1),
	Run:  runServerLogin,
}

var serverLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from current server",
	Long:  `Logout from the current remote server and switch back to local server.`,
	Args:  cobra.NoArgs,
	Run:   runServerLogout,
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current server status",
	Long:  `Show detailed information about the current server connection.`,
	Args:  cobra.NoArgs,
	Run:   runServerStatus,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverLoginCmd)
	serverCmd.AddCommand(serverLogoutCmd)
	serverCmd.AddCommand(serverStatusCmd)
}

func runServerList(cmd *cobra.Command, args []string) {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tURL\tSTATUS")

	// Always show local server first
	status, port, _, _ := getLocalServerStatus()
	marker := "  "
	if cfg.CurrentServer == "local" || cfg.CurrentServer == "" {
		marker = "* "
	}
	localURL := "localhost"
	if port > 0 {
		localURL = fmt.Sprintf("localhost:%d", port)
	}
	fmt.Fprintf(w, "%slocal\t%s\t%s\n", marker, localURL, status)

	// Show remote servers
	for name, server := range cfg.Servers {
		if name == "local" {
			continue
		}
		marker = "  "
		if cfg.CurrentServer == name {
			marker = "* "
		}
		status := "configured"
		if server.Token != "" {
			status = "authenticated"
		}
		fmt.Fprintf(w, "%s%s\t%s\t%s\n", marker, name, server.URL, status)
	}
	w.Flush()
}

func runServerLogin(cmd *cobra.Command, args []string) {
	target := args[0]

	if target == "local" {
		// Switch to local server
		cfg, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		cfg.CurrentServer = "local"
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Switched to local server")
		return
	}

	// Login to remote server
	serverURL := target

	// Normalize URL
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "https://" + serverURL
	}
	serverURL = strings.TrimSuffix(serverURL, "/")

	// Prompt for credentials
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading username: %v\n", err)
		os.Exit(1)
	}
	username = strings.TrimSpace(username)

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println() // newline after password
	password := string(passwordBytes)

	// Create API client for this server
	clientCfg := client.NewConfiguration()
	clientCfg.Servers = client.ServerConfigurations{
		{URL: serverURL + "/api/v1"},
	}
	tempClient := client.NewAPIClient(clientCfg)

	// Attempt login
	loginReq := client.AuthLoginRequest{
		Username: username,
		Password: password,
	}

	resp, httpResp, err := tempClient.AuthAPI.AuthLoginPost(context.Background()).Credentials(loginReq).Execute()
	if err != nil {
		if httpResp != nil {
			switch httpResp.StatusCode {
			case 401:
				fmt.Fprintln(os.Stderr, "Error: Invalid username or password")
			case 400:
				fmt.Fprintln(os.Stderr, "Error: Invalid request")
			default:
				fmt.Fprintf(os.Stderr, "Error: Login failed (%d)\n", httpResp.StatusCode)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: Could not connect to server: %v\n", err)
		}
		os.Exit(1)
	}

	// Generate a name for this server based on the URL
	serverName := generateServerName(serverURL)

	// Save config
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	cfg.Servers[serverName] = ServerConfig{
		URL:   serverURL,
		Token: resp.GetToken(),
	}
	cfg.CurrentServer = serverName

	// Reset cached client and token since we're switching servers
	apiClient = nil
	cachedToken = ""
	cachedConfig = nil

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	user := resp.GetUser()
	fmt.Printf("Logged in as %s (%s)\n", user.GetUsername(), user.GetEmail())
	fmt.Printf("Server '%s' is now active\n", serverName)
}

func runServerLogout(cmd *cobra.Command, args []string) {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg.CurrentServer == "local" || cfg.CurrentServer == "" {
		fmt.Println("Already using local server (no logout needed)")
		return
	}

	// Clear token for current server
	if server, ok := cfg.Servers[cfg.CurrentServer]; ok {
		server.Token = ""
		cfg.Servers[cfg.CurrentServer] = server
	}

	oldServer := cfg.CurrentServer
	cfg.CurrentServer = "local"

	// Reset cached client and token
	apiClient = nil
	cachedToken = ""
	cachedConfig = nil

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Logged out from '%s', switched to local server\n", oldServer)
}

func runServerStatus(cmd *cobra.Command, args []string) {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg.CurrentServer == "local" || cfg.CurrentServer == "" {
		// Show detailed local server info
		status, port, pid, uptime := getLocalServerStatus()
		paths, _ := getLocalServerPaths()

		fmt.Println("Server:     local")
		fmt.Printf("Status:     %s\n", status)

		if port > 0 {
			fmt.Printf("Port:       %d\n", port)
		}
		if pid > 0 {
			fmt.Printf("PID:        %d\n", pid)
		}
		if uptime > 0 {
			fmt.Printf("Uptime:     %s\n", formatDuration(uptime))
		}
		if paths != nil {
			fmt.Printf("Logs:       %s\n", paths.LogFile)
			fmt.Printf("Database:   %s\n", paths.Database)
		}
	} else {
		// Show remote server info
		server, ok := cfg.Servers[cfg.CurrentServer]
		if !ok {
			fmt.Printf("Server:     %s (not found in config)\n", cfg.CurrentServer)
			return
		}

		fmt.Printf("Server:     %s\n", cfg.CurrentServer)
		fmt.Printf("URL:        %s\n", server.URL)

		if server.Token == "" {
			fmt.Println("Status:     not authenticated")
			return
		}

		fmt.Println("Status:     authenticated")

		// Try to get user info
		client, err := getAPIClient()
		if err == nil {
			ctx, err := getAuthContext()
			if err == nil {
				user, _, err := client.AuthAPI.AuthMeGet(ctx).Execute()
				if err == nil {
					fmt.Printf("User:       %s\n", user.GetUsername())
				}
			}
		}
	}
}

// generateServerName creates a short name from a URL
func generateServerName(url string) string {
	// Remove protocol
	name := strings.TrimPrefix(url, "https://")
	name = strings.TrimPrefix(name, "http://")

	// Take the hostname
	if idx := strings.Index(name, "/"); idx > 0 {
		name = name[:idx]
	}

	// Remove port
	if idx := strings.Index(name, ":"); idx > 0 {
		name = name[:idx]
	}

	// Replace dots with hyphens for readability
	name = strings.ReplaceAll(name, ".", "-")

	return name
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
