#!/usr/bin/env bash
set -euo pipefail

workflow_name="${1:-Release}"
poll_interval="${RELEASE_WORKFLOW_POLL_INTERVAL_SECONDS:-30}"
# Default to 45 minutes: enough for the Release workflow to build the draft,
# but short enough to avoid idling paid macOS/Windows release runners for hours.
max_attempts="${RELEASE_WORKFLOW_MAX_ATTEMPTS:-90}"

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"

if command -v python3 >/dev/null 2>&1; then
  python_bin=python3
elif command -v python >/dev/null 2>&1; then
  python_bin=python
else
  echo "python3 or python is required to wait for the release workflow."
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

attempt=1
while [ "$attempt" -le "$max_attempts" ]; do
  runs_json="$tmpdir/workflow-runs.json"
  gh api "repos/${GITHUB_REPOSITORY}/actions/runs?event=push&head_sha=${GITHUB_SHA}&per_page=100" > "$runs_json"

  result="$("$python_bin" .github/scripts/select-release-workflow-run.py "$workflow_name" "$GITHUB_SHA" "$GITHUB_REF_NAME" "$runs_json")"

  status="$(printf '%s\n' "$result" | awk -F '\t' '{print $1}')"
  conclusion="$(printf '%s\n' "$result" | awk -F '\t' '{print $2}')"
  run_id="$(printf '%s\n' "$result" | awk -F '\t' '{print $3}')"
  run_url="$(printf '%s\n' "$result" | awk -F '\t' '{print $4}')"

  if [ "$status" = "completed" ]; then
    if [ "$conclusion" = "success" ]; then
      echo "${workflow_name} workflow completed successfully for ${GITHUB_REF_NAME} (${run_url})."
      exit 0
    fi
    echo "${workflow_name} workflow ${run_id} completed with conclusion '${conclusion}'."
    [ -z "$run_url" ] || echo "$run_url"
    exit 1
  fi

  if [ "$status" = "not_found" ]; then
    echo "Waiting for ${workflow_name} workflow for ${GITHUB_REF_NAME} (${GITHUB_SHA}) to start..."
  else
    echo "Waiting for ${workflow_name} workflow ${run_id} to complete; current status is '${status}'."
    [ -z "$run_url" ] || echo "$run_url"
  fi

  attempt=$((attempt + 1))
  sleep "$poll_interval"
done

echo "Timed out waiting for ${workflow_name} workflow for ${GITHUB_REF_NAME} (${GITHUB_SHA})."
exit 1
