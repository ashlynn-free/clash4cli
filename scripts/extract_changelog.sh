#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
changelog="${2:-CHANGELOG.md}"

if [[ -z "$tag" ]]; then
  echo "Usage: $0 <tag> [CHANGELOG.md]" >&2
  exit 2
fi

if [[ ! -f "$changelog" ]]; then
  echo "Error: changelog not found: $changelog" >&2
  exit 1
fi

# Print the section body for "## <tag> ..." until the next "## ".
awk -v tag="$tag" '
  BEGIN { found=0 }
  $0 ~ "^##[[:space:]]+" tag "([[:space:]]|$)" { found=1; next }
  found && $0 ~ "^##[[:space:]]+" { exit }
  found { print }
' "$changelog"

