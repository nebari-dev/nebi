package artifact

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultEnvironment is pixi's default environment name.
const DefaultEnvironment = "default"

// PyPIChannel is the Channel value for PyPI packages.
const PyPIChannel = "pypi"

// ErrUnsupportedLock is returned for lock files this package can't read
// (missing version or a format older than v4).
var ErrUnsupportedLock = errors.New("unsupported pixi.lock format")

// ResolvedPackage is one package pixi resolved for a platform.
type ResolvedPackage struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Build    string        `json:"build,omitempty"`
	Source   PackageSource `json:"source"`
	Platform string        `json:"platform"`
	// Channel is the short channel name ("conda-forge") for well-known
	// hosts, the channel base URL otherwise, or "pypi" for PyPI packages.
	Channel string `json:"channel"`
	// ChannelURL is the conda channel base URL
	// ("https://conda.anaconda.org/conda-forge"); empty for PyPI and local
	// packages.
	ChannelURL string `json:"channel_url,omitempty"`
	URL        string `json:"url,omitempty"`
	// Size is the download size in bytes from the lock's size field; 0
	// when the lock doesn't record it (PyPI packages never do).
	Size int64 `json:"size"`
}

// LockEnvironment is one pixi environment in a lock file.
type LockEnvironment struct {
	Name     string   `json:"name"`
	Channels []string `json:"channels"`
	// Packages maps platform to resolved packages, sorted by name then source.
	Packages map[string][]ResolvedPackage `json:"packages"`
}

// Platforms returns the environment's platforms, sorted.
func (e *LockEnvironment) Platforms() []string {
	ps := make([]string, 0, len(e.Packages))
	for p := range e.Packages {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// Lock is a parsed pixi.lock.
type Lock struct {
	Version      int                         `json:"version"`
	Environments map[string]*LockEnvironment `json:"environments"`
}

// Environment returns the named environment ("" means default) or nil.
func (l *Lock) Environment(name string) *LockEnvironment {
	if l == nil {
		return nil
	}
	if name == "" {
		name = DefaultEnvironment
	}
	return l.Environments[name]
}

// Resolved returns the packages for env ("" means default) and platform,
// or nil when either is missing.
func (l *Lock) Resolved(env, platform string) []ResolvedPackage {
	e := l.Environment(env)
	if e == nil {
		return nil
	}
	return e.Packages[platform]
}

// TotalSize sums Size over pkgs.
func TotalSize(pkgs []ResolvedPackage) int64 {
	var n int64
	for _, p := range pkgs {
		n += p.Size
	}
	return n
}

type rawLock struct {
	Version      int `yaml:"version"`
	Environments map[string]struct {
		Channels []struct {
			URL string `yaml:"url"`
		} `yaml:"channels"`
		Packages map[string][]map[string]yaml.Node `yaml:"packages"`
	} `yaml:"environments"`
	Packages []map[string]yaml.Node `yaml:"packages"`
}

type lockRecord struct {
	source  PackageSource
	url     string
	name    string
	version string
	build   string
	subdir  string
	size    int64
}

func scalar(m map[string]yaml.Node, key string) string {
	n, ok := m[key]
	if !ok || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// ParseLock parses pixi.lock formats v4, v5 and v6. Newer versions are read
// with the v6 rules. Empty content returns an empty Lock.
func ParseLock(content []byte) (*Lock, error) {
	lock := &Lock{Environments: map[string]*LockEnvironment{}}
	if len(strings.TrimSpace(string(content))) == 0 {
		return lock, nil
	}
	var raw rawLock
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse pixi.lock: %w", err)
	}
	if raw.Version < 4 {
		return nil, fmt.Errorf("%w: version %d", ErrUnsupportedLock, raw.Version)
	}
	lock.Version = raw.Version

	type key struct {
		source PackageSource
		url    string
	}
	index := map[key]lockRecord{}
	for _, p := range raw.Packages {
		r, ok := recordFromLockEntry(p)
		if !ok {
			continue
		}
		k := key{r.source, r.url}
		if _, dup := index[k]; !dup {
			index[k] = r
		}
	}

	for envName, env := range raw.Environments {
		le := &LockEnvironment{Name: envName, Channels: []string{}, Packages: map[string][]ResolvedPackage{}}
		for _, c := range env.Channels {
			le.Channels = append(le.Channels, c.URL)
		}
		for platform, refs := range env.Packages {
			pkgs := []ResolvedPackage{}
			for _, ref := range refs {
				var k key
				switch {
				case scalar(ref, "conda") != "":
					k = key{SourceConda, scalar(ref, "conda")}
				case scalar(ref, "pypi") != "":
					k = key{SourcePyPI, scalar(ref, "pypi")}
				default:
					continue
				}
				r, ok := index[k]
				if !ok {
					r = lockRecord{source: k.source, url: k.url}
					fillFromURL(&r)
				}
				if r.name == "" {
					continue
				}
				pkgs = append(pkgs, r.resolved(platform))
			}
			sortResolved(pkgs)
			le.Packages[platform] = pkgs
		}
		lock.Environments[envName] = le
	}
	return lock, nil
}

func recordFromLockEntry(p map[string]yaml.Node) (lockRecord, bool) {
	var r lockRecord
	switch {
	case scalar(p, "conda") != "":
		r.source, r.url = SourceConda, scalar(p, "conda")
	case scalar(p, "pypi") != "":
		r.source, r.url = SourcePyPI, scalar(p, "pypi")
	case scalar(p, "kind") == "conda":
		r.source, r.url = SourceConda, scalar(p, "url")
	case scalar(p, "kind") == "pypi":
		r.source, r.url = SourcePyPI, scalar(p, "url")
	default:
		return r, false
	}
	r.name = scalar(p, "name")
	r.version = scalar(p, "version")
	r.build = scalar(p, "build")
	r.subdir = scalar(p, "subdir")
	if s := scalar(p, "size"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			r.size = n
		}
	}
	fillFromURL(&r)
	return r, true
}

// fillFromURL fills name/version/build from a conda filename when the lock
// entry doesn't spell them out (v6 omits them for binary packages).
func fillFromURL(r *lockRecord) {
	if r.source != SourceConda || (r.name != "" && r.version != "" && r.build != "") {
		return
	}
	n, v, b := ParseCondaFilename(r.url)
	if r.name == "" {
		r.name = n
	}
	if r.version == "" {
		r.version = v
	}
	if r.build == "" {
		r.build = b
	}
}

func (r lockRecord) resolved(platform string) ResolvedPackage {
	p := ResolvedPackage{
		Name: r.name, Version: r.version, Build: r.build, Source: r.source,
		Platform: platform, URL: r.url, Size: r.size,
	}
	if r.source == SourcePyPI {
		p.Channel = PyPIChannel
		return p
	}
	p.Channel, p.ChannelURL = ChannelFromPackageURL(r.url, r.subdir)
	return p
}

func sortResolved(pkgs []ResolvedPackage) {
	sort.SliceStable(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Source < pkgs[j].Source
	})
}

// ParseCondaFilename extracts name, version and build from a conda package
// URL or filename (name-version-build.conda or .tar.bz2). It returns empty
// strings when the filename doesn't have that shape.
func ParseCondaFilename(u string) (name, version, build string) {
	filename := path.Base(u)
	switch {
	case strings.HasSuffix(filename, ".tar.bz2"):
		filename = strings.TrimSuffix(filename, ".tar.bz2")
	case strings.HasSuffix(filename, ".conda"):
		filename = strings.TrimSuffix(filename, ".conda")
	default:
		return "", "", ""
	}
	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		return "", "", ""
	}
	buildIdx := len(parts) - 1
	versionIdx := -1
	for i := buildIdx - 1; i >= 1; i-- {
		if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
			versionIdx = i
			break
		}
	}
	if versionIdx < 1 {
		return "", "", ""
	}
	return strings.Join(parts[:versionIdx], "-"), parts[versionIdx], strings.Join(parts[versionIdx+1:], "-")
}

// shortChannelHosts are hosts whose channel URLs are shown by path
// ("https://conda.anaconda.org/conda-forge" shows as "conda-forge").
var shortChannelHosts = []string{"conda.anaconda.org", "prefix.dev", "repo.prefix.dev"}

// ChannelFromPackageURL derives the channel display name and base URL from a
// conda package URL. subdir (linux-64, noarch, ...) is stripped when known;
// otherwise a trailing platform directory is. Non-URL locations (local
// source packages) return ("", "").
func ChannelFromPackageURL(pkgURL, subdir string) (name, base string) {
	u, err := url.Parse(pkgURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ""
	}
	dir := path.Dir(u.Path)
	last := path.Base(dir)
	if (subdir != "" && last == subdir) || IsKnownPlatform(last) {
		dir = path.Dir(dir)
	}
	dir = strings.TrimSuffix(dir, "/")
	u.Path, u.RawPath, u.RawQuery, u.Fragment = dir, "", "", ""
	base = u.String()
	return ChannelDisplayName(base), base
}

// ChannelDisplayName shortens a channel base URL on a well-known host to its
// path ("conda-forge"); other URLs and plain names are returned unchanged.
func ChannelDisplayName(channel string) string {
	u, err := url.Parse(channel)
	if err != nil || u.Host == "" {
		return channel
	}
	if slices.Contains(shortChannelHosts, u.Host) {
		if p := strings.Trim(u.Path, "/"); p != "" {
			return p
		}
	}
	return strings.TrimSuffix(channel, "/")
}
