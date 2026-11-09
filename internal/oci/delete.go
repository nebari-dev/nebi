package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// ErrTagDeletionUnsupported is returned by DeleteTag when the registry
// refuses tag deletion through the OCI distribution API (HTTP 405 or an
// UNSUPPORTED error code). Such registries need their vendor API.
var ErrTagDeletionUnsupported = errors.New("this registry can't delete tags through the registry API")

// TagDeletionSupport reports whether Nebi can delete a tag on the registry
// at host through the OCI distribution API (issue #649, gap 6). It is a
// static check on well-known hosts that are known not to support it; any
// other registry is assumed to support it and DeleteTag reports
// ErrTagDeletionUnsupported if it turns out not to. reason is user-facing
// copy explaining why unpublish is unavailable.
func TagDeletionSupport(host string) (supported bool, reason string) {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(h, "/"); i >= 0 {
		h = h[:i]
	}
	switch h {
	case "ghcr.io":
		return false, "GitHub Container Registry can’t delete tags through the registry API. Delete the package version on GitHub instead."
	case "docker.io", "registry-1.docker.io", "index.docker.io", "registry.hub.docker.com":
		return false, "Docker Hub can’t delete tags through the registry API. Delete the tag on Docker Hub instead."
	}
	return true, ""
}

// DeleteTag removes tag from host[/namespace]/repo by sending
// DELETE /v2/<repository>/manifests/<tag>. Only the tag is removed: the
// manifest stays reachable through any other tag that points at it, which
// is why this never falls back to deleting by digest. A tag that does not
// exist is not an error.
func DeleteTag(ctx context.Context, reg Registry, repo, tag string) error {
	fullRepo := buildRepoRef(reg, repo)
	ref, err := registry.ParseReference(fullRepo + ":" + tag)
	if err != nil {
		return fmt.Errorf("invalid reference %s:%s: %w", fullRepo, tag, err)
	}
	scheme := "https"
	if reg.PlainHTTP {
		scheme = "http"
	}
	endpoint := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, ref.Host(), ref.Repository, ref.Reference)

	client := newAuthClient(reg.Username, reg.Password)
	if client == nil {
		client = auth.DefaultClient
	}
	ctx = auth.AppendRepositoryScope(ctx, ref, auth.ActionPull, auth.ActionDelete)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete tag %s:%s: %w", fullRepo, tag, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return nil
	case resp.StatusCode == http.StatusMethodNotAllowed:
		return ErrTagDeletionUnsupported
	}
	code, msg := registryErrorBody(body)
	if code == "UNSUPPORTED" {
		return ErrTagDeletionUnsupported
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("the registry refused to delete %s:%s (%d): %s. Check the registry credentials", fullRepo, tag, resp.StatusCode, msg)
	}
	return fmt.Errorf("delete tag %s:%s: registry returned %d: %s", fullRepo, tag, resp.StatusCode, msg)
}

// registryErrorBody extracts the first error of an OCI distribution error
// body ({"errors":[{"code","message"}]}).
func registryErrorBody(body []byte) (code, message string) {
	var e struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &e) == nil && len(e.Errors) > 0 {
		return e.Errors[0].Code, e.Errors[0].Message
	}
	return "", strings.TrimSpace(string(body))
}
