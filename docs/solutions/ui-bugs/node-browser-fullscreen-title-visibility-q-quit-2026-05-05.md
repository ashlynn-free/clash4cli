---
title: "Node Browser Fullscreen Mode, Title Visibility Fix, and Always-Available Quit"
date: 2026-05-05
category: ui-bugs
module: internal/tui
problem_type: ui_bug
component: tooling
severity: medium
symptoms:
  - "Node browser rendered within tab bar + status bar layout, wasting vertical space"
  - "\"Select Proxy\" title invisible on light terminal themes (pink-on-white)"
  - "Cannot quit with q key when modal/overlay views are active"
root_cause: config_error
resolution_type: code_fix
tags:
  - node-browser
  - fullscreen
  - title-visibility
  - keybindings
  - quit
  - tui
related_components:
  - internal/tui/components
  - internal/tui/tabs
  - internal/tui/styles
---

# Node Browser Fullscreen Mode, Title Visibility Fix, and Always-Available Quit

## Problem

The TUI had three usability issues: (1) the proxy node picker was cramped inside the
hamburger layout (tab bar + content + status bar) rather than using the full terminal,
(2) the "Select Proxy" title was invisible on light terminal themes due to
foreground-only pink styling (`#EC4899`), and (3) the `q` quit key was blocked when
modal overlays were active, making it impossible to exit without first backing out
of the overlay.

## Symptoms

- Proxy node picker displayed with tab bar and status bar still visible, wasting
  2 lines of vertical space and reducing the number of visible nodes.
- "Select Proxy" title text appeared blank or nearly invisible on terminals with
  light backgrounds — pink foreground without background on white terminal is
  illegible.
- Pressing `q` while in fullscreen proxy picker had no effect; user had to press
  `Esc` first, then `q`.

## What Didn't Work

- **Original fullscreen attempt** (removed in
  `docs/solutions/ui-bugs/nodebrowser-table-overflow-many-nodes-2026-05-04.md`):
  a bare `m.fullscreen` field on the model and manual `updateSizes()` calls only
  for the slots tab. The approach was fragile and was rolled back.
- **`styles.Title` foreground-only styling**: Adding a dark text color would fail
  on dark terminals. The real fix needed a background to guarantee contrast
  regardless of terminal theme.
- **Same guard for `q` and `ctrl+c`**: Treating both keys identically under the
  same `blockGlobal` guard made `q` unusable in overlays, even though `q` is a
  universal TUI convention for quitting.

## Solution

### Fullscreen Mode

An `IsFullScreen()` method on tabs that opt into owning the entire terminal area.
The main model checks this before the hamburger layout and renders the tab content
at full terminal height.

**`internal/tui/tabs/slots.go`** — signal fullscreen when proxy picker is open:

```go
func (t *SlotsTab) IsFullScreen() bool {
	return t != nil && t.view == slotsViewProxyPicker
}
```

**`internal/tui/tabs/subscriptions.go`** — signal fullscreen when node browser is open:

```go
func (t *SubscriptionsTab) IsFullScreen() bool {
	return t != nil && t.showNodeBrowser
}
```

**`internal/tui/tui.go`** — helper to check the active tab:

```go
func (m *Model) isActiveTabFullScreen() bool {
	return (m.activeTab == TabSlots && m.slotsTab != nil && m.slotsTab.IsFullScreen()) ||
		(m.activeTab == TabSubscriptions && m.subsTab != nil && m.subsTab.IsFullScreen())
}
```

In `View()`, before the hamburger layout:

```go
if m.isActiveTabFullScreen() {
	content := m.renderActiveTab()
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		MaxHeight(m.height).
		Render(content)
}
```

In `updateSizes()`, fullscreen tabs get `m.height` instead of `contentHeight`:

```go
if m.slotsTab != nil && m.slotsTab.IsFullScreen() {
	m.slotsTab.SetSize(m.width, m.height)
} else {
	m.slotsTab.SetSize(m.width, contentHeight)
}
if m.subsTab != nil && m.subsTab.IsFullScreen() {
	m.subsTab.SetSize(m.width, m.height)
} else {
	m.subsTab.SetSize(m.width, contentHeight)
}
```

After each active tab `Update()`, detect fullscreen state changes and re-layout:

```go
case TabSlots:
	wasFullScreen := m.slotsTab.IsFullScreen()
	m.slotsTab, cmd = m.slotsTab.Update(msg)
	if m.slotsTab.IsFullScreen() != wasFullScreen {
		m.updateSizes()
	}
```

Also blocks mouse-based tab-switching when fullscreen (line ~500 in `tui.go`).

### Title Visibility

Added `styles.TitleBar` in `internal/tui/styles/styles.go` — same pink foreground
as `styles.Title` but with a fixed dark background (`#374151`) and a bottom border
to visually separate it from the content below:

```go
TitleBar = lipgloss.NewStyle().
	Bold(true).
	Foreground(PrimaryColor).       // #EC4899 pink
	Background(lipgloss.Color("#374151")).
	Padding(0, 1).
	BorderStyle(lipgloss.NormalBorder()).
	BorderBottom(true).
	BorderForeground(MutedColor)    // #6B7280 gray
```

Changed `internal/tui/components/nodebrowser.go` View() from `styles.Title` to
`styles.TitleBar`:

```go
if b.title != "" {
	parts = append(parts, styles.TitleBar.Render(b.title))
}
```

Updated `reservedLines` in `updateTableSize()` from 7 to 8 to account for the
TitleBar occupying 2 lines (text + border) instead of 1.

### Always-Available Quit

Split `q` from `ctrl+c` in the global key handler so `q` always exits regardless of
overlay state, while `ctrl+c` still respects the `blockGlobal` guard:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("q"))):
	m.quitting = true
	return m, tea.Quit

case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
	if !blockGlobal {
		m.quitting = true
		return m, tea.Quit
	}
```

## Why This Works

- **Fullscreen**: By bypassing the hamburger layout at the `View()` level and
  allocating full terminal height via `updateSizes()`, the node browser gets
  maximum vertical space. The interface-based design (`IsFullScreen()`) is opt-in
  per tab and requires no changes to the layout engine — tabs that don't implement
  it continue using the standard layout.
- **Title visibility**: A foreground-only style depends entirely on the terminal
  background for contrast. Adding a fixed dark background (`#374151`) to the title
  bar decouples readability from the terminal theme. The bottom border provides
  visual separation from the subscription sub-tab bar below.
- **Always-available quit**: `q` as an unconditional exit is a well-established TUI
  convention. Keeping `blockGlobal` for `ctrl+c` preserves safety (avoids
  interfering with copy shortcuts in other views) while ensuring the user can
  always escape.

## Prevention

- When adding new text styles for UI chrome (titles, headers, labels), always
  include a background color to guarantee contrast on both light and dark terminal
  themes.
- When introducing modal or fullscreen views, ensure a keyboard escape path (like
  `q` or `Esc`) remains available without requiring navigation.
- Use interface-based detection (`IsFullScreen()`) for opt-in view behaviors rather
  than modifying the core layout engine for each new view type.

## Related

- Previous (opposite-direction) fix that removed fullscreen:
  `docs/solutions/ui-bugs/nodebrowser-table-overflow-many-nodes-2026-05-04.md`
- Slot card border overflow: `docs/solutions/ui-bugs/slot-card-border-overflow-2026-04-08.md`
