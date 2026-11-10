package artifact

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Archive format: a gzip-compressed tar (.tar.gz) with one top-level
// directory "<name>-v<version>/" holding nebi.json, pixi.lock (when the
// version has one) and pixi.toml, in that (sorted) order. The output is
// byte-for-byte deterministic: fixed mtimes, uid/gid 0, no user names, fixed
// modes and an empty gzip header.
const (
	ArchiveFormatVersion = 1
	ArchiveExtension     = ".tar.gz"
	ArchiveContentType   = "application/gzip"
	ArchiveMetadataFile  = "nebi.json"
	// DefaultArchiveMaxFileBytes bounds each file ReadArchive will accept.
	DefaultArchiveMaxFileBytes int64 = 64 << 20
)

// archiveModTime is the fixed timestamp of every entry.
var archiveModTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// ArchiveMetadata is written to nebi.json.
type ArchiveMetadata struct {
	FormatVersion int      `json:"format_version"`
	Name          string   `json:"name"`
	Version       int      `json:"version"`
	Description   string   `json:"description,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

// ArchiveInput is what WriteArchive packages. FormatVersion in Metadata is
// filled in automatically.
type ArchiveInput struct {
	Metadata ArchiveMetadata
	Toml     []byte
	Lock     []byte // optional
}

// ArchiveContents is what ReadArchive returns.
type ArchiveContents struct {
	Metadata ArchiveMetadata
	Toml     []byte
	Lock     []byte
}

// ArchiveRoot is the top-level directory inside the archive: "<name>-v<N>".
func ArchiveRoot(name string, version int) string {
	return name + "-v" + strconv.Itoa(version)
}

// ArchiveFileName is the suggested download name: "<name>-v<N>.tar.gz".
func ArchiveFileName(name string, version int) string {
	return ArchiveRoot(name, version) + ArchiveExtension
}

// WriteArchive streams a deterministic .tar.gz of one version to w.
func WriteArchive(w io.Writer, in ArchiveInput) error {
	md := in.Metadata
	if err := ValidateWorkspaceName(md.Name); err != nil {
		return err
	}
	if md.Version < 1 {
		return fmt.Errorf("archive version must be at least 1, got %d", md.Version)
	}
	if len(in.Toml) == 0 {
		return errors.New("archive needs a pixi.toml")
	}
	md.FormatVersion = ArchiveFormatVersion
	mdJSON, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	mdJSON = append(mdJSON, '\n')

	root := ArchiveRoot(md.Name, md.Version)
	files := []struct {
		name string
		data []byte
	}{
		{ArchiveMetadataFile, mdJSON},
		{LockFile, in.Lock},
		{ManifestFile, in.Toml},
	}

	gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     root + "/",
		Mode:     0o755,
		ModTime:  archiveModTime,
		Format:   tar.FormatPAX,
	}); err != nil {
		return err
	}
	for _, f := range files {
		if f.name == LockFile && len(f.data) == 0 {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     root + "/" + f.name,
			Mode:     0o644,
			Size:     int64(len(f.data)),
			ModTime:  archiveModTime,
			Format:   tar.FormatPAX,
		}); err != nil {
			return err
		}
		if _, err := tw.Write(f.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// ReadArchive reads an archive written by WriteArchive. Each file is capped
// at maxFileBytes (<= 0 means DefaultArchiveMaxFileBytes). Unknown entries
// are ignored; entries outside the single top-level directory are rejected.
func ReadArchive(r io.Reader, maxFileBytes int64) (*ArchiveContents, error) {
	if maxFileBytes <= 0 {
		maxFileBytes = DefaultArchiveMaxFileBytes
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("archive is not gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := &ArchiveContents{}
	var root string
	var haveMeta bool
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		name := path.Clean(h.Name)
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("archive entry %q escapes the archive", h.Name)
		}
		dir, file := path.Split(name)
		dir = strings.TrimSuffix(dir, "/")
		if h.Typeflag == tar.TypeDir {
			if dir == "" {
				dir = file
			}
			if root == "" {
				root = dir
			}
			continue
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if root == "" {
			root = dir
		}
		if dir != root || strings.Contains(dir, "/") {
			return nil, fmt.Errorf("archive entry %q is outside %q", h.Name, root)
		}
		if h.Size > maxFileBytes {
			return nil, fmt.Errorf("archive entry %q is %d bytes, over the %d byte limit", h.Name, h.Size, maxFileBytes)
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxFileBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", h.Name, err)
		}
		if int64(len(data)) > maxFileBytes {
			return nil, fmt.Errorf("archive entry %q is over the %d byte limit", h.Name, maxFileBytes)
		}
		switch file {
		case ArchiveMetadataFile:
			if err := json.Unmarshal(data, &out.Metadata); err != nil {
				return nil, fmt.Errorf("parse %s: %w", ArchiveMetadataFile, err)
			}
			haveMeta = true
		case ManifestFile:
			out.Toml = data
		case LockFile:
			out.Lock = data
		}
	}
	if !haveMeta {
		return nil, fmt.Errorf("archive has no %s", ArchiveMetadataFile)
	}
	if len(out.Toml) == 0 {
		return nil, fmt.Errorf("archive has no %s", ManifestFile)
	}
	return out, nil
}
