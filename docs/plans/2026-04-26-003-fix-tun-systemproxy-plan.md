---
title: "fix: TUN auto-restart and System Proxy toggle"
type: fix
status: completed
date: 2026-04-26
origin: docs/brainstorms/2026-04-26-tun-systemproxy-fix-requirements.md
---

# TUN 自动重启 & System Proxy 开关修复

## Overview

修复两个 Bug：(1) System Proxy 开关在 UI 中切换后 config 不更新，且 `ApplySystemProxy()` 从未被实际调用；(2) TUN 模式开关保存后不会自动重启 core，导致 mihomo 没有加载新的 TUN 配置。

## Problem Frame

System Proxy 的开关是 macOS 用户最直接的全局代理方式，但当前 toggle 既不保存配置也不实际执行 `networksetup` 命令。TUN 模式虽然在之前的 feat plan 中完成了 macOS 支持和配置保存，但切换后需要用户手动重启 core 才能生效，导致用户以为 TUN 不工作。

## Requirements Trace

- R1. `system_proxy_enabled` 切换时正确更新 Config 并持久化到磁盘
- R2. 启用时调用 `netctl.ApplySystemProxy()` 设置 macOS 网络代理
- R3. 关闭时调用 `netctl.RestoreSystemProxy()` 恢复原始状态
- R4. 退出 c4c 时自动恢复 System Proxy 状态
- R5. 每次 Apply 前先 `SnapshotSystemProxy()` 保存当前状态
- R6. TUN 开关保存后自动触发 core 重启 (`RestartAllSlotsMsg`)

## Scope Boundaries

- macOS 11+ 仅 darwin 平台支持 System Proxy（已有 build tag 保护）
- 不修改 mihomo 内核本身的行为
- TUN 权限（macOS 需 root/sudo）不在此范围（已知限制）
- 不修改 TUN 子页面参数（stack/MTU/device）的保存逻辑，只修改主开关

## Context & Research

### Relevant Code and Patterns

- `internal/tui/tabs/settings.go` — `saveSettings()` (L2826), `portChangedSaveMsg` 模式（L59, L885, L2979）
- `internal/tui/tabs/settings.go` — `systemProxyStatus` 字段（L127），`saveSystemProxySettings()`（L1249）
- `internal/tui/tui.go` — `RestartAllSlotsMsg` 处理（L568），`restartAllSlots()` 实现
- `internal/netctl/system_proxy_darwin.go` — `ApplySystemProxy()`, `RestoreSystemProxy()`, `SnapshotSystemProxy()`
- `internal/netctl/system_proxy_other.go` — 非 macOS stub
- `internal/netctl/system_proxy_snapshot.go` — `SaveSystemProxySnapshot()`, `LoadSystemProxySnapshot()`, `ClearSystemProxySnapshot()`
- `internal/app/app.go` — `App.Cleanup()`（L189），`App` struct（L25）
- `cmd/c4c/main.go` — 信号处理和 Cleanup 调用

### Institutional Learnings

- 已有 TUN macOS 模式完成 plan（`docs/plans/2026-04-24-001-feat-macos-tun-mode-plan.md`），TUN generation 和 TUI 已就绪
- `portChangedSaveMsg` 是触发重启的标准模式：定义新 msg 类型 → saveSettings 中对比旧值 → 返回新 msg → SettingsTab.Update 中 batch 处理 Flashing + RestartAllSlotsMsg

### External References

- `portChangedSaveMsg` 模式直接复用

## Key Technical Decisions

- **System Proxy snapshot 存储位置**：使用内存存储（`SettingsTab.systemProxySnapshot` 字段），不写入磁盘文件。因为 session 结束时 snapshot 无需持久化——退出时只需从内存读取。如果用户在同一 session 内多次 toggle，每次 toggle 前都重新 snapshot 覆盖。
- **退出时恢复机制**：在 `App.Cleanup()` 中检查 `App.systemProxyApplied` bool 字段和 `systemProxySnapshot`，如果两者都存在则调用 `RestoreSystemProxy()`。Snapshot 通过 `App` struct 跨层传递，而非依赖文件读写。
- **TUN 重启消息**：复用 `portChangedSaveMsg` 模式，新建 `tunChangedSaveMsg` 类型。
- **System Proxy 方向检测**：复用 `portChangedSaveMsg` 的闭包变量模式——在 `saveSettings()` 闭包内用 `prevEnabled := t.cfg.Proxy.SystemProxy.Enabled` 捕获 toggle 前的状态，判断是 Off→On 还是 On→Off。

## Open Questions

### Resolved During Planning

- **Snapshot 是否写磁盘？** 否。内存存储即可。退出时通过 App 字段传递。
- **TUN 重启用哪个消息？** `RestartAllSlotsMsg`（与 port change 一致），因为 TUN 是全局设置，影响所有 slot。
- **非 macOS 平台 stubs**：改为返回 nil 而非 error。
- **`systemProxyEnabled` saveSettings 处理流程**：与 `tun_enabled` 类似，在 switch 中添加 case，但额外调用 `SnapshotSystemProxy + ApplySystemProxy`（启用）或 `RestoreSystemProxy`（关闭）。

### Deferred to Implementation

- 具体的闭包结构：`saveSettings()` 内部如何传递 snapshot 到 SettingsTab —— 可以用闭包引用 SettingsTab 的字段
- TUN 参数子页面保存后是否也触发热重启：当前 plan 暂不包含（R6 只覆盖主开关），可在未来迭代

## Implementation Units

- [ ] **Unit 1: Make non-macOS system_proxy stubs safe**

**Goal:** `RestoreSystemProxy` 和 `SnapshotSystemProxy` 在非 macOS 平台返回 nil 而非 error，避免跨平台清理时崩溃。

**Requirements:** R2, R3, R4 (dependency)

**Dependencies:** None

**Files:**
- Modify: `internal/netctl/system_proxy_other.go`

**Approach:**
- 将 `SnapshotSystemProxy` 的 stub 从返回 error 改为返回 nil, nil
- 将 `RestoreSystemProxy` 的 stub 从返回 error 改为返回 nil
- 调用方仍应使用 `runtime.GOOS == "darwin"` 保护，但 stub 本身不再阻塞

**Patterns to follow:**
- 现有 stub 模式，仅修改返回值

**Test scenarios:**
- Happy path: 在非 darwin 系统上调用 `SnapshotSystemProxy()` 返回 nil, nil
- Happy path: 在非 darwin 系统上调用 `RestoreSystemProxy(nil)` 返回 nil

**Verification:**
- `go test ./internal/netctl/...` passes (tests that exist are darwin-only)

---

- [ ] **Unit 2: Implement System Proxy toggle with snapshot and apply**

**Goal:** 当用户在 Settings 主页面切换 System Proxy 开关时，正确保存 config 并实际执行 macOS 代理设置/恢复。

**Requirements:** R1, R2, R3, R5

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/tui/tabs/settings.go`
- Test: `internal/tui/tabs/settings_test.go`

**Approach:**
1. 在 `SettingsTab` struct 添加字段：
   - `systemProxySnapshot *netctl.SystemProxySnapshot` — 存储 apply 前的 snapshot

2. 在 `saveSettings()` 的 switch 中添加 `case "system_proxy_enabled"`：
   - 读取 `field.options[field.selectedIdx].value` → 设置 `t.cfg.Proxy.SystemProxy.Enabled`
   - 调用 `t.cfg.Save()`（已有）
   - 闭包内捕获 `prevEnabled` 判断方向：
     - **如果从 Off → On**：调用 `SnapshotSystemProxy()` → 存到 `t.systemProxySnapshot` → 调用 `ApplySystemProxy(t.cfg.Proxy.SystemProxy)`
     - **如果从 On → Off**：取 `t.systemProxySnapshot` → 调用 `RestoreSystemProxy(snapshot)` → 清空 `t.systemProxySnapshot`

3. 所有调用需要 `runtime.GOOS == "darwin"` 保护

**Patterns to follow:**
- `portChangedSaveMsg` 模式：保存前的状态由调用方（`SettingsTab.Update` 或 `saveSettings` 内部比较）追踪
- `tun_enabled` 的保存模式（L2865-2874）：读取 select 值 → 设置 bool → 保存 config

**Test scenarios:**
- Happy path: 在 macOS 上 toggle System Proxy On → `ApplySystemProxy` 被调用，config 中 `SystemProxy.Enabled = true`
- Happy path: 在 macOS 上 toggle System Proxy Off → `RestoreSystemProxy` 被调用，config 中 `SystemProxy.Enabled = false`
- Edge case: 在非 macOS 上 toggle → config 保存但不调用 `ApplySystemProxy`
- Edge case: 再次 toggle On → 再次 `SnapshotSystemProxy`（覆盖旧 snapshot）
- Integration: config 保存后重新加载，验证 `SystemProxy.Enabled` 持久化

**Verification:**
- `go test ./internal/tui/tabs/...` passes
- 手动验证：macOS 上 toggle On → `networksetup -getwebproxy` 显示代理地址
- 手动验证：macOS 上 toggle Off → 代理恢复到之前的状态

---

- [ ] **Unit 3: Add TUN auto-restart on toggle**

**Goal:** 当用户在 Settings 主页面切换 TUN 开关时，保存 config 后自动重启 core。

**Requirements:** R6

**Dependencies:** None (可以独立于 System Proxy 实现)

**Files:**
- Modify: `internal/tui/tabs/settings.go`
- Test: `internal/tui/tabs/settings_test.go`

**Approach:**
1. 新增 message 类型：
   ```go
   type tunChangedSaveMsg struct {
       notice string
   }
   ```
   放在已有 msg 类型的旁边（~L47-61）

2. 在 `saveSettings()` 中，`tun_enabled` 的 case 之后添加状态对比：
   ```go
   // 在 saveSettings() 中已经捕获了 prevTun := t.cfg.Proxy.TunEnabled
   // 在函数末尾，检查 TUN 是否变化
   ```
   在 `saveSettings()` 的返回值逻辑中增加：
   - 如果 `prevTun != t.cfg.Proxy.TunEnabled`，返回 `tunChangedSaveMsg{notice: notice}` 而非普通 `SettingsSavedMsg`

3. 在 `SettingsTab.Update()` 中添加 `tunChangedSaveMsg` 处理：
   ```go
   case tunChangedSaveMsg:
       msg := "已保存 TUN 设置，正在重启 core..."
       return t, tea.Batch(
           t.startFlash(3*time.Second),
           func() tea.Msg { return RestartAllSlotsMsg{} },
       )
   ```

**Patterns to follow:**
- `portChangedSaveMsg` 的完整模式：
  - 定义 msg 类型（L59-61）
  - saveSettings 中对比新旧值（L2830, L2979-2980）
  - SettingsTab.Update 中处理（L885-895）

**Test scenarios:**
- Happy path: TUN 从 Off 切换到 On → `tunChangedSaveMsg` 被返回，`RestartAllSlotsMsg` 在 batch 中
- Happy path: TUN 从 On 切换到 Off → `tunChangedSaveMsg` 被返回
- Edge case: TUN 保持不变（On→On 或 Off→Off）→ 正常 `SettingsSavedMsg`，无重启
- Integration: `tunChangedSaveMsg` → SettingsTab.Update → restartAllSlots 被触发

**Verification:**
- `go test ./internal/tui/tabs/...` passes
- 手动验证：toggle TUN → core 自动重启 → TUN 设备创建

---

- [ ] **Unit 4: Add system proxy exit cleanup**

**Goal:** 当 c4c 退出时，如果本 session 中启用了 System Proxy，自动调用 `RestoreSystemProxy()` 恢复系统代理状态。

**Requirements:** R4

**Dependencies:** Unit 2

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/tui/tui.go`
- Modify: `cmd/c4c/main.go` — 可能不需要，取决于实现方式

**Approach:**
选项 A（推荐—通过 App struct 字段）：
1. 在 `App` struct 添加字段：
   ```go
   systemProxySnapshot *netctl.SystemProxySnapshot
   systemProxyApplied  bool
   ```
2. 在 `App.Cleanup()` 中添加：
   ```go
   if runtime.GOOS == "darwin" && a.systemProxyApplied && a.systemProxySnapshot != nil {
       _ = netctl.RestoreSystemProxy(a.systemProxySnapshot)
       a.systemProxyApplied = false
       a.systemProxySnapshot = nil
   }
   ```
3. 在 `tui.Model` 或 `SettingsTab` 中，当 System Proxy 被 apply 时，将 snapshot 和 applied 状态写回 `App`。

选项 B（通过消息传递到 Cleanup）：
- 在 `tui.go` 的 `Update()` 中拦截 `tea.QuitMsg`，在退出前调用清理。
- 但信号处理走的是不同的路径（信号 → `os.Exit`），会绕过 Bubble Tea 的退出流程。

推荐选项 A，因为 `App.Cleanup()` 是唯一的、可靠的退出清理入口。

**具体实现：**
1. 在 `App` 上添加导出方法 `SetSystemProxySnapshot(snap *netctl.SystemProxySnapshot)` 和 `SetSystemProxyApplied(v bool)`
2. 在 `tui.Model` 添加 `app *App` 引用（或通过 `SetAppRef` 方法注入）
3. 在 `SettingsTab` 的 `saveSettings()` 中，apply System Proxy 后调用 `t.slotMgr` 或通过消息传递到 `tui.Model`，最终调用 `app.SetSystemProxySnapshot()`

**或者更简单的方式：**
- 在 `App` 上添加 snapshot 和应用状态字段
- 在 `app.New()` 中将这些字段的指针/chan 传给 Model → SettingsTab
- SettingsTab 在 apply/restore System Proxy 时直接写入 `App` 的字段

**Patterns to follow:**
- App struct 已有 `startupWarn string` 等运行时状态字段
- `Cleanup()` 模式的唯一入口（L189-195）

**Test scenarios:**
- Happy path: System Proxy 启用后退出 → `RestoreSystemProxy` 被调用
- Edge case: System Proxy 从未启用 → Cleanup 不调用 `RestoreSystemProxy`
- Edge case: System Proxy 启用后又关闭 → Cleanup 不调用 `RestoreSystemProxy`（snapshot 已清空）
- Edge case: 信号处理路径（`Ctrl+C`）→ Cleanup 正常执行

**Verification:**
- 手动验证：macOS 上启用 System Proxy → Ctrl+C 退出 → `networksetup -getwebproxy` 显示代理已恢复
- 手动验证：不启用 System Proxy → 正常退出 → 无报错

---

- [ ] **Unit 5: Add tests**

**Goal:** 为新的行为添加测试覆盖。

**Requirements:** R1-R6

**Dependencies:** Units 2, 3, 4

**Files:**
- Create or Modify: `internal/tui/tabs/settings_test.go`
- Modify: `internal/netctl/system_proxy_darwin_test.go`

**Approach:**

**settings_test.go:**
- 添加 `TestSettingsTab_SaveSettingsPersistsSystemProxyEnabled` — 验证系统代理保存后 `cfg.Proxy.SystemProxy.Enabled` 正确设置（参考现有 TUN 测试模式 `TestSettingsTab_SaveSettingsPersistsTunAdvancedFields`）
- 添加 `TestSettingsTab_SaveSettingsTriggersTunRestart` — 验证 TUN 切换返回 `tunChangedSaveMsg`（使用 `saveSettings()` 返回的 `tea.Cmd` 模式）
- 添加 `TestSettingsTab_SystemProxyToggleRestore` — 模拟 toggle on/off 验证 Apply/Restore 被调用

**netctl/system_proxy_darwin_test.go:**
- 添加 `TestSnapshotSystemProxy` — 验证 `SnapshotSystemProxy()` 正确执行 `networksetup` 命令

**Patterns to follow:**
- 现有测试模式：`TestSettingsTab_SaveSettingsPersistsTunAdvancedFields` (L171)
- `runNetworksetup` 变量的 mock 注入模式（`system_proxy_darwin_test.go`）

**Test scenarios:**
- Happy path: `saveSettings()` 返回的 msg 类型正确
- Happy path: config 的 `SystemProxy.Enabled` 正确保存和加载
- Edge case: 在非 darwin 平台上的 stubs 返回 nil

**Verification:**
- `go test ./internal/tui/tabs/... ./internal/netctl/...` passes

## System-Wide Impact

- **Interaction graph:** System Proxy toggle → config save → `ApplySystemProxy()` (networksetup) → macOS 系统代理变化。TUN toggle → config save → `tunChangedSaveMsg` → `RestartAllSlotsMsg` → mihomo 重启 → 新配置生效。
- **Error propagation:** `ApplySystemProxy` 和 `RestoreSystemProxy` 的错误在 saveSettings 中通过日志记录，不阻塞 config 保存（不会返回 error 到 TUI）。
- **State lifecycle risks:** System Proxy snapshot 是 session 级别的。如果用户在 session 中多次 toggle，每次 toggle on 都重新 snapshot。退出时根据 latest snapshot 恢复。
- **API surface parity:** c4cgo proxychains 不变。Linux 和 Windows 平台不受影响。
- **Unchanged invariants:** TUN 参数子页面（stack/MTU/device）的保存逻辑不变。Docker redirect 逻辑不变。

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `ApplySystemProxy` 或 `RestoreSystemProxy` 执行 `networksetup` 可能失败（网络服务名称变化等） | 错误被捕获并日志记录，不影响 config 保存；用户可在 TUI 中看到错误消息 |
| 信号处理路径（`os.Signal` → `os.Exit`）中 Cleanup 可能因竞态条件不完整 | Cleanup 中只做关键操作（恢复系统代理），日志和 lock release 可接受失败 |
| System Proxy snapshot 在多网络接口环境下可能不完整 | `SnapshotSystemProxy` 已经遍历所有 `networksetup -listallnetworkservices` 的服务 |

## Sources & References

- **Origin document:** `docs/brainstorms/2026-04-26-tun-systemproxy-fix-requirements.md`
- Related plan: `docs/plans/2026-04-24-001-feat-macos-tun-mode-plan.md` (existing macOS TUN support)
- Pattern: `portChangedSaveMsg` in `internal/tui/tabs/settings.go`
- Related code: `internal/netctl/system_proxy_darwin.go`
