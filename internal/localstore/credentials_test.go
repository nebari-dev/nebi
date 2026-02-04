package localstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.json")

	creds := &Credentials{
		Servers: map[string]*ServerCredential{
			"https://nebi.example.com": {
				Token:    "test-token-123",
				Username: "admin",
			},
		},
	}

	// Write manually to the temp path
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Read back
	readData, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatal(err)
	}

	var loaded Credentials
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatal(err)
	}

	sc, ok := loaded.Servers["https://nebi.example.com"]
	if !ok {
		t.Fatal("server not found")
	}
	if sc.Token != "test-token-123" || sc.Username != "admin" {
		t.Fatalf("unexpected credential: %+v", sc)
	}
}

func TestCredentialsFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.json")

	data := []byte(`{"servers":{}}`)
	if err := os.WriteFile(credPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatal(err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}
}
