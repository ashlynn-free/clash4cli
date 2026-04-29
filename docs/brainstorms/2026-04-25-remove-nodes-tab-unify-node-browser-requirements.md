---
date: 2026-04-25
topic: remove-nodes-tab-unify-node-browser
---

# Remove Nodes Tab, Unify into Reusable NodeBrowser

## Problem Frame

The TUI currently has two separate node-related UIs with heavy overlap:
- **Nodes tab** (`TabNodes`): Full-featured node browser per subscription — testing, detail view, slot indicators, sub-tab switching.
- **Slots tab's proxy picker** (`slotsViewProxyPicker`): Minimal table for selecting a proxy node, no testing/detail/refresh.

The Nodes tab is redundant now that Slots is the primary workflow. Users should manage everything through Slots + Subscriptions. However, Slots' proxy picker is too limited — it lacks testing, detail view, refresh, and other features that the Nodes tab has.

## Requirements

**NodeBrowser Component**
- R1. Extract the current `NodesTab` functionality into a reusable `NodeBrowser` component that can be embedded by other tabs as a sub-view.
- R2. NodeBrowser operates in two modes:
  - **Browse mode**: Pure viewing/testing. Used when entering from Subscriptions tab to inspect nodes.
  - **Pick mode**: Viewing + selection. `Enter` selects the focused node and returns it to the caller. Used when Slots needs a proxy node.
- R3. NodeBrowser preserves all current Nodes tab features:
  - Sub-tab bar to switch between subscriptions (`←`/`→`)
  - Node table with columns: icon, name, server (respecting show/blur/off), latency, status
  - Per-subscription scroll state persistence
  - Preselect mechanism (highlight a specific node on open)
- R4. NodeBrowser supports complete keyboard shortcuts:
  - `↑`/`↓`/`j`/`k`: Navigate
  - `Enter`: In pick mode → select and return. In browse mode → show node info or no-op.
  - `t`: Test all visible nodes
  - `r`: Refresh current subscription's nodes
  - `i`: Open node detail view
  - `←`/`→`/`h`/`l`: Switch subscription sub-tabs
  - `Esc`/`Backspace`: Close NodeBrowser, return to caller
- R5. NodeBrowser shows slot indicators and traffic info for active slots (inherited from current Nodes tab behavior).

**Remove Nodes Tab**
- R6. Remove `TabNodes` from the `Tab` enum and tab bar. Tab bar becomes: Slots, Subscriptions, Settings, Logs (4 tabs).
- R7. Remove `internal/tui/tabs/nodes.go` and `internal/tui/tabs/nodes_test.go`.
- R8. Remove `internal/tui/tabs/node_detail.go` — node detail functionality moves into NodeBrowser.
- R9. Remove all `nodesTab` references from `internal/tui/tui.go` (field, initialization, update routing, rendering, size updates, indicator updates, traffic updates, subscription switching).
- R10. Remove `renderSubTabs()` from `tui.go` — sub-tab rendering becomes NodeBrowser's responsibility.
- R11. Remove Nodes-specific global key handling from `tui.go` (`handleNodesEnterForSlot0`, subscription switch on `←`/`→`, disconnect-all `D` on Nodes tab).

**Slots Integration**
- R12. Replace the current `slotsViewProxyPicker` in `SlotsTab` with an embedded NodeBrowser in pick mode.
- R13. When a node is picked, update the slot's `SlotNodeRef` config and trigger save/restart (same as current behavior).

**Subscriptions Integration**
- R14. Add an entry point in the Subscriptions tab (e.g., `Enter` on a subscription row) to open NodeBrowser in browse mode, showing that subscription's nodes.

**State & Config**
- R15. The NodeBrowser component receives its dependencies (node manager, subscription manager, config) at creation time, not via global state.
- R16. Existing config fields (`UI.ActiveSubscriptionID`, `UI.SelectedNode*`, `UI.ServerDisplay`) remain unchanged.

## Success Criteria
- Tab bar shows exactly 4 tabs: Slots, Subscriptions, Settings, Logs.
- Slots proxy selection has full node features (test, detail, refresh).
- Subscriptions tab can open NodeBrowser to browse a subscription's nodes.
- No regressions in slot start/stop, traffic display, or config persistence.

## Scope Boundaries
- Not changing the underlying `node.Manager`, `proxy.SlotManager`, or `subscription.Manager` APIs.
- Not adding new node management features (add/edit/delete nodes). Only browsing and selecting.
- Not changing the Slots card layout or slot configuration flow — only the proxy picker sub-view is replaced.
- Not migrating the Nodes tab's `Enter → cycle slot 0 mode` shortcut into the NodeBrowser. Slot lifecycle is managed exclusively from the Slots tab.

## Key Decisions
- **NodeBrowser is a component, not a tab**: It lives in `internal/tui/components/` and is embedded by tabs that need it, not listed in the tab bar.
- **Two modes, one code path**: Browse and pick share the same rendering; only `Enter` behavior differs.
- **Sub-tabs move into NodeBrowser**: The subscription sub-tab bar that currently lives in `tui.go`'s `renderSubTabs()` becomes part of NodeBrowser's own View().

## Dependencies / Assumptions
- The `NodeDetailView` from `node_detail.go` is preserved as a sub-component used by NodeBrowser.
- `SlotIndicator` and traffic display types remain in the `tabs` package or move to a shared location.

## Outstanding Questions

### Deferred to Planning
- [Affects R1][Technical] Exact Go interface/type design for NodeBrowser's caller callback (channel, func field, or Tea message).
- [Affects R1][Technical] Whether NodeBrowser lives in `internal/tui/components/` or `internal/tui/tabs/` — depends on whether it imports tab-specific types.
- [Affects R14][UX] What exact key/gesture in the Subscriptions tab opens NodeBrowser, and what the flow looks like.
- [Affects R9][Technical] How to handle `restoreSelectionFromConfig()` and `restoreStateFromProxyLock()` which currently update `nodesTab` — these may need to target the Slot 0 config directly instead.

## Next Steps
→ `/ce:plan` for structured implementation planning
