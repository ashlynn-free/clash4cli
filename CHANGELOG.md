# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project follows Semantic Versioning.

## v0.3.0 - 2026-03-08

### Changed
- Slot ports are now deterministic: slot N always uses `baseMixedPort + N` and `baseAPIPort + N`. If the port is occupied, the slot fails with a clear error instead of silently scanning for alternatives.
- Subscription list is now sorted by name (then ID) for consistent ordering across tabs.
- Subscription sub-tabs on the Nodes page now scroll horizontally when they exceed the terminal width.
- Subscription switching no longer forcefully deselects the running node; slots from other subscriptions keep running independently.
- Settings tab now scrolls to keep the focused item visible.
- Settings: Mixed Port label and help text are now localized (i18n) across all 14 supported languages.
- Removed unused subscription delta fields (`LastUpdateAdded`, `LastUpdateRemoved`) from storage.

### Fixed
- Subscriptions tab UI sync improvements.

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
