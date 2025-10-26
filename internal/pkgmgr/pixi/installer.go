package pixi

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
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
	// Based on actual release asset names from https://github.com/prefix-dev/pixi/releases
	var platform string
	switch os {
	case "linux":
		platform = fmt.Sprintf("%s-unknown-linux-musl", pixiArch)
	case "darwin":
		platform = fmt.Sprintf("%s-apple-darwin", pixiArch)
	case "windows":
		platform = fmt.Sprintf("%s-pc-windows-msvc", pixiArch)
	default:
		return "", fmt.Errorf("unsupported operating system: %s", os)
	}

	return platform, nil
}

// downloadPixi downloads the pixi binary for the current platform
func downloadPixi(ctx context.Context, platform, destDir string) error {
	url := fmt.Sprintf(pixiDownloadURL, pixiVersion, platform)

	slog.Info("Downloading pixi",
		"version", pixiVersion,
		"platform", platform,
		"url", url,
		"dest", destDir)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		slog.Error("Failed to create HTTP request", "error", err, "url", url)
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Failed to download pixi", "error", err, "url", url)
		return fmt.Errorf("failed to download pixi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Pixi download failed with HTTP error",
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"url", url)
		return fmt.Errorf("failed to download pixi from %s: HTTP %d (%s)", url, resp.StatusCode, resp.Status)
	}

	slog.Info("Pixi download started", "content_length", resp.ContentLength)

	// Create a temporary file to store the download
	tmpFile, err := os.CreateTemp("", "pixi-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download to temp file
	bytesWritten, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		slog.Error("Failed to write downloaded file", "error", err, "temp_file", tmpFile.Name())
		return fmt.Errorf("failed to write download: %w", err)
	}

	slog.Info("Download completed", "bytes", bytesWritten, "temp_file", tmpFile.Name())

	// Extract the binary
	slog.Info("Extracting pixi binary", "archive", tmpFile.Name(), "dest", destDir)
	if err := extractPixi(tmpFile.Name(), destDir); err != nil {
		slog.Error("Failed to extract pixi", "error", err)
		return fmt.Errorf("failed to extract pixi: %w", err)
	}

	slog.Info("Pixi extraction completed successfully")

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

			slog.Info("Found pixi binary in archive", "name", header.Name, "size", header.Size)

			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				slog.Error("Failed to create output file", "error", err, "path", destPath)
				return fmt.Errorf("failed to create pixi binary: %w", err)
			}
			defer outFile.Close()

			bytesWritten, err := io.Copy(outFile, tr)
			if err != nil {
				slog.Error("Failed to write pixi binary", "error", err, "path", destPath)
				return fmt.Errorf("failed to write pixi binary: %w", err)
			}

			slog.Info("Pixi binary extracted successfully", "path", destPath, "bytes", bytesWritten)

			return nil
		}
	}

	slog.Error("Pixi binary not found in archive")
	return fmt.Errorf("pixi binary not found in archive")
}

// InstallPixi automatically downloads and installs pixi to ~/.local/bin
func InstallPixi(ctx context.Context) (string, error) {
	slog.Info("Starting pixi auto-installation",
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"version", pixiVersion)

	// Get platform
	platform, err := getPlatform()
	if err != nil {
		slog.Error("Failed to determine platform", "error", err)
		return "", err
	}

	slog.Info("Platform determined", "platform", platform)

	// Get install directory
	installDir, err := getInstallDir()
	if err != nil {
		slog.Error("Failed to get install directory", "error", err)
		return "", err
	}

	slog.Info("Install directory determined", "dir", installDir)

	// Create install directory if it doesn't exist
	if err := os.MkdirAll(installDir, 0755); err != nil {
		slog.Error("Failed to create install directory", "error", err, "dir", installDir)
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	// Download and install pixi
	if err := downloadPixi(ctx, platform, installDir); err != nil {
		slog.Error("Pixi installation failed", "error", err)
		return "", err
	}

	pixiPath := filepath.Join(installDir, "pixi")

	slog.Info("Verifying pixi installation", "path", pixiPath)

	// Verify installation
	cmd := exec.CommandContext(ctx, pixiPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Pixi verification failed", "error", err, "output", string(output))
		return "", fmt.Errorf("pixi installation verification failed: %w", err)
	}

	slog.Info("Pixi installed successfully", "path", pixiPath, "version_output", string(output))

	return pixiPath, nil
}
