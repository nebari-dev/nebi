#!/usr/bin/env bash
# Gate: the Go toolchain pinned in go.mod must be on a currently supported
# release line. Upstream supports a major line only until two newer majors
# exist, so an older pin silently stops receiving security fixes
# (nebari-dev/nebi#451, CWE-1104).
#
# Default mode fails on an unsupported line and warns when a newer patch of
# the pinned line exists. --strict (used by the release gate) also fails on
# an outdated patch, so tagged releases cannot ship a toolchain with
# known-fixed vulnerabilities.
set -euo pipefail

STRICT=false
[ "${1:-}" = "--strict" ] && STRICT=true

cd "$(dirname "$0")/.."

TOOLCHAIN=$(awk '$1 == "toolchain" {print $2}' go.mod)
if [ -z "$TOOLCHAIN" ]; then
  echo "::error::go.mod has no toolchain directive; pin an exact version (e.g. 'toolchain go1.26.5')"
  exit 1
fi

# go.dev/dl?mode=json lists only currently supported stable releases,
# newest patch of each line first.
SUPPORTED=$(curl -fsSL --retry 3 'https://go.dev/dl/?mode=json' |
  python3 -c 'import json, sys
for r in json.load(sys.stdin):
    if r["stable"]:
        print(r["version"])')

release_line() { echo "$1" | sed -E 's/^go([0-9]+\.[0-9]+)(\..*)?$/\1/'; }

OUR_LINE=$(release_line "$TOOLCHAIN")
LATEST_PATCH=""
for v in $SUPPORTED; do
  if [ "$(release_line "$v")" = "$OUR_LINE" ]; then
    LATEST_PATCH="$v"
    break
  fi
done

if [ -z "$LATEST_PATCH" ]; then
  echo "::error::Go $OUR_LINE (toolchain in go.mod) is not a supported release line. Supported: $(printf '%s ' $SUPPORTED)"
  exit 1
fi

if [ "$TOOLCHAIN" != "$LATEST_PATCH" ]; then
  MSG="go.mod pins $TOOLCHAIN but the newest $OUR_LINE patch is $LATEST_PATCH; update the toolchain directive (and the golang image digest in Dockerfile)"
  if $STRICT; then
    echo "::error::$MSG"
    exit 1
  fi
  echo "::warning::$MSG"
else
  echo "Go toolchain $TOOLCHAIN is supported and on the newest patch of its line"
fi
