package remote

// Owner: U6 (sharing). The share dialog in the client, proxied to the
// server's access endpoints (internal/server/api routes_sharing.go). The
// server enforces who may see and change access; the client only checks
// input shape so obvious mistakes fail without a round trip.

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

func accessPath(serverProjectID string) string {
	return "/projects/" + url.PathEscape(serverProjectID) + "/access"
}

func principalPath(serverProjectID string, p shared.PrincipalRef) string {
	return accessPath(serverProjectID) + "/" + url.PathEscape(p.Kind) + "/" + url.PathEscape(p.ID)
}

func checkServerProjectID(id string) error {
	if strings.TrimSpace(id) == "" {
		return &shared.ValidationError{Field: "server_project_id", Message: "Server project ID is required"}
	}
	return nil
}

func checkShareRole(role string) error {
	if role != "edit" && role != "view" {
		return &shared.ValidationError{Field: "role", Message: "Role must be 'edit' or 'view'"}
	}
	return nil
}

func checkPrincipal(p shared.PrincipalRef) error {
	if p.Kind != "user" && p.Kind != "group" {
		return &shared.ValidationError{Field: "kind", Message: "Kind must be 'user' or 'group'"}
	}
	if strings.TrimSpace(p.ID) == "" {
		return &shared.ValidationError{Field: "principal_id", Message: "Principal ID is required"}
	}
	return nil
}

func normalizeAccess(list *shared.AccessList) *shared.AccessList {
	if list.Entries == nil {
		list.Entries = []shared.AccessEntry{}
	}
	return list
}

// ListAccess returns who has access to a server project.
func (s *Service) ListAccess(ctx context.Context, serverProjectID string) (*shared.AccessList, error) {
	if err := checkServerProjectID(serverProjectID); err != nil {
		return nil, err
	}
	var out shared.AccessList
	if err := s.ServerJSON(ctx, http.MethodGet, accessPath(serverProjectID), nil, &out); err != nil {
		return nil, err
	}
	return normalizeAccess(&out), nil
}

// GrantAccess adds people or teams with one role.
func (s *Service) GrantAccess(ctx context.Context, serverProjectID string, req shared.GrantAccessRequest) (*shared.AccessList, error) {
	if err := checkServerProjectID(serverProjectID); err != nil {
		return nil, err
	}
	if err := checkShareRole(req.Role); err != nil {
		return nil, err
	}
	if len(req.Principals) == 0 {
		return nil, &shared.ValidationError{Field: "principals", Message: "Add at least one person or team"}
	}
	for _, p := range req.Principals {
		if err := checkPrincipal(p); err != nil {
			return nil, err
		}
	}
	var out shared.AccessList
	if err := s.ServerJSON(ctx, http.MethodPost, accessPath(serverProjectID), req, &out); err != nil {
		return nil, err
	}
	return normalizeAccess(&out), nil
}

// UpdateAccess changes a principal's role.
func (s *Service) UpdateAccess(ctx context.Context, serverProjectID string, p shared.PrincipalRef, req shared.UpdateAccessRequest) (*shared.AccessList, error) {
	if err := checkServerProjectID(serverProjectID); err != nil {
		return nil, err
	}
	if err := checkPrincipal(p); err != nil {
		return nil, err
	}
	if err := checkShareRole(req.Role); err != nil {
		return nil, err
	}
	var out shared.AccessList
	if err := s.ServerJSON(ctx, http.MethodPut, principalPath(serverProjectID, p), req, &out); err != nil {
		return nil, err
	}
	return normalizeAccess(&out), nil
}

// RevokeAccess removes a principal's access.
func (s *Service) RevokeAccess(ctx context.Context, serverProjectID string, p shared.PrincipalRef) (*shared.AccessList, error) {
	if err := checkServerProjectID(serverProjectID); err != nil {
		return nil, err
	}
	if err := checkPrincipal(p); err != nil {
		return nil, err
	}
	var out shared.AccessList
	if err := s.ServerJSON(ctx, http.MethodDelete, principalPath(serverProjectID, p), nil, &out); err != nil {
		return nil, err
	}
	return normalizeAccess(&out), nil
}

// SearchPrincipals searches people and teams by name, email or team. An
// empty query returns an empty list without calling the server.
func (s *Service) SearchPrincipals(ctx context.Context, query string) ([]shared.Principal, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []shared.Principal{}, nil
	}
	var out []shared.Principal
	if err := s.ServerJSON(ctx, http.MethodGet, "/principals?q="+url.QueryEscape(q), nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []shared.Principal{}
	}
	return out, nil
}
