# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project follows Semantic Versioning.

## Unreleased

- TBD

## v0.1.0 - 2026-02-01

- Settings: show Linux-exclusive features on all platforms (disabled on non-Linux).
- Settings: discard unsaved changes when leaving the Settings tab.
- Nodes: persist selected node across restarts.
- c4cgo: use SOCKS5 in proxychains config for better compatibility.
- DevOps: add branch CI workflow; improve release deps fetch; refine PII scan allowlist.

## v0.0.2 - 2026-01-30

- Rename to **clash4cli** (`c4c`) and `c4cgo`.
- Add manual subscription paste (Subscriptions: `z`, then `Ctrl+S` to save).
- Improve subscription parsing (raw URI list + base64 with whitespace/newlines).
- Add `make deps`, `make all` cross-platform packaging and bundled `mihomo` + `proxychains4`.
- Fix Linux `proxychains-ng` portability by building against an older glibc baseline.
