---
title: "feat: Tab number shortcuts (1-4)"
type: feat
status: active
date: 2026-04-26
origin: docs/brainstorms/2026-04-26-tab-number-shortcuts-requirements.md
---

# feat: Tab number shortcuts (1-4)

## Overview

Add keyboard shortcuts 1-4 to jump directly to the corresponding tab, and display number prefixes on the tab bar so users discover the shortcuts visually.

## Problem Frame

Tab switching currently requires cycling with Tab key or mouse click. For a 4-tab UI this is inefficient — users must press Tab repeatedly or reach for the mouse.

## Requirements Trace

- R1. 按 1-4 数字键直接跳转标签页
- R2. 子组件激活时数字键不触发切换
- R3. 标签栏显示数字前缀（如「1 Slots」）

## Scope Boundaries

- 不改变 Tab 键循环行为
- 不改变鼠标点击行为

## Key Technical Decisions

- **数字前缀格式**: `N Name`（数字+空格+标签名），与现有标签名渲染无缝拼接。不用冒号避免视觉噪声。
- **blockGlobal 复用**: 数字键切换与 Tab 键一样受 `blockGlobal` 保护，确保输入框/overlay 激活时不误切换。
- **tabHitTest 同步更新**: `tabHitTest` 构建的标签名需要与 `renderTabBar` 一致，否则点击坐标会偏移。

## Implementation Units

- [ ] **Unit 1: Add number key handlers in global key switch**

**Goal:** 按数字键 1-4 跳转到对应标签页

**Requirements:** R1, R2

**Dependencies:** None

**Files:**
- Modify: `internal/tui/tui.go`

**Approach:**
在 `Update` 函数的全局按键 switch（约 line 454）中，`case key.Matches(msg, key.NewBinding(key.WithKeys("tab")))` 之后添加数字键 cases。每个 case 调用 `m.setTab(TabSlots)` / `m.setTab(TabSubscriptions)` 等。所有 case 受 `blockGlobal` 保护。

**Patterns to follow:**
- 现有 Tab 键处理（line 461-465）

**Test scenarios:**
- Happy path: 按 "1" 切到 TabSlots，按 "2" 切到 TabSubscriptions，按 "3" 切到 TabSettings，按 "4" 切到 TabLogs
- Edge case: blockGlobal=true 时按数字键不切换标签
- Edge case: 在非活动标签按数字键正确跳转

**Verification:**
- `make test` 通过
- 手动启动 TUI，按 1-4 验证跳转

---

- [ ] **Unit 2: Add number prefixes to tab bar rendering**

**Goal:** 标签栏每个标签名前显示数字前缀

**Requirements:** R3

**Dependencies:** None

**Files:**
- Modify: `internal/tui/tui.go`

**Approach:**
在 `renderTabBar()` 函数中构建 `tabNames` 数组时（约 line 798-803），将每个标签名前加数字前缀，如 `fmt.Sprintf("%d %s", i+1, name)`。同时更新 `tabHitTest()` 中的相同逻辑（约 line 910-917），保持二者一致。

**Patterns to follow:**
- 现有 tabNames 构建方式

**Test scenarios:**
- Happy path: 标签栏显示「1 Slots 2 Subscriptions 3 Settings 4 Logs」
- Edge case: 窄屏滚动模式下数字前缀仍正确显示
- Edge case: tabHitTest 在数字前缀下仍正确计算点击坐标

**Verification:**
- `make test` 通过
- 手动验证标签栏显示和鼠标点击仍正确

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| tabHitTest 偏移 | renderTabBar 和 tabHitTest 共用同一标签名构建逻辑 |

## Sources & References

- **Origin document:** [2026-04-26-tab-number-shortcuts-requirements.md](docs/brainstorms/2026-04-26-tab-number-shortcuts-requirements.md)
- Related code: `internal/tui/tui.go` (Update ~line 454, renderTabBar ~line 796, tabHitTest ~line 908)
