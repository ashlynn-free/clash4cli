#!/usr/bin/env bash
set -euo pipefail

# Install built binaries atomically into $INSTALL_DIR (default: /usr/local/bin).
#
# macOS caches code-signing validation per inode. Overwriting an existing
# signed Mach-O in place (`cp` onto the same path) can leave a stale signature
# cache behind, and the next exec is killed with SIGKILL (the shell reports
# "Killed: 9"). Installing through a temporary file plus rename always creates
# a fresh inode, so the stale cache can never be reused.
#
# Usage:
#   sudo bash scripts/install_bins.sh c4c c4cgo c4cd
#   INSTALL_DIR="$HOME/.local/bin" bash scripts/install_bins.sh c4c c4cgo c4cd

dest="${INSTALL_DIR:-/usr/local/bin}"
mkdir -p "$dest"

for name in "$@"; do
  src="./$name"
  if [[ ! -f "$src" ]]; then
    echo "install_bins: missing build artifact $src (run 'make build' first)" >&2
    exit 1
  fi

  tmp="$dest/.$name.tmp.$$"
  cp "$src" "$tmp"
  chmod 0755 "$tmp"

  # Smoke test before replacing the installed copy: a binary that cannot even
  # print its version would also die on first use (e.g. SIGKILL from a stale
  # code signature), and the previous installation stays untouched.
  if ! "$tmp" --version >/dev/null 2>&1; then
    rm -f "$tmp"
    echo "install_bins: $src failed its smoke test; $dest/$name was not replaced" >&2
    exit 1
  fi

  mv -f "$tmp" "$dest/$name"
  echo "installed $dest/$name"
done

# A running c4cd keeps using its old (now unlinked) binary; c4c restarts it on
# the next start when the versions differ, so no manual step is required here.
