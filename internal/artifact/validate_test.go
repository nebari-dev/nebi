package artifact

import (
	"errors"
	"testing"
)

func msg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestSplitJoinConstraint(t *testing.T) {
	tests := []struct {
		in, op, version string
	}{
		{"", OpAny, ""},
		{"*", OpAny, ""},
		{"  ", OpAny, ""},
		{">=2.0", OpGE, "2.0"},
		{">= 2.0", OpGE, "2.0"},
		{"==1.2.3", OpEQ, "1.2.3"},
		{"<=1", OpLE, "1"},
		{">1", OpGT, "1"},
		{"<1", OpLT, "1"},
		{"!=1.0", OpNE, "1.0"},
		{"~=2.31", OpCompatible, "2.31"},
		{"3.11.*", OpNone, "3.11.*"},
		{"1.2.3", OpNone, "1.2.3"},
		{"=1.2", OpNone, "=1.2"},
		{">=1.2,<2", OpNone, ">=1.2,<2"},
		{">=1|<0.5", OpNone, ">=1|<0.5"},
		{">=", OpNone, ">="},
		{">=1 <2", OpNone, ">=1 <2"},
	}
	for _, tt := range tests {
		op, v := SplitConstraint(tt.in)
		if op != tt.op || v != tt.version {
			t.Errorf("SplitConstraint(%q) = %q, %q; want %q, %q", tt.in, op, v, tt.op, tt.version)
		}
		// Joining what we split gives the normalized original back.
		if got, want := JoinConstraint(op, v), NormalizeConstraint(tt.in); got != stripOpSpace(want) {
			t.Errorf("JoinConstraint(SplitConstraint(%q)) = %q, want %q", tt.in, got, want)
		}
	}

	joins := []struct {
		op, version, want string
	}{
		{OpGE, "", "*"},
		{OpAny, "1.0", "*"},
		{OpNone, "3.11.*", "3.11.*"},
		{"", "1.0", "1.0"},
		{OpGE, "1.0", ">=1.0"},
		{OpGE, "==1.0", "==1.0"},
		{OpEQ, " 2 ", "==2"},
	}
	for _, tt := range joins {
		if got := JoinConstraint(tt.op, tt.version); got != tt.want {
			t.Errorf("JoinConstraint(%q, %q) = %q, want %q", tt.op, tt.version, got, tt.want)
		}
	}
	if len(ConstraintOperators) != 8 || ConstraintOperators[0].Op != OpNone {
		t.Errorf("ConstraintOperators = %+v", ConstraintOperators)
	}
}

// stripOpSpace turns ">= 2.0" into ">=2.0" (the form drops that space).
func stripOpSpace(s string) string {
	op, v := SplitConstraint(s)
	if op == OpNone || op == OpAny {
		return NormalizeConstraint(s)
	}
	return op + v
}

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"my-project", false},
		{"has space", false},
		{"", true},
		{"a/b", true},
		{`a\b`, true},
		{"a:b", true},
		{".", true},
		{"..", true},
	}
	for _, tt := range tests {
		if err := ValidateWorkspaceName(tt.in); (err != nil) != tt.wantErr {
			t.Errorf("ValidateWorkspaceName(%q) = %v", tt.in, err)
		}
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"data-science_2.0", ""},
		{"  ", "Enter a project name."},
		{"has space", "Use letters, numbers, hyphens, underscores or dots."},
		{"a/b", "Use letters, numbers, hyphens, underscores or dots."},
		{"..", "“..” is reserved. Choose another name."},
	}
	for _, tt := range tests {
		err := ValidateProjectName(tt.in)
		if msg(err) != tt.want {
			t.Errorf("ValidateProjectName(%q) = %q, want %q", tt.in, msg(err), tt.want)
		}
		var ve *ValidationError
		if err != nil && (!errors.As(err, &ve) || ve.Field != FieldName) {
			t.Errorf("ValidateProjectName(%q) error is not a name ValidationError: %#v", tt.in, err)
		}
	}
	if DuplicateProjectNameError().Message != "A project with that name already exists." {
		t.Error("duplicate copy changed")
	}
}

func TestValidatePackageName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"numpy", ""},
		{"ruamel.yaml", ""},
		{"", "Enter a package name."},
		{"num py", "Package names use letters, numbers, hyphens, underscores or dots."},
		{"numpy>=2", "Package names use letters, numbers, hyphens, underscores or dots."},
	}
	for _, tt := range tests {
		if got := msg(ValidatePackageName(tt.in)); got != tt.want {
			t.Errorf("ValidatePackageName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidatePlatforms(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{[]string{"linux-64", "osx-arm64", "noarch", "linux-ppc64le", "emscripten-wasm32"}, ""},
		{nil, "Add at least one platform."},
		{[]string{}, "Add at least one platform."},
		{[]string{"Linux"}, "“Linux” isn’t a platform name. Use pixi’s name, e.g. linux-64."},
		{[]string{"linux-128"}, "“linux-128” isn’t a platform pixi knows."},
		{[]string{"linux-64", "linux-64"}, "“linux-64” is listed twice."},
	}
	for _, tt := range tests {
		if got := msg(ValidatePlatforms(tt.in)); got != tt.want {
			t.Errorf("ValidatePlatforms(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidateChannels(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"conda-forge", "https://x/y"}, ""},
		{[]string{" "}, "Channel names can’t be blank."},
		{[]string{"a", "a"}, "“a” is listed twice."},
	}
	for _, tt := range tests {
		if got := msg(ValidateChannels(tt.in)); got != tt.want {
			t.Errorf("ValidateChannels(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidateTags(t *testing.T) {
	tests := []struct{ in, want string }{
		{"latest", ""},
		{"v1.2_rc-1", ""},
		{"1.0", "“1.0” can’t be used: tags must start with a letter."},
		{"", "“” can’t be used: tags must start with a letter."},
		{"bad tag", "“bad tag” can’t be used: tags use letters, numbers, dots, hyphens and underscores."},
	}
	for _, tt := range tests {
		if got := msg(ValidateTag(tt.in)); got != tt.want {
			t.Errorf("ValidateTag(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if err := ValidateTags([]string{"latest", "1x", "ok"}); msg(err) != "“1x” can’t be used: tags must start with a letter." {
		t.Errorf("ValidateTags = %v", err)
	}
	if err := ValidateTags([]string{"latest", "stable"}); err != nil {
		t.Errorf("ValidateTags = %v", err)
	}
}

func TestValidateDescription(t *testing.T) {
	if msg(ValidateDescription("  ")) != "Enter a description." || ValidateDescription("x") != nil {
		t.Error("ValidateDescription")
	}
}

func TestValidateForm(t *testing.T) {
	ok := ManifestForm{Name: "x", Platforms: []string{"linux-64"}, Requested: []RequestedPackage{{Name: "a"}, {Name: "a", Source: SourcePyPI}}}
	if err := ValidateForm(ok); err != nil {
		t.Fatalf("ValidateForm(ok) = %v", err)
	}
	bad := ManifestForm{Name: "", Channels: []string{""}, Requested: []RequestedPackage{{Name: "a"}, {Name: "a", Source: SourceConda}}}
	err := ValidateForm(bad)
	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("ValidateForm(bad) = %#v", err)
	}
	var fields []string
	for _, e := range errs {
		fields = append(fields, e.Field)
	}
	want := []string{FieldName, FieldPlatforms, FieldChannels, FieldPackages}
	if len(fields) != len(want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Fatalf("fields = %v, want %v", fields, want)
		}
	}
	if errs[3].Message != "a is listed twice." {
		t.Errorf("dup message = %q", errs[3].Message)
	}
	if err.Error() == "" {
		t.Error("empty Error()")
	}
	if msg(ValidateForm(ManifestForm{Name: "x", Platforms: []string{"linux-64"}, Requested: []RequestedPackage{{Name: "b c"}}})) !=
		"Package names use letters, numbers, hyphens, underscores or dots." {
		t.Error("bad package name not reported")
	}
}

func TestValidateManifestStructure(t *testing.T) {
	tests := []struct{ in, want string }{
		{"[workspace]\nname='x'\nplatforms=['linux-64']\n", ""},
		{"[project]\nname='x'\nplatforms=['linux-64']\n", ""},
		{"[dependencies]\n", "pixi.toml needs a [workspace] table."},
		{"[workspace]\nname='x'\n", "List at least one platform in platforms = [ ]."},
	}
	for _, tt := range tests {
		m, err := ParseManifest([]byte(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if got := msg(ValidateManifestStructure(m)); got != tt.want {
			t.Errorf("ValidateManifestStructure(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if ValidateManifestStructure(nil) == nil {
		t.Error("nil manifest should fail")
	}
}

func TestPlatforms(t *testing.T) {
	tests := []struct{ goos, goarch, want string }{
		{"linux", "amd64", "linux-64"},
		{"linux", "arm64", "linux-aarch64"},
		{"darwin", "arm64", "osx-arm64"},
		{"darwin", "amd64", "osx-64"},
		{"windows", "amd64", "win-64"},
		{"windows", "arm64", "win-arm64"},
		{"linux", "ppc64le", "linux-ppc64le"},
		{"plan9", "amd64", ""},
	}
	for _, tt := range tests {
		got := DefaultPlatform(tt.goos, tt.goarch)
		if got != tt.want {
			t.Errorf("DefaultPlatform(%s, %s) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
		if got != "" && !IsKnownPlatform(got) {
			t.Errorf("DefaultPlatform returned unknown platform %q", got)
		}
	}
	for _, p := range CommonPlatforms {
		if !IsKnownPlatform(p) || PlatformLabel(p) == "" {
			t.Errorf("common platform %q not known or unlabeled", p)
		}
	}
	if PlatformDisplayName("osx-arm64", "osx-arm64") != "osx-arm64 · this machine" ||
		PlatformDisplayName("linux-64", "osx-arm64") != "linux-64 · Linux" ||
		PlatformDisplayName("linux-ppc64le", "osx-arm64") != "linux-ppc64le" {
		t.Error("PlatformDisplayName")
	}
}
