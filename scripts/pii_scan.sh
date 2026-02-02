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

# Emails (exclude placeholders / known non-PII domains)
# Allowlist "example.*" and all its subdomains, to avoid false positives in test vectors (URI userinfo).
check "email addresses" '(?i)[a-z0-9._%+-]+@(?!([a-z0-9-]+\.)*example\.(?:com|net|org|invalid)\b)(?!users\.noreply\.github\.com\b)(?!localhost\b)[a-z0-9.-]+\.[a-z]{2,}'

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
echo "==> PNG metadata scan"

if command -v python3 >/dev/null 2>&1; then
  set +e
  python3 - <<'PY'
import struct
import sys
from pathlib import Path

PNG_SIG = b"\x89PNG\r\n\x1a\n"
ALLOW = {b"IHDR", b"PLTE", b"tRNS", b"IDAT", b"IEND"}

def list_tracked_pngs():
    # Prefer git-tracked PNGs to avoid scanning build artifacts.
    import subprocess
    try:
        out = subprocess.check_output(["git", "ls-files", "*.png"], text=True)
    except Exception:
        # Fallback: walk repo.
        for p in Path(".").rglob("*.png"):
            yield p.as_posix()
        return
    for line in out.splitlines():
        line = line.strip()
        if line:
            yield line

bad = []
for fname in list_tracked_pngs():
    p = Path(fname)
    try:
        data = p.read_bytes()
    except Exception as e:
        bad.append((fname, f"read failed: {e}"))
        continue
    if len(data) < 8 or data[:8] != PNG_SIG:
        bad.append((fname, "not a PNG signature"))
        continue
    i = 8
    while i + 8 <= len(data):
        length = struct.unpack(">I", data[i:i+4])[0]
        ctype = data[i+4:i+8]
        i += 8
        i += length + 4  # data + CRC
        if ctype not in ALLOW:
            bad.append((fname, f"contains chunk {ctype.decode('ascii', 'replace')}"))
            break
        if ctype == b"IEND":
            break

if bad:
    for fname, reason in bad:
        print(f"{fname}: {reason}", file=sys.stderr)
    sys.exit(1)
PY
  status=$?
  set -e

  if [[ "$status" -ne 0 ]]; then
    echo "^^^ Found PNG metadata chunks (run: python3 scripts/strip_png_metadata.py <file.png>)" >&2
    exit 1
  fi
else
  echo "WARN: python3 not found; skipping PNG metadata scan" >&2
fi

echo ""
if [[ "$fail" -ne 0 ]]; then
  echo "PII scan failed." >&2
  exit 1
fi

echo "PII scan passed."
