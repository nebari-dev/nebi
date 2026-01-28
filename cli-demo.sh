#!/bin/bash
# =============================================================================
# Nebi CLI Demo Script
# =============================================================================
# This script demonstrates most nebi CLI commands.
#
# Prerequisites:
#   1. A running nebi server (e.g., `make dev` or deployed instance)
#   2. pixi installed on your system
#   3. An OCI registry already configured as default (nebi registry add ... --default)
#
# Usage:
#   export NEBI_SERVER=http://localhost:8460
#   ./demo.sh
# =============================================================================

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration - customize these or set via environment
NEBI_SERVER="${NEBI_SERVER:-http://localhost:8460}"

# Demo workspace names
DEMO_REPO="demo-data-science"
DEMO_TAG_V1="v1.0.0"
DEMO_TAG_V2="v2.0.0"

# Helper functions
section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

run() {
    echo -e "${YELLOW}$ $@${NC}"
    "$@"
    echo ""
}

pause() {
    echo -e "${GREEN}Press Enter to continue...${NC}"
    read -r
}

# =============================================================================
# Setup
# =============================================================================
section "Setup: Creating demo workspace"

DEMO_DIR=$(mktemp -d)
echo "Working directory: $DEMO_DIR"
cd "$DEMO_DIR"

# Create a pixi project
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

echo "Created pixi.toml"
cat pixi.toml

echo ""
echo "Generating pixi.lock..."
pixi lock --quiet

pause

# =============================================================================
# 1. Login
# =============================================================================
section "1. Login to Nebi Server"

echo "Logging in to $NEBI_SERVER..."
run nebi login "$NEBI_SERVER"

pause

# =============================================================================
# 2. Cleanup from previous runs
# =============================================================================
section "2. Cleanup from previous runs"

echo "Cleaning up any existing demo environment from previous runs..."

# Delete demo environment from server if it exists (ignore errors)
nebi env delete "$DEMO_REPO" 2>/dev/null && echo "  Deleted existing $DEMO_REPO from server" || echo "  No existing $DEMO_REPO on server (OK)"

# Prune any stale/orphaned local entries
echo "  Pruning stale local entries..."
nebi env prune 2>/dev/null || true

echo ""
echo "Cleanup complete."

pause

# =============================================================================
# 3. Push v1
# =============================================================================
section "3. Push v1 to server and OCI"

echo "Pushing workspace as $DEMO_REPO:$DEMO_TAG_V1..."
run nebi push "$DEMO_REPO:$DEMO_TAG_V1"

pause

# =============================================================================
# 4. List environments on server
# =============================================================================
section "4. List environments on server"

run nebi env list

pause

# =============================================================================
# 5. View environment info and versions
# =============================================================================
section "5. View environment info and versions"

run nebi env info "$DEMO_REPO"

echo ""
echo "Listing versions for $DEMO_REPO:"
run nebi env versions "$DEMO_REPO"

pause

# =============================================================================
# 6. Pull to a new directory
# =============================================================================
section "6. Pull workspace to new directory"

PULL_DIR=$(mktemp -d)
echo "Pulling to: $PULL_DIR"
cd "$PULL_DIR"

run nebi pull "$DEMO_REPO:$DEMO_TAG_V1"

echo ""
echo "Contents of pulled pixi.toml:"
cat pixi.toml

pause

# =============================================================================
# 7. Check status (should be clean)
# =============================================================================
section "7. Check status (clean)"

run nebi status

pause

# =============================================================================
# 8. Check remote status and detect tag movement
# =============================================================================
section "8. Check remote status (detect tag movement)"

echo "Checking if remote tag has changed since pull..."
run nebi status --remote

echo ""
echo "Now let's simulate another user pushing to the same tag..."
echo "We'll push updated content to v1.0.0 from the original directory."

# Save current dir
SAVED_DIR=$(pwd)

# Go to original dir and push to v1.0.0 (moves the tag)
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
requests = ">=2.31"

[feature.jupyter.dependencies]
jupyterlab = ">=4.0"

[environments]
default = ["jupyter"]
EOF

echo "Regenerating pixi.lock..."
pixi lock --quiet

echo ""
echo "Pushing updated content to v1.0.0 (simulating another user)..."
run nebi push "$DEMO_REPO:$DEMO_TAG_V1" --force

# Go back to pulled directory
cd "$SAVED_DIR"

echo ""
echo "Now checking remote status from our pulled copy..."
echo "The tag v1.0.0 should now point to a different version:"
run nebi status --remote

pause

# =============================================================================
# 9. Modify and check status (modified)
# =============================================================================
section "9. Modify workspace and check status"

echo "Adding scipy to dependencies..."
pixi add --quiet "scipy>=1.11"

echo ""
echo "Checking status (should show modified):"
run nebi status || true  # status exits 1 when modified

pause

# =============================================================================
# 10. Push v2
# =============================================================================
section "10. Push v2"

cd "$DEMO_DIR"  # Go back to original

# Update original to match
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

echo "Regenerating pixi.lock..."
pixi lock --quiet

run nebi push "$DEMO_REPO:$DEMO_TAG_V2"

pause

# =============================================================================
# 11. Diff between server versions
# =============================================================================
section "11. Diff between server versions"

echo "Comparing v1 to v2 on server:"
run nebi diff "$DEMO_REPO:$DEMO_TAG_V1" "$DEMO_REPO:$DEMO_TAG_V2" || true  # exits 1 when diff found

pause

# =============================================================================
# 13. Pull globally
# =============================================================================
section "13. Pull globally (to ~/.local/share/nebi/envs/)"

run nebi pull "$DEMO_REPO:$DEMO_TAG_V1" --global

pause

# =============================================================================
# 14. List local environments
# =============================================================================
section "14. List locally pulled environments"

run nebi env list --local

pause

# =============================================================================
# 15. Shell activation (if pixi is available)
# =============================================================================
section "15. Shell activation"

echo "To activate the environment shell, you would run:"
echo -e "${YELLOW}nebi shell $DEMO_REPO${NC}"
echo ""
echo "Or with a specific environment:"
echo -e "${YELLOW}nebi shell $DEMO_REPO --env jupyter${NC}"
echo ""
echo "Or using the global copy:"
echo -e "${YELLOW}nebi shell $DEMO_REPO:$DEMO_TAG_V1 --global${NC}"
echo ""
echo "(Skipping actual shell activation in demo)"

pause

# =============================================================================
# 16. Cleanup and Orphaned Detection
# =============================================================================
section "16. Cleanup and Orphaned Detection"

echo "When you delete a remote environment, local copies become 'orphaned'."
echo ""
read -p "Delete demo repo to demonstrate orphaned detection? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "Deleting remote environment..."
    run nebi env delete "$DEMO_REPO"

    pause

    echo "Now listing local environments (note the 'orphaned' status):"
    run nebi env list --local

    pause

    echo "Pruning removes orphaned entries:"
    echo "  - Local entries: deletes .nebi.toml only (preserves pixi.toml, pixi.lock)"
    echo "  - Global entries: deletes entire directory (managed by nebi)"
    run nebi env prune
fi

# =============================================================================
# 17. Logout
# =============================================================================
section "17. Logout"

run nebi logout

# =============================================================================
# Summary
# =============================================================================
section "Demo Complete!"

echo "Commands demonstrated:"
echo "  - nebi login <server>           # Authenticate"
echo "  - nebi push <env>:<version>     # Push environment to server"
echo "  - nebi pull <env>[:<version>]   # Pull environment"
echo "  - nebi status [--remote]        # Check drift status (--remote detects moved tags)"
echo "  - nebi diff <ref1> <ref2>       # Compare versions"
echo "  - nebi shell <env>              # Activate environment"
echo "  - nebi env list [--local]       # List environments"
echo "  - nebi env info <env>           # Show environment details"
echo "  - nebi env versions <env>       # List versions"
echo "  - nebi env delete <env>         # Delete environment"
echo "  - nebi env prune                # Remove stale/orphaned local entries"
echo "  - nebi publish <env>:<version>  # Publish to OCI"
echo "  - nebi logout                   # Clear credentials"
echo ""
echo "Useful flags:"
echo "  --dry-run     Preview push without making changes"
echo "  --global      Pull to global location (~/.local/share/nebi/envs/)"
echo "  --force       Overwrite existing global pull"
echo "  --yes         Skip confirmation prompts"
echo "  --json        Output in JSON format (for scripting)"
echo "  --remote      Check if remote tag has moved (for nebi status)"
echo ""

# Cleanup temp directories
rm -rf "$DEMO_DIR" "$PULL_DIR"
echo "Cleaned up temporary directories."
