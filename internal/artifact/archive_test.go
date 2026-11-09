package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestArchiveNames(t *testing.T) {
	if ArchiveRoot("demo", 3) != "demo-v3" || ArchiveFileName("demo", 3) != "demo-v3.tar.gz" {
		t.Fatal("archive names")
	}
}

func TestWriteArchiveRoundTripAndDeterminism(t *testing.T) {
	in := ArchiveInput{
		Metadata: ArchiveMetadata{Name: "demo", Version: 7, Description: "Add pandas", Tags: []string{"latest", "stable"}},
		Toml:     []byte("[workspace]\nname = \"demo\"\n"),
		Lock:     []byte("version: 6\n"),
	}
	var a, b bytes.Buffer
	if err := WriteArchive(&a, in); err != nil {
		t.Fatal(err)
	}
	if err := WriteArchive(&b, in); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("archive output is not deterministic")
	}

	// Entry list, order, modes and times.
	gz, err := gzip.NewReader(bytes.NewReader(a.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !gz.ModTime.IsZero() || gz.Name != "" {
		t.Errorf("gzip header not empty: %+v", gz.Header)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if !h.ModTime.Equal(archiveModTime) || h.Uid != 0 || h.Gid != 0 || h.Uname != "" {
			t.Errorf("%s: non-deterministic header %+v", h.Name, h)
		}
	}
	want := []string{"demo-v7/", "demo-v7/nebi.json", "demo-v7/pixi.lock", "demo-v7/pixi.toml"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}

	got, err := ReadArchive(bytes.NewReader(a.Bytes()), 0)
	if err != nil {
		t.Fatal(err)
	}
	wantMD := in.Metadata
	wantMD.FormatVersion = ArchiveFormatVersion
	if !reflect.DeepEqual(got.Metadata, wantMD) || !bytes.Equal(got.Toml, in.Toml) || !bytes.Equal(got.Lock, in.Lock) {
		t.Fatalf("ReadArchive = %+v", got)
	}
}

func TestWriteArchiveWithoutLock(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteArchive(&buf, ArchiveInput{Metadata: ArchiveMetadata{Name: "x", Version: 1}, Toml: []byte("a = 1\n")}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadArchive(&buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lock != nil || string(got.Toml) != "a = 1\n" || got.Metadata.Tags != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestWriteArchiveErrors(t *testing.T) {
	tests := []ArchiveInput{
		{Metadata: ArchiveMetadata{Name: "", Version: 1}, Toml: []byte("x")},
		{Metadata: ArchiveMetadata{Name: "../evil", Version: 1}, Toml: []byte("x")},
		{Metadata: ArchiveMetadata{Name: "x", Version: 0}, Toml: []byte("x")},
		{Metadata: ArchiveMetadata{Name: "x", Version: 1}},
	}
	for i, in := range tests {
		if err := WriteArchive(io.Discard, in); err == nil {
			t.Errorf("%d: want error", i)
		}
	}
}

type tarFile struct {
	name string
	body string
	typ  byte
}

func makeTarGz(t *testing.T, files []tarFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		typ := f.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: f.name, Typeflag: typ, Mode: 0o644, Size: int64(len(f.body))}
		if typ != tar.TypeReg {
			h.Size = 0
		}
		if typ == tar.TypeSymlink {
			h.Linkname = "/etc/passwd"
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(f.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestReadArchiveRejects(t *testing.T) {
	meta := `{"format_version":1,"name":"x","version":1}`
	tests := []struct {
		name    string
		data    []byte
		max     int64
		wantErr string
	}{
		{"not gzip", []byte("plain"), 0, "not gzip"},
		{"escape", makeTarGz(t, []tarFile{{name: "../x/nebi.json", body: meta}}), 0, "escapes"},
		{"absolute", makeTarGz(t, []tarFile{{name: "/x/nebi.json", body: meta}}), 0, "escapes"},
		{"two roots", makeTarGz(t, []tarFile{{name: "a/nebi.json", body: meta}, {name: "b/pixi.toml", body: "x"}}), 0, "outside"},
		{"nested", makeTarGz(t, []tarFile{{name: "a/b/pixi.toml", body: "x"}, {name: "a/nebi.json", body: meta}}), 0, "outside"},
		{"too big", makeTarGz(t, []tarFile{{name: "a/pixi.toml", body: "0123456789"}}), 5, "limit"},
		{"no meta", makeTarGz(t, []tarFile{{name: "a/pixi.toml", body: "x"}}), 0, "no nebi.json"},
		{"bad meta", makeTarGz(t, []tarFile{{name: "a/nebi.json", body: "{"}}), 0, "parse nebi.json"},
		{"no toml", makeTarGz(t, []tarFile{{name: "a/nebi.json", body: meta}}), 0, "no pixi.toml"},
		{"truncated", makeTarGz(t, []tarFile{{name: "a/nebi.json", body: meta}})[:20], 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadArchive(bytes.NewReader(tt.data), tt.max)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestReadArchiveSkipsOtherEntries(t *testing.T) {
	data := makeTarGz(t, []tarFile{
		{name: "a/", typ: tar.TypeDir},
		{name: "a/link", typ: tar.TypeSymlink},
		{name: "a/README", body: "ignored"},
		{name: "a/nebi.json", body: `{"format_version":1,"name":"x","version":2}`},
		{name: "a/pixi.toml", body: "t"},
	})
	got, err := ReadArchive(bytes.NewReader(data), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Version != 2 || string(got.Toml) != "t" {
		t.Fatalf("got %+v", got)
	}
}
