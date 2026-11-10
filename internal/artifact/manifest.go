package artifact

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// PackageSource says where a package comes from.
type PackageSource string

const (
	SourceConda PackageSource = "conda"
	SourcePyPI  PackageSource = "pypi"
)

// Table names the form owns.
const (
	tableWorkspace = "workspace"
	tableProject   = "project"
	tableDeps      = "dependencies"
	tablePypiDeps  = "pypi-dependencies"
)

// File names and defaults shared by callers.
const (
	// DefaultChannel is the channel used when a project lists none.
	DefaultChannel = "conda-forge"
	ManifestFile   = "pixi.toml"
	LockFile       = "pixi.lock"
)

// RequestedPackage is one dependency the user asked for.
type RequestedPackage struct {
	Name       string        `json:"name"`
	Constraint string        `json:"constraint"`       // "*", ">=2.0", "3.11.*", ...
	Source     PackageSource `json:"source,omitempty"` // "" means conda
}

func (p RequestedPackage) sourceOrDefault() PackageSource {
	if p.Source == "" {
		return SourceConda
	}
	return p.Source
}

// Operator returns the form operator for the constraint (see SplitConstraint).
func (p RequestedPackage) Operator() string { op, _ := SplitConstraint(p.Constraint); return op }

// Version returns the form version for the constraint (see SplitConstraint).
func (p RequestedPackage) Version() string { _, v := SplitConstraint(p.Constraint); return v }

// ManifestForm is everything the GUI form can show and edit.
// Description is pixi's optional [workspace] description, not the Nebi
// version description.
type ManifestForm struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Channels    []string           `json:"channels"`
	Platforms   []string           `json:"platforms"`
	Requested   []RequestedPackage `json:"requested"`
}

// Manifest is a parsed pixi.toml.
type Manifest struct {
	Form ManifestForm `json:"form"`
	// HasWorkspace is true when a [workspace] or legacy [project] table exists.
	HasWorkspace bool `json:"has_workspace"`
	// WorkspaceTable is "workspace", "project", or "" when neither exists.
	WorkspaceTable string `json:"workspace_table,omitempty"`
	// PixiVersion is [workspace] version when present (pixi's own semver
	// field, unrelated to Nebi version numbers).
	PixiVersion string `json:"pixi_version,omitempty"`
	// Unrepresentable lists what the form can't show, for the toast
	// "The form can't show <items>". Tables appear as "[tasks]" or
	// "[[name]]", unsupported keys as "workspace.authors" or
	// "dependencies.numpy" (a dependency table with keys besides version).
	Unrepresentable []string `json:"unrepresentable,omitempty"`
}

// formWorkspaceKeys are the [workspace] keys the form owns.
var formWorkspaceKeys = []string{"name", "description", "channels", "platforms"}

type parsedManifest struct {
	m   *Manifest
	raw map[string]any
	doc *tomlDoc
}

// ParseManifest parses pixi.toml content into the form model and reports
// what the form can't represent. It returns an error for invalid TOML.
func ParseManifest(content []byte) (*Manifest, error) {
	p, err := parseManifest(content)
	if err != nil {
		return nil, err
	}
	return p.m, nil
}

func parseManifest(content []byte) (*parsedManifest, error) {
	var raw map[string]any
	if err := toml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse pixi.toml: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	doc, err := scanToml(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse pixi.toml: %w", err)
	}

	m := &Manifest{}
	var unrep []string
	addUnrep := func(s string) {
		if !slices.Contains(unrep, s) {
			unrep = append(unrep, s)
		}
	}

	var ws map[string]any
	for _, t := range []string{tableWorkspace, tableProject} {
		if tbl, ok := raw[t].(map[string]any); ok {
			ws, m.WorkspaceTable, m.HasWorkspace = tbl, t, true
			break
		}
	}

	f := ManifestForm{Channels: []string{}, Platforms: []string{}, Requested: []RequestedPackage{}}
	if ws != nil {
		f.Name, _ = ws["name"].(string)
		f.Description, _ = ws["description"].(string)
		if v, ok := ws["version"].(string); ok {
			m.PixiVersion = v
		}
		if chs, ok := ws["channels"].([]any); ok {
			for _, c := range chs {
				switch t := c.(type) {
				case string:
					f.Channels = append(f.Channels, t)
				case map[string]any:
					// { channel = "x", priority = 1 }: the form shows the
					// name but not the priority.
					if name, ok := t["channel"].(string); ok {
						f.Channels = append(f.Channels, name)
					}
					addUnrep(m.WorkspaceTable + ".channels")
				default:
					addUnrep(m.WorkspaceTable + ".channels")
				}
			}
		}
		if ps, ok := ws["platforms"].([]any); ok {
			for _, p := range ps {
				if s, ok := p.(string); ok {
					f.Platforms = append(f.Platforms, s)
				} else {
					addUnrep(m.WorkspaceTable + ".platforms")
				}
			}
		}
		for _, k := range slices.Sorted(maps.Keys(ws)) {
			if !slices.Contains(formWorkspaceKeys, k) {
				addUnrep(m.WorkspaceTable + "." + k)
			}
		}
	}

	for _, src := range []struct {
		table  string
		source PackageSource
	}{{tableDeps, SourceConda}, {tablePypiDeps, SourcePyPI}} {
		deps, _ := raw[src.table].(map[string]any)
		for _, name := range depOrder(doc, src.table, deps) {
			c, extra := depConstraint(deps[name])
			if extra {
				addUnrep(src.table + "." + name)
			}
			f.Requested = append(f.Requested, RequestedPackage{Name: name, Constraint: c, Source: src.source})
		}
	}

	for _, e := range doc.preamble {
		if e.key != nil {
			if k := e.key[0]; k != tableDeps && k != tablePypiDeps && k != m.WorkspaceTable {
				addUnrep(strings.Join(e.key, "."))
			}
		}
	}
	for _, s := range doc.sections {
		k := strings.Join(s.key, ".")
		if s.array {
			addUnrep("[[" + k + "]]")
			continue
		}
		if k == m.WorkspaceTable || k == tableDeps || k == tablePypiDeps {
			continue
		}
		addUnrep("[" + k + "]")
	}

	m.Form = f
	m.Unrepresentable = unrep
	return &parsedManifest{m: m, raw: raw, doc: doc}, nil
}

// depOrder returns dependency names in document order, falling back to
// sorted order for names not found in a [table] section (inline tables).
func depOrder(doc *tomlDoc, table string, deps map[string]any) []string {
	var order []string
	seen := map[string]bool{}
	if i := doc.findSection(table); i >= 0 {
		for _, e := range doc.sections[i].entries {
			if e.key == nil || seen[e.key[0]] {
				continue
			}
			if _, ok := deps[e.key[0]]; ok {
				order = append(order, e.key[0])
				seen[e.key[0]] = true
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(deps)) {
		if !seen[k] {
			order = append(order, k)
		}
	}
	return order
}

// depConstraint extracts the version constraint from a dependency value and
// reports whether the value carries keys the form can't show.
func depConstraint(v any) (constraint string, extra bool) {
	switch t := v.(type) {
	case string:
		return NormalizeConstraint(t), false
	case map[string]any:
		c := AnyVersion
		if s, ok := t["version"].(string); ok {
			c = NormalizeConstraint(s)
		}
		for k := range t {
			if k != "version" {
				extra = true
			}
		}
		return c, extra
	default:
		return renderInlineValue(t), true
	}
}

// ManifestSpec returns the environment specification part of a pixi.toml:
// channels, platforms and requested packages.
func (m *Manifest) Spec() *Spec {
	return &Spec{
		Channels:  slices.Clone(m.Form.Channels),
		Platforms: slices.Clone(m.Form.Platforms),
		Requested: slices.Clone(m.Form.Requested),
	}
}

// RenderManifest writes a fresh pixi.toml from the form, in the prototype's
// layout. The [dependencies] table is always written; [pypi-dependencies]
// only when there are PyPI packages.
func RenderManifest(f ManifestForm) []byte {
	var sb strings.Builder
	sb.WriteString("[workspace]\n")
	sb.WriteString("name = " + tomlQuote(f.Name) + "\n")
	if f.Description != "" {
		sb.WriteString("description = " + tomlQuote(f.Description) + "\n")
	}
	sb.WriteString("channels = " + renderStringArray(nonNil(f.Channels)) + "\n")
	sb.WriteString("platforms = " + renderStringArray(nonNil(f.Platforms)) + "\n")
	sb.WriteString("\n[dependencies]\n")
	for _, p := range f.Requested {
		if p.sourceOrDefault() == SourceConda {
			sb.WriteString(renderDep(p.Name, p.Constraint))
		}
	}
	var pypi []RequestedPackage
	for _, p := range f.Requested {
		if p.sourceOrDefault() == SourcePyPI {
			pypi = append(pypi, p)
		}
	}
	if len(pypi) > 0 {
		sb.WriteString("\n[pypi-dependencies]\n")
		for _, p := range pypi {
			sb.WriteString(renderDep(p.Name, p.Constraint))
		}
	}
	return []byte(sb.String())
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func renderDep(name, constraint string) string {
	return renderTomlKey(name) + " = " + tomlQuote(NormalizeConstraint(constraint)) + "\n"
}

// ApplyForm writes the form back into an existing pixi.toml. Only the keys
// the form owns change: [workspace] name, description, channels, platforms
// and the entries of [dependencies] and [pypi-dependencies]. Every other
// table, key, comment and blank line is kept byte for byte, as are the
// dependency lines whose constraint did not change. Dependencies written as
// tables keep their other keys; only version is replaced. With empty
// existing content it behaves like RenderManifest.
//
// Limitation: dependencies or workspace keys defined as dotted keys at the
// root of the file (dependencies.numpy = "*") are not edited; the result is
// rejected if it would no longer parse.
func ApplyForm(existing []byte, f ManifestForm) ([]byte, error) {
	if strings.TrimSpace(string(existing)) == "" {
		return RenderManifest(f), nil
	}
	content := string(existing)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	p, err := parseManifest([]byte(content))
	if err != nil {
		return nil, err
	}
	doc := p.doc
	old := p.m.Form

	wsName := p.m.WorkspaceTable
	wsIdx := 0
	if wsName != "" {
		wsIdx = doc.findSection(wsName)
		if wsIdx < 0 {
			return nil, fmt.Errorf("pixi.toml defines %s inline; edit it in the pixi.toml editor", wsName)
		}
	} else {
		insertSection(doc, -1, &tomlSection{header: "[workspace]\n", key: []string{tableWorkspace}})
	}
	applyWorkspace(doc.sections[wsIdx], old, f)

	var conda, pypi []RequestedPackage
	for _, r := range f.Requested {
		if r.sourceOrDefault() == SourcePyPI {
			pypi = append(pypi, r)
		} else {
			conda = append(conda, r)
		}
	}
	rawDeps, _ := p.raw[tableDeps].(map[string]any)
	rawPypi, _ := p.raw[tablePypiDeps].(map[string]any)
	after, err := applyDeps(doc, tableDeps, conda, rawDeps, wsIdx, true)
	if err != nil {
		return nil, err
	}
	if _, err := applyDeps(doc, tablePypiDeps, pypi, rawPypi, after, false); err != nil {
		return nil, err
	}

	out := []byte(doc.String())
	var check map[string]any
	if err := toml.Unmarshal(out, &check); err != nil {
		return nil, fmt.Errorf("form changes would produce invalid pixi.toml: %w", err)
	}
	return out, nil
}

func applyWorkspace(sec *tomlSection, old, f ManifestForm) {
	type managed struct {
		key       string
		value     string
		unchanged bool
		remove    bool
	}
	ms := []managed{
		{key: "name", value: tomlQuote(f.Name), unchanged: old.Name == f.Name},
		{key: "description", value: tomlQuote(f.Description), unchanged: old.Description == f.Description, remove: f.Description == ""},
		{key: "channels", value: renderStringArray(nonNil(f.Channels)), unchanged: slices.Equal(old.Channels, f.Channels) || (len(old.Channels) == 0 && len(f.Channels) == 0)},
		{key: "platforms", value: renderStringArray(nonNil(f.Platforms)), unchanged: slices.Equal(old.Platforms, f.Platforms) || (len(old.Platforms) == 0 && len(f.Platforms) == 0)},
	}
	written := map[string]bool{}
	var entries []tomlEntry
	for _, e := range sec.entries {
		if e.key == nil || len(e.key) != 1 {
			entries = append(entries, e)
			continue
		}
		i := slices.IndexFunc(ms, func(m managed) bool { return m.key == e.key[0] })
		if i < 0 {
			entries = append(entries, e)
			continue
		}
		m := ms[i]
		written[m.key] = true
		switch {
		case m.unchanged:
			entries = append(entries, e)
		case m.remove:
			// dropped
		default:
			entries = append(entries, tomlEntry{raw: m.key + " = " + m.value + "\n", key: []string{m.key}})
		}
	}
	var missing []tomlEntry
	for _, m := range ms {
		if written[m.key] || m.remove {
			continue
		}
		missing = append(missing, tomlEntry{raw: m.key + " = " + m.value + "\n", key: []string{m.key}})
	}
	sec.entries = insertAfterLastKey(entries, missing)
}

// insertAfterLastKey inserts add after the last key/value entry, so trailing
// blank lines and comments (which usually introduce the next table) stay
// below the new entries.
func insertAfterLastKey(entries, add []tomlEntry) []tomlEntry {
	if len(add) == 0 {
		return entries
	}
	pos := 0
	for i, e := range entries {
		if e.key != nil {
			pos = i + 1
		}
	}
	return slices.Insert(slices.Clone(entries), pos, add...)
}

// applyDeps rewrites one dependency table and returns the index of its
// section (or `after` when it has none), for placing the next new table.
func applyDeps(doc *tomlDoc, table string, want []RequestedPackage, raw map[string]any, after int, always bool) (int, error) {
	idx := doc.findSection(table)
	if idx < 0 {
		if raw != nil {
			// Defined inline (dependencies = { ... }) or with dotted root
			// keys: a [table] header would duplicate it.
			if !sameDeps(raw, want) {
				return after, fmt.Errorf("pixi.toml defines %s without a [%s] header; edit it in the pixi.toml editor", table, table)
			}
			return after, nil
		}
		if len(want) == 0 && !always {
			return after, nil
		}
		sec := &tomlSection{header: "[" + table + "]\n", key: []string{table}}
		for _, r := range want {
			sec.entries = append(sec.entries, tomlEntry{raw: renderDep(r.Name, r.Constraint), key: []string{r.Name}})
		}
		insertSection(doc, after, sec)
		return after + 1, nil
	}

	sec := doc.sections[idx]
	wantBy := map[string]RequestedPackage{}
	for _, r := range want {
		wantBy[r.Name] = r
	}
	emitted := map[string]bool{}
	var entries []tomlEntry
	for _, e := range sec.entries {
		if e.key == nil {
			entries = append(entries, e)
			continue
		}
		name := e.key[0]
		r, ok := wantBy[name]
		if !ok {
			continue
		}
		oldC, _ := depConstraint(raw[name])
		if oldC == NormalizeConstraint(r.Constraint) {
			entries = append(entries, e)
			emitted[name] = true
			continue
		}
		if emitted[name] {
			continue
		}
		emitted[name] = true
		entries = append(entries, tomlEntry{raw: renderChangedDep(name, r.Constraint, raw[name]), key: []string{name}})
	}
	var add []tomlEntry
	for _, r := range want {
		if !emitted[r.Name] {
			add = append(add, tomlEntry{raw: renderDep(r.Name, r.Constraint), key: []string{r.Name}})
			emitted[r.Name] = true
		}
	}
	sec.entries = insertAfterLastKey(entries, add)
	return idx, nil
}

func sameDeps(raw map[string]any, want []RequestedPackage) bool {
	if len(raw) != len(want) {
		return false
	}
	for _, r := range want {
		v, ok := raw[r.Name]
		if !ok {
			return false
		}
		if c, _ := depConstraint(v); c != NormalizeConstraint(r.Constraint) {
			return false
		}
	}
	return true
}

// renderChangedDep renders a dependency whose constraint changed, keeping
// the other keys of a table value (channel, build, extras, ...).
func renderChangedDep(name, constraint string, old any) string {
	tbl, ok := old.(map[string]any)
	if !ok {
		return renderDep(name, constraint)
	}
	nt := maps.Clone(tbl)
	nt["version"] = NormalizeConstraint(constraint)
	return renderTomlKey(name) + " = " + renderInlineValue(nt) + "\n"
}

// insertSection inserts sec after section index `after` (-1 means before
// all sections) and keeps one blank line between tables.
func insertSection(doc *tomlDoc, after int, sec *tomlSection) {
	ensureBlankTail := func(entries *[]tomlEntry, header *string) {
		if n := len(*entries); n > 0 {
			if strings.TrimSpace((*entries)[n-1].raw) != "" {
				*entries = append(*entries, tomlEntry{raw: "\n"})
			}
			return
		}
		if header != nil {
			*entries = append(*entries, tomlEntry{raw: "\n"})
		}
	}
	if after >= 0 {
		prev := doc.sections[after]
		ensureBlankTail(&prev.entries, &prev.header)
	} else if len(doc.preamble) > 0 {
		ensureBlankTail(&doc.preamble, nil)
	}
	pos := after + 1
	if pos < len(doc.sections) {
		sec.entries = append(sec.entries, tomlEntry{raw: "\n"})
	}
	doc.sections = slices.Insert(doc.sections, pos, sec)
}

// SortRequested sorts packages by source (conda first) then name; handy for
// stable output in CLI tables.
func SortRequested(ps []RequestedPackage) {
	sort.SliceStable(ps, func(i, j int) bool {
		si, sj := ps[i].sourceOrDefault(), ps[j].sourceOrDefault()
		if si != sj {
			return si == SourceConda
		}
		return ps[i].Name < ps[j].Name
	})
}
