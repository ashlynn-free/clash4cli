---
title: Slot card border overflow — lipgloss Width() excludes border
date: 2026-04-08
category: ui-bugs
module: internal/tui/tabs
problem_type: ui_bug
component: frontend_stimulus
symptoms:
  - Slot cards rendered 2 characters wider than their container
  - Left indicator area overflowed ("flew out") beyond terminal width
  - Overflow visible at all terminal widths (60, 80, 120 columns)
root_cause: logic_error
resolution_type: code_fix
severity: medium
tags: [lipgloss, border, width, card-rendering, tui, bubbletea, overflow]
---

# Slot card border overflow — lipgloss Width() excludes border

## Problem

`renderSlotCard` in `internal/tui/tabs/slots.go` calculated `boxWidth = cardWidth - indicatorPad` without subtracting the lipgloss border width (2 chars). Because `lipgloss.Width(n)` sets content width *excluding* the border, each card rendered 2 characters wider than its allocated space, causing the indicator area to overflow beyond the terminal boundary.

## Symptoms

- Slot card left indicator area visually overflowed ("flew out" / 飞出去了) past the expected boundary
- Cards rendered at `cardWidth + 2` instead of `cardWidth` across all terminal widths
- Both selected (with `▶` indicator) and unselected cards were affected

## What Didn't Work

- Existing tests only covered card **height** (`TestSlotsTab_ViewListNeverExceedsHeight`), with zero coverage for rendered **width**
- The original magic number `4` in `innerWidth := boxWidth - 4` merged border(2) + padding(2) into one value, making the arithmetic error hard to spot during code review
- The assumption that `cardWidth` was the total rendered width was never validated against actual lipgloss output

## Solution

In `internal/tui/tabs/slots.go`, `renderSlotCard` function:

**Before:**
```go
const indicatorPad = 3
boxWidth := cardWidth - indicatorPad       // missing border subtraction
innerWidth := boxWidth - 4                 // magic number hides the error
```

**After:**
```go
const indicatorPad = 3 // "▶  " or "   " — 3 chars
const borderW = 2      // left+right border chars
const padW = 2         // left+right padding chars

boxWidth := cardWidth - indicatorPad - borderW // content width passed to lipgloss (border added on top)
innerWidth := boxWidth - padW                   // text area after padding removed
```

New test in `internal/tui/tabs/slots_test.go`:

```go
func TestSlotsTab_SingleCardNeverExceedsWidth(t *testing.T) {
    t.Setenv("CLASH4CLI_HOME", t.TempDir())
    config.EnsureAllDirs()

    for _, width := range []int{60, 80, 120} {
        for _, selected := range []bool{false, true} {
            tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
            tab.SetSize(width, 24)
            card := tab.renderSlotCard(0, width, selected)
            for i, line := range strings.Split(card, "\n") {
                if dw := lipgloss.Width(line); dw > width {
                    t.Errorf("width=%d selected=%v card line %d: display width %d exceeds %d",
                        width, selected, i, dw, width)
                }
            }
        }
    }
}
```

## Why This Works

lipgloss `Width(n)` sets the **content** width, which includes padding but **excludes** the border. A style with `Width(77).Padding(0,1).BorderStyle(RoundedBorder())` renders at **79** display width (77 content + 2 border). The original code subtracted only `indicatorPad` (3) from `cardWidth` to get `boxWidth`, then passed that to lipgloss `Width()`. The border's 2 extra chars went unaccounted for, making the total `3 + (cardWidth - 3 + 2) = cardWidth + 2`. The fix explicitly subtracts `borderW` from `boxWidth`, so the rendered output fits exactly: `3 + (cardWidth - 5 + 2) = cardWidth`.

## Prevention

- **Rendered width assertions**: For any lipgloss component, write tests using `lipgloss.Width()` to measure actual output width. Assert it does not exceed the allocated container width. Cover multiple terminal widths (narrow 60, standard 80, wide 120) and component states (selected/unselected).
- **Named constants over magic numbers**: Border width, padding, and margin must be named constants. The original `boxWidth - 4` made it impossible to see that 2 came from border and 2 from padding.
- **lipgloss width semantics**: `lipgloss.Style.Width(n)` = content width (includes padding, excludes border). Actual rendered width = `n + border`. When calculating available space for child components, subtract border and padding from the outer allocation in separate steps.

## Related Issues

- None. This is a novel issue with no prior documentation or GitHub issue tracking.
