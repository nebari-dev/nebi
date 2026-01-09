package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var (
	loginUsername string
	loginPassword string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the Darb server",
	Long: `Authenticate with the Darb server and store the token for future requests.

Examples:
  # Interactive login (prompts for credentials)
  darb login

  # Login with flags
  darb login --username admin --password secret

  # Login to a different server
  darb login --server https://darb.example.com`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().StringVarP(&loginUsername, "username", "u", "", "Username")
	loginCmd.Flags().StringVarP(&loginPassword, "password", "p", "", "Password")
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Get credentials interactively if not provided
	username := loginUsername
	password := loginPassword

	if username == "" {
		fmt.Print("Username: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		username = strings.TrimSpace(input)
	}

	if password == "" {
		fmt.Print("Password: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // newline after password input
		password = string(bytePassword)
	}

	// Create API client
	apiClient := getAPIClient()

	// Perform login
	loginReq := client.AuthLoginRequest{
		Username: username,
		Password: password,
	}

	resp, httpResp, err := apiClient.AuthAPI.AuthLoginPost(cmd.Context()).
		Credentials(loginReq).
		Execute()

	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 401 {
			return fmt.Errorf("invalid credentials")
		}
		return fmt.Errorf("login failed: %w", err)
	}

	// Store token
	viper.Set("token", resp.Token)
	viper.Set("server", viper.GetString("server"))

	if err := saveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Successfully logged in as %s\n", username)

	return nil
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored authentication",
	Long:  `Remove the stored authentication token.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.Set("token", "")
		if err := saveConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Logged out successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authenticated user",
	Long:  `Display information about the currently authenticated user.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		user, httpResp, err := apiClient.AuthAPI.AuthMeGet(ctx).Execute()
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == 401 {
				return fmt.Errorf("not logged in. Run 'darb login' first")
			}
			return fmt.Errorf("failed to get user info: %w", err)
		}

		fmt.Printf("Username: %s\n", user.Username)
		fmt.Printf("Email:    %s\n", user.Email)
		fmt.Printf("ID:       %s\n", user.Id)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
