package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/aktech/darb/cli/client"
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin commands (requires admin privileges)",
	Long:  `Administrative commands for managing users, permissions, and system settings.`,
}

func init() {
	rootCmd.AddCommand(adminCmd)
}

// ==================== Users ====================

var adminUsersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
	Long:  `List, create, and manage users.`,
}

func init() {
	adminCmd.AddCommand(adminUsersCmd)
}

// List users
var adminUsersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		users, httpResp, err := apiClient.AdminAPI.AdminUsersGet(ctx).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "list users")
		}

		if len(users) == 0 {
			fmt.Println("No users found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tADMIN\tCREATED")
		for _, user := range users {
			isAdmin := "no"
			if user.IsAdmin != nil && *user.IsAdmin {
				isAdmin = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				user.Id,
				user.Username,
				user.Email,
				isAdmin,
				user.CreatedAt,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	adminUsersCmd.AddCommand(adminUsersListCmd)
}

// Create user
var (
	createUserUsername string
	createUserEmail    string
	createUserPassword string
	createUserIsAdmin  bool
)

var adminUsersCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		req := client.HandlersCreateUserRequest{
			Username: createUserUsername,
			Email:    createUserEmail,
			Password: createUserPassword,
		}
		if createUserIsAdmin {
			req.IsAdmin = &createUserIsAdmin
		}

		user, httpResp, err := apiClient.AdminAPI.AdminUsersPost(ctx).
			User(req).
			Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "create user")
		}

		fmt.Printf("User created successfully!\n")
		fmt.Printf("  ID:       %s\n", user.Id)
		fmt.Printf("  Username: %s\n", user.Username)
		fmt.Printf("  Email:    %s\n", user.Email)

		return nil
	},
}

func init() {
	adminUsersCmd.AddCommand(adminUsersCreateCmd)

	adminUsersCreateCmd.Flags().StringVar(&createUserUsername, "username", "", "Username (required)")
	adminUsersCreateCmd.Flags().StringVar(&createUserEmail, "email", "", "Email (required)")
	adminUsersCreateCmd.Flags().StringVar(&createUserPassword, "password", "", "Password (required)")
	adminUsersCreateCmd.Flags().BoolVar(&createUserIsAdmin, "admin", false, "Make user an admin")

	adminUsersCreateCmd.MarkFlagRequired("username")
	adminUsersCreateCmd.MarkFlagRequired("email")
	adminUsersCreateCmd.MarkFlagRequired("password")
}

// Get user
var adminUsersGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get user details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		user, httpResp, err := apiClient.AdminAPI.AdminUsersIdGet(ctx, args[0]).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "get user")
		}

		isAdmin := "no"
		if user.IsAdmin != nil && *user.IsAdmin {
			isAdmin = "yes"
		}

		fmt.Printf("ID:       %s\n", user.Id)
		fmt.Printf("Username: %s\n", user.Username)
		fmt.Printf("Email:    %s\n", user.Email)
		fmt.Printf("Admin:    %s\n", isAdmin)
		fmt.Printf("Created:  %s\n", user.CreatedAt)

		return nil
	},
}

func init() {
	adminUsersCmd.AddCommand(adminUsersGetCmd)
}

// Delete user
var adminUsersDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		httpResp, err := apiClient.AdminAPI.AdminUsersIdDelete(ctx, args[0]).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "delete user")
		}

		fmt.Printf("User %s deleted successfully\n", args[0])
		return nil
	},
}

func init() {
	adminUsersCmd.AddCommand(adminUsersDeleteCmd)
}

// Toggle admin
var adminUsersToggleAdminCmd = &cobra.Command{
	Use:   "toggle-admin [id]",
	Short: "Toggle admin status for a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		user, httpResp, err := apiClient.AdminAPI.AdminUsersIdToggleAdminPost(ctx, args[0]).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "toggle admin status")
		}

		status := "revoked"
		if user.IsAdmin != nil && *user.IsAdmin {
			status = "granted"
		}

		fmt.Printf("Admin status %s for user %s\n", status, user.Username)
		return nil
	},
}

func init() {
	adminUsersCmd.AddCommand(adminUsersToggleAdminCmd)
}

// ==================== Roles ====================

var adminRolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "Manage roles",
	Long:  `List available roles.`,
}

func init() {
	adminCmd.AddCommand(adminRolesCmd)
}

var adminRolesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all roles",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		roles, httpResp, err := apiClient.AdminAPI.AdminRolesGet(ctx).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "list roles")
		}

		if len(roles) == 0 {
			fmt.Println("No roles found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tDESCRIPTION")
		for _, role := range roles {
			description := ""
			if role.Description != nil {
				description = *role.Description
			}
			fmt.Fprintf(w, "%d\t%s\t%s\n",
				role.Id,
				role.Name,
				description,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	adminRolesCmd.AddCommand(adminRolesListCmd)
}

// ==================== Permissions ====================

var adminPermissionsCmd = &cobra.Command{
	Use:   "permissions",
	Short: "Manage permissions",
	Long:  `Grant, list, and revoke permissions.`,
}

func init() {
	adminCmd.AddCommand(adminPermissionsCmd)
}

var adminPermissionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all permissions",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		permissions, httpResp, err := apiClient.AdminAPI.AdminPermissionsGet(ctx).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "list permissions")
		}

		if len(permissions) == 0 {
			fmt.Println("No permissions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUSER_ID\tENVIRONMENT_ID\tROLE_ID")
		for _, perm := range permissions {
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\n",
				perm.Id,
				perm.UserId,
				perm.EnvironmentId,
				perm.RoleId,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	adminPermissionsCmd.AddCommand(adminPermissionsListCmd)
}

// Grant permission
var (
	grantPermUserID  string
	grantPermEnvID   string
	grantPermRoleID  int32
)

var adminPermissionsGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant permission to a user",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		req := client.HandlersGrantPermissionRequest{
			UserId:        grantPermUserID,
			EnvironmentId: grantPermEnvID,
			RoleId:        grantPermRoleID,
		}

		perm, httpResp, err := apiClient.AdminAPI.AdminPermissionsPost(ctx).
			Permission(req).
			Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "grant permission")
		}

		fmt.Printf("Permission granted successfully!\n")
		fmt.Printf("  ID: %d\n", perm.Id)

		return nil
	},
}

func init() {
	adminPermissionsCmd.AddCommand(adminPermissionsGrantCmd)

	adminPermissionsGrantCmd.Flags().StringVar(&grantPermUserID, "user-id", "", "User ID (required)")
	adminPermissionsGrantCmd.Flags().StringVar(&grantPermEnvID, "environment-id", "", "Environment ID (required)")
	adminPermissionsGrantCmd.Flags().Int32Var(&grantPermRoleID, "role-id", 0, "Role ID (required)")

	adminPermissionsGrantCmd.MarkFlagRequired("user-id")
	adminPermissionsGrantCmd.MarkFlagRequired("environment-id")
	adminPermissionsGrantCmd.MarkFlagRequired("role-id")
}

// Revoke permission
var adminPermissionsRevokeCmd = &cobra.Command{
	Use:   "revoke [id]",
	Short: "Revoke a permission",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		httpResp, err := apiClient.AdminAPI.AdminPermissionsIdDelete(ctx, args[0]).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "revoke permission")
		}

		fmt.Printf("Permission %s revoked successfully\n", args[0])
		return nil
	},
}

func init() {
	adminPermissionsCmd.AddCommand(adminPermissionsRevokeCmd)
}

// ==================== Audit Logs ====================

var (
	auditLogsUserID string
	auditLogsAction string
)

var adminAuditLogsCmd = &cobra.Command{
	Use:   "audit-logs",
	Short: "View audit logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		req := apiClient.AdminAPI.AdminAuditLogsGet(ctx)
		if auditLogsUserID != "" {
			req = req.UserId(auditLogsUserID)
		}
		if auditLogsAction != "" {
			req = req.Action(auditLogsAction)
		}

		logs, httpResp, err := req.Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "list audit logs")
		}

		if len(logs) == 0 {
			fmt.Println("No audit logs found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tUSER_ID\tACTION\tRESOURCE")
		for _, log := range logs {
			resource := ""
			if log.Resource != nil {
				resource = *log.Resource
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				log.Timestamp,
				log.UserId,
				log.Action,
				resource,
			)
		}
		w.Flush()

		return nil
	},
}

func init() {
	adminCmd.AddCommand(adminAuditLogsCmd)

	adminAuditLogsCmd.Flags().StringVar(&auditLogsUserID, "user-id", "", "Filter by user ID")
	adminAuditLogsCmd.Flags().StringVar(&auditLogsAction, "action", "", "Filter by action")
}

// ==================== Dashboard ====================

var adminDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "View dashboard statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient := getAPIClient()
		ctx := getAuthContext()

		stats, httpResp, err := apiClient.AdminAPI.AdminDashboardStatsGet(ctx).Execute()
		if err != nil {
			return handleAdminError(httpResp, err, "get dashboard stats")
		}

		fmt.Printf("Dashboard Statistics\n")
		fmt.Printf("====================\n")
		fmt.Printf("Total Disk Usage: %s (%d bytes)\n",
			stats.TotalDiskUsageFormatted,
			stats.TotalDiskUsageBytes,
		)

		return nil
	},
}

func init() {
	adminCmd.AddCommand(adminDashboardCmd)
}

// ==================== Helpers ====================

func handleAdminError(httpResp *http.Response, err error, action string) error {
	if httpResp != nil {
		switch httpResp.StatusCode {
		case 401:
			return fmt.Errorf("not logged in. Run 'darb login' first")
		case 403:
			return fmt.Errorf("permission denied. Admin privileges required")
		case 404:
			return fmt.Errorf("resource not found")
		}
	}
	return fmt.Errorf("failed to %s: %w", action, err)
}
