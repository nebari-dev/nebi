package oci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	// MediaTypePixiConfig is the media type for pixi config (empty JSON)
	MediaTypePixiConfig = "application/vnd.pixi.config.v1+toml"
	// MediaTypePixiToml is the media type for pixi.toml manifest
	MediaTypePixiToml = "application/vnd.pixi.toml.v1+toml"
	// MediaTypePixiLock is the media type for pixi.lock lockfile
	MediaTypePixiLock = "application/vnd.pixi.lock.v1+yaml"
	// MediaTypeNebiAsset is the media type for arbitrary bundled workspace
	// files. Path is carried in the layer's AnnotationTitle.
	MediaTypeNebiAsset = "application/vnd.nebi.asset.v1"
)

// defaultConcurrency is the fallback parallelism for blob transfers when
// no concurrency option is provided.
const defaultConcurrency = 8

// Registry identifies an OCI registry endpoint plus credentials. Host is
// the scheme-less hostname (e.g. "ghcr.io", "localhost:5000"). PlainHTTP
// forces HTTP — dev/test registries only.
type Registry struct {
	Host      string
	Namespace string
	Username  string
	Password  string
	PlainHTTP bool
}

// Asset is a single file bundled into a Nebi OCI artifact beyond the core
// pixi.toml / pixi.lock layers. RelPath is the bundle-relative path (forward
// slashes); AbsPath is the resolved on-disk source. Callers normally never
// construct this — Preview and the internal walker do.
type Asset struct {
	RelPath string
	AbsPath string
	Size    int64
}

// PublishResult summarizes a successful publish.
type PublishResult struct {
	Repository string // fully-qualified host/namespace/repo
	Tag        string
	Digest     string // sha256:... of the manifest
	AssetCount int    // files bundled beyond pixi.toml/pixi.lock
}

// PublishOption tunes Publish / PublishPixiOnly. Zero options = defaults.
type PublishOption func(*publishConfig)

type publishConfig struct {
	extraTags      []string
	concurrency    int
	progress       func(label string, pushed, total int)
	assetsOverride *[]Asset // non-nil when WithAssets was used
}

// WithExtraTags applies additional tags to the manifest after the primary
// tag lands. There is no implicit "latest" — callers opt in explicitly.
func WithExtraTags(tags ...string) PublishOption {
	return func(c *publishConfig) { c.extraTags = append([]string(nil), tags...) }
}

// WithConcurrency bounds parallel blob pushes. Values ≤ 0 fall back to
// the default (8).
func WithConcurrency(n int) PublishOption {
	return func(c *publishConfig) { c.concurrency = n }
}

// WithProgress installs a per-blob progress callback. Called once per
// blob after it completes successfully. The callback must be safe to call
// concurrently.
func WithProgress(fn func(label string, pushed, total int)) PublishOption {
	return func(c *publishConfig) { c.progress = fn }
}

// withAssets bypasses the workspace walker and publishes the supplied
// list verbatim. Unexported on purpose: the only legitimate callers are
// the walker and Preview, both of which produce validated Asset values.
// Leaving it exported would let Go callers supply AbsPaths that escape
// the workspace — a non-threat today (package is `internal/`) but a
// footgun worth not planting.
func withAssets(assets []Asset) PublishOption {
	return func(c *publishConfig) {
		cp := append([]Asset(nil), assets...)
		c.assetsOverride = &cp
	}
}

// Publish publishes the workspace at workspaceDir to reg/repo:tag. The
// workspace is walked using hardcoded drops (.git/, .pixi/) →
// [tool.nebi.bundle].include → .gitignore → [tool.nebi.bundle].exclude →
// force-include of pixi.toml/pixi.lock. pixi.toml and pixi.lock always
// become typed layers 0 and 1; every surviving file rides as a
// MediaTypeNebiAsset layer. Unsafe asset paths abort before any blob
// leaves the process.
func Publish(
	ctx context.Context,
	workspaceDir string,
	reg Registry,
	repo, tag string,
	opts ...PublishOption,
) (PublishResult, error) {
	cfg := resolveConfig(opts)

	// Reject a symlinked pixi.toml up front — before we parse it — so
	// a hostile link to a non-TOML target surfaces as a clear
	// "regular file" error rather than a confusing parse error.
	if err := assertCoreFile(filepath.Join(workspaceDir, "pixi.toml")); err != nil {
		return PublishResult{}, err
	}

	var assets []Asset
	if cfg.assetsOverride != nil {
		assets = *cfg.assetsOverride
	} else {
		bundleCfg, err := loadBundleConfig(filepath.Join(workspaceDir, "pixi.toml"))
		if err != nil {
			return PublishResult{}, fmt.Errorf("invalid bundle config: %w", err)
		}
		files, err := walkBundle(workspaceDir, bundleCfg)
		if err != nil {
			return PublishResult{}, fmt.Errorf("walk workspace: %w", err)
		}
		// publishBundle strips core files unconditionally; pass the
		// walker output through.
		assets = files
	}
	return publishBundle(ctx, workspaceDir, reg, repo, tag, assets, cfg)
}

// PublishPixiOnly publishes only pixi.toml and pixi.lock from coreDir,
// producing a legacy two-layer bundle byte-compatible with pre-bundle
// artifacts. The walker is never invoked; stray files in coreDir are
// ignored. Intended for server-side publish, where no user workspace
// exists on disk.
func PublishPixiOnly(
	ctx context.Context,
	coreDir string,
	reg Registry,
	repo, tag string,
	opts ...PublishOption,
) (PublishResult, error) {
	cfg := resolveConfig(opts)
	return publishBundle(ctx, coreDir, reg, repo, tag, nil, cfg)
}

// Preview returns the files Publish would bundle, in deterministic order,
// without touching the network. Useful for pre-publish confirmation UI
// and future "nebi bundle ls"-style commands.
func Preview(ctx context.Context, workspaceDir string) ([]Asset, error) {
	_ = ctx
	bundleCfg, err := loadBundleConfig(filepath.Join(workspaceDir, "pixi.toml"))
	if err != nil {
		return nil, fmt.Errorf("invalid bundle config: %w", err)
	}
	files, err := walkBundle(workspaceDir, bundleCfg)
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}
	return files, nil
}

func resolveConfig(opts []PublishOption) *publishConfig {
	cfg := &publishConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.concurrency <= 0 {
		cfg.concurrency = defaultConcurrency
	}
	return cfg
}

// assertCoreFile verifies that path exists and is a regular file
// (not a symlink, device, or directory). Called for pixi.toml / pixi.lock
// before the publisher hands them to file.Store, whose Add follows
// symlinks when reading.
func assertCoreFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pixi files not found: %s", path)
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file, got mode %s", path, info.Mode())
	}
	return nil
}

// stripCoreFiles removes pixi.toml / pixi.lock from an asset slice so
// they don't double up on the typed core layers the publisher emits.
func stripCoreFiles(in []Asset) []Asset {
	out := make([]Asset, 0, len(in))
	for _, a := range in {
		if a.RelPath == "pixi.toml" || a.RelPath == "pixi.lock" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// publishBundle is the shared pipeline behind Publish and PublishPixiOnly.
func publishBundle(
	ctx context.Context,
	dir string,
	reg Registry,
	repo, tag string,
	assets []Asset,
	cfg *publishConfig,
) (PublishResult, error) {
	pixiTomlPath := filepath.Join(dir, "pixi.toml")
	pixiLockPath := filepath.Join(dir, "pixi.lock")
	// Lstat (not Stat) so a symlink at pixi.toml / pixi.lock pointing
	// outside the workspace is rejected rather than silently bundled —
	// file.Store.Add follows symlinks when it reads the file, so this
	// check is the only thing between a hostile symlink and an artifact
	// that leaks the target file's contents.
	if err := assertCoreFile(pixiTomlPath); err != nil {
		return PublishResult{}, err
	}
	if err := assertCoreFile(pixiLockPath); err != nil {
		return PublishResult{}, err
	}

	assets = stripCoreFiles(assets)
	paths := make([]string, 0, len(assets))
	for _, a := range assets {
		paths = append(paths, a.RelPath)
	}
	if err := validateAssetPaths(paths); err != nil {
		return PublishResult{}, fmt.Errorf("unsafe path in bundle: %w", err)
	}

	fullRepo := buildRepoRef(reg, repo)

	fs, err := file.New(dir)
	if err != nil {
		return PublishResult{}, fmt.Errorf("failed to create file store: %w", err)
	}
	defer fs.Close()

	tomlDesc, err := fs.Add(ctx, "pixi.toml", MediaTypePixiToml, "")
	if err != nil {
		return PublishResult{}, fmt.Errorf("failed to add pixi.toml: %w", err)
	}
	tomlDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "pixi.toml"}

	lockDesc, err := fs.Add(ctx, "pixi.lock", MediaTypePixiLock, "")
	if err != nil {
		return PublishResult{}, fmt.Errorf("failed to add pixi.lock: %w", err)
	}
	lockDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "pixi.lock"}

	layers := make([]ocispec.Descriptor, 0, 2+len(assets))
	layers = append(layers, tomlDesc, lockDesc)
	assetDescs := make([]ocispec.Descriptor, 0, len(assets))
	for _, asset := range assets {
		desc, err := fs.Add(ctx, asset.RelPath, MediaTypeNebiAsset, asset.AbsPath)
		if err != nil {
			return PublishResult{}, fmt.Errorf("cannot read %s: %w", asset.RelPath, err)
		}
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: asset.RelPath}
		layers = append(layers, desc)
		assetDescs = append(assetDescs, desc)
	}

	configData := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: MediaTypePixiConfig,
		Digest:    digest.FromBytes(configData),
		Size:      int64(len(configData)),
	}
	if err := fs.Push(ctx, configDesc, bytes.NewReader(configData)); err != nil {
		return PublishResult{}, fmt.Errorf("failed to push config: %w", err)
	}

	manifestDesc, err := oras.Pack(ctx, fs, "", layers, oras.PackOptions{
		ConfigDescriptor:  &configDesc,
		PackImageManifest: true,
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationDescription: fmt.Sprintf("%s:%s", fullRepo, tag),
		},
	})
	if err != nil {
		return PublishResult{}, fmt.Errorf("failed to pack manifest: %w", err)
	}

	remoteRepo, err := remote.NewRepository(fullRepo)
	if err != nil {
		return PublishResult{}, fmt.Errorf("failed to create repository: %w", err)
	}
	remoteRepo.PlainHTTP = reg.PlainHTTP
	if c := newAuthClient(reg.Username, reg.Password); c != nil {
		remoteRepo.Client = c
	}

	blobs := make([]blobJob, 0, 3+len(assetDescs))
	blobs = append(blobs,
		blobJob{desc: configDesc, label: "config"},
		blobJob{desc: tomlDesc, label: "pixi.toml"},
		blobJob{desc: lockDesc, label: "pixi.lock"},
	)
	for i, d := range assetDescs {
		blobs = append(blobs, blobJob{desc: d, label: assets[i].RelPath})
	}
	if err := pushBlobsParallel(ctx, fs, remoteRepo, blobs, cfg.concurrency, cfg.progress); err != nil {
		return PublishResult{}, err
	}

	manifestReader, err := fs.Fetch(ctx, manifestDesc)
	if err != nil {
		return PublishResult{}, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer manifestReader.Close()
	if err := remoteRepo.PushReference(ctx, manifestDesc, manifestReader, tag); err != nil {
		return PublishResult{}, fmt.Errorf("failed to push manifest: %w", err)
	}
	for _, extraTag := range cfg.extraTags {
		if err := remoteRepo.Tag(ctx, manifestDesc, extraTag); err != nil {
			return PublishResult{}, fmt.Errorf("failed to tag manifest as %q: %w", extraTag, err)
		}
	}

	return PublishResult{
		Repository: fullRepo,
		Tag:        tag,
		Digest:     manifestDesc.Digest.String(),
		AssetCount: len(assets),
	}, nil
}

// PublishOptions is the pre-bundle publish option shape. Kept for the
// server-side caller (internal/service/workspace_publishing.go) which
// builds a concatenated repository string rather than a Registry struct.
// New callers should use Publish / PublishPixiOnly instead.
type PublishOptions struct {
	Repository   string
	Tag          string
	ExtraTags    []string
	Username     string
	Password     string
	RegistryHost string
}

// PublishWorkspace is a thin shim retained for the server caller. It
// publishes pixi.toml + pixi.lock from envPath (no walker) — equivalent
// to PublishPixiOnly applied to a pre-assembled repository string.
// Returns the manifest digest so the caller's existing signature is
// preserved.
func PublishWorkspace(ctx context.Context, envPath string, opts PublishOptions) (string, error) {
	host, ns, repoName := splitRepoRef(opts.Repository)
	reg := Registry{
		Host:      host,
		Namespace: ns,
		Username:  opts.Username,
		Password:  opts.Password,
	}
	res, err := PublishPixiOnly(ctx, envPath, reg, repoName, opts.Tag,
		WithExtraTags(opts.ExtraTags...),
	)
	if err != nil {
		return "", err
	}
	return res.Digest, nil
}

// splitRepoRef inverts buildRepoRef for the legacy string-based API.
// Input "host/ns/repo" → (host, ns, repo); "host/repo" → (host, "", repo).
func splitRepoRef(full string) (host, namespace, repo string) {
	parts := strings.SplitN(full, "/", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], "", parts[1]
	default:
		return full, "", ""
	}
}

// buildRepoRef assembles the full repository reference "host[/namespace]/repo".
func buildRepoRef(reg Registry, repo string) string {
	base := reg.Host
	if reg.Namespace != "" {
		base = base + "/" + reg.Namespace
	}
	return base + "/" + repo
}

type blobJob struct {
	desc  ocispec.Descriptor
	label string
}

// pushBlobsParallel pushes all blobs concurrently up to `limit` in flight.
// First error cancels the rest via errgroup context; the error is annotated
// with the blob's label. Progress, if non-nil, is invoked once per
// successful blob push.
func pushBlobsParallel(
	ctx context.Context,
	fs *file.Store,
	repo *remote.Repository,
	jobs []blobJob,
	limit int,
	progress func(label string, pushed, total int),
) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	total := len(jobs)
	doneCh := make(chan string, total)
	for _, j := range jobs {
		j := j
		g.Go(func() error {
			r, err := fs.Fetch(ctx, j.desc)
			if err != nil {
				return fmt.Errorf("fetch local %s: %w", j.label, err)
			}
			defer r.Close()
			if err := repo.Push(ctx, j.desc, r); err != nil {
				return fmt.Errorf("failed to push %s: %w", j.label, err)
			}
			doneCh <- j.label
			return nil
		})
	}
	err := g.Wait()
	close(doneCh)
	if progress != nil {
		pushed := 0
		for label := range doneCh {
			pushed++
			progress(label, pushed, total)
		}
	}
	return err
}
