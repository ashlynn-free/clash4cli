#!/usr/bin/env bash
set -euo pipefail

# Lightweight repo scan for accidental PII / secrets before publishing.
# This is best-effort and intentionally conservative (may miss things).

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v rg >/dev/null 2>&1; then
  echo "Error: ripgrep (rg) is required" >&2
  exit 2
fi

echo "==> PII scan"

exclude=(
  -g '!.build/**'
  -g '!dist/**'
  -g '!third_party/**'
  -g '!**/*.png'
  -g '!**/*.jpg'
  -g '!**/*.jpeg'
  -g '!**/*.gif'
  -g '!**/*.zip'
  -g '!**/*.tar'
  -g '!**/*.tar.gz'
  -g '!**/*.tgz'
  -g '!**/*.gz'
)

fail=0

check() {
  local label="$1"
  local pattern="$2"

  set +e
  rg -n -P "${exclude[@]}" -- "$pattern" .
  local status=$?
  set -e

  case "$status" in
    0)
      echo "^^^ Found matches: ${label}" >&2
      fail=1
      ;;
    1)
      ;;
    *)
      echo "Error: rg failed while scanning for ${label}" >&2
      exit 2
      ;;
  esac
}

# Emails
check "email addresses" '(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}'

# Absolute home paths (common accidental leaks)
check "macOS home paths" '/Users/[A-Za-z0-9._-]+/'
check "Linux home paths" '/home/[A-Za-z0-9._-]+/'

# Common token formats
check "GitHub classic tokens" '\\bghp_[A-Za-z0-9]{30,}\\b'
check "GitHub fine-grained tokens" '\\bgithub_pat_[A-Za-z0-9_]{20,}\\b'
check "AWS access keys" '\\bAKIA[0-9A-Z]{16}\\b'
check "Slack tokens" '\\bxox[baprs]-[A-Za-z0-9-]{10,}\\b'

# Private keys
check "private keys" '-----BEGIN (RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----'

echo ""
if [[ "$fail" -ne 0 ]]; then
  echo "PII scan failed." >&2
  exit 1
fi

echo "PII scan passed."
