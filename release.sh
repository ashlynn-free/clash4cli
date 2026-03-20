#!/usr/bin/env bash
set -euo pipefail

# Publish a GitHub Release from the current workspace.
#
# Requirements:
#   - env: GITHUB_TOKEN (PAT or GitHub Actions token with contents:write)
#   - tools: gh, git, make, go, and nfpm (for .deb/.rpm)
#
# What it does:
#   1) make deps
#   2) make all (tar.gz for all supported platforms)
#   3) make linux_pkgs (deb/rpm for linux amd64/arm64)
#   4) generate dist/SHA256SUMS
#   5) create/update GitHub Release and upload assets

require() {
  local bin="$1"
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "Error: required command not found: $bin" >&2
    exit 2
  fi
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$script_dir"
tools_bin_dir="$project_dir/.build/tools/bin"
release_notes_file=""

cleanup() {
  if [[ -n "${release_notes_file}" && -f "${release_notes_file}" ]]; then
    rm -f "${release_notes_file}" || true
  fi
}
trap cleanup EXIT

repo_from_origin() {
  local url repo
  url="$(git remote get-url origin 2>/dev/null || true)"
  if [[ -z "$url" ]]; then
    return 1
  fi
  repo="$(
    printf '%s' "$url" | sed -E 's#.*github\.com[:/]+([^/]+/[^/]+)(\.git)?$#\1#'
  )"
  if [[ -z "$repo" || "$repo" == "$url" ]]; then
    return 1
  fi
  printf '%s\n' "$repo"
}

write_sha256sums() {
  local dist_dir="$1"
  local out="$dist_dir/SHA256SUMS"
  local -a files=()

  shopt -s nullglob
  files+=( "$dist_dir"/*.tar.gz "$dist_dir"/*.deb "$dist_dir"/*.rpm )
  shopt -u nullglob

  if [[ "${#files[@]}" -eq 0 ]]; then
    echo "Error: no release artifacts found under: $dist_dir" >&2
    exit 1
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    local -a names=()
    local f
    for f in "${files[@]}"; do
      names+=( "$(basename "$f")" )
    done
    (cd "$dist_dir" && sha256sum "${names[@]}" >"$(basename "$out")")
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    local -a names=()
    local f
    for f in "${files[@]}"; do
      names+=( "$(basename "$f")" )
    done
    (cd "$dist_dir" && shasum -a 256 "${names[@]}" >"$(basename "$out")")
    return 0
  fi

  echo "Error: need sha256sum or shasum to generate checksums" >&2
  exit 2
}

ensure_nfpm() {
  if command -v nfpm >/dev/null 2>&1; then
    return 0
  fi

  mkdir -p "$tools_bin_dir"
  if [[ -x "$tools_bin_dir/nfpm" ]]; then
    export PATH="$tools_bin_dir:$PATH"
    return 0
  fi

  require go
  local ver="${NFPM_VERSION:-latest}"
  echo "nfpm not found; installing nfpm@${ver} into ${tools_bin_dir}..." >&2
  GOBIN="$tools_bin_dir" go install "github.com/goreleaser/nfpm/v2/cmd/nfpm@${ver}"
  export PATH="$tools_bin_dir:$PATH"

  if ! command -v nfpm >/dev/null 2>&1; then
    echo "Error: nfpm install succeeded but nfpm is still not found in PATH" >&2
    exit 2
  fi
}

resolve_release_target() {
  local repo="$1"
  local target="${RELEASE_TARGET:-}"

  if [[ -n "$target" ]]; then
    target="$(tr -d '[:space:]' <<<"$target")"
    if gh api "repos/${repo}/commits/${target}" >/dev/null 2>&1; then
      printf '%s\n' "$target"
      return 0
    fi
    echo "Error: RELEASE_TARGET is invalid for ${repo}: ${target}" >&2
    exit 1
  fi

  # Prefer tagging the exact local HEAD *if* it exists in the remote repo.
  local head_sha
  head_sha="$(git rev-parse HEAD 2>/dev/null || true)"
  head_sha="$(tr -d '[:space:]' <<<"$head_sha")"
  if [[ -n "$head_sha" ]] && gh api "repos/${repo}/commits/${head_sha}" >/dev/null 2>&1; then
    printf '%s\n' "$head_sha"
    return 0
  fi

  # Fallback: use the repo's default branch (always valid). This avoids the common
  # situation where local HEAD isn't pushed, but we still want to upload artifacts.
  local default_branch
  default_branch="$(gh api "repos/${repo}" --jq .default_branch 2>/dev/null || true)"
  default_branch="$(tr -d '[:space:]' <<<"$default_branch")"
  if [[ -z "$default_branch" ]]; then
    default_branch="main"
  fi

  if gh api "repos/${repo}/commits/${default_branch}" >/dev/null 2>&1; then
    if [[ -n "$head_sha" ]]; then
      echo "WARN: local HEAD is not found on ${repo}; tagging default branch instead: ${default_branch}" >&2
      echo "      If you want a different target, set RELEASE_TARGET=<branch|sha>." >&2
    fi
    printf '%s\n' "$default_branch"
    return 0
  fi

  echo "Error: cannot resolve a valid release target for ${repo}." >&2
  echo "Set RELEASE_TARGET=<branch|sha> and try again." >&2
  exit 1
}

main() {
  if [[ -z "${GITHUB_TOKEN:-}" ]]; then
    echo "Error: GITHUB_TOKEN is required" >&2
    exit 2
  fi

  require gh
  require git
  require make

  # Target repo is intentionally hardcoded to avoid accidental publishing to a wrong remote.
  # Change this only when you deliberately move the release destination.
  local repo="ashlynn-free/clash4cli"

  local version
  version="$(cat VERSION 2>/dev/null | tr -d '[:space:]' || true)"
  if [[ -z "$version" ]]; then
    echo "Error: VERSION file missing or empty" >&2
    exit 2
  fi
  local tag="v${version}"
  local title="clash4cli ${tag}"

  # Prepare release notes from CHANGELOG.
  release_notes_file="$(mktemp)"
  local body
  body="$(bash scripts/extract_changelog.sh "$tag" 2>/dev/null || true)"
  if [[ -z "$(tr -d '[:space:]' <<<"$body")" ]]; then
    echo "Error: CHANGELOG.md has no section for ${tag}" >&2
    echo "Add a section like: ## ${tag} - YYYY-MM-DD" >&2
    exit 1
  fi
  {
    echo "# ${title}"
    echo ""
    echo "$body"
    echo ""
    echo "Full changelog: see CHANGELOG.md"
  } >"$release_notes_file"

  # Build artifacts.
  make deps
  make all

  # deb/rpm require nfpm (install locally if missing).
  ensure_nfpm
  make linux_pkgs

  # Generate checksums.
  write_sha256sums "dist"

  shopt -s nullglob
  local -a assets=(dist/*.tar.gz dist/*.deb dist/*.rpm dist/SHA256SUMS)
  shopt -u nullglob

  export GH_TOKEN="$GITHUB_TOKEN"

  if gh release view "$tag" --repo "$repo" >/dev/null 2>&1; then
    gh release edit "$tag" --repo "$repo" --title "$title" --notes-file "$release_notes_file"
    gh release upload "$tag" --repo "$repo" --clobber "${assets[@]}"
  else
    local target
    target="$(resolve_release_target "$repo")"
    gh release create "$tag" --repo "$repo" --target "$target" --title "$title" --notes-file "$release_notes_file" "${assets[@]}"
  fi

  echo "Release published: ${repo} ${tag}"

  # npm publish (optional: only if NPM_TOKEN is set)
  if [[ -n "${NPM_TOKEN:-}" ]]; then
    echo ""
    echo "=== Publishing to npm ==="
    bash "$script_dir/scripts/npm_assemble.sh"
    bash "$script_dir/scripts/npm_publish.sh"
  else
    echo ""
    echo "Skipping npm publish (NPM_TOKEN not set)."
  fi
}

main "$@"
