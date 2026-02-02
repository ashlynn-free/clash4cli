#!/usr/bin/env bash
set -euo pipefail

# Fetch mihomo binary for bundling with clash4cli.
#
# Output:
#   third_party/mihomo/mihomo-<os>-<arch>
#
# This script does NOT install anything system-wide.

MIHOMO_REPO="${MIHOMO_REPO:-MetaCubeX/mihomo}"
DEFAULT_VERSION="${MIHOMO_VERSION:-1.19.19}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
THIRD_PARTY_DIR="$PROJECT_DIR/third_party/mihomo"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/fetch_mihomo.sh [--os OS] [--arch ARCH] [--version VERSION] [--force]

Options:
  --os OS            Target OS: darwin|linux (default: current OS)
  --arch ARCH        Target arch: amd64|arm64 (default: current arch)
  --version VERSION  mihomo version tag (default: ${MIHOMO_VERSION:-1.19.19})
  --force            Re-fetch even if artifacts already exist

Notes:
  - Requires network access to GitHub.
  - Writes only to ./third_party/mihomo.
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
      token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
      headers=()
      if [[ -n "$token" && "$url" == https://api.github.com/* ]]; then
        headers+=( -H "Authorization: Bearer ${token}" )
      fi
      # Prefer downloading release assets via GitHub API (often works even when github.com is blocked),
      # which requires an octet-stream Accept header.
      if [[ "$url" == https://api.github.com/*/releases/assets/* ]]; then
        headers+=( -H "Accept: application/octet-stream" )
      fi

      if curl -fsSL --http1.1 --connect-timeout 10 --max-time 600 "${headers[@]}" -o "$out" "$url"; then
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

mkdir -p "$THIRD_PARTY_DIR"

bin_name="mihomo-${target_os}-${target_arch}"
bin_path="$THIRD_PARTY_DIR/$bin_name"
version_file="$THIRD_PARTY_DIR/${bin_name}.version"

if [[ "$force" -eq 0 && -f "$bin_path" ]]; then
  if [[ -f "$version_file" ]]; then
    existing_version="$(cat "$version_file" 2>/dev/null || true)"
    if [[ "$existing_version" == "$version" ]]; then
      echo "mihomo already present (${version}): $bin_path"
      exit 0
    fi
    echo "mihomo version mismatch: have '${existing_version}', need '${version}' - re-fetching..."
  else
    echo "mihomo already present but version unknown; re-fetching to ensure '${version}'..."
  fi
fi

tag="$version"
if [[ "$tag" != v* ]]; then
  tag="v$tag"
fi

tmp_dir="$(mktemp -d)"
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT

release_json="$tmp_dir/release.json"
api_url="https://api.github.com/repos/${MIHOMO_REPO}/releases/tags/${tag}"

echo "=== Fetching mihomo ${tag} for ${target_os}-${target_arch} ==="
download "$api_url" "$release_json"

if ! command -v python3 >/dev/null 2>&1; then
  echo "Error: python3 not found (required to parse GitHub release JSON)" >&2
  exit 1
fi

asset_info_file="$tmp_dir/asset.info"
TARGET_OS="$target_os" TARGET_ARCH="$target_arch" VERSION="$version" python3 - "$release_json" >"$asset_info_file" <<'PY'
import json
import os
import sys

release_json_path = sys.argv[1]
target_os = os.environ["TARGET_OS"].lower()
target_arch = os.environ["TARGET_ARCH"].lower()
version = os.environ["VERSION"]

arch_aliases = {
    "amd64": ["amd64", "x86_64", "x64"],
    "arm64": ["arm64", "aarch64"],
}.get(target_arch, [target_arch])

def is_bad_asset(name: str) -> bool:
    lower = name.lower()
    bad_suffixes = (".sha256", ".sha256sum", ".sha512", ".asc", ".sig", ".txt")
    if lower.endswith(bad_suffixes):
        return True
    return False

def score(name: str) -> int:
    lower = name.lower()
    if is_bad_asset(name):
        return -1
    if "mihomo" not in lower:
        return -1
    s = 0
    base = f"mihomo-{target_os}-{target_arch}"
    if lower == base:
        s += 200
    if lower.startswith(base):
        s += 120
    if base in lower:
        s += 100
    if target_os in lower:
        s += 20
    if any(a in lower for a in arch_aliases):
        s += 20
    # Prefer tagged artifacts if present in filename
    if version in lower or f"v{version}" in lower:
        s += 10
    # Penalize archives that are likely source
    if "source" in lower:
        s -= 50
    return s

with open(release_json_path, "r", encoding="utf-8") as f:
    data = json.load(f)

assets = data.get("assets") or []
best = None
best_score = -1
for a in assets:
    name = a.get("name") or ""
    api_url = a.get("url") or ""
    browser_url = a.get("browser_download_url") or ""
    if not name or not (api_url or browser_url):
        continue
    s = score(name)
    if s > best_score:
        best_score = s
        # Prefer GitHub API asset URL when available (more connectivity friendly); fall back to browser URL.
        resolved = api_url or browser_url
        best = (name, resolved)

if not best or best_score < 0:
    sys.stderr.write(f"Error: no suitable mihomo asset found for {target_os}/{target_arch} in this release.\n")
    sys.exit(1)

print(best[0])
print(best[1])
PY

asset_name="$(sed -n '1p' "$asset_info_file")"
asset_url="$(sed -n '2p' "$asset_info_file")"

if [[ -z "$asset_name" || -z "$asset_url" ]]; then
  echo "Error: failed to resolve mihomo asset URL from release JSON" >&2
  exit 1
fi

asset_path="$tmp_dir/$asset_name"
download "$asset_url" "$asset_path"

tmp_out="$tmp_dir/mihomo.out"

extract_from_dir() {
  local dir="$1"
  local found=""

  if [[ -f "$dir/mihomo" ]]; then
    found="$dir/mihomo"
  else
    found="$(find "$dir" -maxdepth 2 -type f -name 'mihomo*' | head -n 1 || true)"
  fi

  if [[ -z "$found" || ! -f "$found" ]]; then
    echo "Error: failed to locate mihomo binary in extracted archive" >&2
    exit 1
  fi

  cp -f "$found" "$tmp_out"
}

case "$asset_name" in
  *.gz)
    if ! command -v gzip >/dev/null 2>&1; then
      echo "Error: gzip not found (required to extract ${asset_name})" >&2
      exit 1
    fi
    gzip -dc "$asset_path" >"$tmp_out"
    ;;
  *.zip)
    if ! command -v unzip >/dev/null 2>&1; then
      echo "Error: unzip not found (required to extract ${asset_name})" >&2
      exit 1
    fi
    extract_dir="$tmp_dir/unzip"
    mkdir -p "$extract_dir"
    unzip -q "$asset_path" -d "$extract_dir"
    extract_from_dir "$extract_dir"
    ;;
  *.tar.gz|*.tgz)
    if ! command -v tar >/dev/null 2>&1; then
      echo "Error: tar not found (required to extract ${asset_name})" >&2
      exit 1
    fi
    extract_dir="$tmp_dir/tar"
    mkdir -p "$extract_dir"
    tar -xzf "$asset_path" -C "$extract_dir"
    extract_from_dir "$extract_dir"
    ;;
  *)
    cp -f "$asset_path" "$tmp_out"
    ;;
esac

mv -f "$tmp_out" "$bin_path"
chmod +x "$bin_path" || true
printf "%s\n" "$version" > "$version_file"

echo ""
echo "=== mihomo fetch complete ==="
echo "Output: $bin_path"
ls -la "$bin_path" "$version_file"
