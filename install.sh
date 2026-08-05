#!/bin/sh
# Nebi installer script for Linux and macOS
# Usage: curl -fsSL https://nebi.nebari.dev/install.sh | sh
#
# Flags:
#   --version <ver>       Install specific version (e.g. v0.5.0). Default: latest
#   --install-dir <path>  Install directory. Default: ~/.local/bin
#   --desktop             Also install the desktop app

set -e

REPO="nebari-dev/nebi"
INSTALL_DIR="$HOME/.local/bin"
VERSION=""
DESKTOP=0
TMPDIR=""
COSIGN_ISSUER="https://token.actions.githubusercontent.com"
RELEASE_WORKFLOW="release.yml"
DESKTOP_WORKFLOW="desktop.yml"

usage() {
    cat <<EOF
Usage: install.sh [OPTIONS]

Options:
    --version <ver>       Install specific version (e.g. v0.5.0)
    --install-dir <path>  Install directory (default: ~/.local/bin)
    --desktop             Also install the desktop app
    -h, --help            Show this help message
EOF
    exit 0
}

cleanup() {
    if [ -n "$TMPDIR" ] && [ -d "$TMPDIR" ]; then
        rm -rf "$TMPDIR"
    fi
}
trap cleanup EXIT INT TERM

info() {
    printf "\033[1;34m==>\033[0m %s\n" "$1"
}

error() {
    printf "\033[1;31mError:\033[0m %s\n" "$1" >&2
    exit 1
}

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        --version)
            VERSION="$2"
            shift 2
            ;;
        --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        --desktop)
            DESKTOP=1
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Detect download command
if command -v curl >/dev/null 2>&1; then
    DOWNLOAD_OUT="curl -fsSL -o"
elif command -v wget >/dev/null 2>&1; then
    DOWNLOAD_OUT="wget -qO"
else
    error "Neither curl nor wget found. Please install one of them."
fi

# Helper for GitHub API requests (uses GITHUB_TOKEN if available to avoid rate limits)
github_api_get() {
    if command -v curl >/dev/null 2>&1; then
        if [ -n "$GITHUB_TOKEN" ]; then
            curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "$1"
        else
            curl -fsSL "$1"
        fi
    else
        if [ -n "$GITHUB_TOKEN" ]; then
            wget --header="Authorization: token ${GITHUB_TOKEN}" -qO- "$1"
        else
            wget -qO- "$1"
        fi
    fi
}

download_file() {
    $DOWNLOAD_OUT "$1" "$2"
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        error "Neither sha256sum nor shasum found. Please install one of them."
    fi
}

verify_cosign_blob() {
    artifact_path="$1"
    bundle_path="$2"
    workflow="$3"

    if ! command -v cosign >/dev/null 2>&1; then
        error "cosign is required to verify nebi release signatures. Install cosign and rerun this installer."
    fi

    identity="https://github.com/${REPO}/.github/workflows/${workflow}@refs/tags/${VERSION}"
    if ! cosign verify-blob \
        --bundle "$bundle_path" \
        --certificate-identity "$identity" \
        --certificate-oidc-issuer "$COSIGN_ISSUER" \
        "$artifact_path" >/dev/null 2>&1; then
        error "Signature verification failed for $(basename "$artifact_path")."
    fi
}

verify_checksum_from_file() {
    artifact_path="$1"
    checksums_path="$2"
    artifact_name="$(basename "$artifact_path")"
    expected="$(awk -v name="$artifact_name" '
        {
            candidate = $2
            sub(/^\*/, "", candidate)
            if (candidate == name) {
                print $1
                exit
            }
        }
    ' "$checksums_path")"

    if [ -z "$expected" ]; then
        error "Checksum for ${artifact_name} not found in $(basename "$checksums_path")."
    fi

    actual="$(sha256_file "$artifact_path")"
    if [ "$actual" != "$expected" ]; then
        error "Checksum verification failed for ${artifact_name}."
    fi
}

verify_checksum_sidecar() {
    artifact_path="$1"
    checksum_path="$2"
    expected="$(awk 'NF >= 1 {print $1; exit}' "$checksum_path")"
    actual="$(sha256_file "$artifact_path")"

    if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
        error "Checksum verification failed for $(basename "$artifact_path")."
    fi
}

verify_asset_signature() {
    artifact_path="$1"
    artifact_url="$2"
    workflow="$3"
    bundle_path="${artifact_path}.sigstore.json"

    info "Downloading signature for $(basename "$artifact_path")..."
    download_file "$bundle_path" "${artifact_url}.sigstore.json" || \
        error "Failed to download signature: ${artifact_url}.sigstore.json"

    verify_cosign_blob "$artifact_path" "$bundle_path" "$workflow"
}

verify_signed_asset() {
    artifact_path="$1"
    artifact_url="$2"
    workflow="$3"
    checksum_path="${artifact_path}.sha256"

    info "Downloading checksum for $(basename "$artifact_path")..."
    download_file "$checksum_path" "${artifact_url}.sha256" || \
        error "Failed to download checksum: ${artifact_url}.sha256"

    verify_checksum_sidecar "$artifact_path" "$checksum_path"
    verify_asset_signature "$artifact_path" "$artifact_url" "$workflow"
}

# Detect OS
OS="$(uname -s)"
case "$OS" in
    Linux)  OS_NAME="linux" ; ARCHIVE_OS="linux" ;;
    Darwin) OS_NAME="macos" ; ARCHIVE_OS="macOS" ;;
    *)      error "Unsupported operating system: $OS" ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)   ARCH_NAME="x86_64" ;;
    aarch64|arm64)   ARCH_NAME="arm64" ;;
    *)               error "Unsupported architecture: $ARCH" ;;
esac

# Determine version
if [ -z "$VERSION" ]; then
    info "Fetching latest release version..."
    VERSION="$(github_api_get "https://api.github.com/repos/${REPO}/releases/latest" | \
        sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')"
    if [ -z "$VERSION" ]; then
        error "Could not determine latest version. Please specify with --version."
    fi
fi

# Strip v prefix for archive name (GoReleaser convention)
VERSION_NUM="${VERSION#v}"

info "Installing nebi ${VERSION} for ${OS_NAME}/${ARCH_NAME}..."

# Create temp directory
TMPDIR="$(mktemp -d)"

# Download and install CLI
ARCHIVE_NAME="nebi_${VERSION_NUM}_${ARCHIVE_OS}_${ARCH_NAME}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"

info "Downloading ${ARCHIVE_NAME}..."
if ! download_file "${TMPDIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL" 2>/dev/null; then
    info "No binary available for nebi ${VERSION} on ${OS_NAME}/${ARCH_NAME}. Skipping installation."
    exit 2
fi

CHECKSUMS_PATH="${TMPDIR}/checksums.txt"
CHECKSUMS_SIG_PATH="${CHECKSUMS_PATH}.sigstore.json"
info "Downloading release checksums..."
download_file "$CHECKSUMS_PATH" "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" || \
    error "Failed to download release checksums."
download_file "$CHECKSUMS_SIG_PATH" "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt.sigstore.json" || \
    error "Failed to download release checksum signature."

info "Verifying ${ARCHIVE_NAME}..."
verify_cosign_blob "$CHECKSUMS_PATH" "$CHECKSUMS_SIG_PATH" "$RELEASE_WORKFLOW"
verify_checksum_from_file "${TMPDIR}/${ARCHIVE_NAME}" "$CHECKSUMS_PATH"
verify_asset_signature "${TMPDIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL" "$RELEASE_WORKFLOW"

info "Extracting archive..."
tar -xzf "${TMPDIR}/${ARCHIVE_NAME}" -C "$TMPDIR"

# Install binary
mkdir -p "$INSTALL_DIR"
if [ -w "$INSTALL_DIR" ]; then
    cp "${TMPDIR}/nebi" "${INSTALL_DIR}/nebi"
    chmod +x "${INSTALL_DIR}/nebi"
else
    info "Install directory ${INSTALL_DIR} requires elevated permissions, using sudo..."
    sudo cp "${TMPDIR}/nebi" "${INSTALL_DIR}/nebi"
    sudo chmod +x "${INSTALL_DIR}/nebi"
fi

info "nebi installed to ${INSTALL_DIR}/nebi"

# Verify installation
if [ -x "${INSTALL_DIR}/nebi" ]; then
    INSTALLED_VERSION="$("${INSTALL_DIR}/nebi" version 2>/dev/null || true)"
    info "Installed: ${INSTALLED_VERSION}"
fi

# Desktop app installation
if [ "$DESKTOP" -eq 1 ]; then
    info "Installing desktop app..."
    case "$OS_NAME" in
        linux)
            DESKTOP_ARCHIVE="nebi-desktop-linux-amd64.tar.gz"
            DESKTOP_URL="https://github.com/${REPO}/releases/download/${VERSION}/${DESKTOP_ARCHIVE}"
            info "Downloading ${DESKTOP_ARCHIVE}..."
            download_file "${TMPDIR}/${DESKTOP_ARCHIVE}" "$DESKTOP_URL" || \
                error "Failed to download desktop app: ${DESKTOP_URL}"
            info "Verifying ${DESKTOP_ARCHIVE}..."
            verify_signed_asset "${TMPDIR}/${DESKTOP_ARCHIVE}" "$DESKTOP_URL" "$DESKTOP_WORKFLOW"
            tar -xzf "${TMPDIR}/${DESKTOP_ARCHIVE}" -C "$TMPDIR"
            DESKTOP_BIN="${TMPDIR}/Nebi"
            if [ ! -f "$DESKTOP_BIN" ]; then
                DESKTOP_BIN="${TMPDIR}/nebi-desktop"
            fi
            if [ ! -f "$DESKTOP_BIN" ]; then
                error "Desktop executable not found in the downloaded archive."
            fi
            if [ -w "$INSTALL_DIR" ]; then
                cp "$DESKTOP_BIN" "${INSTALL_DIR}/nebi-desktop"
                chmod +x "${INSTALL_DIR}/nebi-desktop"
            else
                sudo cp "$DESKTOP_BIN" "${INSTALL_DIR}/nebi-desktop"
                sudo chmod +x "${INSTALL_DIR}/nebi-desktop"
            fi
            info "Desktop app installed to ${INSTALL_DIR}/nebi-desktop"
            ;;
        macos)
            DESKTOP_ARCHIVE="nebi-desktop-macos-universal.zip"
            DESKTOP_URL="https://github.com/${REPO}/releases/download/${VERSION}/${DESKTOP_ARCHIVE}"
            info "Downloading ${DESKTOP_ARCHIVE}..."
            download_file "${TMPDIR}/${DESKTOP_ARCHIVE}" "$DESKTOP_URL" || \
                error "Failed to download desktop app: ${DESKTOP_URL}"
            info "Verifying ${DESKTOP_ARCHIVE}..."
            verify_signed_asset "${TMPDIR}/${DESKTOP_ARCHIVE}" "$DESKTOP_URL" "$DESKTOP_WORKFLOW"
            unzip -q "${TMPDIR}/${DESKTOP_ARCHIVE}" -d "$TMPDIR"
            if [ -d "${TMPDIR}/Nebi.app" ]; then
                if [ -w "/Applications" ]; then
                    cp -R "${TMPDIR}/Nebi.app" "/Applications/Nebi.app"
                else
                    sudo cp -R "${TMPDIR}/Nebi.app" "/Applications/Nebi.app"
                fi
                info "Desktop app installed to /Applications/Nebi.app"
            else
                error "Nebi.app not found in the downloaded archive."
            fi
            ;;
    esac
fi

info "Installation complete!"

# Hint about PATH if install dir is not in PATH
case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *) printf "\033[1;33mNote:\033[0m %s is not in your PATH. Add it with:\n  export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR" "$INSTALL_DIR" ;;
esac
