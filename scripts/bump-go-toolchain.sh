#!/usr/bin/env bash
# Rewrite both places that pin the Go toolchain to the newest patch of the
# line already in go.mod: the toolchain directive, and the golang builder
# image (tag + digest) in the Dockerfile.
#
# A stale patch is not cosmetic. Every Go patch release closes standard
# library vulnerabilities, so the govulncheck gates and the Trivy image scan
# start failing on main and on every open PR the day one ships
# (nebari-dev/nebi#510). This script is what .github/workflows/go-toolchain-bump.yml
# runs on a schedule so that shows up as a two-line PR instead.
#
# Patch bumps only. Moving to a new major line changes the language version
# and the go directive, so an unsupported line exits 2 for a human to handle.
#
# Exit codes: 0 = rewrote or already current, 1 = malformed go.mod or an
# upstream lookup failed, 2 = release line is no longer supported.
set -euo pipefail

cd "$(dirname "$0")/.."

TOOLCHAIN=$(awk '$1 == "toolchain" {print $2}' go.mod)
if [ -z "$TOOLCHAIN" ]; then
  echo "::error::go.mod has no toolchain directive; pin an exact version (e.g. 'toolchain go1.26.6')"
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

emit() { [ -n "${GITHUB_OUTPUT:-}" ] && echo "$1" >>"$GITHUB_OUTPUT"; return 0; }

if [ -z "$LATEST_PATCH" ]; then
  # Deliberately not automated: a major-line move needs a human to decide on
  # the language version and the go directive alongside the toolchain.
  echo "::error::Go $OUR_LINE (toolchain in go.mod) is not a supported release line, so this needs a manual major-line upgrade. Supported: $(printf '%s ' $SUPPORTED)"
  emit "changed=false"
  exit 2
fi

if [ "$TOOLCHAIN" = "$LATEST_PATCH" ]; then
  echo "Go toolchain $TOOLCHAIN is already the newest patch of its line"
  emit "changed=false"
  exit 0
fi

NEW_VERSION=${LATEST_PATCH#go}
echo "Bumping $TOOLCHAIN -> $LATEST_PATCH"

# Resolve the multi-arch index digest for the new builder image and confirm it
# really is the Go version we asked for before pinning it. Trusting the tag
# alone would let a mismatched or mid-push image get pinned by digest, which
# is the one failure mode a digest pin is supposed to prevent.
DIGEST=$(python3 - "$NEW_VERSION" <<'PY'
import json, sys, urllib.request

version = sys.argv[1]
repo, tag = "library/golang", f"{version}-alpine"
registry = "https://registry-1.docker.io/v2/" + repo
ACCEPT = ", ".join([
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
    "application/vnd.oci.image.manifest.v1+json",
    "application/vnd.docker.distribution.manifest.v2+json",
])


def fetch(url, token=None, accept=None):
    headers = {}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if accept:
        headers["Accept"] = accept
    return urllib.request.urlopen(urllib.request.Request(url, headers=headers))


auth = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull" % repo
token = json.load(fetch(auth))["token"]

resp = fetch(f"{registry}/manifests/{tag}", token, ACCEPT)
index = json.load(resp)
digest = resp.headers["Docker-Content-Digest"]

# Verify via one concrete platform: the index itself carries no version label.
amd64 = next(
    m["digest"] for m in index["manifests"]
    if m.get("platform", {}).get("architecture") == "amd64"
    and m.get("platform", {}).get("os") == "linux"
)
config = json.load(fetch(f"{registry}/manifests/{amd64}", token, ACCEPT))["config"]["digest"]
env = json.load(fetch(f"{registry}/blobs/{config}", token))["config"].get("Env", [])

got = next((e.split("=", 1)[1] for e in env if e.startswith("GOLANG_VERSION=")), None)
if got != version:
    sys.exit(f"::error::golang:{tag} reports GOLANG_VERSION={got}, expected {version}; refusing to pin {digest}")

print(digest)
PY
)

python3 - "$TOOLCHAIN" "$LATEST_PATCH" "$NEW_VERSION" "$DIGEST" <<'PY'
import pathlib, re, sys

old_toolchain, new_toolchain, version, digest = sys.argv[1:5]

gomod = pathlib.Path("go.mod")
text = gomod.read_text()
patched = re.sub(rf"^toolchain {re.escape(old_toolchain)}$", f"toolchain {new_toolchain}",
                 text, count=1, flags=re.M)
if patched == text:
    sys.exit(f"::error::could not rewrite 'toolchain {old_toolchain}' in go.mod")
gomod.write_text(patched)

dockerfile = pathlib.Path("Dockerfile")
text = dockerfile.read_text()
patched, n = re.subn(r"^FROM golang:\S+ AS backend-builder$",
                     f"FROM golang:{version}-alpine@{digest} AS backend-builder",
                     text, count=1, flags=re.M)
if n != 1:
    sys.exit("::error::could not find the 'FROM golang:... AS backend-builder' line in Dockerfile")
dockerfile.write_text(patched)
PY

emit "changed=true"
emit "version=$NEW_VERSION"
emit "toolchain=$LATEST_PATCH"
emit "previous=$TOOLCHAIN"
emit "digest=$DIGEST"

echo "go.mod -> toolchain $LATEST_PATCH"
echo "Dockerfile -> golang:${NEW_VERSION}-alpine@${DIGEST}"
