---
title: "Subscription Detail View for URL Inspection"
date: 2026-05-05
category: developer-experience
module: tui
problem_type: developer_experience
component: tooling
severity: low
applies_when:
  - "The subscription table is too wide for the terminal width, causing horizontal scroll or cramped columns"
  - "Users need to inspect the full subscription URL but do not need it visible in every table row"
tags:
  - subscription
  - tui
  - detail-view
  - ui
related_components:
  - subscription
---

# Subscription Detail View for URL Inspection

## Context

The subscription list table displayed raw subscription URLs in a dedicated column. These URLs are often very long (requiring aggressive 37-character truncation with "..."), creating visual clutter in the table without providing useful information at a glance. Users rarely need to see the URL unless inspecting a specific subscription's details.

## Guidance

- Hide non-essential but potentially useful information behind a detail/inspect view rather than displaying it directly in the table.
- Use uppercase `I` (uppercase i) as the keybinding for "inspect" or "info" — consistent with common TUI conventions where uppercase letters access secondary actions.
- Replace raw data columns (like URLs) with categorized source type columns (`URL`, `Manual`, `Cus`) that compress the same semantic information into a compact, scannable form.
- Store the detail-view data (URL, timestamps, error status) on the model struct so it is preserved and accessible on demand without redundant lookups.

## Why This Matters

Reduces visual noise and makes the table more scannable. The user can quickly identify the source type of each subscription at a glance. Full detail information (including the complete URL, timestamps, and update status) remains accessible with a single keystroke — nothing is lost, it is merely moved behind an intentional disclosure pattern.

## When to Apply

- When designing TUI table views that include fields with variable or unbounded length (URLs, file paths, descriptions).
- When a data column in a list view serves a minority of use cases — push it to a detail view and free the list for high-density scanning.
- When adding a secondary action keybinding, prefer uppercase letters (shift+key) to avoid conflicts with primary single-key shortcuts.

## Examples

**Before** — Table columns included a URL column with truncation and switch-case source labeling:

```go
columns := []components.Column{
    {Title: "Name", Width: 42},
    {Title: "Nodes", Width: 8},
    {Title: "Last", Width: 14},
    {Title: "URL", Width: 36},  // raw URL cluttering the table
}
// Row rendering had URL truncation logic:
url := sub.URL
if len(url) > 37 {
    url = url[:37] + "..."
}
```

**After** — Table columns replaced URL with Source type; detail popup behind 'I':

```go
// Table columns:
columns := []components.Column{
    {Title: "Name", Width: 42},
    {Title: "Nodes", Width: 8},
    {Title: "Last", Width: 14},
    {Title: "Source", Width: 8},  // compact type label
}

// Row rendering uses a simple switch:
source := string(sub.Source)
if subscription.IsCustomSubscription(sub) {
    source = "Cus"
} else if sub.Source == subscription.SourceManual {
    source = "Manual"
} else if sub.Source == subscription.SourceURL {
    source = "URL"
}

// 'I' keybinding dispatches to showDetailForSelected():
case key.Matches(msg, key.NewBinding(key.WithKeys("I"))):
    return t, t.showDetailForSelected()

// Detail popup renders full URL via renderDetail():
func (t *SubscriptionsTab) renderDetail() string {
    // Shows: Name, Source, URL (full, no truncation),
    //        Nodes count, Created time, Updated time with status
    // Closed by 'I' or 'Esc'
}
```

## Related

- [Node Browser Fullscreen Title Visibility](../ui-bugs/node-browser-fullscreen-title-visibility-q-quit-2026-05-05.md) — same file (`subscriptions.go`), keybinding patterns, and full-screen mode conventions
- [Node Browser Table Overflow](../ui-bugs/nodebrowser-table-overflow-many-nodes-2026-05-04.md) — TUI height budget management when adding views
- [Subscription Manual Dedup](../logic-errors/subscription-manual-dedup-content-hash-2026-05-05.md) — subscription domain context
