---
title: refactor: settings UI multi-column layout
type: refactor
status: completed
date: 2026-05-05
completed: 2026-07-28
---

# refactor: settings UI multi-column layout

## Overview

Current settings tab is a ~3100-line single-column flat list with screen-filling sub-pages. Refactor to a yazi-inspired multi-column layout: left sidebar for category navigation, right panel for content. Sub-pages open within the right panel instead of covering the full view.

## Problem Frame

The settings UI has grown organically. Each new feature added fields to the bottom of a single list. Sub-pages (TUN settings, system proxy settings, upstream proxy editor) replace the entire view, hiding navigation. The code has accumulated ~5 duplicated focus/scroll/render patterns across sub-pages. This makes the UI harder to navigate as more settings are added, and harder to maintain.

## Requirements Trace

- R1. Left sidebar shows the applicable categories: 代理, 上游代理, 系统代理, TUN, Docker, 显示, 信息, 日志
- R2. Right panel shows fields for the selected category
- R3. Sub-pages (upstream editor, TUN settings, etc.) render within the right panel, sidebar stays visible
- R4. Instant-save behavior is preserved
- R5. Bottom help bar is preserved
- R6. Platform-conditional fields are dynamically hidden (not greyed out)
- R7. Existing settings functionality is preserved identically

## Scope Boundaries

- Slot detail/list sub-pages remain in right panel (not full-screen modal)
- System info remains a read-only view in the right panel
- No changes to config data model or save semantics (save behavior is preserved, iteration structure changes for categories)
- The former top-level log tab moves into the settings sidebar and shares the same live log model
- No mouse support addition (beyond what already exists)

## Context & Research

### Relevant Code and Patterns

- `internal/tui/tabs/settings.go` (~3100 lines): monolithic settings tab with all sub-pages inline
- `internal/tui/components/table.go`: existing reusable component pattern to follow for sidebar
- `internal/tui/components/statusbar.go`: simple component pattern
- `internal/tui/styles/styles.go`: shared lipgloss styles (PrimaryColor, MutedColor, BackgroundColor #374151, TableRowSelected pattern)

### Institutional Learnings

- `docs/solutions/ui-bugs/slot-card-border-overflow-2026-04-08.md`: existing UI rendering patterns
- No existing settings-UI-specific learnings documented

### Key Observations from Codebase

- Current field ordering in flat list: mixed_port, reset_port, allow_lan, update_via_proxy → Upstream Proxies header → System Proxy → TUN → Docker → Display → Info
- 7 boolean flags control sub-page visibility (`showTunSettings`, `showSystemProxySettings`, etc.)
- 5 duplicate focus systems, 5 duplicate scroll implementations, 3 near-identical sub-page renderers
- `components.Table` uses direct methods (`MoveUp()`, `MoveDown()`, `SetSelected()`, `View()`, `SetHeight()`) rather than a message-returning `Update()` — follow similar direct-method pattern for sidebar
- Bottom help bar is always rendered after content with `lipgloss.JoinVertical(lipgloss.Top, content, helpBar)`

## Key Technical Decisions

- **Sidebar as reusable component**: Follow `components.Table` pattern (`sidebar.go`) — takes category labels, selection index, supports click/keyboard nav. This keeps it testable independently.
- **Category-to-fields mapping**: Keep one canonical field store and use ordered category groups plus ordered field-key mappings for scoped rendering. A single enum replaces the mutually exclusive sub-page booleans.
- **Sub-page as overlay on right panel**: Replace full-screen modal approach. Sub-page renders conditionally in the right panel area only. The sidebar stays visible and interactive (though sub-page mode traps focus within the panel).
- **Dynamic field visibility**: At field init time, filter out platform-conditional fields that don't apply (instead of rendering them disabled). Store the full field list per category, filter at construction.
- **One focus/scroll system**: Replace 5 duplicates with shared helpers operating on the active field array. A `activeFields()`, `activeFocusIdx`, `activeScrollOffset` pattern reduces duplication.

## Implementation Units

- [x] **Unit 1: Create sidebar component**

**Goal:** A reusable vertical navigation list component

**Requirements:** R1

**Dependencies:** None

**Files:**
- Create: `internal/tui/components/sidebar.go`
- Test: `internal/tui/components/sidebar_test.go`

**Approach:**
- Follow `components/table.go` pattern: `Sidebar` struct with `New()`, `MoveUp()`, `MoveDown()`, `SetSelected()`, `View()`, `SetSize()`
- Takes `items []string` (category labels) and `selectedIndex int`
- Renders each item as a row with highlight on selection
- Exports `SetSelected(idx)`, `Selected()` methods
- Settings-level letter shortcuts select sidebar categories; field navigation keeps `up/k` and `down/j`
- Styling: use existing `TableRowSelected` (BackgroundColor #374151 with bold foreground) for the selected item, muted for inactive. Consistent with the existing component pattern.

**Patterns to follow:**
- `components/table.go`: `Table` struct, `New()`, `Update()`, `View()`, `SetSize()` pattern
- `components/table_test.go`: test pattern for component behavior
- `styles.go`: `TableRowSelected` style for active item

**Test scenarios:**
- Happy path: Sidebar renders items in order, first item selected by default
- Happy path: `down/j` moves selection down, wraps at bottom
- Happy path: `up/k` moves selection up, wraps at top
- Edge case: Single item — nav keys do nothing (already at the only position)
- Edge case: `SetSelected(-1)` clamps to 0, `SetSelected(N)` clamps to N-1
- Edge case: `SetSize` with insufficient height — what happens (should still render at least 1 item)

**Verification:**
- Component unit tests pass
- Manual visual: sidebar renders as vertical list with selected item highlighted

- [x] **Unit 2: Refactor SettingsTab — categorized field model**

**Goal:** Add categorized field views and replace the 7 boolean sub-page flags with one state

**Requirements:** R1, R2, R6

**Dependencies:** Unit 1 (sidebar component)

**Files:**
- Modify: `internal/tui/tabs/settings.go`
- Test: `internal/tui/tabs/settings_test.go`

**Approach:**
- Define ordered categories and ordered field-key mappings
- Define categories in order: 代理, 上游代理, 系统代理, TUN, Docker, 显示, 信息, 日志
- Add `sidebar *components.Sidebar`, `activeCategory int`, `activeSubPage subPageType` (enum: none, upstreamList, upstreamDetail, tunSettings, systemProxySettings, slotList, slotDetail, systemInfo) to `SettingsTab`
- Preserve one canonical field slice and project category-specific views from it, avoiding duplicated text-input state
- Keep `saveSettings()` on the canonical field slice. Sub-page save functions remain in their dedicated handlers and are NOT called from `saveSettings()`.
- Remove the 7 boolean flags (`showSystemInfo`, `showTunSettings`, etc.) — replace with `activeSubPage`
- Preserve all existing save/validation/effect logic unchanged

**Patterns to follow:**
- Current `settingField` struct is unchanged — only its organization changes
- Current `saveSettings()` effect logic (port change → restart, TUN change → permission check) stays as-is

**Test scenarios:**
- Happy path: All categories have the expected fields mapped correctly
- Happy path: Platform-conditional categories (TUN, Docker, System Proxy) are absent on inapplicable platforms
- Edge case: Switching categories preserves sub-page state (going back returns to previous view)

**Verification:**
- Existing unit tests pass
- All previous fields render in correct categories

- [x] **Unit 3: Refactor main page rendering — sidebar + content panel**

**Goal:** Render sidebar on left, active category content on right, bottom help bar

**Requirements:** R2, R5

**Dependencies:** Unit 2

**Files:**
- Modify: `internal/tui/tabs/settings.go`
- Modify: `internal/tui/styles/styles.go` (add sidebar-specific styles if needed)

**Approach:**
- `View()` becomes: `lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, contentView)`
  - `sidebarView = t.sidebar.View()` — rendered at `sidebarWidth` (fixed, e.g. 20 cols)
  - `contentView = renderCategoryFields()` — the active category's fields, using existing field rendering helpers but scoped to the category's field list
  - `availableWidth = t.width - sidebarWidth - borders`
- Content rendering reuses existing per-field rendering (input, select, button, header) but scoped to `t.categories[t.activeCategory].fields`
- Bottom help bar: `lipgloss.JoinVertical(lipgloss.Top, body, helpBar)`
- Add a subtle vertical separator between sidebar and content: `lipgloss.NewStyle().BorderLeft(...).Render(sidebarView)`
- `SetSize()` distributes width: `sidebar.SetSize(20, height)`, inputs get `width - sidebarWidth - margins`
- Scroll is per-content-panel using existing scroll helpers but applied to the active field array

**Patterns to follow:**
- Current `renderSelect()`, input rendering, button rendering — reuse as-is
- Current `renderSettingsSubpage()` pattern — adapt for panel-width rendering

**Test scenarios:**
- Happy path: Sidebar and content render side by side
- Happy path: Switching category updates content panel
- Edge case: Narrow terminal — sidebar collapses or scrolls
- Edge case: Content panel overflows — scroll indicator shown on right content only

**Verification:**
- Manual visual: sidebar + content render as two columns
- Category switching works via keyboard

- [x] **Unit 4: Refactor sub-page rendering — inline in right panel**

**Goal:** Sub-pages (upstream editor, TUN settings, System Proxy settings, slot detail/list, system info) render inside the right content panel instead of full-screen

**Requirements:** R3

**Dependencies:** Unit 3

**Files:**
- Modify: `internal/tui/tabs/settings.go`

**Approach:**
- In `View()`, when `activeSubPage != none`, the content panel renders the sub-page view instead of category fields
- Sidebar remains rendered on the left, dimmed (MutedColor foreground + suppressed keyboard events) to indicate sub-page is active. The sidebar-content divider border also shifts to MutedColor.
- Each sub-page's rendering function is updated to accept a `width` parameter (content panel width) instead of using `t.width`
- Sub-page navigation: Esc/Backspace exits sub-page (same as before), returns to category fields view
- Consolidate `renderSettingsSubpage()`, `viewUpstreamDetail()`, `viewSlotDetail()` into one or two shared helpers that differ only in field array source

**Patterns to follow:**
- Current sub-page rendering logic — but scoped to content panel width

**Test scenarios:**
- Happy path: Entering upstream editor from 上游代理 category shows editor in right panel
- Happy path: Esc exits sub-page, returns to category fields
- Edge case: TUN settings sub-page content overflows — scroll works within content panel
- Integration: Enter sub-page, change a value, Esc back — value persists

**Verification:**
- Sub-pages render within right panel, sidebar visible alongside

- [x] **Unit 5: Consolidate duplicated focus/scroll logic**

**Goal:** Remove the 5 duplicated focus and scroll implementations

**Requirements:** R7 (no behavior change)

**Dependencies:** Unit 4

**Files:**
- Modify: `internal/tui/tabs/settings.go`

**Approach:**
- Replace `tunFocusNext()`, `slotFocusNext()`, `upstreamFocusNext()`, `systemProxyFocusNext()` with one `focusNextField(fields []settingField, currentIdx int) int` helper
- Same for `focusPrevField()`, `isFieldFocusable()`
- Replace per-scroll-offset with a single scroll helper: `adjustScroll(focusIdx, scrollOffset, visibleHeight) (newOffset int)`
- The active field array and focus index are determined by current state (which category or which sub-page), selected by `activeSubPage` switch

**Patterns to follow:**
- Existing focus/scroll algorithms (correct, just duplicated)

**Test scenarios:**
- Happy path: Focus next skips header fields
- Happy path: Focus prev wraps correctly at boundaries
- Edge case: All fields in array are disabled — focus stays at current position (no infinite loop)
- Edge case: Scroll adjustment with negative offset handled correctly

**Verification:**
- Existing focus navigation behavior is preserved (tests prove this)
- Line count of settings.go is reduced

- [x] **Unit 6: Update and expand tests**

**Goal:** Ensure refactored code has coverage for new navigation patterns

**Requirements:** All

**Dependencies:** Unit 5

**Files:**
- Modify: `internal/tui/tabs/settings_test.go`
- Modify: `internal/tui/components/sidebar_test.go`

**Approach:**
- Add category-shortcut tests: switching categories and verifying focus behavior
- Update existing settings tests to work with new categorized structure
- Add sub-page-in-panel tests

**Test scenarios:**
- Happy path: category letters switch the sidebar selection; `down/j` stays within the active category
- Happy path: Enter on a category with sub-page button opens sub-page in right panel
- Happy path: Esc from sub-page returns to category view (discard test preserved)
- Edge case: All platform-conditional fields excluded on current platform

**Verification:**
- All tests pass: `make test`

## System-Wide Impact

- **Interaction graph:** Only `internal/tui/tui.go` references `SettingsTab` — its `Update()`/`View()`/`SetSize()` interface is unchanged. No other files need updating.
- **Error propagation:** Unchanged (save/validation logic is preserved verbatim).
- **State lifecycle risks:** Instant-save mode means no unsaved-state concerns. Sub-page state is reset on exit (same as current behavior).
- **Integration coverage:** Tab switching (settings → other → settings) must preserve state — existing `settings_discard_test.go` covers this.
- **Unchanged invariants:** Config model, field types, the `tea.Model` interface, instant-save behavior, and global `Tab` switching.

## Outcome

- Categories are platform-filtered and rendered through the reusable sidebar. Letter shortcuts select categories while arrow/vi keys continue to navigate fields.
- A single `settingsSubPage` state replaces the mutually exclusive page booleans, so sub-pages cannot overlap.
- The existing flat field slice remains the canonical persistence store; ordered category key lists provide scoped views without duplicating field state.
- Upstream, TUN, system proxy, slot, and system-info views render inside the right panel with the sidebar retained.
- Logs now render as a settings category backed by the same live `LogsTab` instance.
- Shared focus and scroll helpers replace the duplicated navigation loops.
- Full verification: `go test ./...`.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Sidebar width wastes horizontal space on narrow terminals | Use minimum width check; if width < threshold, collapse sidebar to icon-only or auto-hide with `Tab` toggle |
| Field mapping to categories misses a field or duplicates it | Cross-reference against current flat field list (17 fields + virtual restart button) during implementation |
| Sub-page rendering breaks in new layout | Sub-pages follow same rendering pattern but scoped to panel width — test each sub-page after implementation |

## Documentation / Operational Notes

- No documentation changes needed (CLAUDE.md, README unaffected)
- No migration needed (config format unchanged)
- No deployment impact

## Sources & References

- Current code: `internal/tui/tabs/settings.go`
- Component pattern: `internal/tui/components/table.go`
- Styles: `internal/tui/styles/styles.go`
- Design reference: yazi file manager (dual-column layout with left nav + right content)
