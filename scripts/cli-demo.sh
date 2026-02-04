#!/bin/bash
# =============================================================================
# Nebi CLI Demo Script
# =============================================================================
# Demonstrates the nebi CLI for local-first workspace management and
# server-based sync of Pixi environments.
#
# Prerequisites:
#   1. A running nebi server (e.g., `make dev` or deployed instance)
#   2. pixi installed on your system
#   3. nebi binary built and on PATH
#
# Usage:
#   export NEBI_SERVER=http://localhost:8460
#   ./cli-demo.sh
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

NEBI_SERVER="${NEBI_SERVER:-http://localhost:8460}"
DEMO_WORKSPACE="demo-data-science"
TAG_V1="v1.0"
TAG_V2="v2.0"

section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

run() {
    echo -e "${YELLOW}\$ $@${NC}"
    "$@"
    echo ""
}

pause() {
    echo -e "${GREEN}Press Enter to continue...${NC}"
    read -r
}

# =============================================================================
# Pre-flight cleanup
# =============================================================================
cleanup_existing() {
    local found=0
    if nebi workspace list 2>/dev/null | grep -q "my-datascience\|promoted-ws"; then
        found=1
    fi
    if nebi server list 2>/dev/null | grep -q "demo"; then
        found=1
    fi

    if [ "$found" -eq 1 ]; then
        echo -e "${YELLOW}Found artifacts from a previous demo run.${NC}"
        echo -n "Remove them before continuing? [y/N] "
        read -r answer
        if [[ "$answer" =~ ^[Yy]$ ]]; then
            nebi workspace remove my-datascience 2>/dev/null || true
            nebi workspace remove promoted-ws 2>/dev/null || true
            nebi server remove demo 2>/dev/null || true
            echo -e "${GREEN}Cleaned up previous demo artifacts.${NC}"
        else
            echo "Continuing without cleanup."
        fi
        echo ""
    fi
}

cleanup_existing

# =============================================================================
# Setup
# =============================================================================
section "Setup: Creating a demo pixi workspace"

DEMO_DIR=$(mktemp -d)
echo "Working directory: $DEMO_DIR"
cd "$DEMO_DIR"

cat > pixi.toml << 'EOF'
[workspace]
name = "demo-data-science"
channels = ["conda-forge"]
platforms = ["linux-64", "osx-arm64", "osx-64"]

[dependencies]
python = ">=3.11"
numpy = ">=1.26"
pandas = ">=2.0"

[feature.jupyter.dependencies]
jupyterlab = ">=4.0"

[environments]
default = ["jupyter"]
EOF

echo "Created pixi.toml:"
cat pixi.toml
echo ""

echo "Generating pixi.lock..."
pixi lock --quiet
echo "Done."

pause

# =============================================================================
# 1. Server Setup
# =============================================================================
section "1. Register and authenticate with a server"

echo "Adding server 'demo' pointing to $NEBI_SERVER..."
run nebi server add demo "$NEBI_SERVER" || echo "  (server 'demo' may already exist)"

echo "Registered servers:"
run nebi server list

echo "Logging in..."
run nebi login demo

pause

# =============================================================================
# 2. Init — track the workspace locally
# =============================================================================
section "2. Initialize workspace tracking"

run nebi init

echo "Local workspaces:"
run nebi workspace list

pause

# =============================================================================
# 3. Push v1
# =============================================================================
section "3. Push workspace to server"

echo "Pushing as $DEMO_WORKSPACE:$TAG_V1..."
run nebi push "$DEMO_WORKSPACE:$TAG_V1" -s demo

pause

# =============================================================================
# 4. List workspaces and tags on the server
# =============================================================================
section "4. Browse server workspaces"

echo "Workspaces on server:"
run nebi workspace list -s demo

echo "Tags for $DEMO_WORKSPACE:"
run nebi workspace tags "$DEMO_WORKSPACE" -s demo

pause

# =============================================================================
# 5. Modify and push v2
# =============================================================================
section "5. Update workspace and push v2"

echo "Adding scipy and matplotlib..."
cat > pixi.toml << 'EOF'
[workspace]
name = "demo-data-science"
channels = ["conda-forge"]
platforms = ["linux-64", "osx-arm64", "osx-64"]

[dependencies]
python = ">=3.11"
numpy = ">=1.26"
pandas = ">=2.0"
scipy = ">=1.11"
matplotlib = ">=3.8"

[feature.jupyter.dependencies]
jupyterlab = ">=4.0"

[environments]
default = ["jupyter"]
EOF

echo "Updated pixi.toml:"
cat pixi.toml
echo ""

echo "Regenerating pixi.lock..."
pixi lock --quiet

echo "Pushing as $DEMO_WORKSPACE:$TAG_V2..."
run nebi push "$DEMO_WORKSPACE:$TAG_V2" -s demo

echo "Tags now:"
run nebi workspace tags "$DEMO_WORKSPACE" -s demo

pause

# =============================================================================
# 6. Diff between versions
# =============================================================================
section "6. Diff between server versions"

echo "Comparing $TAG_V1 to $TAG_V2 on server:"
run nebi diff "$DEMO_WORKSPACE:$TAG_V1" "$DEMO_WORKSPACE:$TAG_V2" -s demo || true

echo ""
echo "With --lock to also compare lockfiles:"
run nebi diff "$DEMO_WORKSPACE:$TAG_V1" "$DEMO_WORKSPACE:$TAG_V2" -s demo --lock || true

pause

# =============================================================================
# 7. Pull to a new directory
# =============================================================================
section "7. Pull workspace to a new directory"

PULL_DIR=$(mktemp -d)
echo "Pulling $DEMO_WORKSPACE:$TAG_V1 to $PULL_DIR..."
run nebi pull "$DEMO_WORKSPACE:$TAG_V1" -s demo -o "$PULL_DIR"

echo "Contents of pulled pixi.toml:"
cat "$PULL_DIR/pixi.toml"

pause

# =============================================================================
# 8. Diff local directory against server version
# =============================================================================
section "8. Diff local directory vs server version"

echo "Comparing pulled v1 directory against server v2:"
run nebi diff "$PULL_DIR" "$DEMO_WORKSPACE:$TAG_V2" -s demo || true

pause

# =============================================================================
# 9. Global workspaces
# =============================================================================
section "9. Global workspaces"

echo "Pull a workspace globally (stored in ~/.local/share/nebi/):"
run nebi pull "$DEMO_WORKSPACE:$TAG_V2" --global my-datascience -s demo

echo "Promote current tracked workspace to a global workspace:"
cd "$DEMO_DIR"
run nebi workspace promote promoted-ws

echo "List all workspaces (local + global):"
run nebi workspace list

echo ""
echo "Diff between two global workspaces:"
run nebi diff my-datascience promoted-ws || true

pause

# =============================================================================
# 10. Shell and Run
# =============================================================================
section "10. Shell and Run"

echo "nebi shell and nebi run wrap pixi shell/run with workspace lookup"
echo "and auto-initialization. All args pass through to pixi."
echo ""
echo "Shell examples:"
echo -e "  ${YELLOW}nebi shell${NC}                          # current directory (auto-initializes)"
echo -e "  ${YELLOW}nebi shell my-datascience${NC}            # global workspace by name"
echo -e "  ${YELLOW}nebi shell ./some-project${NC}            # local directory by path"
echo -e "  ${YELLOW}nebi shell my-datascience -e dev${NC}     # args pass through to pixi shell"
echo ""
echo "Run examples:"
echo -e "  ${YELLOW}nebi run my-task${NC}                     # run a pixi task (auto-initializes)"
echo -e "  ${YELLOW}nebi run my-datascience my-task${NC}      # run a task in a global workspace"
echo -e "  ${YELLOW}nebi run ./some-project my-task${NC}      # run a task in a local directory"
echo -e "  ${YELLOW}nebi run -e dev my-task${NC}              # args pass through to pixi run"
echo ""
echo "(Skipping actual shell/run activation in demo)"

pause

# =============================================================================
# 11. Publish to OCI registry
# =============================================================================
section "11. Publish to OCI registry"

echo "To publish a server-hosted workspace to an OCI registry:"
echo ""
echo -e "  ${YELLOW}nebi publish $DEMO_WORKSPACE:$TAG_V1 -s demo${NC}"
echo -e "  ${YELLOW}nebi publish $DEMO_WORKSPACE:$TAG_V1 -s demo myorg/myenv:latest${NC}"
echo -e "  ${YELLOW}nebi publish $DEMO_WORKSPACE:$TAG_V1 -s demo --registry ghcr myorg/myenv:latest${NC}"
echo ""

echo "Listing registries on server:"
run nebi registry list -s demo || echo "  (no registries configured)"

echo ""
echo "(Skipping actual publish — requires an OCI registry configured on the server)"

pause

# =============================================================================
# 12. Cleanup
# =============================================================================
section "12. Cleanup"

echo "Removing global workspaces..."
run nebi workspace remove my-datascience || true
run nebi workspace remove promoted-ws || true

echo "Removing server..."
run nebi server remove demo || true

echo "Cleaning up temporary directories..."
rm -rf "$DEMO_DIR" "$PULL_DIR"

echo -e "${GREEN}Done. All demo artifacts removed.${NC}"
