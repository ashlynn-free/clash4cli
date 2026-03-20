#!/usr/bin/env bash
set -euo pipefail

# Assemble npm packages from dist/ build artifacts into dist/npm/.
#
# Expects dist/clash4cli-<os>-<goarch>-<version>/ directories to already exist
# (produced by `make all`).

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"

version="$(cat "$project_dir/VERSION" 2>/dev/null | tr -d '[:space:]' || true)"
if [[ -z "$version" ]]; then
  echo "Error: VERSION file missing or empty" >&2
  exit 1
fi

dist_dir="$project_dir/dist"
npm_dir="$dist_dir/npm"
template_dir="$project_dir/npm-distribution"

# Platform mapping: "npm-name:go-suffix" pairs
PLATFORMS="darwin-arm64:darwin-arm64 darwin-x64:darwin-amd64 linux-arm64:linux-arm64 linux-x64:linux-amd64"

echo "=== Assembling npm packages (v${version}) ==="

# Clean previous output
rm -rf "$npm_dir"

# --- Platform packages ---
for entry in $PLATFORMS; do
  npm_platform="${entry%%:*}"
  go_suffix="${entry##*:}"
  src_dir="$dist_dir/clash4cli-${go_suffix}-${version}"
  dest_dir="$npm_dir/@clash4cli/${npm_platform}"

  if [[ ! -d "$src_dir" ]]; then
    echo "Error: build directory not found: $src_dir" >&2
    echo "       Run 'make all' first." >&2
    exit 1
  fi

  echo "  @clash4cli/${npm_platform} <- ${src_dir##*/}"
  mkdir -p "$dest_dir"

  # Copy all files from the build directory
  cp -a "$src_dir"/* "$dest_dir/"

  # Write package.json with version replaced
  sed "s/\"0.0.0\"/\"${version}\"/g" \
    "$template_dir/@clash4cli/${npm_platform}/package.json" \
    > "$dest_dir/package.json"

  # Ensure binaries are executable
  chmod +x "$dest_dir/c4c" 2>/dev/null || true
  chmod +x "$dest_dir/c4cgo" 2>/dev/null || true
  chmod +x "$dest_dir/mihomo" 2>/dev/null || true
done

# --- Main package ---
main_dest="$npm_dir/clash4cli"
echo "  clash4cli (wrapper)"
mkdir -p "$main_dest/bin"

# Copy JS wrappers
cp "$template_dir/clash4cli/bin/c4c" "$main_dest/bin/c4c"
cp "$template_dir/clash4cli/bin/c4cgo" "$main_dest/bin/c4cgo"
chmod +x "$main_dest/bin/c4c" "$main_dest/bin/c4cgo"

# Write package.json with version replaced (both top-level and optionalDependencies)
sed "s/\"0.0.0\"/\"${version}\"/g" \
  "$template_dir/clash4cli/package.json" \
  > "$main_dest/package.json"

# Copy project docs
cp "$project_dir/README.md" "$main_dest/" 2>/dev/null || true
cp "$project_dir/LICENSE" "$main_dest/" 2>/dev/null || true

echo ""
echo "=== npm packages assembled in dist/npm/ ==="
echo ""
ls -la "$npm_dir/"
echo ""
ls -la "$npm_dir/@clash4cli/"
