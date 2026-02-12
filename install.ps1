# Nebi installer script for Windows
# Usage: irm https://raw.githubusercontent.com/nebari-dev/nebi/main/install.ps1 | iex
#
# Parameters:
#   -Version <ver>        Install specific version (e.g. v0.5.0). Default: latest
#   -InstallDir <path>    Install directory. Default: $env:LOCALAPPDATA\nebi
#   -Desktop              Also install the desktop app

param(
    [string]$Version = "",
    [string]$InstallDir = "",
    [switch]$Desktop
)

$ErrorActionPreference = "Stop"

$Repo = "nebari-dev/nebi"

if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "nebi"
}

function Write-Info {
    param([string]$Message)
    Write-Host "==> $Message" -ForegroundColor Blue
}

function Write-Err {
    param([string]$Message)
    Write-Host "Error: $Message" -ForegroundColor Red
    exit 1
}

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) "nebi-install-$([System.Guid]::NewGuid().ToString('N').Substring(0,8))"

try {
    # Detect architecture
    $Arch = $env:PROCESSOR_ARCHITECTURE
    switch ($Arch) {
        "AMD64" { $ArchName = "x86_64" }
        default { Write-Err "Unsupported architecture: $Arch" }
    }

    # Determine version
    if (-not $Version) {
        Write-Info "Fetching latest release version..."
        $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $Release.tag_name
        if (-not $Version) {
            Write-Err "Could not determine latest version. Please specify with -Version."
        }
    }

    # Strip v prefix for archive name (GoReleaser convention)
    $VersionNum = $Version -replace '^v', ''

    Write-Info "Installing nebi $Version for windows/$ArchName..."

    # Create temp directory
    New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

    # Download CLI
    $ArchiveName = "nebi_${VersionNum}_windows_${ArchName}.zip"
    $DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$ArchiveName"

    Write-Info "Downloading $ArchiveName..."
    $ArchivePath = Join-Path $TempDir $ArchiveName
    try {
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $ArchivePath -UseBasicParsing
    } catch {
        Write-Info "No Windows binary available for nebi $Version. Skipping installation."
        $env:NEBI_INSTALL_SKIPPED = "true"
        return
    }

    # Extract archive
    Write-Info "Extracting archive..."
    $ExtractDir = Join-Path $TempDir "extracted"
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force

    # Install binary
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    Copy-Item -Path (Join-Path $ExtractDir "nebi.exe") -Destination (Join-Path $InstallDir "nebi.exe") -Force

    Write-Info "nebi installed to $(Join-Path $InstallDir 'nebi.exe')"

    # Add to PATH if not already present
    $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        Write-Info "Adding $InstallDir to user PATH..."
        [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$UserPath", "User")
        $env:PATH = "$InstallDir;$env:PATH"
    }

    # Verify installation
    $NebiBin = Join-Path $InstallDir "nebi.exe"
    if (Test-Path $NebiBin) {
        $InstalledVersion = & $NebiBin version 2>$null
        Write-Info "Installed: $InstalledVersion"
    }

    # Desktop app installation
    if ($Desktop) {
        Write-Info "Installing desktop app..."
        $DesktopExe = "nebi-desktop-windows-amd64.exe"
        $DesktopUrl = "https://github.com/$Repo/releases/download/$Version/$DesktopExe"
        $DesktopDir = Join-Path $env:LOCALAPPDATA "Programs\Nebi"

        if (-not (Test-Path $DesktopDir)) {
            New-Item -ItemType Directory -Path $DesktopDir -Force | Out-Null
        }

        $DesktopPath = Join-Path $DesktopDir "Nebi.exe"
        Write-Info "Downloading $DesktopExe..."
        Invoke-WebRequest -Uri $DesktopUrl -OutFile $DesktopPath -UseBasicParsing

        Write-Info "Desktop app installed to $DesktopPath"
    }

    Write-Info "Installation complete!"
    Write-Host ""
    Write-Host "To get started, run: nebi --help" -ForegroundColor Green
    Write-Host "You may need to restart your terminal for PATH changes to take effect." -ForegroundColor Yellow

} finally {
    # Cleanup temp directory
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
