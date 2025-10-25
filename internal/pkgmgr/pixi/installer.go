package pixi

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	pixiVersion     = "v0.58.0"
	pixiDownloadURL = "https://github.com/prefix-dev/pixi/releases/download/%s/pixi-%s.tar.gz"
)

// getInstallDir returns the directory where pixi should be installed
func getInstallDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	installDir := filepath.Join(homeDir, ".local", "bin")
	return installDir, nil
}

// getPlatform returns the platform string for pixi downloads
func getPlatform() (string, error) {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go arch to pixi arch naming
	archMap := map[string]string{
		"amd64": "x86_64",
		"arm64": "aarch64",
	}

	pixiArch, ok := archMap[arch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Map Go OS to pixi OS naming
	osMap := map[string]string{
		"linux":  "linux",
		"darwin": "apple-darwin",
		"windows": "pc-windows-msvc",
	}

	pixiOS, ok := osMap[os]
	if !ok {
		return "", fmt.Errorf("unsupported operating system: %s", os)
	}

	return fmt.Sprintf("%s-%s", pixiArch, pixiOS), nil
}

// downloadPixi downloads the pixi binary for the current platform
func downloadPixi(ctx context.Context, platform, destDir string) error {
	url := fmt.Sprintf(pixiDownloadURL, pixiVersion, platform)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download pixi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download pixi: HTTP %d", resp.StatusCode)
	}

	// Create a temporary file to store the download
	tmpFile, err := os.CreateTemp("", "pixi-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write download: %w", err)
	}

	// Extract the binary
	if err := extractPixi(tmpFile.Name(), destDir); err != nil {
		return fmt.Errorf("failed to extract pixi: %w", err)
	}

	return nil
}

// extractPixi extracts the pixi binary from the tar.gz archive
func extractPixi(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open tar file: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Find and extract the pixi binary
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		// Look for the pixi binary (it's usually in the root of the archive)
		if strings.HasSuffix(header.Name, "pixi") || header.Name == "pixi" {
			destPath := filepath.Join(destDir, "pixi")

			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("failed to create pixi binary: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tr); err != nil {
				return fmt.Errorf("failed to write pixi binary: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("pixi binary not found in archive")
}

// InstallPixi automatically downloads and installs pixi to ~/.local/bin
func InstallPixi(ctx context.Context) (string, error) {
	// Get platform
	platform, err := getPlatform()
	if err != nil {
		return "", err
	}

	// Get install directory
	installDir, err := getInstallDir()
	if err != nil {
		return "", err
	}

	// Create install directory if it doesn't exist
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	// Download and install pixi
	if err := downloadPixi(ctx, platform, installDir); err != nil {
		return "", err
	}

	pixiPath := filepath.Join(installDir, "pixi")

	// Verify installation
	if err := exec.CommandContext(ctx, pixiPath, "--version").Run(); err != nil {
		return "", fmt.Errorf("pixi installation verification failed: %w", err)
	}

	return pixiPath, nil
}
