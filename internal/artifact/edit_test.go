package artifact

import (
	"errors"
	"strings"
	"testing"
)

func TestSetWorkspaceFieldsReplacesNameKeepsRest(t *testing.T) {
	in := `# my env
[workspace]
name = "something-else"   # old name
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
name = "not the workspace name"
numpy = ">=1.26"
`
	want := `# my env
[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
name = "not the workspace name"
numpy = ">=1.26"
`
	got, err := SetWorkspaceFields(in, "demo", "")
	if err != nil {
		t.Fatalf("SetWorkspaceFields: %v", err)
	}
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSetWorkspaceFieldsInsertsMissingKeys(t *testing.T) {
	in := "[workspace]\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	got, err := SetWorkspaceFields(in, "demo", "my description")
	if err != nil {
		t.Fatalf("SetWorkspaceFields: %v", err)
	}
	want := "[workspace]\nname = \"demo\"\ndescription = \"my description\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestSetWorkspaceFieldsInsertsDescriptionAfterExistingName(t *testing.T) {
	in := "[workspace]\nchannels = [\"conda-forge\"]\nname = \"x\"\nplatforms = [\"linux-64\"]\n"
	got, err := SetWorkspaceFields(in, "demo", "d")
	if err != nil {
		t.Fatal(err)
	}
	want := "[workspace]\nchannels = [\"conda-forge\"]\nname = \"demo\"\ndescription = \"d\"\nplatforms = [\"linux-64\"]\n"
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestSetWorkspaceFieldsReplacesMultilineDescription(t *testing.T) {
	in := "[workspace]\nname = \"x\"\ndescription = \"\"\"\nfirst line\nsecond line\n\"\"\"\nplatforms = [\"linux-64\"]\n"
	got, err := SetWorkspaceFields(in, "demo", "short")
	if err != nil {
		t.Fatalf("SetWorkspaceFields: %v", err)
	}
	want := "[workspace]\nname = \"demo\"\ndescription = \"short\"\nplatforms = [\"linux-64\"]\n"
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestSetWorkspaceFieldsKeepsDescriptionWhenEmpty(t *testing.T) {
	in := "[workspace]\nname = \"x\"\ndescription = \"keep me\"\nplatforms = [\"linux-64\"]\n"
	got, err := SetWorkspaceFields(in, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(got)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "demo" || m.Description != "keep me" {
		t.Fatalf("got %+v", m)
	}
}

func TestSetWorkspaceFieldsLegacyProjectTable(t *testing.T) {
	in := "[project]\nname = \"old\"\nplatforms = [\"linux-64\"]\n"
	got, err := SetWorkspaceFields(in, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "[project]\nname = \"demo\"\nplatforms = [\"linux-64\"]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSetWorkspaceFieldsDoesNotTouchSubTables(t *testing.T) {
	in := "[workspace]\nplatforms = [\"linux-64\"]\n\n[workspace.extra]\nname = \"inner\"\n"
	got, err := SetWorkspaceFields(in, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "name = \"inner\"") {
		t.Fatalf("sub-table key was changed:\n%s", got)
	}
	if !strings.HasPrefix(got, "[workspace]\nname = \"demo\"\n") {
		t.Fatalf("name not inserted in [workspace]:\n%s", got)
	}
}

func TestSetWorkspaceFieldsPreservesCRLF(t *testing.T) {
	in := "[workspace]\r\nname = \"x\"\r\nplatforms = [\"linux-64\"]\r\n"
	got, err := SetWorkspaceFields(in, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "[workspace]\r\nname = \"demo\"\r\nplatforms = [\"linux-64\"]\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSetWorkspaceFieldsRejectsUnsupportedLayouts(t *testing.T) {
	cases := map[string]string{
		"invalid toml":          "[workspace\nname = ",
		"no workspace table":    "[dependencies]\nnumpy = \"*\"\n",
		"dotted keys, no table": "workspace.name = \"x\"\nworkspace.platforms = [\"linux-64\"]\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SetWorkspaceFields(in, "demo", ""); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}
