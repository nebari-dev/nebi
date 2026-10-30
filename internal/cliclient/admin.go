package cliclient

import (
	"context"
	"fmt"
)

// ListUsers returns all users (admin only).
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var users []User
	_, err := c.Get(ctx, "/admin/users", &users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// ListAuditLogs returns audit logs with optional filters (admin only).
func (c *Client) ListAuditLogs(ctx context.Context, userID, action string) ([]AuditLog, error) {
	path := "/admin/audit-logs"
	params := []string{}
	if userID != "" {
		params = append(params, fmt.Sprintf("user_id=%s", userID))
	}
	if action != "" {
		params = append(params, fmt.Sprintf("action=%s", action))
	}
	if len(params) > 0 {
		path += "?"
		for i, p := range params {
			if i > 0 {
				path += "&"
			}
			path += p
		}
	}

	var logs []AuditLog
	_, err := c.Get(ctx, path, &logs)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

// GetDashboardStats returns admin dashboard statistics (admin only).
func (c *Client) GetDashboardStats(ctx context.Context) (*DashboardStats, error) {
	var stats DashboardStats
	_, err := c.Get(ctx, "/admin/dashboard/stats", &stats)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}
