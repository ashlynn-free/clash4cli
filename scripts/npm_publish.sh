#!/usr/bin/env bash
set -euo pipefail

# Publish assembled npm packages from dist/npm/.
#
# Platform packages are published first (npm requires optionalDependencies
# to exist before the main package is published).
#
# Environment variables:
#   NPM_TOKEN     - npm auth token (required unless NPM_DRY_RUN=1)
#   NPM_DRY_RUN   - set to "1" for dry-run mode (no actual publish)
#   NPM_TAG        - npm dist-tag (default: "latest")

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"

npm_dir="$project_dir/dist/npm"
dry_run="${NPM_DRY_RUN:-0}"
npm_tag="${NPM_TAG:-latest}"

if [[ ! -d "$npm_dir" ]]; then
  echo "Error: dist/npm/ not found. Run 'make npm-assemble' first." >&2
  exit 1
fi

publish_flags=("--access" "public" "--tag" "$npm_tag")
npmrc_content=""
if [[ "$dry_run" == "1" ]]; then
  publish_flags+=("--dry-run")
  echo "=== npm publish (DRY RUN) ==="
else
  if [[ -z "${NPM_TOKEN:-}" ]]; then
    echo "Error: NPM_TOKEN is required (or set NPM_DRY_RUN=1)" >&2
    exit 2
  fi
  npmrc_content="//registry.npmjs.org/:_authToken=${NPM_TOKEN}"
  echo "=== npm publish ==="
fi

# All package directories to publish (platform packages first, main package last)
ALL_PKGS=(
  "@clash4cli/darwin-arm64"
  "@clash4cli/darwin-x64"
  "@clash4cli/linux-arm64"
  "@clash4cli/linux-x64"
  "clash4cli"
)

# Write .npmrc into each package directory so npm picks up the token
if [[ -n "$npmrc_content" ]]; then
  for pkg in "${ALL_PKGS[@]}"; do
    echo "$npmrc_content" > "$npm_dir/$pkg/.npmrc"
  done
fi

for pkg in "${ALL_PKGS[@]}"; do
  pkg_dir="$npm_dir/$pkg"
  if [[ ! -d "$pkg_dir" ]]; then
    echo "Error: package directory not found: $pkg_dir" >&2
    exit 1
  fi
  echo "  Publishing $pkg..."
  (cd "$pkg_dir" && npm publish "${publish_flags[@]}")
done

# Clean up all .npmrc files
for pkg in "${ALL_PKGS[@]}"; do
  rm -f "$npm_dir/$pkg/.npmrc"
done

echo ""
echo "=== npm publish complete ==="
