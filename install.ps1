# Nebi installer script for Windows
# Usage: irm https://nebi.nebari.dev/install.ps1 | iex
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
$CosignIssuer = "https://token.actions.githubusercontent.com"
$ReleaseWorkflow = "release.yml"
$DesktopWorkflow = "desktop.yml"

if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "nebi"
}

function Write-Info {
    param([string]$Message)
    Write-Host "==> $Message" -ForegroundColor Blue
}

function Write-Err {
    param([string]$Message)
    [Console]::Error.WriteLine("Error: $Message")
    exit 1
}

function Invoke-Download {
    param(
        [string]$Uri,
        [string]$OutFile
    )
    Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing
}

function Confirm-CosignBlob {
    param(
        [string]$ArtifactPath,
        [string]$BundlePath,
        [string]$Workflow
    )

    if (-not (Get-Command cosign -ErrorAction SilentlyContinue)) {
        Write-Err "cosign is required to verify nebi release signatures. Install cosign and rerun this installer."
    }

    $identity = "https://github.com/$Repo/.github/workflows/$Workflow@refs/tags/$Version"
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & cosign verify-blob `
            --bundle $BundlePath `
            --certificate-identity $identity `
            --certificate-oidc-issuer $CosignIssuer `
            $ArtifactPath 2>&1
    } finally {
        $ErrorActionPreference = $prevEap
    }
    if ($LASTEXITCODE -ne 0) {
        $output | ForEach-Object { Write-Host $_ -ForegroundColor Red }
        Write-Err "Signature verification failed for $(Split-Path -Leaf $ArtifactPath)."
    }
}

function Get-Sha256 {
    param([string]$Path)
    return (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToLowerInvariant()
}

function Confirm-ChecksumFromFile {
    param(
        [string]$ArtifactPath,
        [string]$ChecksumsPath
    )

    $artifactName = Split-Path -Leaf $ArtifactPath
    $expected = $null
    foreach ($line in Get-Content $ChecksumsPath) {
        $parts = $line -split '\s+'
        if ($parts.Length -ge 2) {
            $candidate = $parts[1].TrimStart("*")
            if ($candidate -eq $artifactName) {
                $expected = $parts[0].ToLowerInvariant()
                break
            }
        }
    }

    if (-not $expected) {
        Write-Err "Checksum for $artifactName not found in $(Split-Path -Leaf $ChecksumsPath)."
    }

    $actual = Get-Sha256 $ArtifactPath
    if ($actual -ne $expected) {
        Write-Err "Checksum verification failed for $artifactName."
    }
}

function Confirm-AssetSignature {
    param(
        [string]$ArtifactPath,
        [string]$ArtifactUrl,
        [string]$Workflow
    )

    $bundlePath = "$ArtifactPath.sigstore.json"
    Write-Info "Downloading signature for $(Split-Path -Leaf $ArtifactPath)..."
    Invoke-Download -Uri "$ArtifactUrl.sigstore.json" -OutFile $bundlePath
    Confirm-CosignBlob -ArtifactPath $ArtifactPath -BundlePath $bundlePath -Workflow $Workflow
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
        Invoke-Download -Uri $DownloadUrl -OutFile $ArchivePath
    } catch {
        Write-Info "No Windows binary available for nebi $Version. Skipping installation."
        $env:NEBI_INSTALL_SKIPPED = "true"
        return
    }

    $ChecksumsPath = Join-Path $TempDir "checksums.txt"
    $ChecksumsSigPath = "$ChecksumsPath.sigstore.json"
    Write-Info "Downloading release checksums..."
    Invoke-Download -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" -OutFile $ChecksumsPath
    Invoke-Download -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt.sigstore.json" -OutFile $ChecksumsSigPath

    Write-Info "Verifying $ArchiveName..."
    Confirm-CosignBlob -ArtifactPath $ChecksumsPath -BundlePath $ChecksumsSigPath -Workflow $ReleaseWorkflow
    Confirm-ChecksumFromFile -ArtifactPath $ArchivePath -ChecksumsPath $ChecksumsPath
    Confirm-AssetSignature -ArtifactPath $ArchivePath -ArtifactUrl $DownloadUrl -Workflow $ReleaseWorkflow

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
        $DesktopDownloadPath = Join-Path $TempDir $DesktopExe
        Write-Info "Downloading $DesktopExe..."
        Invoke-Download -Uri $DesktopUrl -OutFile $DesktopDownloadPath
        Write-Info "Verifying $DesktopExe..."
        Confirm-AssetSignature -ArtifactPath $DesktopDownloadPath -ArtifactUrl $DesktopUrl -Workflow $DesktopWorkflow
        Copy-Item -Path $DesktopDownloadPath -Destination $DesktopPath -Force

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
