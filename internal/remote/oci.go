package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// latestTag is the ref Pull defaults to. The server side upserts a
// "latest" tag on every push; OCIRemote.Push mirrors that with an
// extra tag so the two backends resolve "give me the newest version"
// the same way.
const latestTag = "latest"

// OCIRemote implements Remote against an OCI Distribution registry,
// wrapping internal/oci for bundle push/pull and talking oras directly
// for listing (the existing oci.ListRepositories/ListTags helpers
// cannot reach plain-HTTP registries — a spike finding, see README).
type OCIRemote struct {
	// Host and Namespace locate the registry ("localhost:5000",
	// "quay.io") and the org/user prefix repositories live under
	// (may be empty).
	Host      string
	Namespace string
	// PlainHTTP talks to the registry over HTTP. Test/local registries
	// only.
	PlainHTTP bool
	// AllowAnonymous marks a public registry that needs no
	// authentication: SupportedAuthentication returns nil and all
	// operations run unauthenticated. Note this is *configuration*,
	// not discovery — an OCI registry cannot be asked up front which
	// schemes it wants (see README).
	AllowAnonymous bool

	creds *AuthenticationCredential // set by a successful Authenticate
}

var _ Remote = (*OCIRemote)(nil)

func (r *OCIRemote) SupportedAuthentication() []AuthenticationCredentialType {
	if r.AllowAnonymous {
		return nil
	}
	// Token here would be a registry-specific REST token (e.g. Quay's
	// API token, already special-cased in internal/oci); out of scope
	// for the spike.
	return []AuthenticationCredentialType{BasicAuthenticationCredentialType}
}

// Authenticate verifies basic credentials by listing the catalog. OCI
// has no session concept — every request re-sends credentials — so
// "authenticate" can only mean "prove the credentials work once and
// remember them".
func (r *OCIRemote) Authenticate(ctx context.Context, ac AuthenticationCredential) error {
	if err := ac.Validate(); err != nil {
		return err
	}
	if ac.Type != BasicAuthenticationCredentialType {
		return fmt.Errorf("oci remote: unsupported auth type %q", ac.Type)
	}
	reg, err := r.registry(&ac)
	if err != nil {
		return err
	}
	if err := reg.Repositories(ctx, "", func([]string) error { return nil }); err != nil {
		return mapOCIError(err)
	}
	r.creds = &ac
	return nil
}

func (r *OCIRemote) List(ctx context.Context) ([]WorkspacePreview, error) {
	if err := r.requireAuth(); err != nil {
		return nil, err
	}
	reg, err := r.registry(r.creds)
	if err != nil {
		return nil, err
	}
	var repoNames []string
	err = reg.Repositories(ctx, "", func(page []string) error {
		repoNames = append(repoNames, page...)
		return nil
	})
	if err != nil {
		return nil, mapOCIError(err)
	}

	prefix := ""
	if r.Namespace != "" {
		prefix = r.Namespace + "/"
	}
	var previews []WorkspacePreview
	for _, full := range repoNames {
		if prefix != "" && (len(full) <= len(prefix) || full[:len(prefix)] != prefix) {
			continue
		}
		name := full[len(prefix):]
		repo, err := reg.Repository(ctx, full)
		if err != nil {
			return nil, mapOCIError(err)
		}
		var refs []string
		hasLatest := false
		err = repo.Tags(ctx, "", func(tags []string) error {
			for _, t := range tags {
				if t == latestTag {
					hasLatest = true
					continue // "latest" is an alias, not a version of its own
				}
				refs = append(refs, t)
			}
			return nil
		})
		if err != nil {
			return nil, mapOCIError(err)
		}
		p := WorkspacePreview{ID: name, Name: name, Refs: refs}
		if hasLatest {
			p.LatestRef = latestTag
		} else if len(refs) > 0 {
			p.LatestRef = refs[len(refs)-1]
		}
		// NOTE: a real implementation must also filter to nebi-shaped
		// repositories (cf. oci.FilterNebiRepositories), which costs a
		// manifest fetch per repo. Skipped in the spike; see README.
		previews = append(previews, p)
	}
	return previews, nil
}

func (r *OCIRemote) Pull(ctx context.Context, id string) (Workspace, error) {
	if err := r.requireAuth(); err != nil {
		return Workspace{}, err
	}
	name, ref := splitIDRef(id)
	if ref == "" {
		ref = latestTag
	}
	res, err := oci.PullBundle(ctx, r.repoRef(name), ref, r.pullOptions())
	if err != nil {
		return Workspace{}, mapOCIError(err)
	}
	ws := Workspace{
		WorkspacePreview: WorkspacePreview{ID: name, Name: name},
		Ref:              ref,
		PixiToml:         res.PixiToml,
		PixiLock:         res.PixiLock,
	}
	for _, a := range res.Assets {
		ws.AssetPaths = append(ws.AssetPaths, a.Path)
	}
	return ws, nil
}

// Push publishes ws as repo <ws.Name>:<ws.Ref> (also updating
// "latest"). The conflict check is read-before-write — OCI push has no
// native compare-and-set on tags, so a concurrent pusher can still win
// the race (spike finding).
func (r *OCIRemote) Push(ctx context.Context, ws Workspace) error {
	if err := r.requireAuth(); err != nil {
		return err
	}
	if ws.Name == "" || ws.Ref == "" {
		return errors.New("oci remote: push requires Name and Ref")
	}
	if ws.Ref == latestTag {
		return fmt.Errorf("oci remote: %q is reserved", latestTag)
	}

	reg, err := r.registry(r.creds)
	if err != nil {
		return err
	}
	repo, err := reg.Repository(ctx, r.repoName(ws.Name))
	if err != nil {
		return mapOCIError(err)
	}
	if _, err := repo.Resolve(ctx, ws.Ref); err == nil {
		return fmt.Errorf("%w: tag %q on %s", ErrConflict, ws.Ref, ws.Name)
	} else if !errors.Is(mapOCIError(err), ErrNotFound) {
		return mapOCIError(err)
	}

	// The publishers take a directory, not content — stage the core
	// files in a temp dir (this is also what the server-side publish
	// path does for managed workspaces).
	dir, err := os.MkdirTemp("", "nebi-remote-push-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(ws.PixiToml), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte(ws.PixiLock), 0o644); err != nil {
		return err
	}

	_, err = oci.PublishPixiOnly(ctx, dir, r.ociRegistry(), ws.Name, ws.Ref,
		oci.WithExtraTags(latestTag))
	return mapOCIError(err)
}

func (r *OCIRemote) requireAuth() error {
	if r.AllowAnonymous || r.creds != nil {
		return nil
	}
	return ErrNotAuthenticated
}

// repoName is the host-relative repository path; repoRef prefixes it
// with the host for the internal/oci helpers that want a full ref.
func (r *OCIRemote) repoName(name string) string {
	if r.Namespace != "" {
		return r.Namespace + "/" + name
	}
	return name
}

func (r *OCIRemote) repoRef(name string) string {
	return r.Host + "/" + r.repoName(name)
}

func (r *OCIRemote) registry(ac *AuthenticationCredential) (*remote.Registry, error) {
	reg, err := remote.NewRegistry(r.Host)
	if err != nil {
		return nil, err
	}
	reg.PlainHTTP = r.PlainHTTP
	if ac != nil {
		reg.Client = &auth.Client{
			Credential: auth.StaticCredential(r.Host, auth.Credential{
				Username: ac.Username,
				Password: ac.Password,
			}),
		}
	}
	return reg, nil
}

func (r *OCIRemote) ociRegistry() oci.Registry {
	reg := oci.Registry{Host: r.Host, Namespace: r.Namespace, PlainHTTP: r.PlainHTTP}
	if r.creds != nil {
		reg.Username = r.creds.Username
		reg.Password = r.creds.Password
	}
	return reg
}

func (r *OCIRemote) pullOptions() oci.PullOptions {
	opts := oci.PullOptions{PlainHTTP: r.PlainHTTP}
	if r.creds != nil {
		opts.Username = r.creds.Username
		opts.Password = r.creds.Password
	}
	return opts
}

// mapOCIError folds oras transport errors into the package sentinels.
// internal/oci wraps with %w throughout, so the chain survives.
func mapOCIError(err error) error {
	if err == nil {
		return nil
	}
	var er *errcode.ErrorResponse
	if errors.As(err, &er) {
		switch er.StatusCode {
		case 401:
			return fmt.Errorf("%w: %v", ErrUnauthorized, err)
		case 403:
			return fmt.Errorf("%w: %v", ErrForbidden, err)
		case 404:
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		}
	}
	if errors.Is(err, errdef.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
