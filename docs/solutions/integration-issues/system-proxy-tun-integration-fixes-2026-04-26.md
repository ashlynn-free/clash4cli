---
title: Fix system proxy and TUN mode integration issues across macOS and Linux
date: 2026-04-26
category: integration-issues
module: settings
problem_type: integration_issue
component: tooling
severity: high
symptoms:
  - System Proxy toggle saves config but never calls ApplySystemProxy() to set macOS network proxy
  - TUN mode toggle saves config but does not restart mihomo core, so TUN never activates
  - Non-macOS builds crash on exit cleanup because stub functions return errors instead of nil
root_cause: missing_workflow_step
resolution_type: code_fix
tags:
  - system-proxy
  - tun
  - settings-toggle
  - core-restart
---

# Fix system proxy and TUN mode integration issues across macOS and Linux

## Problem

The Settings UI had toggles for System Proxy (macOS) and TUN mode that appeared to work visually — config was saved — but never triggered the runtime side effects needed to actually apply the settings. System Proxy never called `networksetup`, TUN never restarted mihomo, and the TUN subpage required a manual Save button.

## Symptoms

- Toggling "System Proxy" On/Off updated `config.yaml` but had no effect on macOS network proxy (no `networksetup` commands executed)
- Toggling TUN On/Off correctly persisted `tun_enabled` but the running mihomo used the old config without TUN — required manual core restart
- TUN subpage fields (Stack, MTU, Device, DNS) required pressing a separate "Save" button; changes never took effect until manual restart
- Running `./c4c` (without sudo) showed TUN toggle as enabled even though TUN requires root on macOS
- Exit cleanup on Linux would crash trying to restore system proxy

## What Didn't Work

- The existing `system_proxy_enabled` case in `saveSettings()` was completely missing — the toggle updated the UI appearance but never wrote to `Config.Proxy.SystemProxy.Enabled`
- `prevTun` was captured in `saveSettings()` but only used for a Linux root-notice message, never to trigger a restart
- `manualApply: true` on TUN subpage fields and a Save button created unnecessary friction
- The `os.Geteuid() != 0` root check was macOS-only disabled, leaving TUN toggle enabled for non-root macOS users
- `SnapshotSystemProxy()` and `RestoreSystemProxy()` stubs in `system_proxy_other.go` returned errors, preventing safe cross-platform cleanup

## Solution

### 1. System Proxy toggle lifecycle

Added `system_proxy_enabled` case to `saveSettings()` switch. On enable: `SnapshotSystemProxy()` → `SaveSystemProxySnapshot()` (disk) → `ApplySystemProxy()`. On disable: `LoadSystemProxySnapshot()` → `RestoreSystemProxy()`. Added exit cleanup in `App.Cleanup()` using the disk-based snapshot.

### 2. TUN auto-restart on toggle (core fix)

Added `tunChangedSaveMsg` following the existing `portChangedSaveMsg` pattern:

```go
type tunChangedSaveMsg struct {
    notice string
}

// In saveSettings(), after config save:
if prevTun != t.cfg.Proxy.TunEnabled {
    return tunChangedSaveMsg{notice: notice}
}
```

And in `SettingsTab.Update()`: `tunChangedSaveMsg` triggers `RestartAllSlotsMsg` which restarts the core with the new TUN config.

### 3. TUN subpage auto-save (removed Save button)

Removed the Save button. Modified `updateTunSettings()` so every navigation action auto-saves:

```go
// Left/right on select fields:
case key.WithKeys("left", "h"):
    f.selectedIdx--
    return t, t.saveTunSettings()  // instead of return t, nil

// Up/down leaving input fields:
case key.WithKeys("up", "k"):
    cmd := t.saveTunSettings()
    t.tunFocusPrev()
    return t, cmd
```

`saveTunSettings()` now returns `tunChangedSaveMsg` which triggers core restart.

### 4. Root permission guard

```go
tunDisabled := (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || os.Geteuid() != 0
```

Added macOS to the startup root warning (was Linux-only).

### 5. Non-macOS stub safety

```go
func SnapshotSystemProxy() (*SystemProxySnapshot, error) {
    return nil, nil  // was: return nil, fmt.Errorf("macOS only")
}
func RestoreSystemProxy(snapshot *SystemProxySnapshot) error {
    return nil  // was: return fmt.Errorf("macOS only")
}
```

## Why This Works

The fundamental issue was that `saveSettings()` persisted config to disk but never signaled the runtime layer that certain changes require active side effects — restarting the mihomo core (TUN) or executing OS-level commands (System Proxy). By introducing typed message returns (`tunChangedSaveMsg`) and matching handlers in the UI update loop, config persistence and runtime effects are now properly coupled. The `portChangedSaveMsg` pattern was already the established convention for this coupling; the TUN and System Proxy toggles simply weren't following it.

## Prevention

- Any new config toggle that requires a runtime side effect (restart, OS command, network reconfiguration) must follow the `portChangedSaveMsg`/`tunChangedSaveMsg` pattern: add a message type, return it from `saveSettings()` when the value changes, handle it in `SettingsTab.Update()`
- Avoid `manualApply: true` on TUI settings fields unless there is a strong reason (batch validation across interdependent fields) — prefer immediate application for single-field toggles
- Always add `os.Geteuid() != 0` permission checks alongside feature flags for OS-level privileged features (TUN, VPN, firewall)
- Ensure stub functions on unsupported platforms return nil rather than errors to prevent crashes in shared cleanup paths

## Related Issues

- Related plan: `docs/plans/2026-04-26-003-fix-tun-systemproxy-plan.md`
- Precursor plan: `docs/plans/2026-04-24-001-feat-macos-tun-mode-plan.md`
- Affected files: `internal/tui/tabs/settings.go`, `internal/app/app.go`, `internal/netctl/system_proxy_other.go`
