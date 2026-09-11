#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"

version_num="${GITHUB_REF_NAME#v}"
cosign_issuer="https://token.actions.githubusercontent.com"

cli_archives=(
  "nebi_${version_num}_linux_x86_64.tar.gz"
  "nebi_${version_num}_linux_arm64.tar.gz"
  "nebi_${version_num}_macOS_x86_64.tar.gz"
  "nebi_${version_num}_macOS_arm64.tar.gz"
  "nebi_${version_num}_windows_x86_64.zip"
)

desktop_assets=(
  nebi-desktop-linux-amd64.tar.gz
  nebi-desktop-macos-universal.zip
  nebi-desktop-windows-amd64.exe
)

marker_assets=(
  nebi-cli-release.json
  nebi-desktop-linux-amd64.tar.gz.release.json
  nebi-desktop-macos-universal.zip.release.json
  nebi-desktop-windows-amd64.exe.release.json
  nebi-container-image.json
)

release_assets=(
  checksums.txt
  checksums.txt.sigstore.json
  nebi-cli-provenance.sigstore.json
)

for archive in "${cli_archives[@]}"; do
  release_assets+=("$archive" "${archive}.sigstore.json")
done

for asset in "${desktop_assets[@]}"; do
  release_assets+=(
    "$asset"
    "${asset}.sigstore.json"
    "${asset}.sbom.spdx.json"
    "${asset}.sbom.spdx.json.sigstore.json"
  )
done

workflow_file_for_marker() {
  case "$1" in
    nebi-cli-release.json)
      printf '%s\n' "release.yml"
      ;;
    nebi-desktop-*.release.json)
      printf '%s\n' "desktop.yml"
      ;;
    nebi-container-image.json)
      printf '%s\n' "docker.yml"
      ;;
    *)
      echo "No workflow identity mapping for release marker $1."
      exit 1
      ;;
  esac
}

verify_blob_signature() {
  artifact_path="$1"
  bundle_path="$2"
  workflow_file="$3"
  identity="https://github.com/${GITHUB_REPOSITORY}/.github/workflows/${workflow_file}@refs/tags/${GITHUB_REF_NAME}"

  cosign verify-blob \
    --bundle "$bundle_path" \
    --certificate-identity "$identity" \
    --certificate-oidc-issuer "$cosign_issuer" \
    "$artifact_path" >/dev/null
}

download_release_asset() {
  asset="$1"
  gh release download "${GITHUB_REF_NAME}" \
    --pattern "$asset" \
    --dir "$tmpdir" \
    --clobber >/dev/null
}

is_draft="$(gh release view "${GITHUB_REF_NAME}" --json isDraft --jq '.isDraft')"
if [ "$is_draft" != "true" ]; then
  echo "Release ${GITHUB_REF_NAME} is already public."
  exit 0
fi

asset_names="$(gh release view "${GITHUB_REF_NAME}" --json assets --jq '.assets[].name')"

missing_markers=()
for marker in "${marker_assets[@]}"; do
  for expected in "$marker" "${marker}.sigstore.json"; do
    if ! printf '%s\n' "$asset_names" | grep -Fxq "$expected"; then
      missing_markers+=("$expected")
    fi
  done
done

if [ "${#missing_markers[@]}" -gt 0 ]; then
  missing_list="${missing_markers[*]}"
  echo "::notice::Release ${GITHUB_REF_NAME} remains draft; waiting for release markers: ${missing_list}"
  echo "Release ${GITHUB_REF_NAME} remains draft; waiting for release markers:"
  printf '  %s\n' "${missing_markers[@]}"
  exit 0
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if ! command -v cosign >/dev/null 2>&1; then
  echo "cosign is required to verify release marker signatures."
  exit 1
fi

for marker in "${marker_assets[@]}"; do
  download_release_asset "$marker"
  download_release_asset "${marker}.sigstore.json"
  verify_blob_signature "$tmpdir/$marker" "$tmpdir/${marker}.sigstore.json" "$(workflow_file_for_marker "$marker")"
done

runs_json="$tmpdir/workflow-runs.json"
gh api "repos/${GITHUB_REPOSITORY}/actions/runs?event=push&head_sha=${GITHUB_SHA}&per_page=100" > "$runs_json"

if command -v python3 >/dev/null 2>&1; then
  python_bin=python3
elif command -v python >/dev/null 2>&1; then
  python_bin=python
else
  echo "python3 or python is required to validate release marker JSON."
  exit 1
fi

marker_paths=()
for marker in "${marker_assets[@]}"; do
  marker_paths+=("$tmpdir/$marker")
done

"$python_bin" - "$GITHUB_SHA" "$GITHUB_REF_NAME" "$runs_json" ".github/scripts/select-release-workflow-run.py" "${marker_paths[@]}" <<'PY'
import importlib.util
import json
import sys

expected_commit, expected_ref_name, runs_path, selector_path, *marker_paths = sys.argv[1:]
with open(runs_path, encoding="utf-8") as f:
    runs = json.load(f).get("workflow_runs", [])

spec = importlib.util.spec_from_file_location("release_selector", selector_path)
selector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(selector)

for path in marker_paths:
    with open(path, encoding="utf-8") as f:
        data = json.load(f)

    missing = [field for field in ("source_commit", "workflow", "run_id") if not data.get(field)]
    if missing:
        raise SystemExit(f"{path} is missing required marker fields: {', '.join(missing)}")

    actual_commit = data["source_commit"]
    if actual_commit != expected_commit:
        raise SystemExit(
            f"{path} was produced for source_commit {actual_commit}, expected {expected_commit}"
        )

    workflow = data["workflow"]
    latest_run = selector.latest_run(runs, workflow, expected_commit, expected_ref_name)
    if latest_run is None:
        raise SystemExit(f"{path} references workflow {workflow}, but no matching run was found")

    marker_run_id = str(data["run_id"])
    latest_run_id = str(latest_run["id"])
    if marker_run_id != latest_run_id:
        raise SystemExit(
            f"{path} was produced by run_id {marker_run_id}, "
            f"but latest {workflow} run for {expected_commit} is {latest_run_id}"
        )
PY

missing_release_assets=()
for expected in "${release_assets[@]}"; do
  if ! printf '%s\n' "$asset_names" | grep -Fxq "$expected"; then
    missing_release_assets+=("$expected")
  fi
done

if [ "${#missing_release_assets[@]}" -gt 0 ]; then
  echo "::error::Release ${GITHUB_REF_NAME} has current markers but is missing required assets."
  printf '  %s\n' "${missing_release_assets[@]}"
  exit 1
fi

hash_assets=(checksums.txt checksums.txt.sigstore.json nebi-cli-provenance.sigstore.json)
for archive in "${cli_archives[@]}"; do
  hash_assets+=("$archive")
done
for asset in "${desktop_assets[@]}"; do
  hash_assets+=("$asset" "${asset}.sbom.spdx.json")
done

for asset in "${hash_assets[@]}"; do
  download_release_asset "$asset"
done

verify_blob_signature "$tmpdir/checksums.txt" "$tmpdir/checksums.txt.sigstore.json" "release.yml"

"$python_bin" - "$GITHUB_REF_NAME" "$tmpdir" "${marker_paths[@]}" <<'PY'
import hashlib
import json
import sys
from pathlib import Path

ref_name, tmpdir_arg, *marker_paths = sys.argv[1:]
tmpdir = Path(tmpdir_arg)
version_num = ref_name.removeprefix("v")


def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def require_digest(path, expected, label):
    if not expected:
        raise SystemExit(f"{label} is missing an expected sha256 value")
    actual = sha256_file(path)
    if actual != expected:
        raise SystemExit(f"{label} sha256 mismatch: {actual}, expected {expected}")


checksums_path = tmpdir / "checksums.txt"
checksums = {}
with open(checksums_path, encoding="utf-8") as f:
    for line in f:
        fields = line.split()
        if len(fields) < 2:
            continue
        checksums[fields[1].lstrip("*")] = fields[0]

cli_archives = [
    f"nebi_{version_num}_linux_x86_64.tar.gz",
    f"nebi_{version_num}_linux_arm64.tar.gz",
    f"nebi_{version_num}_macOS_x86_64.tar.gz",
    f"nebi_{version_num}_macOS_arm64.tar.gz",
    f"nebi_{version_num}_windows_x86_64.zip",
]
for archive in cli_archives:
    if archive not in checksums:
        raise SystemExit(f"checksums.txt is missing {archive}")
    require_digest(tmpdir / archive, checksums[archive], archive)

for marker_path in marker_paths:
    with open(marker_path, encoding="utf-8") as f:
        marker = json.load(f)

    component = marker.get("component")
    if component == "cli":
        require_digest(
            checksums_path,
            marker.get("checksums_sha256"),
            "checksums.txt marker payload",
        )
        provenance_bundle = marker.get("provenance_bundle")
        if provenance_bundle and not (tmpdir / provenance_bundle).exists():
            raise SystemExit(f"CLI marker references missing provenance bundle {provenance_bundle}")
    elif component == "desktop":
        asset = marker.get("asset")
        if not asset:
            raise SystemExit(f"{marker_path} is missing desktop asset")
        require_digest(tmpdir / asset, marker.get("artifact_sha256"), asset)
        sbom = f"{asset}.sbom.spdx.json"
        require_digest(tmpdir / sbom, marker.get("sbom_sha256"), sbom)
    elif component == "container":
        image = marker.get("image")
        digest = marker.get("digest")
        reference = marker.get("reference")
        if not image or not digest or not reference:
            raise SystemExit(f"{marker_path} is missing container image, digest, or reference")
        if not digest.startswith("sha256:"):
            raise SystemExit(f"{marker_path} has invalid container digest {digest}")
        if reference != f"{image}@{digest}":
            raise SystemExit(f"{marker_path} reference {reference} does not match image@digest")
    else:
        raise SystemExit(f"{marker_path} has unknown component {component}")
PY

release_id="$(gh release view "${GITHUB_REF_NAME}" --json databaseId --jq '.databaseId')"
gh api --method PATCH "repos/${GITHUB_REPOSITORY}/releases/${release_id}" -F draft=false >/dev/null
echo "Published release ${GITHUB_REF_NAME}."
