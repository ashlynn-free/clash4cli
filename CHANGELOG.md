# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project follows Semantic Versioning.

## v0.8.4 - 2026-04-11

### Added
- Slots tab: press `D` to disconnect all running slots at once.
- proxychains config now includes `quiet_mode` to suppress `[proxychains]` log lines.

## v0.8.3 - 2026-03-21

### Added
- Node detail view now shows IP info (IP address, location, organization) from ipinfo.io when testing, helping verify the actual exit IP of a proxy node.
- IP info query reuses the same temporary core as site latency tests to avoid extra startup overhead for unbound nodes.
- Nodes panel Status column now shows the upstream proxy label when a slot has an upstream proxy configured (e.g. `:7890 ↑1.2K ↓3.4M VPN`).
- Custom subscription tabs now display a 📌 pin emoji prefix for easier identification.
- i18n: IP info strings added for all 14 supported languages.

### Fixed
- Nodes table: when the Server column is hidden, Latency and Status columns now move forward instead of leaving a huge gap after Name.

## v0.8.2 - 2026-03-21

### Added
- Nodes tab now remembers scroll position independently per subscription; switching subscriptions no longer resets or shares cursor position.
- Wrap-around scrolling in Nodes table: scrolling past the top jumps to the bottom and vice versa.

## v0.8.1 - 2026-03-21

### Changed
- Nodes slot key behavior is now: first press starts the slot in `Global`, a second press within a short double-press window starts it in `Rule`, and any press on a running slot stops it immediately.
- Global startup now enters `global` directly from the generated mihomo config instead of starting in `rule` and switching via the API afterward.
- Slot restarts now preserve the slot's previous mode (`rule` or `global`) instead of always falling back to `rule`.

### Fixed
- Settings text inputs now keep `Left`/`Right`, `Backspace`, and in-input editing behavior instead of letting page-level navigation consume those keys.
- Fixed a regression where slot start shortcuts could fail to land in `Global` because the post-start mode switch ran before the mihomo API was ready.

## v0.8.0 - 2026-03-21

### Added
- Reserved local subscription `Cus`: always present as a built-in custom node bucket, participates in the same Nodes/slots flow as normal subscriptions.
- Subscriptions tab custom-node manager for `Cus`: select `Cus` and press `E` to enter a CRUD view, then add/edit/delete nodes using raw URI links such as `vless://...`.

### Changed
- Custom node editing is now scoped to the Subscriptions tab instead of the Nodes tab, keeping node connection and node management separate.
- VLESS/Trojan URI normalization now maps transport/TLS-related query parameters into mihomo-compatible fields; `type=tcp&headerType=http` style VLESS links now convert to usable HTTP transport config instead of corrupting the proxy type.

## v0.7.0 - 2026-03-20

### Added
- Node detail view: press a key on any node to see full info and run per-site reachability tests (latency to Google, YouTube, GitHub, etc.).
- Per-slot upstream proxy chaining: configure an HTTP/HTTPS/SOCKS5 upstream proxy for each slot in Settings, enabling traffic to flow through two proxies in series.
- System Info page in Settings showing core version, uptime, rules count, OS info, and app version.
- Temporary core probing: test site reachability for nodes that are not bound to a running slot.
- npm distribution scaffolding (`@clash4cli/*` platform packages and `clash4cli` wrapper) with assemble/publish scripts.
- i18n: node detail view strings added for all 14 supported languages.

### Changed
- c4cgo slot listing and verbose output now display the full proxy chain when upstream is enabled (e.g. `node -> socks5://host:port`).
- Speed test latency probing is now slot-aware; uses the correct API endpoint and effective proxy name per slot.
- Settings tab accepts a SlotManager reference for per-slot upstream configuration and supports restarting individual or all slots.
- Proxy config generation now receives a slot ID to resolve per-slot upstream settings.
- Global keys (q, tab, digit shortcuts) are suppressed while the node detail view is open.

## v0.6.0 - 2026-03-08

### Changed
- c4cgo now defaults to quiet output; use `-v`/`--verbose` to show detailed proxy info (version, mode, binary path, proxy address).
- c4cgo proxychains4 mode now displays the binary path when verbose is enabled.
- `--version` flag no longer has a `-v` short form; `-v` is now `--verbose`.

## v0.5.0 - 2026-03-08

### Changed
- c4cgo slot listing now shows proxy mode emoji (📜 rule / 🌐 global) by querying the mihomo API, consistent with the TUI display.
- c4cgo slot listing format improved to: `📜 Slot 0  ● running at 17890, google-node(MySub)`.

## v0.4.0 - 2026-03-08

### Changed
- Slot ports are now deterministic: slot N always uses `baseMixedPort + N` and `baseAPIPort + N`. If the port is occupied, the slot fails with a clear error instead of silently scanning for alternatives.
- Subscription list is now sorted by name (then ID) for consistent ordering across tabs.
- Subscription sub-tabs on the Nodes page now scroll horizontally when they exceed the terminal width.
- Subscription switching no longer forcefully deselects the running node; slots from other subscriptions keep running independently.
- Settings tab now scrolls to keep the focused item visible.
- Settings tab scrolling now behaves more like the Nodes table: it follows the focused item, shows a scroll position indicator, and keeps the help line fixed at the bottom.
- Settings: Mixed Port label and help text are now localized (i18n) across all 14 supported languages.
- c4cgo slot listing now shows the subscription's user-defined name instead of only the internal subscription ID when available.
- Removed unused subscription delta fields (`LastUpdateAdded`, `LastUpdateRemoved`) from storage.

### Fixed
- Subscriptions tab UI sync improvements.
- Settings tab no longer pushes the top tab bar off-screen at very short terminal heights; scrolling now clips by rendered lines and tab-switch sizing is consistent.

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
