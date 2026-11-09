package main

// Owner: U3 (newversion).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

// newVersionOptions are the flags of `nebi project version new`.
type newVersionOptions struct {
	from           int
	tomlFile       string
	description    string
	tags           []string
	addPackages    []string
	removePackages []string
	addPlatforms   []string
	removePlatform []string
	addChannels    []string
	removeChannels []string
	install        bool
	dryRun         bool
}

func (o *newVersionOptions) formEdits() bool {
	return len(o.addPackages)+len(o.removePackages)+len(o.addPlatforms)+
		len(o.removePlatform)+len(o.addChannels)+len(o.removeChannels) > 0
}

func registerNewVersionCommands(root *cobra.Command) {
	var o newVersionOptions
	cmd := &cobra.Command{
		Use:   "new <project> [--from N] (--pixi-toml FILE | --add PKG ... )",
		Short: "Create a new version from a base version",
		Long: `Create a new version of a project from any base version on this machine.

The new version starts with the base version's pixi.toml. Change it either
with an edited pixi.toml (--pixi-toml, "-" reads stdin) or with form-style
edits (--add, --remove, --add-platform, ...), which keep everything else in
the base pixi.toml as it is. pixi.lock is solved for the new version; the
installed version is not changed unless --install is given.

Tags move to the new version: a tag stays with a version until a newer
version uses the same tag. latest does this automatically.

Without --description the description is suggested from the package changes.

Examples:
  nebi project version new ml-baseline --add pandas=">=2.2" --add rich
  nebi project version new ml-baseline --from 5 --remove numpy --tag stable
  nebi project version new ml-baseline --pixi-toml ./pixi.toml -d "Bumped Python to 3.12"
  nebi project version new ml-baseline --add pypi:requests --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.tomlFile != "" && o.formEdits() {
				return errors.New("use either --pixi-toml or the --add/--remove flags, not both")
			}
			if o.tomlFile == "" && !o.formEdits() {
				return errors.New("nothing to change: pass --pixi-toml FILE or --add/--remove/--add-platform/--remove-platform/--add-channel/--remove-channel")
			}
			var tomlText string
			if o.tomlFile != "" {
				data, err := readTomlArg(cmd.InOrStdin(), o.tomlFile)
				if err != nil {
					return err
				}
				tomlText = string(data)
			}
			return withStack(cmd, func(ctx context.Context, s *client.Stack) error {
				return runNewVersion(ctx, cmd.OutOrStdout(), s, args[0], tomlText, o)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&o.from, "from", 0, "Base version number (default: the latest version)")
	f.StringVar(&o.tomlFile, "pixi-toml", "", `Edited pixi.toml for the new version ("-" reads stdin)`)
	f.StringVarP(&o.description, "description", "d", "", "About this version (default: suggested from your changes)")
	f.StringSliceVar(&o.tags, "tag", nil, "Tag to move to the new version (repeatable)")
	f.StringArrayVar(&o.addPackages, "add", nil, `Add or change a package: name, name=constraint (e.g. numpy=">=2.0") or pypi:name (repeatable)`)
	f.StringArrayVar(&o.removePackages, "remove", nil, "Remove a package by name, or pypi:name (repeatable)")
	f.StringArrayVar(&o.addPlatforms, "add-platform", nil, "Add a platform, e.g. osx-arm64 (repeatable)")
	f.StringArrayVar(&o.removePlatform, "remove-platform", nil, "Remove a platform (repeatable)")
	f.StringArrayVar(&o.addChannels, "add-channel", nil, "Add a channel (repeatable)")
	f.StringArrayVar(&o.removeChannels, "remove-channel", nil, "Remove a channel (repeatable)")
	f.BoolVar(&o.install, "install", false, "Install the new version once it is created")
	f.BoolVar(&o.dryRun, "dry-run", false, "Show the summary of changes without creating the version")
	projectVersionCmd.AddCommand(cmd)
}

func readTomlArg(stdin io.Reader, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func runNewVersion(ctx context.Context, out io.Writer, s *client.Stack, projectRef, tomlText string, o newVersionOptions) error {
	id, err := resolveProjectID(ctx, s, projectRef)
	if err != nil {
		return err
	}
	draft, err := s.SDK.Local.GetVersionDraft(ctx, id, o.from)
	if err != nil {
		return err
	}
	req := shared.CreateVersionRequest{
		BaseVersion: draft.BaseVersion,
		Description: strings.TrimSpace(o.description),
		Tags:        o.tags,
	}
	if o.tomlFile != "" {
		req.PixiToml = tomlText
	} else {
		form, err := applyFormEdits(draft.Form, o)
		if err != nil {
			return err
		}
		req.Form = &form
	}
	newToml, err := s.SDK.Local.DraftManifest(ctx, id, req.BaseVersion, req.PixiToml, req.Form)
	if err != nil {
		return err
	}
	diff, err := s.SDK.Local.PreviewChanges(ctx, id, req.BaseVersion, newToml)
	if err != nil {
		return err
	}
	if draft.BaseVersion < draft.LatestVersion {
		fmt.Fprintf(out, "Version %d is older than the latest version, v%d. Changes made in %s won't carry over.\n",
			draft.BaseVersion, draft.LatestVersion, versionList(draft.NewerVersions))
	}
	printVersionSummary(out, draft.ProjectName, draft.BaseVersion, diff)
	if o.dryRun {
		return nil
	}
	if req.Description == "" {
		req.Description = local.SuggestDescription(draft.PixiToml, newToml)
		if req.Description == "" {
			return errors.New("description: Enter a description. (pass --description)")
		}
		fmt.Fprintf(out, "Description (suggested from your changes): %s\n", req.Description)
	}

	job, err := s.SDK.Local.CreateVersion(ctx, id, req)
	if err != nil {
		return err
	}
	if job.VersionNumber == nil {
		fmt.Fprintf(out, "Create version job %s: %s\n", job.ID, job.Status)
		return nil
	}
	n := *job.VersionNumber
	parts := append([]string{fmt.Sprintf("Started from version %d", draft.BaseVersion)},
		artifact.SummarizeRequestedChanges(draft.Form.Requested, requestedOf(newToml))...)
	fmt.Fprintf(out, "Version %d created\n%s\n", n, strings.Join(parts, " · "))
	if !o.install {
		return nil
	}
	inst, err := s.SDK.Exec.InstallVersion(ctx, id, shared.InstallRequest{Version: &n})
	if err != nil {
		return fmt.Errorf("version %d was created but not installed: %w", n, err)
	}
	if inst.Status == "completed" {
		fmt.Fprintf(out, "Version %d installed\n", n)
	}
	return nil
}

func requestedOf(toml string) []artifact.RequestedPackage {
	m, err := artifact.ParseManifest([]byte(toml))
	if err != nil {
		return nil
	}
	return m.Form.Requested
}

// applyFormEdits applies the --add/--remove style flags to the base form.
func applyFormEdits(base shared.ManifestForm, o newVersionOptions) (shared.ManifestForm, error) {
	f := base
	f.Channels = slices.Clone(base.Channels)
	f.Platforms = slices.Clone(base.Platforms)
	f.Requested = slices.Clone(base.Requested)

	for _, raw := range o.removePackages {
		source, name := splitSource(strings.TrimSpace(raw))
		before := len(f.Requested)
		f.Requested = slices.DeleteFunc(f.Requested, func(p artifact.RequestedPackage) bool {
			return sameSource(p.Source, source) && strings.EqualFold(p.Name, name)
		})
		if len(f.Requested) == before {
			return f, fmt.Errorf("--remove %s: the base version has no package %q", raw, name)
		}
	}
	for _, raw := range o.addPackages {
		pkg, err := parsePackageArg(raw)
		if err != nil {
			return f, err
		}
		i := slices.IndexFunc(f.Requested, func(p artifact.RequestedPackage) bool {
			return sameSource(p.Source, pkg.Source) && strings.EqualFold(p.Name, pkg.Name)
		})
		if i >= 0 {
			f.Requested[i].Constraint = pkg.Constraint
		} else {
			f.Requested = append(f.Requested, pkg)
		}
	}
	f.Platforms = editList(f.Platforms, o.addPlatforms, o.removePlatform)
	f.Channels = editList(f.Channels, o.addChannels, o.removeChannels)
	return f, nil
}

func editList(list, add, remove []string) []string {
	for _, r := range remove {
		list = slices.DeleteFunc(list, func(x string) bool { return x == strings.TrimSpace(r) })
	}
	for _, a := range add {
		a = strings.TrimSpace(a)
		if a != "" && !slices.Contains(list, a) {
			list = append(list, a)
		}
	}
	return list
}

func splitSource(s string) (artifact.PackageSource, string) {
	if rest, ok := strings.CutPrefix(s, "pypi:"); ok {
		return artifact.SourcePyPI, rest
	}
	return artifact.SourceConda, strings.TrimPrefix(s, "conda:")
}

func sameSource(a, b artifact.PackageSource) bool {
	norm := func(s artifact.PackageSource) artifact.PackageSource {
		if s == "" {
			return artifact.SourceConda
		}
		return s
	}
	return norm(a) == norm(b)
}

// parsePackageArg parses "name", "name=constraint", "name>=1.0" or
// "pypi:name==2". A single "=" separates the name from the constraint;
// "==" is kept as the exact-version operator.
func parsePackageArg(raw string) (artifact.RequestedPackage, error) {
	source, s := splitSource(strings.TrimSpace(raw))
	end := strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	})
	name, rest := s, ""
	if end >= 0 {
		name, rest = s[:end], strings.TrimSpace(s[end:])
	}
	if err := artifact.ValidatePackageName(name); err != nil {
		return artifact.RequestedPackage{}, fmt.Errorf("--add %s: %w", raw, err)
	}
	if strings.HasPrefix(rest, "=") && !strings.HasPrefix(rest, "==") {
		rest = strings.TrimSpace(rest[1:])
	}
	pkg := artifact.RequestedPackage{Name: name, Constraint: artifact.NormalizeConstraint(rest)}
	if source == artifact.SourcePyPI {
		pkg.Source = artifact.SourcePyPI
	}
	return pkg, nil
}

// printVersionSummary prints the "Summary of changes" against the base.
func printVersionSummary(w io.Writer, project string, base int, d *shared.VersionDiff) {
	changed := d.RequestedCounts.HasChanges() ||
		len(d.Channels.Added)+len(d.Channels.Removed)+len(d.Platforms.Added)+len(d.Platforms.Removed) > 0
	if !changed {
		fmt.Fprintln(w, local.NothingChangedMessage(project))
		return
	}
	fmt.Fprintf(w, "Summary of changes from version %d:\n", base)
	for _, e := range d.Requested {
		name := e.Name
		if e.Source == artifact.SourcePyPI {
			name = "pypi:" + name
		}
		switch e.Change {
		case artifact.ChangeAdded:
			fmt.Fprintf(w, "  + %s %s\n", name, constraintOf(e.Compare))
		case artifact.ChangeRemoved:
			fmt.Fprintf(w, "  - %s %s\n", name, constraintOf(e.Base))
		case artifact.ChangeChanged:
			fmt.Fprintf(w, "  ~ %s %s -> %s\n", name, constraintOf(e.Base), constraintOf(e.Compare))
		}
	}
	printListChange(w, "channels", d.Channels)
	printListChange(w, "platforms", d.Platforms)
}

func printListChange(w io.Writer, label string, l artifact.ListDiff) {
	var parts []string
	for _, a := range l.Added {
		parts = append(parts, "+"+a)
	}
	for _, r := range l.Removed {
		parts = append(parts, "-"+r)
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "  %s: %s\n", label, strings.Join(parts, " "))
	}
}

func constraintOf(r *artifact.PackageRef) string {
	if r == nil {
		return ""
	}
	if r.Constraint != "" {
		return r.Constraint
	}
	return r.Version
}

// versionList is defined in unit_remote.go.
