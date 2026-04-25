package oci

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewAssetRefs_SkipsCoreFiles(t *testing.T) {
	dir := t.TempDir()
	writePreviewFile(t, filepath.Join(dir, "pixi.toml"), `[project]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]
`)
	writePreviewFile(t, filepath.Join(dir, "pixi.lock"), "version: 6\n")
	writePreviewFile(t, filepath.Join(dir, "notebook.ipynb"), `{"cells": []}`)
	writePreviewFile(t, filepath.Join(dir, "data.csv"), "a,b\n1,2\n")

	refs, err := PreviewAssetRefs(dir)
	if err != nil {
		t.Fatalf("PreviewAssetRefs: %v", err)
	}

	gotPaths := map[string]string{}
	for _, r := range refs {
		gotPaths[r.Path] = r.Digest
	}
	for _, core := range []string{"pixi.toml", "pixi.lock"} {
		if _, ok := gotPaths[core]; ok {
			t.Errorf("PreviewAssetRefs included core file %s; expected skip", core)
		}
	}

	for _, rel := range []string{"notebook.ipynb", "data.csv"} {
		digest, ok := gotPaths[rel]
		if !ok {
			t.Errorf("expected asset %s in refs, got %+v", rel, refs)
			continue
		}
		want := "sha256:" + sha256HexPreview(t, filepath.Join(dir, rel))
		if digest != want {
			t.Errorf("digest mismatch for %s: got %s want %s", rel, digest, want)
		}
	}
}

func writePreviewFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256HexPreview(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
