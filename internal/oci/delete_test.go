package oci

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestTagDeletionSupport(t *testing.T) {
	cases := map[string]bool{
		"ghcr.io":            false,
		"GHCR.io/org":        false,
		"docker.io":          false,
		"quay.io":            true,
		"harbor.example.com": true,
		"localhost:5000":     true,
	}
	for host, want := range cases {
		got, reason := TagDeletionSupport(host)
		if got != want {
			t.Errorf("%s: supported = %v, want %v", host, got, want)
		}
		if !got && reason == "" {
			t.Errorf("%s: unsupported without a reason", host)
		}
	}
}

func TestPublishFilesAndDeleteTag(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	reg := testRegistry(host, "")

	toml := []byte("[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n")
	lock := []byte("version: 6\n")
	res, err := PublishFiles(ctx, toml, lock, reg, "team/demo", "5")
	if err != nil {
		t.Fatalf("PublishFiles: %v", err)
	}
	if res.Digest == "" || res.Repository != host+"/team/demo" {
		t.Fatalf("unexpected result %+v", res)
	}
	// A second tag on the same manifest must survive deleting the first.
	if _, err := PublishFiles(ctx, toml, lock, reg, "team/demo", "stable"); err != nil {
		t.Fatalf("PublishFiles stable: %v", err)
	}

	pulled, err := PullBundle(ctx, host+"/team/demo", "5", PullOptions{PlainHTTP: true})
	if err != nil {
		t.Fatalf("PullBundle: %v", err)
	}
	if pulled.PixiLock != "version: 6" {
		t.Fatalf("lock = %q", pulled.PixiLock)
	}

	if err := DeleteTag(ctx, reg, "team/demo", "5"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := PullBundle(ctx, host+"/team/demo", "5", PullOptions{PlainHTTP: true}); err == nil {
		t.Fatal("tag 5 still resolves after DeleteTag")
	}
	if _, err := PullBundle(ctx, host+"/team/demo", "stable", PullOptions{PlainHTTP: true}); err != nil {
		t.Fatalf("tag stable was removed with tag 5: %v", err)
	}
	// Deleting a missing tag is not an error.
	if err := DeleteTag(ctx, reg, "team/demo", "5"); err != nil {
		t.Fatalf("DeleteTag on a missing tag: %v", err)
	}

	tags, err := ListTags(ctx, host+"/team/demo", BrowseOptions{PlainHTTP: true})
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "stable" {
		t.Fatalf("tags = %+v", tags)
	}
	repos, err := ListRepositories(ctx, BrowseOptions{RegistryHost: host, PlainHTTP: true})
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "team/demo" {
		t.Fatalf("repos = %+v", repos)
	}
}

func TestDeleteTagUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":"UNSUPPORTED","message":"not supported"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	err := DeleteTag(context.Background(), Registry{Host: u.Host, PlainHTTP: true}, "a/b", "1")
	if !errors.Is(err, ErrTagDeletionUnsupported) {
		t.Fatalf("err = %v, want ErrTagDeletionUnsupported", err)
	}

	srv405 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	t.Cleanup(srv405.Close)
	u, _ = url.Parse(srv405.URL)
	err = DeleteTag(context.Background(), Registry{Host: u.Host, PlainHTTP: true}, "a/b", "1")
	if !errors.Is(err, ErrTagDeletionUnsupported) {
		t.Fatalf("405: err = %v, want ErrTagDeletionUnsupported", err)
	}
}
