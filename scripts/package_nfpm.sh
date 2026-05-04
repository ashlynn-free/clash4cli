#!/usr/bin/env bash
set -euo pipefail

# Build .deb/.rpm packages from the already-built linux dist folder using nfpm.
#
# Prereqs:
#   - `make linux/<arch>` has been run (creates dist/clash4cli-linux-<arch>-<version>/)
#   - `nfpm` is installed (https://nfpm.goreleaser.com/)
#
# Example:
#   ./scripts/package_nfpm.sh --arch amd64 --packager deb,rpm

usage() {
  cat <<'EOF'
Usage:
  ./scripts/package_nfpm.sh --arch <amd64|arm64> [--packager <deb|rpm|deb,rpm>] [--version <x.y.z>] [--dist <dir>]

Options:
  --arch        Target arch: amd64|arm64 (required)
  --packager    Package format(s): deb, rpm, or deb,rpm (default: deb,rpm)
  --version     App version (default: read from ./VERSION)
  --dist        Dist directory (default: ./dist)

Notes:
  - This script does not fetch deps. Run: make deps
  - This script does not build by itself. Run: make linux/<arch>
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"

arch=""
packager="deb,rpm"
version=""
dist_dir="$project_dir/dist"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --arch)
      arch="${2:-}"
      shift 2
      ;;
    --packager)
      packager="${2:-}"
      shift 2
      ;;
    --version)
      version="${2:-}"
      shift 2
      ;;
    --dist)
      dist_dir="${2:-}"
      shift 2
      ;;
    *)
      echo "Error: unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$arch" ]]; then
  echo "Error: --arch is required" >&2
  usage >&2
  exit 1
fi
case "$arch" in
  amd64|arm64) ;;
  *)
    echo "Error: unsupported --arch: $arch (expected amd64|arm64)" >&2
    exit 1
    ;;
esac

if [[ -z "$version" ]]; then
  version="$(cat "$project_dir/VERSION" 2>/dev/null || true)"
fi
if [[ -z "$version" ]]; then
  echo "Error: cannot determine version (missing VERSION file?)" >&2
  exit 1
fi

nfpm_bin="$(command -v nfpm 2>/dev/null || true)"
if [[ -z "$nfpm_bin" ]]; then
  gobin="$(go env GOBIN 2>/dev/null || true)"
  if [[ -n "$gobin" && -x "$gobin/nfpm" ]]; then
    nfpm_bin="$gobin/nfpm"
  else
    gopath="$(go env GOPATH 2>/dev/null || true)"
    if [[ -n "$gopath" && -x "$gopath/bin/nfpm" ]]; then
      nfpm_bin="$gopath/bin/nfpm"
    fi
  fi
fi
if [[ -z "$nfpm_bin" ]]; then
  echo "Error: nfpm not found." >&2
  echo "Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" >&2
  exit 1
fi

build_dir="$dist_dir/clash4cli-linux-${arch}-${version}"
if [[ ! -d "$build_dir" ]]; then
  echo "Error: build folder not found: $build_dir" >&2
  echo "Run: make linux/${arch}" >&2
  exit 1
fi

req_files=( "$build_dir/c4c" "$build_dir/c4cgo" "$build_dir/c4cd" "$build_dir/mihomo" )
for f in "${req_files[@]}"; do
  if [[ ! -f "$f" ]]; then
    echo "Error: required file missing: $f" >&2
    exit 1
  fi
done

lib_dir="/usr/lib/clash4cli"
doc_dir="/usr/share/doc/clash4cli"

tmp_cfg="$(mktemp)"
cleanup() { rm -f "$tmp_cfg"; }
trap cleanup EXIT

{
  echo "name: clash4cli"
  echo "arch: ${arch}"
  echo "platform: linux"
  echo "version: \"${version}\""
  echo "release: \"1\""
  echo "section: net"
  echo "priority: optional"
  echo "maintainer: \"clash4cli contributors\""
  echo "description: \"Clash for CLI: terminal proxy manager (bundles mihomo + proxychains-ng).\""
  echo "license: \"MIT (bundles GPL components; see THIRD_PARTY_NOTICES.md)\""
  echo "contents:"
  echo "  - src: ${build_dir}/c4c"
  echo "    dst: /usr/bin/c4c"
  echo "    file_info:"
  echo "      mode: 0755"
  echo "  - src: ${build_dir}/c4cgo"
  echo "    dst: /usr/bin/c4cgo"
  echo "    file_info:"
  echo "      mode: 0755"
  echo "  - src: ${build_dir}/c4cd"
  echo "    dst: /usr/bin/c4cd"
  echo "    file_info:"
  echo "      mode: 0755"
  echo "  - src: ${build_dir}/mihomo"
  echo "    dst: ${lib_dir}/mihomo"
  echo "    file_info:"
  echo "      mode: 0755"

  if [[ -f "${build_dir}/proxychains4" && -f "${build_dir}/libproxychains4.so" ]]; then
    echo "  - src: ${build_dir}/proxychains4"
    echo "    dst: ${lib_dir}/proxychains4"
    echo "    file_info:"
    echo "      mode: 0755"
    echo "  - src: ${build_dir}/libproxychains4.so"
    echo "    dst: ${lib_dir}/libproxychains4.so"
    echo "    file_info:"
    echo "      mode: 0644"
  fi

  for f in README.md LICENSE THIRD_PARTY_NOTICES.md VERSION CHANGELOG.md; do
    if [[ -f "${build_dir}/${f}" ]]; then
      echo "  - src: ${build_dir}/${f}"
      echo "    dst: ${doc_dir}/${f}"
      echo "    file_info:"
      echo "      mode: 0644"
    elif [[ -f "${project_dir}/${f}" ]]; then
      echo "  - src: ${project_dir}/${f}"
      echo "    dst: ${doc_dir}/${f}"
      echo "    file_info:"
      echo "      mode: 0644"
    fi
  done

  for f in mihomo.version proxychains-ng.version LICENSE.mihomo; do
    if [[ -f "${build_dir}/${f}" ]]; then
      echo "  - src: ${build_dir}/${f}"
      echo "    dst: ${doc_dir}/${f}"
      echo "    file_info:"
      echo "      mode: 0644"
    fi
  done

  shopt -s nullglob
  for f in "${build_dir}/"*.proxychains-ng; do
    base="$(basename "$f")"
    echo "  - src: ${f}"
    echo "    dst: ${doc_dir}/${base}"
    echo "    file_info:"
    echo "      mode: 0644"
  done
  shopt -u nullglob
} >"$tmp_cfg"

echo "=== nfpm config ==="
echo "build_dir: $build_dir"
echo "packager:  $packager"
echo "target:    $dist_dir"

IFS=',' read -r -a packagers <<<"$packager"
for p in "${packagers[@]}"; do
  case "$p" in
    deb|rpm) ;;
    *)
      echo "Error: unsupported packager: $p (expected deb|rpm|deb,rpm)" >&2
      exit 1
      ;;
  esac
done

for p in "${packagers[@]}"; do
  echo ""
  echo "=== Building .$p ==="
  "$nfpm_bin" pkg --packager "$p" --target "$dist_dir" --config "$tmp_cfg"
done

echo ""
echo "=== Done ==="
ls -lh "$dist_dir"/*.deb "$dist_dir"/*.rpm 2>/dev/null || true
