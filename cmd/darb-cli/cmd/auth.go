package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/openteams-ai/darb/cli/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login <url>",
	Short: "Login to Darb server",
	Long: `Login to a Darb server to enable server mode.

Example:
  darb login https://darb.company.com
  darb login http://localhost:8460`,
	Args: cobra.ExactArgs(1),
	Run:  runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from server",
	Long:  `Logout from the Darb server and clear stored credentials.`,
	Args:  cobra.NoArgs,
	Run:   runLogout,
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
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

	// Create API client for this server
	clientCfg := client.NewConfiguration()
	clientCfg.Servers = client.ServerConfigurations{
		{URL: serverURL + "/api/v1"},
	}
	apiClient := client.NewAPIClient(clientCfg)

	// Attempt login
	loginReq := client.AuthLoginRequest{
		Username: username,
		Password: password,
	}

	resp, httpResp, err := apiClient.AuthAPI.AuthLoginPost(context.Background()).Credentials(loginReq).Execute()
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

	// Save config
	cfg := &CLIConfig{
		ServerURL: serverURL,
		Token:     resp.GetToken(),
	}

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	user := resp.GetUser()
	fmt.Printf("Logged in as %s (%s)\n", user.GetUsername(), user.GetEmail())

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

	// Clear config
	cfg.ServerURL = ""
	cfg.Token = ""

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Logged out successfully")
}
