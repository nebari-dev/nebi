package artifact

import (
	"slices"
	"sort"
	"strings"
)

// Change is how a package differs between two versions.
type Change string

const (
	ChangeAdded   Change = "added"
	ChangeRemoved Change = "removed"
	ChangeChanged Change = "changed"
	ChangeSame    Change = "same"
)

// PackageRef is one side of a diff row.
type PackageRef struct {
	// Constraint is the requested constraint (requested rows only).
	Constraint string `json:"constraint,omitempty"`
	// Version is the resolved version; for requested rows it falls back to
	// the constraint when the lock has no entry (as the prototype does).
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
	Channel string `json:"channel,omitempty"`
}

// PackageDiffEntry is one row of a version comparison.
type PackageDiffEntry struct {
	Name    string        `json:"name"`
	Source  PackageSource `json:"source"`
	Change  Change        `json:"change"`
	Base    *PackageRef   `json:"base"`    // nil when added
	Compare *PackageRef   `json:"compare"` // nil when removed
}

// DiffCounts counts rows by change.
type DiffCounts struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
	Same    int `json:"same"`
}

// HasChanges reports whether anything was added, removed or changed.
func (c DiffCounts) HasChanges() bool { return c.Added+c.Removed+c.Changed > 0 }

// CountChanges counts entries by change.
func CountChanges(entries []PackageDiffEntry) DiffCounts {
	var c DiffCounts
	for _, e := range entries {
		switch e.Change {
		case ChangeAdded:
			c.Added++
		case ChangeRemoved:
			c.Removed++
		case ChangeChanged:
			c.Changed++
		case ChangeSame:
			c.Same++
		}
	}
	return c
}

// ListDiff is the added/removed items of a string list (channels, platforms).
type ListDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// VersionContent is the pixi.toml and pixi.lock of one version. Lock may be
// empty.
type VersionContent struct {
	Toml []byte
	Lock []byte
}

// DiffOptions selects which lock environment and platform resolved versions
// come from. Empty Environment means "default"; empty Platform picks one
// (see DiffVersions).
type DiffOptions struct {
	Environment string
	Platform    string
}

// VersionDiff is the Compare versions result.
type VersionDiff struct {
	// Platform is the platform resolved versions were read from ("" when
	// neither lock has packages).
	Platform        string             `json:"platform"`
	Requested       []PackageDiffEntry `json:"requested"`
	RequestedCounts DiffCounts         `json:"requested_counts"`
	Resolved        []PackageDiffEntry `json:"resolved"`
	ResolvedCounts  DiffCounts         `json:"resolved_counts"`
	Channels        ListDiff           `json:"channels"`
	Platforms       ListDiff           `json:"platforms"`
}

// DiffVersions compares two versions. Requested rows cover the union of
// requested packages (a row is changed when the constraint or the resolved
// version differs); Resolved rows cover every package in the lock for the
// chosen platform (changed when version or build differs).
//
// When opts.Platform is empty the platform is the first of compare's
// manifest platforms present in a lock, then base's, then the first lock
// platform in sorted order.
func DiffVersions(base, compare VersionContent, opts DiffOptions) (*VersionDiff, error) {
	bm, err := ParseManifest(base.Toml)
	if err != nil {
		return nil, err
	}
	cm, err := ParseManifest(compare.Toml)
	if err != nil {
		return nil, err
	}
	bl, err := ParseLock(base.Lock)
	if err != nil {
		return nil, err
	}
	cl, err := ParseLock(compare.Lock)
	if err != nil {
		return nil, err
	}

	platform := opts.Platform
	if platform == "" {
		platform = choosePlatform(opts.Environment, [][]string{cm.Form.Platforms, bm.Form.Platforms}, cl, bl)
	}
	bRes := bl.Resolved(opts.Environment, platform)
	cRes := cl.Resolved(opts.Environment, platform)

	d := &VersionDiff{
		Platform:  platform,
		Requested: DiffRequested(bm.Form.Requested, cm.Form.Requested, bRes, cRes),
		Resolved:  DiffResolved(bRes, cRes),
		Channels:  DiffList(bm.Form.Channels, cm.Form.Channels),
		Platforms: DiffList(bm.Form.Platforms, cm.Form.Platforms),
	}
	d.RequestedCounts = CountChanges(d.Requested)
	d.ResolvedCounts = CountChanges(d.Resolved)
	return d, nil
}

func choosePlatform(env string, manifests [][]string, locks ...*Lock) string {
	has := func(p string) bool {
		for _, l := range locks {
			if e := l.Environment(env); e != nil {
				if _, ok := e.Packages[p]; ok {
					return true
				}
			}
		}
		return false
	}
	for _, ps := range manifests {
		for _, p := range ps {
			if has(p) {
				return p
			}
		}
	}
	for _, l := range locks {
		if e := l.Environment(env); e != nil {
			if ps := e.Platforms(); len(ps) > 0 {
				return ps[0]
			}
		}
	}
	return ""
}

func pkgKey(source PackageSource, name string) string {
	if source == "" {
		source = SourceConda
	}
	return string(source) + "\x00" + strings.ToLower(name)
}

// DiffRequested compares requested packages; resolved lists (may be nil)
// supply the resolved version shown for each side.
func DiffRequested(base, compare []RequestedPackage, baseResolved, compareResolved []ResolvedPackage) []PackageDiffEntry {
	side := func(reqs []RequestedPackage, res []ResolvedPackage) map[string]*PackageRef {
		rv := map[string]ResolvedPackage{}
		for _, r := range res {
			if _, ok := rv[pkgKey(r.Source, r.Name)]; !ok {
				rv[pkgKey(r.Source, r.Name)] = r
			}
		}
		out := map[string]*PackageRef{}
		for _, q := range reqs {
			c := NormalizeConstraint(q.Constraint)
			ref := &PackageRef{Constraint: c, Version: c}
			if r, ok := rv[pkgKey(q.sourceOrDefault(), q.Name)]; ok {
				ref.Version, ref.Build, ref.Channel = r.Version, r.Build, r.Channel
			}
			out[pkgKey(q.sourceOrDefault(), q.Name)] = ref
		}
		return out
	}
	names := map[string]RequestedPackage{}
	for _, q := range slices.Concat(base, compare) {
		names[pkgKey(q.sourceOrDefault(), q.Name)] = RequestedPackage{Name: q.Name, Source: q.sourceOrDefault()}
	}
	b, c := side(base, baseResolved), side(compare, compareResolved)
	entries := make([]PackageDiffEntry, 0, len(names))
	for k, q := range names {
		e := PackageDiffEntry{Name: q.Name, Source: q.Source, Base: b[k], Compare: c[k]}
		switch {
		case e.Base == nil:
			e.Change = ChangeAdded
		case e.Compare == nil:
			e.Change = ChangeRemoved
		case e.Base.Constraint != e.Compare.Constraint || e.Base.Version != e.Compare.Version:
			e.Change = ChangeChanged
		default:
			e.Change = ChangeSame
		}
		entries = append(entries, e)
	}
	sortEntries(entries)
	return entries
}

// DiffResolved compares two resolved package lists (one platform each).
func DiffResolved(base, compare []ResolvedPackage) []PackageDiffEntry {
	index := func(res []ResolvedPackage) map[string]ResolvedPackage {
		m := map[string]ResolvedPackage{}
		for _, r := range res {
			if _, ok := m[pkgKey(r.Source, r.Name)]; !ok {
				m[pkgKey(r.Source, r.Name)] = r
			}
		}
		return m
	}
	b, c := index(base), index(compare)
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range c {
		keys[k] = true
	}
	entries := make([]PackageDiffEntry, 0, len(keys))
	for k := range keys {
		br, bok := b[k]
		cr, cok := c[k]
		e := PackageDiffEntry{}
		if bok {
			e.Name, e.Source = br.Name, br.Source
			e.Base = &PackageRef{Version: br.Version, Build: br.Build, Channel: br.Channel}
		}
		if cok {
			e.Name, e.Source = cr.Name, cr.Source
			e.Compare = &PackageRef{Version: cr.Version, Build: cr.Build, Channel: cr.Channel}
		}
		switch {
		case !bok:
			e.Change = ChangeAdded
		case !cok:
			e.Change = ChangeRemoved
		case br.Version != cr.Version || br.Build != cr.Build:
			e.Change = ChangeChanged
		default:
			e.Change = ChangeSame
		}
		entries = append(entries, e)
	}
	sortEntries(entries)
	return entries
}

func sortEntries(entries []PackageDiffEntry) {
	sort.Slice(entries, func(i, j int) bool {
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a != b {
			return a < b
		}
		return entries[i].Source < entries[j].Source
	})
}

// DiffList returns items added to and removed from a list, in list order.
func DiffList(base, compare []string) ListDiff {
	d := ListDiff{Added: []string{}, Removed: []string{}}
	for _, c := range compare {
		if !slices.Contains(base, c) {
			d.Added = append(d.Added, c)
		}
	}
	for _, b := range base {
		if !slices.Contains(compare, b) {
			d.Removed = append(d.Removed, b)
		}
	}
	return d
}

// SummarizeRequestedChanges describes requested-package changes the way the
// prototype words them in version descriptions:
// ["added pandas and rich", "removed numpy", "changed python"].
func SummarizeRequestedChanges(base, compare []RequestedPackage) []string {
	bm := map[string]string{}
	for _, p := range base {
		bm[p.Name] = NormalizeConstraint(p.Constraint)
	}
	cm := map[string]string{}
	for _, p := range compare {
		cm[p.Name] = NormalizeConstraint(p.Constraint)
	}
	var added, removed, changed []string
	for _, p := range compare {
		if old, ok := bm[p.Name]; !ok {
			added = append(added, p.Name)
		} else if old != NormalizeConstraint(p.Constraint) {
			changed = append(changed, p.Name)
		}
	}
	for _, p := range base {
		if _, ok := cm[p.Name]; !ok {
			removed = append(removed, p.Name)
		}
	}
	var out []string
	for _, g := range []struct {
		verb  string
		names []string
	}{{"added", added}, {"removed", removed}, {"changed", changed}} {
		if len(g.names) > 0 {
			out = append(out, g.verb+" "+joinAnd(g.names))
		}
	}
	return out
}

func joinAnd(xs []string) string {
	if len(xs) <= 2 {
		return strings.Join(xs, " and ")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}
