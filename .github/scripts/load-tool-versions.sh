#!/usr/bin/env bash
set -euo pipefail

versions_file="${1:-.github/tool-versions.env}"

while IFS='=' read -r name value || [ -n "$name" ]; do
  case "$name" in
    ""|\#*) continue ;;
  esac

  echo "${name}=${value}" >> "${GITHUB_ENV:?GITHUB_ENV is required}"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    echo "${name}=${value}" >> "$GITHUB_OUTPUT"
  fi
done < "$versions_file"
