package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login <url>",
	Short: "Login to Nebi server",
	Long: `Login to a Nebi server to enable server mode.

Example:
  nebi login https://nebi.company.com
  nebi login http://localhost:8460`,
	Args: cobra.ExactArgs(1),
	Run:  runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from server",
	Long:  `Logout from the Nebi server and clear stored credentials.`,
	Args:  cobra.NoArgs,
	Run:   runLogout,
}

func runLogin(cmd *cobra.Command, args []string) {
	serverURL := args[0]

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

	// Create API client for this server (without auth for login)
	client := cliclient.NewWithoutAuth(serverURL)

	// Attempt login
	resp, err := client.Login(context.Background(), username, password)
	if err != nil {
		if cliclient.IsUnauthorized(err) {
			fmt.Fprintln(os.Stderr, "Error: Invalid username or password")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Could not connect to server: %v\n", err)
		}
		os.Exit(1)
	}

	// Save config
	cfg := &CLIConfig{
		ServerURL: serverURL,
		Token:     resp.Token,
	}

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Logged in as %s (%s)\n", resp.User.Username, resp.User.Email)

	configPath, _ := getConfigPath()
	fmt.Printf("Credentials saved to %s\n", configPath)
}

func runLogout(cmd *cobra.Command, args []string) {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cfg.ServerURL == "" && cfg.Token == "" {
		fmt.Println("Not logged in")
		return
	}

	// Clear config (including UseLocal so user gets prompted again)
	cfg.ServerURL = ""
	cfg.Token = ""
	cfg.UseLocal = false

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Logged out successfully")
}
