#!/usr/bin/env bash
set -euo pipefail

required_assets=(
  checksums.txt
  checksums.txt.sigstore.json
  nebi-cli-provenance.sigstore.json
  nebi-cli-release.json
  nebi-cli-release.json.sigstore.json
  nebi-desktop-linux-amd64.tar.gz
  nebi-desktop-linux-amd64.tar.gz.sha256
  nebi-desktop-linux-amd64.tar.gz.sigstore.json
  nebi-desktop-linux-amd64.tar.gz.sbom.spdx.json
  nebi-desktop-linux-amd64.tar.gz.sbom.spdx.json.sigstore.json
  nebi-desktop-linux-amd64.tar.gz.release.json
  nebi-desktop-linux-amd64.tar.gz.release.json.sigstore.json
  nebi-desktop-macos-universal.zip
  nebi-desktop-macos-universal.zip.sha256
  nebi-desktop-macos-universal.zip.sigstore.json
  nebi-desktop-macos-universal.zip.sbom.spdx.json
  nebi-desktop-macos-universal.zip.sbom.spdx.json.sigstore.json
  nebi-desktop-macos-universal.zip.release.json
  nebi-desktop-macos-universal.zip.release.json.sigstore.json
  nebi-desktop-windows-amd64.exe
  nebi-desktop-windows-amd64.exe.sha256
  nebi-desktop-windows-amd64.exe.sigstore.json
  nebi-desktop-windows-amd64.exe.sbom.spdx.json
  nebi-desktop-windows-amd64.exe.sbom.spdx.json.sigstore.json
  nebi-desktop-windows-amd64.exe.release.json
  nebi-desktop-windows-amd64.exe.release.json.sigstore.json
  nebi-container-image.json
  nebi-container-image.json.sigstore.json
)

marker_assets=(
  nebi-cli-release.json
  nebi-desktop-linux-amd64.tar.gz.release.json
  nebi-desktop-macos-universal.zip.release.json
  nebi-desktop-windows-amd64.exe.release.json
  nebi-container-image.json
)

is_draft="$(gh release view "${GITHUB_REF_NAME}" --json isDraft --jq '.isDraft')"
if [ "$is_draft" != "true" ]; then
  echo "Release ${GITHUB_REF_NAME} is already public."
  exit 0
fi

mapfile -t assets < <(gh release view "${GITHUB_REF_NAME}" --json assets --jq '.assets[].name')

missing=()
for expected in "${required_assets[@]}"; do
  found=false
  for actual in "${assets[@]}"; do
    if [ "$actual" = "$expected" ]; then
      found=true
      break
    fi
  done
  if [ "$found" = "false" ]; then
    missing+=("$expected")
  fi
done

if [ "${#missing[@]}" -gt 0 ]; then
  echo "Release ${GITHUB_REF_NAME} remains draft; missing gated assets:"
  printf '  %s\n' "${missing[@]}"
  exit 0
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

for marker in "${marker_assets[@]}"; do
  gh release download "${GITHUB_REF_NAME}" \
    --pattern "$marker" \
    --dir "$tmpdir" \
    --clobber >/dev/null
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

"$python_bin" - "$GITHUB_SHA" "$runs_json" "${marker_paths[@]}" <<'PY'
import json
import sys

expected_commit, runs_path, *marker_paths = sys.argv[1:]
with open(runs_path, encoding="utf-8") as f:
    runs = json.load(f)["workflow_runs"]

latest_by_workflow = {}
for run in runs:
    if run.get("event") != "push" or run.get("head_sha") != expected_commit:
        continue
    workflow = run.get("name")
    previous = latest_by_workflow.get(workflow)
    if previous is None or (
        run.get("created_at", ""),
        int(run.get("run_attempt") or 0),
        int(run.get("id") or 0),
    ) > (
        previous.get("created_at", ""),
        int(previous.get("run_attempt") or 0),
        int(previous.get("id") or 0),
    ):
        latest_by_workflow[workflow] = run

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
    latest_run = latest_by_workflow.get(workflow)
    if latest_run is None:
        raise SystemExit(f"{path} references workflow {workflow}, but no matching run was found")

    marker_run_id = str(data["run_id"])
    latest_run_id = str(latest_run["id"])
    if marker_run_id != latest_run_id:
        raise SystemExit(
            f"{path} was produced by run_id {marker_run_id}, "
            f"but latest {workflow} run for {expected_commit} is {latest_run_id}"
        )

    latest_attempt = latest_run.get("run_attempt")
    marker_attempt = data.get("run_attempt")
    if latest_attempt is not None and marker_attempt is not None and str(marker_attempt) != str(latest_attempt):
        raise SystemExit(
            f"{path} was produced by run_attempt {marker_attempt}, "
            f"but latest {workflow} attempt for {expected_commit} is {latest_attempt}"
        )
PY

release_id="$(gh release view "${GITHUB_REF_NAME}" --json databaseId --jq '.databaseId')"
gh api --method PATCH "repos/${GITHUB_REPOSITORY}/releases/${release_id}" -F draft=false >/dev/null
echo "Published release ${GITHUB_REF_NAME}."
