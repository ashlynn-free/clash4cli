---
title: "refactor: Remove Nodes tab, unify into reusable NodeBrowser component"
type: refactor
status: active
date: 2026-04-25
origin: docs/brainstorms/2026-04-25-remove-nodes-tab-unify-node-browser-requirements.md
---

# refactor: Remove Nodes tab, unify into reusable NodeBrowser component

## Overview

Remove the Nodes tab from the tab bar and extract its functionality into a reusable `NodeBrowser` component. SlotsTab uses it for proxy selection (pick mode), SubscriptionsTab uses it for node browsing (browse mode). The tab bar shrinks from 5 tabs to 4.

## Problem Frame

The Nodes tab and SlotsTab's proxy picker duplicate node browsing logic. The Nodes tab is a full-featured browser (testing, detail, sub-tabs), while the proxy picker is a minimal table. Unifying them into one reusable component eliminates duplication and gives every entry point the same rich experience.

## Requirements Trace

- R1. Extract NodesTab into reusable NodeBrowser component — **Unit 1**
- R2. NodeBrowser supports browse and pick modes — **Unit 1**
- R3. NodeBrowser preserves all Nodes tab features (sub-tabs, table, scroll state, preselect) — **Unit 1**
- R4. Complete keyboard shortcuts (t, r, i, navigation, Esc) — **Unit 1**
- R5. NodeBrowser shows slot indicators and traffic — **Unit 1**
- R6. Remove TabNodes from tab enum — **Unit 3**
- R7–R8. Delete nodes.go, nodes_test.go, node_detail.go — **Unit 3**
- R9–R11. Remove all nodesTab references from tui.go — **Unit 3**
- R12–R13. Replace SlotsTab proxy picker with NodeBrowser pick mode — **Unit 2**
- R14. SubscriptionsTab opens NodeBrowser in browse mode — **Unit 2**
- R15. NodeBrowser receives dependencies at creation — **Unit 1**
- R16. Existing config fields unchanged — all units

## Scope Boundaries

- Not changing `node.Manager`, `proxy.SlotManager`, or `subscription.Manager` APIs
- Not adding new node management features (add/edit/delete)
- Not changing Slots card layout or slot configuration flow — only the proxy picker sub-view is replaced
- Not migrating the Nodes tab's `Enter → cycle slot 0 mode` into NodeBrowser. Slot lifecycle is managed exclusively from the Slots tab

## Context & Research

### Relevant Code and Patterns

- **Sub-view pattern A (separate struct):** `NodeDetailView` in `internal/tui/tabs/node_detail.go` — own `Update()/View()/SetSize()`, parent delegates via flag check. This is the pattern for NodeBrowser.
- **Sub-view pattern B (inline state machine):** SlotsTab's `slotsView` enum with sub-handlers. NodeBrowser replaces the `slotsViewProxyPicker` variant.
- **Table component:** `internal/tui/components/table.go` — `NewTable`, `SetColumns`, `SetRows`, `SetHeight`, `MoveUp/Down`, `Selected`, `View`.
- **Current proxy picker:** Lines 268–306 of `internal/tui/tabs/slots.go` — inline table + sub-tabs + node selection. NodeBrowser subsumes this.
- **Subscription sub-tabs:** `renderProxySubTabs()` in slots.go (lines 684–702) — pattern to replicate inside NodeBrowser.
- **Message flow:** `NodeSelectedMsg`, `NodesRefreshedMsg` defined in nodes.go, consumed in tui.go. NodeBrowser will define equivalent messages.

### Institutional Learnings

- **lipgloss width trap** (`docs/solutions/ui-bugs/slot-card-border-overflow-2026-04-08.md`): `lipgloss.Style.Width(n)` sets content width *excluding* border. Subtract `borderW` and `padW` separately. Use named constants, assert with `lipgloss.Width()`.

## Key Technical Decisions

- **NodeBrowser is a separate struct in `internal/tui/components/`**: Follows Pattern A (like NodeDetailView). Both SlotsTab and SubscriptionsTab embed it as a field. Does not live in tabs/ because it's not a tab itself. *(see origin: requirements doc "Key Decisions")*
- **Callback via Tea messages**: NodeBrowser emits `NodeBrowserSelectedMsg{Node}` when a node is picked. The parent tab handles this message in its own Update(). This is the natural Bubble Tea pattern — no channels or func fields needed.
- **NodeDetailView moves to `internal/tui/components/`**: NodeBrowser embeds it. It becomes a dependency of the component, not the tabs package.
- **Sub-tabs move into NodeBrowser**: The subscription sub-tab bar currently in `tui.go`'s `renderSubTabs()` and `slots.go`'s `renderProxySubTabs()` both become NodeBrowser's internal responsibility. Delete both external renderers.
- **Slot 0 lifecycle stays out of NodeBrowser**: The current `handleNodesEnterForSlot0()` logic in tui.go is Slots-specific. NodeBrowser in pick mode just returns the selected node; the parent decides what to do with it.

## Open Questions

### Resolved During Planning

- **Package location**: `internal/tui/components/` — NodeBrowser imports `subscription`, `node`, `config`, `components.Table`, `styles`, `i18n`. No tab-specific types needed.
- **Callback mechanism**: Tea message `NodeBrowserSelectedMsg`. Natural for Bubble Tea, consistent with existing patterns like `NodeSelectedMsg`.
- **Subscriptions tab entry**: `Enter` on a subscription row opens NodeBrowser in browse mode.

### Deferred to Implementation

- **Exact i18n section naming**: Whether to reuse the existing `nodes:` / `node_detail:` YAML sections or create a new `node_browser:` section. Low risk — cosmetic decision during implementation.
- **Preselect behavior in browse mode**: Whether browse mode should auto-select a node (e.g., the currently running node for that subscription) or start at index 0. Can be a parameter on `Open()`.

## Implementation Units

- [ ] **Unit 1: Create NodeBrowser component**

**Goal:** Extract NodesTab functionality into a reusable `NodeBrowser` struct in `internal/tui/components/`.

**Requirements:** R1, R2, R3, R4, R5, R15

**Dependencies:** None

**Files:**
- Create: `internal/tui/components/nodebrowser.go`
- Create: `internal/tui/components/nodebrowser_test.go`
- Move from: `internal/tui/tabs/node_detail.go` → `internal/tui/components/node_detail.go`

**Approach:**

Create `NodeBrowser` struct with:
- Fields: `table`, `nodeManager`, `subscriptionManager`, `config`, `nodes []subscription.Node`, `subscriptionID`, `scrollStates`, `slotIndicators`, `slotTraffic`, `serverDisplay`, `showDetail`, `detailView`, `mode` (browse/pick), `width`, `height`, `subTabIndex`
- Constructor: `NewNodeBrowser(cfg, nodeMgr, subMgr)` — takes dependencies at creation (R15)
- Mode control: `SetMode(mode BrowserMode)` and `Open(subscriptionID string, preselectNode *subscription.Node)` / `Close()`
- Public methods: `Init() tea.Cmd`, `Update(tea.Msg) (tea.Cmd, NodeBrowserEvent)`, `View() string`, `SetSize(w, h int)`, `SetSubscriptionID(id string)`, `SetSlotIndicators(...)`, `SetSlotTraffic(...)`, `BlocksGlobalKeys() bool`
- Return events (not Tea messages) from Update: `NodeBrowserEvent` is a sum type — `None`, `NodeSelected{Node}`, `Closed`
- Internal message types: `nodeTestResultMsg`, `nodesRefreshedMsg`, `testCompleteMsg` — all lowercase, unexported
- Sub-tab rendering: inline in `View()`, same pattern as `renderProxySubTabs()` in current slots.go

Move `NodeDetailView` from `tabs/` to `components/` — NodeBrowser embeds it. Update `NodeDetailCloseMsg` to be an unexported message within the component.

The `Enter` key behavior differs by mode:
- **Pick mode**: emit `NodeSelected` event, caller handles it
- **Browse mode**: no-op or open detail view (same as current `i` shortcut)

All other keys (t, r, i, ↑↓, ←→, Esc) behave identically in both modes.

**Patterns to follow:**
- `internal/tui/tabs/nodes.go` — the source code being extracted
- `internal/tui/components/table.go` — component structure pattern
- `internal/tui/tabs/node_detail.go` — sub-view delegation pattern

**Test scenarios:**
- Happy path — browse mode: create NodeBrowser, set subscription, verify nodes render in View()
- Happy path — pick mode: navigate to node, press Enter, verify NodeSelected event returned with correct node
- Happy path — test all: press `t`, verify test commands issued, verify latency updates on result messages
- Happy path — refresh: press `r`, verify nodes refreshed from subscription manager
- Happy path — detail view: press `i`, verify detail view renders; press Esc, verify returns to list
- Happy path — sub-tab switch: press ←→, verify subscription changes, node list updates
- Edge case — empty subscription: verify graceful "no nodes" message
- Edge case — preselect: Open() with a preselectNode, verify cursor lands on that node
- Edge case — scroll state persistence: switch subscription away and back, verify cursor position restored
- Edge case — BlocksGlobalKeys: returns true when detail view is open, false otherwise

**Verification:**
- `go build ./internal/tui/components/` compiles
- All test scenarios pass
- NodeBrowser has no imports from `internal/tui/tabs/`

---

- [ ] **Unit 2: Integrate NodeBrowser into SlotsTab and SubscriptionsTab**

**Goal:** Replace SlotsTab's inline proxy picker with NodeBrowser in pick mode. Add NodeBrowser browse mode to SubscriptionsTab.

**Requirements:** R12, R13, R14

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/tui/tabs/slots.go`
- Modify: `internal/tui/tabs/slots_test.go`
- Modify: `internal/tui/tabs/subscriptions.go`
- Test: `internal/tui/tabs/slots_test.go` (update existing proxy picker tests)
- Test: `internal/tui/tabs/subscriptions_test.go` (new tests for node browsing)

**Approach:**

**SlotsTab changes:**
- Add field `nodeBrowser *components.NodeBrowser`
- Replace `slotsViewProxyPicker` state with NodeBrowser delegation
- Remove fields that NodeBrowser now owns: `proxyList *components.Table`, `proxySubID string`, `proxySubIndex int`
- Remove methods that NodeBrowser subsumes: `refreshProxyRows`, `openProxyPicker`, `updateProxyPicker`, `viewProxyPicker`, `renderProxySubTabs`, `switchProxySubscription`, `nodesForProxyPicker`, `sortedSubscriptions`, `subscriptionIndex`, `proxyTableColumns`
- In `Update()`: when `view == slotsViewProxyPicker`, delegate to `nodeBrowser.Update(msg)`. Handle `NodeSelected` event → set `SlotNodeRef`, save, restart slot. Handle `Closed` event → set `view = slotsViewList`
- In `View()`: when `view == slotsViewProxyPicker`, return `nodeBrowser.View()`
- Keep `sortedSubscriptions()` if still needed for non-NodeBrowser purposes, otherwise remove

**SubscriptionsTab changes:**
- Add field `nodeBrowser *components.NodeBrowser`
- Add new view state: `showNodeBrowser bool`
- On `Enter` key in the main subscription list: set `nodeBrowser.Open(subID, nil)` with browse mode, set `showNodeBrowser = true`
- In `Update()`: when `showNodeBrowser`, delegate to `nodeBrowser.Update(msg)`. Handle `Closed` event → set `showNodeBrowser = false`
- In `View()`: when `showNodeBrowser`, return `nodeBrowser.View()`
- `BlocksGlobalKeys()` returns true when `showNodeBrowser` is true

**Patterns to follow:**
- SlotsTab's existing sub-view delegation pattern (the `slotsView` enum dispatch)
- NodeDetailView embedding pattern in current NodesTab

**Test scenarios:**
- Happy path — Slots pick mode: open proxy picker, navigate to node, press Enter, verify SlotNodeRef updated
- Happy path — Slots cancel: open proxy picker, press Esc, verify returns to slot list without changes
- Happy path — Subscriptions browse: select subscription, press Enter, verify node browser opens showing that subscription's nodes
- Happy path — Subscriptions close: in node browser, press Esc, verify returns to subscription list
- Edge case — Slots preselect: opening proxy picker preselects the current slot's configured node
- Edge case — Slots full-screen: NodeBrowser correctly reports BlocksGlobalKeys

**Verification:**
- `go build ./...` compiles
- Existing SlotsTab tests pass (updated for new proxy picker integration)
- New SubscriptionsTab browse tests pass

---

- [ ] **Unit 3: Remove NodesTab and clean up tui.go**

**Goal:** Delete the Nodes tab, remove all references from the main model, simplify tab routing.

**Requirements:** R6, R7, R8, R9, R10, R11

**Dependencies:** Unit 1, Unit 2

**Files:**
- Delete: `internal/tui/tabs/nodes.go`
- Delete: `internal/tui/tabs/nodes_test.go`
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/state_restore_test.go`

**Approach:**

**Tab enum change:**
```
TabSlots Tab = iota    // was TabNodes=0, now Slots=0
TabSubscriptions
TabSettings
TabLogs
```

**Remove from Model struct:**
- `nodesTab *tabs.NodesTab` field

**Remove from New():**
- `nodesTab: tabs.NewNodesTab(nodeMgr)` initialization
- `nodesTab.SetServerDisplay(...)` call
- `nodesTab.SetSubscriptionID(...)` call

**Remove from Init():**
- `m.nodesTab.Init()` call

**Remove from Update() — key handling:**
- `nodeOverlay` variable and `m.activeTab == TabNodes` checks
- `handleNodesEnterForSlot0()` and all references to it
- Subscription switch (`←`/`→`) on Nodes tab — no longer a global shortcut
- `D` disconnect-all on Nodes tab — already available in Slots tab

**Remove from Update() — message handling:**
- `tabs.NodeSelectedMsg` case — slot 0 selection from nodes (now handled within SlotsTab)
- `tabs.NodesRefreshedMsg` case — always-forward-to-nodes
- `tabs.SubscriptionAddedMsg` → `m.nodesTab.Init()` call
- `tabs.SubscriptionUpdatedMsg` → `m.nodesTab.Init()` call
- `tabs.CustomNodesChangedMsg` → `m.nodesTab.SetPreselect()` and related
- `tabs.SubsRefreshedMsg` → `m.nodesTab.SetSubscriptionID()` and `m.nodesTab.Init()` calls
- `tabs.SettingsSavedMsg` → `m.nodesTab.SetServerDisplay()` call
- `slotTrafficMsg` → `m.nodesTab.SetSlotTraffic()` and Slot-0-only traffic logic
- `slotActionDoneMsg` → `m.nodesTab.RestoreCursor()` call

**Remove from Update() — active tab routing:**
- `case TabNodes: m.nodesTab, cmd = m.nodesTab.Update(msg)`

**Remove from View():**
- `case TabNodes:` rendering branch
- `m.renderSubTabs()` method entirely

**Remove from other methods:**
- `renderSubTabs()` — delete entirely
- `updateSlotIndicators()` → `m.nodesTab.SetSlotIndicators(...)` call
- `updateSizes()` → `m.nodesTab.SetSize(...)` call
- `restoreSelectionFromConfig()` → `m.nodesTab.SetPreselect(...)` call
- `restoreStateFromProxyLock()` → `m.nodesTab.SetSubscriptionID()` and `SetPreselect()` calls
- `switchSubscription()` → `m.nodesTab.SetSubscriptionID()` and `m.nodesTab.Init()` calls
- `disconnectAllSlots()` → `m.nodesTab.SetSlotIndicators(nil)` call
- `nextTab()`, `prevTab()`, `setTab()` → `if m.activeTab == TabNodes { m.updateSlotIndicators() }` checks

**Simplify tab count:**
- `nextTab()`: `(int(m.activeTab) + 1) % 4` (was 5)
- `prevTab()`: `(int(m.activeTab) + 3) % 4` (was 4)
- `setTab()`: bounds check `t >= TabSlots && t <= TabLogs` (was `TabNodes`)
- `renderTabBar()`: 4 tab names instead of 5

**Update `restoreSelectionFromConfig()` and `restoreStateFromProxyLock()`:**
- These currently update `nodesTab` to set the preselected node. After removal, they should update the status bar and slot 0 config directly (set `SlotNodeRef` in config, update `selectedNode` field). The NodeBrowser is opened lazily — preselect is passed at Open() time.

**Remove exported message types from deleted files:**
- `NodeSelectedMsg`, `NodesRefreshedMsg` — delete from nodes.go. If still referenced anywhere, move to tui.go or the consuming tab.
- `NodeTestResultMsg`, `TestCompleteMsg` — internal to NodeBrowser now.
- `SlotIndicator` — move to components package (used by NodeBrowser).

**Update state_restore_test.go:**
- Remove references to `m.nodesTab` — the test creates a Model and checks node restoration. Update to verify restoration via slot 0 config or status bar instead.

**Test expectation: none** — this is deletion + mechanical cleanup; correctness is verified by compilation and existing tests.

**Verification:**
- `go build ./...` compiles with zero references to `NodesTab`, `TabNodes`, `nodes.go`
- `make test` passes (with updated test files)
- Tab bar shows 4 tabs: Slots, Subscriptions, Settings, Logs

---

- [ ] **Unit 4: i18n and test cleanup**

**Goal:** Update i18n strings for the new 4-tab layout. Add missing tests. Fix any remaining references.

**Requirements:** R6 (tab bar names)

**Dependencies:** Unit 3

**Files:**
- Modify: `internal/i18n/lang/en_us.yaml` (and all 13 other language files)
- Modify: `internal/i18n/i18n.go` (if Messages struct needs updating)
- Test: run full `make test`

**Approach:**

- Remove `tabs.nodes` key from all 14 YAML files (or leave it if NodeBrowser reuses it — implementation decision)
- Verify tab bar rendering with 4 tabs — no visual regression
- Update `Messages.Tabs` struct if needed
- Add `node_browser` section to Messages struct and YAML files if NodeBrowser needs its own strings (or reuse existing `nodes:` section — deferred to implementation)
- Run `make test` to verify all tests pass

**Test scenarios:**
- Integration — tab bar renders 4 tabs in correct order
- Integration — Tab key cycles through 4 tabs correctly
- Integration — tab hit test (mouse click) works with 4 tabs

**Verification:**
- `make test` passes
- Manual visual check: tab bar shows Slots, Subscriptions, Settings, Logs

## System-Wide Impact

- **Interaction graph:** `tui.go` no longer routes `NodeSelectedMsg`, `NodesRefreshedMsg`. Node selection is entirely within SlotsTab. Node refresh is handled within NodeBrowser component.
- **Error propagation:** NodeBrowser internal errors (test failures, refresh failures) are displayed inline in the component view, not bubbled to tui.go.
- **State lifecycle risks:** `restoreSelectionFromConfig()` and `restoreStateFromProxyLock()` must still correctly restore slot 0 state without nodesTab. Preselect data is stored in config fields (`UI.SelectedNode*`), not in the NodesTab.
- **API surface parity:** No external API changes. This is a pure UI refactoring.
- **Unchanged invariants:** Slot lifecycle (start/stop/restart), traffic polling, config persistence, log forwarding — all remain unchanged.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Large tui.go deletion may break tab routing | Unit 3 is last — verify with `make test` after each prior unit |
| NodeBrowser event type design may not fit both callers | Unit 1 starts with a simple sum type (None/Selected/Closed); extend if needed |
| Sub-tabs rendering in NodeBrowser may differ from current | Copy `renderProxySubTabs()` logic from slots.go as starting point |
| lipgloss width miscalculation in NodeBrowser layout | Follow learning from `docs/solutions/ui-bugs/slot-card-border-overflow-2026-04-08.md`: named constants, subtract border/padding separately, assert widths in tests |

## Sources & References

- **Origin document:** [docs/brainstorms/2026-04-25-remove-nodes-tab-unify-node-browser-requirements.md](docs/brainstorms/2026-04-25-remove-nodes-tab-unify-node-browser-requirements.md)
- Source for extraction: `internal/tui/tabs/nodes.go`
- Proxy picker to replace: `internal/tui/tabs/slots.go` lines 268–306, 530–629, 684–758
- Component pattern: `internal/tui/components/table.go`
- Sub-view pattern: `internal/tui/tabs/node_detail.go`
- lipgloss learning: `docs/solutions/ui-bugs/slot-card-border-overflow-2026-04-08.md`
