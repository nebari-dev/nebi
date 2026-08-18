package cliclient

import (
	"context"
	"fmt"
	"net/url"
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

// ListFederatedIdentityReviews returns federated identity reviews.
func (c *Client) ListFederatedIdentityReviews(ctx context.Context, status string) ([]FederatedIdentityReview, error) {
	var reviews []FederatedIdentityReview
	path := "/admin/federated-identity-reviews"
	if status != "" {
		values := url.Values{}
		values.Set("status", status)
		path += "?" + values.Encode()
	}
	_, err := c.Get(ctx, path, &reviews)
	if err != nil {
		return nil, err
	}
	return reviews, nil
}

// ApproveFederatedIdentityReview approves a pending federated identity review.
func (c *Client) ApproveFederatedIdentityReview(ctx context.Context, reviewID string) (*FederatedIdentity, error) {
	var identity FederatedIdentity
	_, err := c.Post(ctx, fmt.Sprintf("/admin/federated-identity-reviews/%s/approve", reviewID), nil, &identity)
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

// RejectFederatedIdentityReview rejects a pending federated identity review.
func (c *Client) RejectFederatedIdentityReview(ctx context.Context, reviewID string) error {
	_, err := c.Post(ctx, fmt.Sprintf("/admin/federated-identity-reviews/%s/reject", reviewID), nil, nil)
	return err
}

// DiscardFederatedIdentityReview permanently deletes a federated identity review.
func (c *Client) DiscardFederatedIdentityReview(ctx context.Context, reviewID string) error {
	_, err := c.Delete(ctx, fmt.Sprintf("/admin/federated-identity-reviews/%s", reviewID))
	return err
}
