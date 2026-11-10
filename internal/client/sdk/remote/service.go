// Package remote is the client SDK for the connected team server: the
// connection itself, server project listing, pull, push, version sync
// markers, update-available, and the share dialog proxied to the server
// (issue #649). It talks to internal/server/api endpoints over HTTP.
//
// File ownership: service.go is frozen; connection.go, server_projects.go,
// pull.go and sync.go belong to U5; sharing.go belongs to U6.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/store"
)

// Service is the remote SDK.
type Service struct {
	deps  shared.Deps
	local *local.Service
}

// New builds the remote SDK.
func New(deps shared.Deps, l *local.Service) *Service {
	return &Service{deps: deps, local: l}
}

// endpoint returns the connected server URL and token from
// store_config/store_credentials.
func (s *Service) endpoint() (string, string, error) {
	var cfg store.Config
	if err := s.deps.DB.First(&cfg, 1).Error; err != nil || strings.TrimSpace(cfg.ServerURL) == "" {
		return "", "", shared.ErrNotConnected
	}
	var creds store.Credentials
	_ = s.deps.DB.First(&creds, 1).Error
	return strings.TrimRight(cfg.ServerURL, "/"), creds.Token, nil
}

func (s *Service) httpClient() *http.Client {
	if s.deps.HTTPClient != nil {
		return s.deps.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// ServerJSON calls the connected team server. path is relative to /api/v1
// (e.g. "/catalog/projects"). body (if non-nil) is sent as JSON and the
// response decoded into out (if non-nil). Non-2xx responses become
// *shared.RemoteError with the server's {"error": ...} message; transport
// failures become *shared.RemoteError with Status 0. Returns
// shared.ErrNotConnected when no server is connected.
func (s *Service) ServerJSON(ctx context.Context, method, path string, body, out any) error {
	base, token, err := s.endpoint()
	if err != nil {
		return err
	}
	return s.doJSON(ctx, base, token, method, path, body, out)
}

// doJSON is ServerJSON against an explicit server (used while connecting).
func (s *Service) doJSON(ctx context.Context, base, token, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+"/api/v1"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return &shared.RemoteError{Status: 0, Message: fmt.Sprintf("server unreachable: %v", err)}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return &shared.RemoteError{Status: 0, Message: fmt.Sprintf("reading server response: %v", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return &shared.RemoteError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode server response: %w", err)
		}
	}
	return nil
}
