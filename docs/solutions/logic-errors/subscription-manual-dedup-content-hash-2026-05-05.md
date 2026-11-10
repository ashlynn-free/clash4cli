---
title: "Manual subscription paste creates duplicate files due to random ID generation"
date: 2026-05-05
category: logic-errors
module: internal/subscription
problem_type: logic_error
component: tooling
symptoms:
  - "Pasting the same manual subscription content multiple times creates multiple duplicate subscription files on disk"
  - "The existing 'subscription already exists' duplicate check never fires for manual subscriptions"
  - "Each paste of identical content produces a different on-disk filename with a random hex ID"
root_cause: logic_error
resolution_type: code_fix
severity: medium
tags:
  - subscription
  - manual-subscription
  - deduplication
  - content-hash
  - add-manual
  - crypto-rand
---

# Manual subscription paste creates duplicate files due to random ID generation

## Problem

`AddManual()` in the subscription manager generated a random 8-byte hex ID via `crypto/rand` for every new manual subscription. Pasting the same content twice created two separate subscription files with different IDs, defeating the deduplication check and accumulating duplicates in `~/.clash4cli/subscriptions/`.

## Symptoms

- Pasting identical subscription content (e.g. via the `z` key in the TUI) produced a new file on disk each time instead of being rejected as "already exists."
- Four duplicate files were observed on a user's system, all with the same content (`ss://...`) and name `"sub"`, created at microsecond-scale intervals (703-704 us apart).
- The `~/.clash4cli/subscriptions/` directory accumulated stale duplicate files that could not be deduplicated automatically.

## What Didn't Work

- **Relying on the existence check alone**: because the ID was random and always unique, `m.subscriptions[id]` was never found, so the `"subscription already exists"` guard never fired.
- **Post-hoc dedup by content after creation**: would require scanning the whole directory on every add and introduces a TOCTOU window where two concurrent adds could both succeed.

## Solution

Changed `AddManual()` to use a deterministic ID from `sha256(name + content)[:8]` instead of `randomID(8)`. The existing dedup check then catches repeat pastes reliably.

**Before** (`internal/subscription/manager.go`):

```go
id, err := randomID(8)
if err != nil {
    return nil, err
}

sub := &Subscription{
    ID:      id,
    Name:    name,
    Content: content,
    // ...
}

m.mu.Lock()
m.subscriptions[id] = sub
m.mu.Unlock()

if err := m.saveSub(sub); err != nil {
    return nil, err
}
```

**After**:

```go
hash := sha256.Sum256([]byte(name + "\x00" + content))
id := hex.EncodeToString(hash[:8])

m.mu.Lock()
defer m.mu.Unlock()

if _, exists := m.subscriptions[id]; exists {
    return nil, fmt.Errorf("subscription already exists")
}

sub := &Subscription{
    ID:      id,
    Name:    name,
    Content: content,
    // ...
}

m.subscriptions[id] = sub
if err := m.saveSub(sub); err != nil {
    return nil, err
}
```

The now-dead `randomID` function was also removed from the file to prevent accidental reintroduction.

## Why This Works

- **Deterministic identity**: Two calls with the same `(name, content)` pair always produce the same ID. The existence check becomes a true content-dedup guard.
- **Collision safety**: SHA-256 truncated to 8 bytes gives a 64-bit space. Collision probability is negligible (birthday bound of ~1.5e-5 at 1 million entries), well within limits for a local CLI tool.
- **Null separator**: The `"\x00"` delimiter between name and content prevents ambiguity (e.g. `("ab", "c")` vs `("a", "bc")` producing the same hash).
- **Lock ordering**: The deterministic ID is computed outside the lock, with only the map read/write under `m.mu.Lock()`. This avoids holding the lock during SHA-256 computation while still guaranteeing atomic check-then-insert.

## Prevention

- **Use content-hash IDs for deduplication**: when user-provided data is stored in a file-per-entry directory, derive the entry key from a cryptographic hash of the canonical content (plus a namespace discriminator if needed).
- **Remove dead code after migration**: delete the old `randomID` generator immediately after switching to deterministic IDs to prevent accidental reintroduction.
- **Add a unit test for duplicate rejection**: a test that calls `AddManual` twice with the same inputs and asserts the second call returns `"subscription already exists"`.

## Related Issues

- [Subscription manager source](internal/subscription/manager.go)
- [TUI subscription tab (z-key handler)](internal/tui/tabs/subscriptions.go)
