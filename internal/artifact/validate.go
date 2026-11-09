package artifact

import (
	"fmt"
	"regexp"
	"strings"
)

// Field names used in ValidationError.Field.
const (
	FieldName        = "name"
	FieldPlatforms   = "platforms"
	FieldChannels    = "channels"
	FieldPackages    = "packages"
	FieldTags        = "tags"
	FieldDescription = "description"
	FieldManifest    = "manifest"
)

// ValidationError is a user-facing validation failure for one field.
// Message is UI copy (matches the prototype where it has copy).
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Message }

// ValidationErrors is a list of field errors; it is itself an error.
type ValidationErrors []*ValidationError

func (v ValidationErrors) Error() string {
	msgs := make([]string, len(v))
	for i, e := range v {
		msgs[i] = e.Message
	}
	return strings.Join(msgs, " ")
}

// errOrNil returns nil for an empty list so callers can `return errs.errOrNil()`.
func (v ValidationErrors) errOrNil() error {
	if len(v) == 0 {
		return nil
	}
	return v
}

func verr(field, msg string) *ValidationError { return &ValidationError{Field: field, Message: msg} }

var (
	projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	tagPattern         = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]*$`)
)

// ValidateWorkspaceName is the permissive rule for names that already exist
// (read from a pixi.toml, a server ref or a pulled project): not empty, no
// '/', '\' or ':', and not "." or "..". Moved from internal/pixi.
func ValidateWorkspaceName(name string) error {
	if name == "" {
		return fmt.Errorf("project name must not be empty")
	}
	if strings.ContainsAny(name, `/\:`) {
		return fmt.Errorf("project name %q must not contain '/', '\\', or ':'", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("project name %q is reserved and cannot be used", name)
	}
	return nil
}

// ValidateProjectName is the strict rule for creating or renaming a project:
// letters, numbers, hyphens, underscores or dots, and not "." or "..".
// The duplicate-name check needs the project index and lives in the SDK.
func ValidateProjectName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return verr(FieldName, "Enter a project name.")
	}
	if !projectNamePattern.MatchString(n) {
		return verr(FieldName, "Use letters, numbers, hyphens, underscores or dots.")
	}
	if n == "." || n == ".." {
		return verr(FieldName, fmt.Sprintf("“%s” is reserved. Choose another name.", n))
	}
	return nil
}

// DuplicateProjectNameError is the copy for a name collision (the SDK does
// the lookup and returns this).
func DuplicateProjectNameError() *ValidationError {
	return verr(FieldName, "A project with that name already exists.")
}

// ValidatePackageName checks a package name as the form does: letters,
// numbers, hyphens, underscores or dots. Callers lowercase conda names.
func ValidatePackageName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return verr(FieldPackages, "Enter a package name.")
	}
	if !packageNamePattern.MatchString(n) {
		return verr(FieldPackages, "Package names use letters, numbers, hyphens, underscores or dots.")
	}
	return nil
}

// ValidatePlatforms requires at least one platform, each a name pixi knows,
// with no duplicates.
func ValidatePlatforms(platforms []string) error {
	if len(platforms) == 0 {
		return verr(FieldPlatforms, "Add at least one platform.")
	}
	seen := map[string]bool{}
	for _, p := range platforms {
		if !platformPattern.MatchString(p) && p != "noarch" {
			return verr(FieldPlatforms, fmt.Sprintf("“%s” isn’t a platform name. Use pixi’s name, e.g. linux-64.", p))
		}
		if !IsKnownPlatform(p) {
			return verr(FieldPlatforms, fmt.Sprintf("“%s” isn’t a platform pixi knows.", p))
		}
		if seen[p] {
			return verr(FieldPlatforms, fmt.Sprintf("“%s” is listed twice.", p))
		}
		seen[p] = true
	}
	return nil
}

// ValidateChannels rejects blank or duplicate channels. An empty list is
// allowed (callers default to conda-forge as the prototype does).
func ValidateChannels(channels []string) error {
	seen := map[string]bool{}
	for _, c := range channels {
		if strings.TrimSpace(c) == "" {
			return verr(FieldChannels, "Channel names can’t be blank.")
		}
		if seen[c] {
			return verr(FieldChannels, fmt.Sprintf("“%s” is listed twice.", c))
		}
		seen[c] = true
	}
	return nil
}

// ValidateTag checks one version tag: starts with a letter, then letters,
// numbers, dots, hyphens and underscores.
func ValidateTag(tag string) error {
	if tagPattern.MatchString(tag) {
		return nil
	}
	if tag != "" && (tag[0] >= 'a' && tag[0] <= 'z' || tag[0] >= 'A' && tag[0] <= 'Z') {
		return verr(FieldTags, fmt.Sprintf("“%s” can’t be used: tags use letters, numbers, dots, hyphens and underscores.", tag))
	}
	return verr(FieldTags, fmt.Sprintf("“%s” can’t be used: tags must start with a letter.", tag))
}

// ValidateTags validates every tag and returns the first failure.
func ValidateTags(tags []string) error {
	for _, t := range tags {
		if err := ValidateTag(t); err != nil {
			return err
		}
	}
	return nil
}

// ValidateDescription requires a non-blank version description.
func ValidateDescription(description string) error {
	if strings.TrimSpace(description) == "" {
		return verr(FieldDescription, "Enter a description.")
	}
	return nil
}

// ValidateForm validates the GUI form model and returns ValidationErrors
// (one per failing field) or nil.
func ValidateForm(f ManifestForm) error {
	var errs ValidationErrors
	add := func(err error) {
		if ve, ok := err.(*ValidationError); ok {
			errs = append(errs, ve)
		}
	}
	add(ValidateProjectName(f.Name))
	add(ValidatePlatforms(f.Platforms))
	add(ValidateChannels(f.Channels))
	seen := map[string]bool{}
	for _, p := range f.Requested {
		if err := ValidatePackageName(p.Name); err != nil {
			add(err)
			break
		}
		k := string(p.sourceOrDefault()) + ":" + p.Name
		if seen[k] {
			add(verr(FieldPackages, fmt.Sprintf("%s is listed twice.", p.Name)))
			break
		}
		seen[k] = true
	}
	return errs.errOrNil()
}

// ValidateManifestStructure checks what the pixi.toml editor needs before
// save: a [workspace] (or legacy [project]) table and at least one platform.
func ValidateManifestStructure(m *Manifest) error {
	if m == nil || !m.HasWorkspace {
		return verr(FieldManifest, "pixi.toml needs a [workspace] table.")
	}
	if len(m.Form.Platforms) == 0 {
		return verr(FieldManifest, "List at least one platform in platforms = [ ].")
	}
	return nil
}
