#!/bin/sh
# Nebi installer script for Linux and macOS
# Usage: curl -fsSL https://raw.githubusercontent.com/nebari-dev/nebi/main/install.sh | sh
#
# Flags:
#   --version <ver>       Install specific version (e.g. v0.5.0). Default: latest
#   --install-dir <path>  Install directory. Default: /usr/local/bin
#   --desktop             Also install the desktop app

set -e

REPO="nebari-dev/nebi"
INSTALL_DIR="/usr/local/bin"
VERSION=""
DESKTOP=0
TMPDIR=""

usage() {
    cat <<EOF
Usage: install.sh [OPTIONS]

Options:
    --version <ver>       Install specific version (e.g. v0.5.0)
    --install-dir <path>  Install directory (default: /usr/local/bin)
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
    DOWNLOAD="curl -fsSL"
    DOWNLOAD_OUT="curl -fsSL -o"
elif command -v wget >/dev/null 2>&1; then
    DOWNLOAD="wget -qO-"
    DOWNLOAD_OUT="wget -qO"
else
    error "Neither curl nor wget found. Please install one of them."
fi

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
    VERSION="$($DOWNLOAD "https://api.github.com/repos/${REPO}/releases/latest" | \
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
$DOWNLOAD_OUT "${TMPDIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL" || \
    error "Failed to download ${DOWNLOAD_URL}"

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
            $DOWNLOAD_OUT "${TMPDIR}/${DESKTOP_ARCHIVE}" "$DESKTOP_URL" || \
                error "Failed to download desktop app: ${DESKTOP_URL}"
            tar -xzf "${TMPDIR}/${DESKTOP_ARCHIVE}" -C "$TMPDIR"
            if [ -w "$INSTALL_DIR" ]; then
                cp "${TMPDIR}/nebi-desktop" "${INSTALL_DIR}/nebi-desktop"
                chmod +x "${INSTALL_DIR}/nebi-desktop"
            else
                sudo cp "${TMPDIR}/nebi-desktop" "${INSTALL_DIR}/nebi-desktop"
                sudo chmod +x "${INSTALL_DIR}/nebi-desktop"
            fi
            info "Desktop app installed to ${INSTALL_DIR}/nebi-desktop"
            ;;
        macos)
            DESKTOP_ARCHIVE="nebi-desktop-macos-universal.zip"
            DESKTOP_URL="https://github.com/${REPO}/releases/download/${VERSION}/${DESKTOP_ARCHIVE}"
            info "Downloading ${DESKTOP_ARCHIVE}..."
            $DOWNLOAD_OUT "${TMPDIR}/${DESKTOP_ARCHIVE}" "$DESKTOP_URL" || \
                error "Failed to download desktop app: ${DESKTOP_URL}"
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
