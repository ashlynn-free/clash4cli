#!/usr/bin/env bash
set -euo pipefail

# Fetch all runtime dependencies (mihomo + proxychains-ng) for supported platforms.
#
# Supported platforms:
#   darwin/amd64, darwin/arm64, linux/amd64
#
# This script only writes into:
#   ./third_party/{mihomo,proxychains} and ./.build/proxychains
#
# It does NOT install anything system-wide.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"

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

host_os="$(detect_os)"

usage() {
  cat <<'EOF'
Usage:
  ./pull_deps.sh [--force]

Options:
  --force   Re-fetch/rebuild even if artifacts exist

Notes:
  - Versions are pinned in Makefile:
      MIHOMO_VERSION, PROXYCHAINS_NG_VERSION
  - Requires network access.
  - Building proxychains linux/amd64 on macOS requires Docker.
EOF
}

force=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
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

read_make_var() {
  local name="$1"
  local fallback="$2"
  local makefile="$PROJECT_DIR/Makefile"

  # 1) Environment override
  if [[ -n "${!name:-}" ]]; then
    printf '%s\n' "${!name}"
    return 0
  fi

  # 2) Parse Makefile (expects: VAR ?= value OR VAR = value)
  if [[ -f "$makefile" ]]; then
    local v
    v="$(
      awk -v var="$name" '
        $1==var && ($2=="?=" || $2=="=") {print $3; exit}
      ' "$makefile" 2>/dev/null || true
    )"
    if [[ -n "$v" ]]; then
      printf '%s\n' "$v"
      return 0
    fi
  fi

  # 3) Fallback
  printf '%s\n' "$fallback"
}

mihomo_version="$(read_make_var "MIHOMO_VERSION" "1.19.19")"
proxychains_version="$(read_make_var "PROXYCHAINS_NG_VERSION" "4.17")"

platforms=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
)

echo "=== clash4cli deps ==="
echo "MIHOMO_VERSION=${mihomo_version}"
echo "PROXYCHAINS_NG_VERSION=${proxychains_version}"
echo ""

for p in "${platforms[@]}"; do
  os="${p%% *}"
  arch="${p##* }"

  echo "==> mihomo: ${os}/${arch}"
  if [[ "$force" -eq 1 ]]; then
    bash "$PROJECT_DIR/scripts/fetch_mihomo.sh" --os "$os" --arch "$arch" --version "$mihomo_version" --force
  else
    bash "$PROJECT_DIR/scripts/fetch_mihomo.sh" --os "$os" --arch "$arch" --version "$mihomo_version"
  fi
  echo ""

  echo "==> proxychains-ng: ${os}/${arch}"
  if [[ "$os" == "linux" && "$host_os" != "linux" ]]; then
    if ! command -v docker >/dev/null 2>&1; then
      echo "WARN: docker not found; skipping proxychains-ng for ${os}/${arch}." >&2
      echo "      Tip: install/start Docker, then run: make proxychains_all" >&2
      echo ""
      continue
    fi
    if ! docker info >/dev/null 2>&1; then
      echo "WARN: docker not available (is Docker running?); skipping proxychains-ng for ${os}/${arch}." >&2
      echo "      Tip: start Docker, then run: make proxychains_all" >&2
      echo ""
      continue
    fi
  fi
  if [[ "$os" == "darwin" && "$host_os" != "darwin" ]]; then
    echo "WARN: cannot build proxychains-ng for ${os}/${arch} on ${host_os}; skipping." >&2
    echo ""
    continue
  fi

  if [[ "$force" -eq 1 ]]; then
    bash "$PROJECT_DIR/scripts/fetch_proxychains.sh" --os "$os" --arch "$arch" --version "$proxychains_version" --force
  else
    bash "$PROJECT_DIR/scripts/fetch_proxychains.sh" --os "$os" --arch "$arch" --version "$proxychains_version"
  fi
  echo ""
done

echo "=== deps complete ==="
