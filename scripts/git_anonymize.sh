#!/usr/bin/env bash
set -euo pipefail

# Configure per-repo Git identity to reduce accidental PII leakage.
# This only writes to .git/config (local repo config), not global config.

name="${1:-clash4cli-bot}"
email="${2:-anon@example.invalid}"

if ! command -v git >/dev/null 2>&1; then
  echo "Error: git not found" >&2
  exit 2
fi

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "Error: not a git repository (cannot set local git config)" >&2
  exit 1
fi

git config --local user.name "$name"
git config --local user.email "$email"
git config --local user.useConfigOnly true

# Avoid signature metadata unless explicitly configured.
git config --local commit.gpgsign false
git config --local tag.gpgsign false

echo "Local git identity configured:"
echo "  user.name=$(git config --local user.name)"
echo "  user.email=$(git config --local user.email)"
echo "  user.useConfigOnly=$(git config --local user.useConfigOnly)"

