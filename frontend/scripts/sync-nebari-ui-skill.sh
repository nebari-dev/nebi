#!/usr/bin/env bash
# Sync the vendored nebari-ui agent skill from the upstream Nebari design
# registry. The vendored copy is upstream-managed: never edit it by hand.
#
#   scripts/sync-nebari-ui-skill.sh          # overwrite the vendored copy
#   scripts/sync-nebari-ui-skill.sh --check  # exit 1 if it has drifted
set -euo pipefail

REF="${NEBARI_DESIGN_REF:-main}"
URL="https://raw.githubusercontent.com/nebari-dev/nebari-design/${REF}/registry/nebari/skills/nebari-ui/SKILL.md"
DEST="$(cd "$(dirname "$0")/.." && pwd)/.agents/skills/nebari-ui/SKILL.md"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL "$URL" -o "$tmp"

if [[ "${1:-}" == "--check" ]]; then
  if ! diff -q "$tmp" "$DEST" >/dev/null; then
    echo "nebari-ui skill has drifted from nebari-design@${REF}." >&2
    echo "Run: npm run skills:sync" >&2
    exit 1
  fi
  echo "nebari-ui skill is in sync with nebari-design@${REF}."
  exit 0
fi

cp "$tmp" "$DEST"
echo "Synced nebari-ui skill from nebari-design@${REF}."
