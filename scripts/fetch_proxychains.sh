#!/usr/bin/env bash
set -euo pipefail

# Fetch + build proxychains-ng for bundling with clash4cli.
#
# Output:
#   third_party/proxychains/<os>-<arch>/{proxychains4,libproxychains4.*}
#
# This script does NOT install anything system-wide.

PROXYCHAINS_REPO="rofl0r/proxychains-ng"
DEFAULT_VERSION="${PROXYCHAINS_NG_VERSION:-4.17}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
THIRD_PARTY_DIR="$PROJECT_DIR/third_party/proxychains"
BUILD_DIR="$PROJECT_DIR/.build/proxychains"
# Linux artifacts should run on "most" distros. Build them against an older glibc baseline.
# CentOS 7 ships glibc 2.17 (a common portability target for x86_64).
LINUX_DOCKER_IMAGE="${PROXYCHAINS_LINUX_DOCKER_IMAGE:-centos:7}"
LINUX_BUILD_MODE="${PROXYCHAINS_LINUX_BUILD_MODE:-auto}" # auto|docker|native

usage() {
  cat <<'EOF'
Usage:
  ./scripts/fetch_proxychains.sh [--os OS] [--arch ARCH] [--version VERSION] [--force]

Options:
  --os OS            Target OS: darwin|linux (default: current OS)
  --arch ARCH        Target arch: amd64|arm64 (default: current arch)
  --version VERSION  proxychains-ng version to build (default: ${PROXYCHAINS_NG_VERSION:-4.17})
  --force            Re-fetch/rebuild even if artifacts already exist

Notes:
  - Requires build toolchain (cc, make) and network access to GitHub.
  - For linux cross-build via Docker, image can be overridden with:
      PROXYCHAINS_LINUX_DOCKER_IMAGE (default: centos:7)
  - Linux build mode can be overridden with:
      PROXYCHAINS_LINUX_BUILD_MODE=auto|docker|native (default: auto)
  - Writes only to ./third_party/proxychains and ./.build/proxychains.
EOF
}

version="$DEFAULT_VERSION"
force=0
target_os=""
target_arch=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --version)
      if [[ $# -lt 2 ]]; then
        echo "Error: --version requires a value" >&2
        exit 1
      fi
      version="$2"
      shift 2
      ;;
    --os)
      if [[ $# -lt 2 ]]; then
        echo "Error: --os requires a value (darwin|linux)" >&2
        exit 1
      fi
      target_os="$2"
      shift 2
      ;;
    --arch)
      if [[ $# -lt 2 ]]; then
        echo "Error: --arch requires a value (amd64|arm64)" >&2
        exit 1
      fi
      target_arch="$2"
      shift 2
      ;;
    --force)
      force=1
      shift 1
      ;;
    *)
      echo "Error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin|linux) echo "$os" ;;
    *)
      echo "Error: unsupported OS: $os" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "Error: unsupported ARCH: $arch" >&2
      exit 1
      ;;
  esac
}

validate_target_os() {
  case "$1" in
    darwin|linux) return 0 ;;
    *)
      echo "Error: unsupported target OS: $1 (expected darwin|linux)" >&2
      exit 1
      ;;
  esac
}

validate_target_arch() {
  case "$1" in
    amd64|arm64) return 0 ;;
    *)
      echo "Error: unsupported target ARCH: $1 (expected amd64|arm64)" >&2
      exit 1
      ;;
  esac
}

download() {
  local url="$1"
  local out="$2"
  local attempt=1
  local max_attempts=5

  while [[ "$attempt" -le "$max_attempts" ]]; do
    if command -v curl >/dev/null 2>&1; then
      # Some environments have flaky HTTP/2; force HTTP/1.1 for reliability.
      if curl -fsSL --http1.1 --connect-timeout 10 --max-time 600 -o "$out" "$url"; then
        return 0
      fi
    elif command -v wget >/dev/null 2>&1; then
      if wget -qO "$out" "$url"; then
        return 0
      fi
    else
      echo "Error: neither curl nor wget is available" >&2
      exit 1
    fi

    if [[ "$attempt" -lt "$max_attempts" ]]; then
      sleep_time=$((attempt * 2))
      echo "Download failed (attempt ${attempt}/${max_attempts}), retrying in ${sleep_time}s: $url" >&2
      sleep "$sleep_time"
    fi
    attempt=$((attempt + 1))
  done

  echo "Error: failed to download after ${max_attempts} attempts: $url" >&2
  exit 1
}

host_os="$(detect_os)"
host_arch="$(detect_arch)"

if [[ -z "$target_os" ]]; then
  target_os="$host_os"
fi
if [[ -z "$target_arch" ]]; then
  target_arch="$host_arch"
fi
validate_target_os "$target_os"
validate_target_arch "$target_arch"

mkdir -p "$THIRD_PARTY_DIR" "$BUILD_DIR"

output_dir="$THIRD_PARTY_DIR/${target_os}-${target_arch}"
mkdir -p "$output_dir"

bin_path="$output_dir/proxychains4"
# proxychains-ng builds the library as libproxychains4.so even on macOS (Mach-O dylib with .so suffix).
lib_name="libproxychains4.so"
lib_path="$output_dir/$lib_name"
version_file="$output_dir/proxychains-ng.version"

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

use_docker=0
if [[ "$target_os" == "linux" ]]; then
  case "$LINUX_BUILD_MODE" in
    docker) use_docker=1 ;;
    native) use_docker=0 ;;
    auto|*)
      if docker_available; then
        use_docker=1
      fi
      ;;
  esac
fi

build_marker="native"
if [[ "$target_os" == "linux" && "$use_docker" -eq 1 ]]; then
  build_marker="docker:${LINUX_DOCKER_IMAGE}"
fi

if [[ "$force" -eq 0 && -f "$bin_path" && -f "$lib_path" ]]; then
  if [[ -f "$version_file" ]]; then
    existing_version="$(sed -n '1p' "$version_file" 2>/dev/null | tr -d '\r' || true)"
    existing_marker="$(sed -n '2p' "$version_file" 2>/dev/null | tr -d '\r' || true)"
    if [[ "$existing_version" == "$version" ]]; then
      if [[ -n "$existing_marker" && "$existing_marker" == "$build_marker" ]]; then
        echo "proxychains already present (${version}): $output_dir"
        exit 0
      fi
      if [[ -n "$existing_marker" && "$existing_marker" != "$build_marker" ]]; then
        echo "proxychains build marker mismatch: have '${existing_marker}', need '${build_marker}' - rebuilding..."
      elif [[ -z "$existing_marker" && "$build_marker" != "native" ]]; then
        echo "proxychains build marker missing; rebuilding to ensure linux binary compatibility..."
      else
        # Backward-compatible behavior: if the version matches and we have no marker requirement,
        # keep existing artifacts.
        echo "proxychains already present (${version}): $output_dir"
        exit 0
      fi
    else
      echo "proxychains version mismatch: have '${existing_version}', need '${version}' - rebuilding..."
    fi
  else
    echo "proxychains already present but version unknown; rebuilding to ensure '${version}'..."
  fi
fi

tag="$version"
if [[ "$tag" != v* ]]; then
  tag="v$tag"
fi

tmp_dir="$(mktemp -d)"
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT

src_tgz="$tmp_dir/proxychains-ng-${tag}.tar.gz"
# Use codeload endpoint for better connectivity in some networks where github.com is flaky/blocked.
src_url="https://codeload.github.com/${PROXYCHAINS_REPO}/tar.gz/refs/tags/${tag}"

echo "=== Fetching proxychains-ng ${tag} for ${target_os}-${target_arch} ==="
download "$src_url" "$src_tgz"

tar -xzf "$src_tgz" -C "$tmp_dir"
src_dir="$(find "$tmp_dir" -maxdepth 1 -type d -name 'proxychains-ng-*' | head -n 1 || true)"
if [[ -z "$src_dir" ]]; then
  echo "Error: failed to locate extracted proxychains-ng source directory" >&2
  exit 1
fi

jobs="$( (command -v nproc >/dev/null 2>&1 && nproc) || (sysctl -n hw.ncpu 2>/dev/null) || echo 4 )"

patch_sources() {
  # proxychains-ng v4.17 has an incorrect freeaddrinfo() typedef on macOS (and other POSIX),
  # which breaks builds when MONTEREY_HOOKING uses direct &freeaddrinfo assignment.
  local core_h="$src_dir/src/core.h"
  if [[ -f "$core_h" ]] && grep -q "typedef int (\\*freeaddrinfo_t)(struct addrinfo \\*);" "$core_h"; then
    echo "Patching proxychains-ng: freeaddrinfo_t should be void (core.h)"
    sed -i.bak \
      's|typedef int (\*freeaddrinfo_t)(struct addrinfo \*);|typedef void (*freeaddrinfo_t)(struct addrinfo *);|' \
      "$core_h"
    rm -f "${core_h}.bak" || true
  fi
}

find_installed_lib() {
  local install_dir="$1"
  local candidates=(
    "$install_dir/lib/libproxychains4.dylib"
    "$install_dir/lib/libproxychains4.so"
    "$install_dir/lib64/libproxychains4.dylib"
    "$install_dir/lib64/libproxychains4.so"
  )
  local candidate
  for candidate in "${candidates[@]}"; do
    if [[ -f "$candidate" ]]; then
      printf "%s\n" "$candidate"
      return 0
    fi
  done
  return 1
}

patch_sources

build_native() {
  local install_dir="$1"
  mkdir -p "$install_dir"

  pushd "$src_dir" >/dev/null
    echo "Configuring..."
    ./configure --prefix="$install_dir" --sysconfdir="$install_dir/etc"

    echo "Building..."
    make -j"$jobs"

    echo "Installing to staging directory..."
    make install
  popd >/dev/null

  local lib_src
  lib_src="$(find_installed_lib "$install_dir")"

  cp -f "$install_dir/bin/proxychains4" "$bin_path"
  cp -f "$lib_src" "$lib_path"
  if [[ "$target_os" == "darwin" ]]; then
    cp -f "$lib_src" "$output_dir/libproxychains4.dylib" || true
  fi
  chmod +x "$bin_path" || true
}

darwin_arch_to_lipo() {
  case "$1" in
    amd64) echo "x86_64" ;;
    arm64) echo "arm64" ;;
    *)
      echo "Error: unsupported darwin arch: $1" >&2
      exit 1
      ;;
  esac
}

build_darwin_thin() {
  if [[ "$host_os" != "darwin" ]]; then
    echo "Error: cannot build darwin targets from ${host_os}" >&2
    exit 1
  fi
  if [[ "$target_arch" == "$host_arch" ]]; then
    build_native "$tmp_dir/install-darwin-${target_arch}"
    return
  fi
  if ! command -v lipo >/dev/null 2>&1; then
    echo "Error: 'lipo' not found (required on macOS to produce per-arch binaries)" >&2
    exit 1
  fi

  local install_dir="$tmp_dir/install-darwin-universal"
  mkdir -p "$install_dir"

  pushd "$src_dir" >/dev/null
    echo "Configuring (universal build: x86_64 + arm64)..."
    # clang cannot run the preprocessor with multiple -arch values; set CPP to a single-arch preprocessor
    # (compile/run tests still succeed because the produced binaries are universal).
    local pp_arch
    pp_arch="$(darwin_arch_to_lipo "$host_arch")"
    CFLAGS="-O2 -arch x86_64 -arch arm64" LDFLAGS="-arch x86_64 -arch arm64" CPP="cc -E -arch ${pp_arch}" \
      ./configure --prefix="$install_dir" --sysconfdir="$install_dir/etc"

    echo "Building..."
    make -j"$jobs"

    echo "Installing to staging directory..."
    make install
  popd >/dev/null

  local thin_arch
  thin_arch="$(darwin_arch_to_lipo "$target_arch")"
  local lib_src
  lib_src="$(find_installed_lib "$install_dir")"
  lipo -thin "$thin_arch" -output "$bin_path" "$install_dir/bin/proxychains4"
  lipo -thin "$thin_arch" -output "$lib_path" "$lib_src"
  cp -f "$lib_path" "$output_dir/libproxychains4.dylib" || true
  chmod +x "$bin_path" || true
}

build_linux_via_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "Error: docker not found; cannot build linux/${target_arch} from ${host_os}/${host_arch}" >&2
    echo "Hint: run this script on a linux/${target_arch} machine, or install Docker." >&2
    exit 1
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "Error: docker is not available (is Docker running?)" >&2
    exit 1
  fi

  local platform="linux/${target_arch}"
  local src_inside="/work/$(basename "$src_dir")"
  local install_inside="/work/install-linux-${target_arch}"
  local uid gid
  uid="$(id -u 2>/dev/null || echo 0)"
  gid="$(id -g 2>/dev/null || echo 0)"

  docker run --rm --platform "$platform" \
    -e HOST_UID="$uid" -e HOST_GID="$gid" \
    -v "$tmp_dir:/work" \
    "$LINUX_DOCKER_IMAGE" \
    bash -lc "set -euo pipefail
      install_deps() {
        if command -v apt-get >/dev/null 2>&1; then
          export DEBIAN_FRONTEND=noninteractive
          apt-get update
          apt-get install -y --no-install-recommends ca-certificates build-essential
          rm -rf /var/lib/apt/lists/*
          return 0
        fi
        if command -v yum >/dev/null 2>&1; then
          # CentOS/RHEL family (used for old glibc baseline).
          # CentOS 7 is EOL; repo mirrorlists may be dead. Fall back to vault if needed.
          if ! yum -y install ca-certificates gcc make glibc-devel >/dev/null 2>&1; then
            echo 'yum failed; trying CentOS vault repos...'
            for repo in /etc/yum.repos.d/*.repo; do
              [ -f \"\$repo\" ] || continue
              sed -i \
                -e 's/^mirrorlist=/#mirrorlist=/g' \
                -e 's|^#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' \
                -e 's|^baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' \
                \"\$repo\" || true
            done
            yum -y install ca-certificates gcc make glibc-devel
          fi
          yum clean all || true
          return 0
        fi
        if command -v dnf >/dev/null 2>&1; then
          dnf -y install ca-certificates gcc make glibc-devel
          dnf clean all || true
          return 0
        fi
        echo 'Error: no supported package manager (apt-get/yum/dnf) found in image' >&2
        exit 1
      }

      install_deps
      cd '$src_inside'
      CC=gcc ./configure --prefix='$install_inside' --sysconfdir='$install_inside/etc'
      make -j\$(nproc)
      make install
      chown -R \"\${HOST_UID}:\${HOST_GID}\" /work || true
    "

  local install_dir="$tmp_dir/install-linux-${target_arch}"
  if [[ ! -f "$install_dir/bin/proxychains4" ]]; then
    echo "Error: docker build did not produce proxychains4 binary" >&2
    exit 1
  fi
  local lib_src
  lib_src="$(find_installed_lib "$install_dir")"

  cp -f "$install_dir/bin/proxychains4" "$bin_path"
  cp -f "$lib_src" "$lib_path"
  chmod +x "$bin_path" || true
}

case "$target_os" in
  darwin)
    build_darwin_thin
    ;;
  linux)
    if [[ "$use_docker" -eq 1 ]]; then
      build_linux_via_docker
    elif [[ "$host_os" == "linux" && "$host_arch" == "$target_arch" ]]; then
      echo "WARN: building linux/${target_arch} natively; result may require this machine's glibc" >&2
      echo "      Tip: install Docker and set PROXYCHAINS_LINUX_BUILD_MODE=docker for portable artifacts" >&2
      build_native "$tmp_dir/install-linux-${target_arch}"
    else
      echo "Error: cannot build linux/${target_arch} from ${host_os}/${host_arch} without Docker" >&2
      exit 1
    fi
    ;;
  *)
    echo "Error: unsupported target OS: $target_os" >&2
    exit 1
    ;;
esac

{
  printf "%s\n" "$version"
  printf "%s\n" "$build_marker"
} > "$version_file"

# Copy license files (for redistribution compliance)
shopt -s nullglob
for f in "$src_dir"/LICENSE* "$src_dir"/COPYING*; do
  base="$(basename "$f")"
  cp -f "$f" "$output_dir/${base}.proxychains-ng"
done
shopt -u nullglob

echo ""
echo "=== proxychains fetch complete ==="
echo "Output: $output_dir"
ls -la "$output_dir"
