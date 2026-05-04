---
title: Node browser table layout overflow when subscriptions have many nodes
date: 2026-05-04
category: docs/solutions/ui-bugs/
module: internal/tui
problem_type: ui_bug
component: tooling
symptoms:
  - Opening the proxy picker in the Slots panel caused the top tab bar and bottom status bar to "fly out" of the visible terminal area
  - The subscription sub-tab bar disappeared when selecting a subscription with many nodes
  - NodeBrowser.View() output was 1 line taller than the terminal height, causing visual overflow
root_cause: logic_error
resolution_type: code_fix
severity: medium
tags: [tui, bubbletea, layout, overflow, nodebrowser, lipgloss, reserved-lines]
related_components:
  - internal/tui/components
  - internal/tui/tabs
---

# Node browser table layout overflow when subscriptions have many nodes

## Problem

When opening the node selector (proxy picker) in the Slots panel for subscriptions with many nodes, the TUI overflows the terminal height. The top tab bar and bottom status bar "fly out" — pushed above the visible viewport by excess content lines. The `reservedLines` constant in `NodeBrowser.updateTableSize()` was off by one (6 instead of 7), causing the rendered output to exceed the available height.

## Symptoms

- Opening the node selector in Slots panel with a subscription that has many nodes causes the top tab bar and bottom status bar to scroll off screen
- The subscription sub-tab bar disappears when selecting a subscription with many nodes
- UI elements appear to "fly out" of the visible terminal area
- Most noticeable on smaller terminal windows or with subscriptions containing dozens of nodes

## What Didn't Work

Attempting to fix the overflow by adjusting `MaxHeight` constraints alone was ineffective. The root cause was twofold: (1) the `reservedLines` constant was numerically wrong, and (2) `SlotsTab` entered fullscreen mode (`IsFullScreen()` returning `true`) when the node browser was open, bypassing the normal content clipping path. Without fixing both issues, no amount of height tuning would prevent overflow.

## Solution

A two-pronged fix across three files:

**Fix 1 — `internal/tui/components/nodebrowser.go:544`**: Corrected `reservedLines` from 6 to 7.

```go
// Before (bug):
const reservedLines = 6

// After (fix):
const reservedLines = 7
```

**Fix 2 — `internal/tui/tabs/slots.go:139`**: Removed the `IsFullScreen()` method from `SlotsTab`. Previously this returned `true` for all sub-views (proxy picker, port picker, upstream picker), hiding the tab bar and status bar. After removal, chrome is always visible.

```go
// Removed entirely:
func (t *SlotsTab) IsFullScreen() bool {
    return t != nil && t.view != slotsViewList
}
```

**Fix 3 — `internal/tui/tui.go`**: Cleaned up ~20 lines of dead fullscreen infrastructure:
- Mouse guard checking `IsFullScreen()` (was unreachable)
- `wasFullScreen` transition tracking in `case TabSlots`
- Dead fullscreen `View()` path
- `IsFullScreen` conditional in `updateSizes()`
- `isActiveTabFullScreen()` method

## Why This Works

`NodeBrowser.View()` computes table height as `b.height - reservedLines`. The actual non-table element overhead is:

| Element | Lines |
|---------|-------|
| Title | 1 |
| SubTabs (TabBar with `BorderBottom(true)`) | 2 |
| Table header | 1 |
| Table separator | 1 |
| Help bar | 1 |
| Table scroll indicator (when rows > height) | 1 |
| **Total** | **7** |

Before the fix, `reservedLines = 6` produced total output `b.height + 1`, exceeding terminal height by exactly one line. In Bubble Tea's alternate screen buffer, excess lines push earlier content upward off screen — the exact "飞出去" behavior described.

After correcting to `reservedLines = 7`, total output equals `b.height`, fitting within terminal boundaries.

Removing fullscreen mode ensures the normal layout path always applies: tab bar + content (clipped via `MaxHeight(contentHeight)`) + status bar. No sub-view can overflow beyond its allocated region.

## Prevention

- When adding new UI elements (lines, borders, indicators) to a Bubble Tea `View()` method, update the corresponding height budget constant. Document the line budget explicitly in a comment listing each element.
- Prefer keeping chrome (tab bar, status bar) always visible rather than entering fullscreen mode. This preserves spatial context and avoids the class of bugs where fullscreen content has no clipping boundary.
- Consider adding a regression test: set a small terminal height, open the node browser with many mock nodes, and assert `len(strings.Split(View(), "\n")) <= height`.

## Related

- [Slot card border overflow — lipgloss Width() excludes border](slot-card-border-overflow-2026-04-08.md) — related TUI overflow bug, same category but different dimension (width vs height)
