---
title: "feat: macOS TUN Mode Support"
type: feat
status: completed
date: 2026-04-24
origin: docs/brainstorms/2026-04-24-macos-tun-mode-requirements.md
---

# macOS TUN Mode Support

## Overview

Enable TUN mode on macOS by removing the `runtime.GOOS == "linux"` platform guard. TUN provides global transparent proxy without requiring per-app proxy configuration. The mihomo core already supports macOS TUN — this plan unlocks it in clash4cli's TUI and config generation.

## Problem Frame

macOS users cannot enable TUN mode through the TUI. The code hard-gates TUN to Linux, but mihomo itself fully supports macOS TUN without requiring root privileges. macOS users are limited to proxychains (blocked by SIP for system binaries) or manual system proxy configuration. (see origin: docs/brainstorms/2026-04-24-macos-tun-mode-requirements.md)

## Requirements Trace

**Config Generation**
- R1. Remove `runtime.GOOS == "linux"` guard on TUN config generation
- R2. macOS default: `mixed` stack + `auto-route` + `auto-detect-interface`
- R3. No root warning on macOS for TUN (already gated by `runtime.GOOS == "linux"`)

**User Config Model**
- R5. User-configurable TUN stack: `mixed` (default on macOS), `gvisor`, `system`
- R6. User-configurable MTU and device name

**TUI Integration**
- R8. TUN toggle available on macOS in TUI settings

**Invariants**
- R4. c4cgo proxychains mechanism stays independent
- R9. Docker TCP redirect stays disabled on macOS (iptables-only)

~~R7. DNS config~~ — dropped per user decision; DNS handled by subscription config

## Scope Boundaries

- No macOS Docker redirect (iptables unavailable)
- No system-level DNS management — DNS left to subscription config or manual mihomo config
- No macOS 26.x compatibility workarounds (mihomo core responsibility)
- c4cgo proxychains unchanged
- Linux TUN behavior unchanged (keeps `system` stack default)

## Context & Research

### Relevant Code and Patterns

- `internal/proxy/config.go:132-138` — TUN block generation, currently Linux-only
- `internal/proxy/config.go:33-45` — `TunConfig` struct already has all needed fields (Stack, MTU, Device, DNSHijack, etc.)
- `internal/config/config.go` — `ProxyConfig` struct; `TunEnabled bool` is the only TUN field
- `internal/tui/tabs/settings.go:276-322` — Settings TUI; `linuxOnlyDisabled` controls both TUN and Docker
- `internal/tui/tabs/settings.go:2221` — Save-side root warning (correctly Linux-only)
- `internal/app/app.go:130` — Startup root warning (correctly Linux-only)
- `internal/proxy/controller.go:203` — Docker redir-port (correctly Linux-only)
- 14 `internal/i18n/lang/*.yaml` files — contain "TUN mode (Linux)" strings

### Institutional Learnings

No relevant learnings in `docs/solutions/`.

## Key Technical Decisions

- **macOS default stack = `mixed`**, Linux stays `system**: macOS benefits from mixed (TCP via system stack, UDP via gvisor) per mihomo docs. Linux `system` stack is battle-tested and unchanged to avoid regression. Platform-conditional default logic needed in config generation.
- **R7 (DNS) dropped**: dns-hijack is a TUN-layer concern but user chose to keep DNS entirely in subscription config to avoid maintenance burden.
- **`TunStack`, `TunMTU`, `TunDevice` fields added to ProxyConfig**: Extends the existing config model. Empty/zero values fall back to platform defaults. No migration needed — absent fields use defaults.
- **TUI section renamed**: "Linux Exclusive Features" → platform-neutral label. TUN available on macOS+Linux, Docker stays Linux-only.
- **No kill-switch mechanism**: macOS automatically cleans up utun devices when the creating process exits. Unlike Linux, manual route cleanup is not needed. mihomo's auto-route is removed when the process terminates.

## Open Questions

### Resolved During Planning

- **Which files need modification?** Repo research identified 7+ files with TUN-related platform guards: config.go, settings.go, settings_test.go, config_test.go, 14 i18n YAML files, i18n.go
- **R7 DNS scope**: Dropped entirely per user decision
- **Default stack per platform**: Linux keeps `system`, macOS gets `mixed`
- **Kill-switch**: Not needed on macOS (automatic utun cleanup)

### Deferred to Implementation

- **TUI layout for advanced TUN options (R5-R6)**: Exact widget arrangement (dropdown for stack, input for MTU/device) — follow existing settings field patterns
- **Advanced options visibility**: Whether to show stack/MTU/device fields inline or behind an expandable section — follow existing TUI patterns

## Implementation Units

- [ ] **Unit 1: Expand TUN config generation for macOS**

**Goal:** Allow TUN config to be generated on macOS with platform-appropriate defaults.

**Requirements:** R1, R2

**Dependencies:** None

**Files:**
- Modify: `internal/proxy/config.go`
- Test: `internal/proxy/config_test.go`

**Approach:**
- Change the guard at line 132 from `runtime.GOOS == "linux"` to `runtime.GOOS == "linux" || runtime.GOOS == "darwin"`
- Add platform-conditional stack default: `"system"` for Linux, `"mixed"` for macOS
- When advanced fields (TunStack, TunMTU, TunDevice) exist on ProxyConfig, use them; otherwise fall back to platform defaults
- `auto-route` and `auto-detect-interface` stay `true` on both platforms

**Patterns to follow:**
- Existing `GenerateConfig()` function structure
- Platform-conditional logic pattern used elsewhere (e.g., `process_unix.go`)

**Test scenarios:**
- Happy path: `GenerateConfig` with `TunEnabled=true` on `darwin` produces `Tun` block with `Stack: "mixed"`, `AutoRoute: true`
- Happy path: `GenerateConfig` with `TunEnabled=true` on `linux` still produces `Stack: "system"` (no regression)
- Happy path: `GenerateConfig` with `TunEnabled=false` on `darwin` produces `Tun: nil`
- Edge case: Custom stack value via advanced config overrides platform default
- Edge case: Custom MTU/device value passed through when set

**Verification:**
- `go test ./internal/proxy/...` passes on both macOS and Linux
- Generated config YAML contains correct TUN block for each platform

---

- [ ] **Unit 2: Extend ProxyConfig with TUN advanced fields**

**Goal:** Add configurable TUN parameters (stack, MTU, device) to the user config model.

**Requirements:** R5, R6

**Dependencies:** None

**Files:**
- Modify: `internal/config/config.go`

**Approach:**
- Add fields to `ProxyConfig` (in the `Proxy` sub-struct): `TunStack string`, `TunMTU int`, `TunDevice string`
- Use YAML tags with `omitempty` so existing configs without these fields deserialize to zero values (which means "use platform default")
- No migration needed — absent fields naturally fall back to defaults in config generation
- Stack values constrained to: `""` (use default), `"mixed"`, `"gvisor"`, `"system"`

**Patterns to follow:**
- Existing `ProxyConfig` field patterns (YAML tags, omitempty)
- `TunEnabled bool` as the model for how TUN fields are added

**Test scenarios:**
- Happy path: Config with `tun_stack: "gvisor"` deserializes correctly
- Edge case: Config without `tun_stack` field deserializes to empty string (backward compatible)
- Edge case: Config with `tun_mtu: 9000` deserializes correctly; zero value means "use default"

**Verification:**
- `go test ./internal/config/...` passes
- Existing config files without new fields load without error

---

- [ ] **Unit 3: Update TUI settings for macOS TUN**

**Goal:** Enable TUN toggle on macOS; add advanced TUN options; restructure the settings section.

**Requirements:** R8, R9, R5

**Dependencies:** Unit 2

**Files:**
- Modify: `internal/tui/tabs/settings.go`
- Test: `internal/tui/tabs/settings_test.go`

**Approach:**
- Split `linuxOnlyDisabled` into two variables: `tunDisabled` (false on Linux and macOS) and `dockerDisabled` (false only on Linux)
- Change TUN label fallback from `"TUN mode (Linux)"` to `"TUN mode"`
- Rename section header from `"Linux Exclusive Features"` to something like `"Advanced Network Features"` or `"TUN & Network Features"`
- Add stack selection field (dropdown: mixed/gvisor/system) — disabled when TUN is off; shows platform default as hint
- Add MTU and device name input fields — hidden or disabled when TUN is off
- Docker TCP redirect field stays disabled on non-Linux (unchanged behavior)

**Patterns to follow:**
- Existing toggle/input field patterns in settings.go
- Existing conditional visibility/disable patterns (e.g., upstream proxy fields)

**Test scenarios:**
- Happy path: On macOS, `tun_enabled` field is focusable and enabled
- Happy path: On macOS, Docker redirect field is still disabled
- Happy path: On Linux, both TUN and Docker fields remain enabled (no regression)
- Edge case: Stack dropdown shows correct platform default label
- Integration: Toggling TUN on shows stack/MTU/device fields; toggling off hides/disables them

**Verification:**
- `go test ./internal/tui/tabs/...` passes
- Manual: TUI shows TUN toggle as interactive on macOS

---

- [ ] **Unit 4: Update i18n strings**

**Goal:** Remove "(Linux)" from TUN label across all languages; update section header.

**Requirements:** R8

**Dependencies:** None (parallel with Unit 1-2)

**Files:**
- Modify: `internal/i18n/i18n.go` (consider renaming `LinuxFeatures` field)
- Modify: 14 files in `internal/i18n/lang/`

**Approach:**
- Change `tun` value from `"TUN mode (Linux)"` to `"TUN mode"` in all 14 language files
- Change `linux_features` value to platform-neutral text (e.g., `"Advanced Network Features"`, `"高级网络特性"`)
- Keep `docker_tcp` with "(Linux)" suffix since it genuinely remains Linux-only
- If renaming the `LinuxFeatures` struct field in i18n.go, update all YAML keys accordingly. If keeping the struct field name, only update the values.

**Patterns to follow:**
- Existing i18n key-value patterns in lang files

**Test scenarios:**
- Test expectation: none — i18n string changes are verified by TUI rendering in Unit 3

**Verification:**
- `go test ./internal/i18n/...` passes
- No missing translation keys (verify all 14 files have same keys)

---

- [ ] **Unit 5: Update tests for cross-platform TUN**

**Goal:** Ensure existing tests pass with the new macOS TUN behavior and add macOS-specific test coverage.

**Requirements:** R1, R2, R3

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/proxy/config_test.go`
- Modify: `internal/tui/tabs/settings_test.go`

**Approach:**
- `config_test.go`: Change the `runtime.GOOS` branch to expect TUN enabled on both `"linux"` and `"darwin"`. Add assertion for correct Stack value per platform.
- `settings_test.go`: Change focus test to expect `tun_enabled` focusable on both Linux and macOS (skip only on other platforms).

**Patterns to follow:**
- Existing test patterns in both files (table-driven, runtime.GOOS branches)

**Test scenarios:**
- Happy path: `GenerateConfig` with TUN enabled produces non-nil `Tun` on both Linux and macOS
- Happy path: TUN stack value is `"system"` on Linux, `"mixed"` on macOS
- Edge case: TUN is still nil on non-Linux, non-macOS platforms (Windows, etc.)
- Happy path: Settings focus test reaches `tun_enabled` on both Linux and macOS

**Verification:**
- `go test ./internal/proxy/... ./internal/tui/tabs/...` passes

## System-Wide Impact

- **Interaction graph:** TUN toggle in settings → config save → core restart/reload → mihomo config generation → TUN device creation. No new callbacks or observers.
- **Error propagation:** If TUN fails to start on macOS, mihomo logs the error. clash4cli shows the mihomo log output. No new error paths needed.
- **State lifecycle risks:** Toggling TUN requires core restart to take effect (same as Linux). No partial-write concerns — config is written atomically before restart.
- **API surface parity:** c4cgo proxychains is explicitly unchanged (R4). TUN mode is orthogonal to proxychains — TUN intercepts at the network layer, proxychains intercepts at the library layer.
- **Unchanged invariants:** Docker redirect stays Linux-only. Root warning stays Linux-only. Linux TUN defaults remain `system` stack.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| macOS TUN may require one-time firewall approval | Document in UI hint text; mihomo handles the prompt |
| `mixed` stack may have issues on certain macOS versions | User can switch to `gvisor` or `system` via R5; fallback documented |
| Advanced config fields add maintenance surface | Fields use omitempty + defaults; minimal ongoing cost |
| mihomo macOS TUN regressions in future versions | User controls mihomo version; issue belongs to mihomo core |

## Sources & References

- **Origin document:** [docs/brainstorms/2026-04-24-macos-tun-mode-requirements.md](docs/brainstorms/2026-04-24-macos-tun-mode-requirements.md)
- mihomo TUN docs: https://wiki.metacubex.one/en/config/inbound/tun/
- Related code: `internal/proxy/config.go`, `internal/tui/tabs/settings.go`
