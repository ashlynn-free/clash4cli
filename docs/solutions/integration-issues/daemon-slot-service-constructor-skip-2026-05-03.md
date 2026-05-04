---
title: Daemon-first migration: frontend SlotService constructor bypass broke TUI runtime
date: 2026-05-03
category: integration-issues
module: internal/app
problem_type: integration_issue
component: development_workflow
symptoms:
  - TUI starts but slots show "Ready" never become "Running" after pressing space
  - No error message visible in main TUI view
  - Daemon health endpoint responds normally
  - HTTP curl to daemon slot start endpoint works independently
root_cause: logic_error
resolution_type: code_fix
severity: high
tags:
  - daemon-first
  - daemon-slot-service
  - constructor-bypass
  - tui
  - log-stream
  - parallel-edit
---

# Daemon-first migration: constructor bypass and endpoint loss broke TUI runtime slot control

## Problem

After migrating `c4c` to daemon-first architecture, the TUI connects to the daemon but slots never transition from "Ready" to "Running" when toggled. The `daemonSlotService` proxy silently discards all slot operations, and the TUI log tab receives no log events from the daemon.

## Symptoms

- TUI boots and shows configured slots as "Ready"
- Pressing space (toggle) or Enter (start) does not change slot state
- Switching to Logs tab shows no daemon log entries
- Running `./c4cgo` from terminal shows "No active slots"
- Direct curl to daemon HTTP API succeeds (e.g., `POST /slots/0/start` returns running slot state)
- `daemonSlotService` methods like `GetSlotState` return nil for all slot IDs

## What Didn't Work

- Restarting c4cd and c4c — same behavior
- Checking daemon HTTP handlers — curl proved the daemon is fine
- Looking at `SlotService` interface implementation — it compiled, so all method signatures matched
- Checking daemon port/liveness — health endpoint returned `{"healthy":true}`

## Solution

Two issues needed fixing:

### 1. Constructor bypass in `internal/app/app.go`

The `daemonSlotService` was initialized with a bare struct literal, skipping the constructor function that sets up the log stream and channel:

```go
// BAD — bypasses newDaemonSlotService constructor
slotMgr := proxy.SlotService(&daemonSlotService{client: client})
```

Must use the proper constructor:

```go
// GOOD — uses proper constructor with log stream wiring
slotMgr := proxy.SlotService(newDaemonSlotService(client))
```

The constructor `newDaemonSlotService` (in `internal/app/daemon_slot_service.go`) does critical initialization:
- Creates the `logCh` channel (capacity 128)
- Connects to the daemon SSE log stream via `client.StreamLogs()`
- Launches a goroutine to forward log entries to `logCh`

Without this, `LogChannel()` returns a nil channel, and all log-based UI features are dead.

### 2. Missing `/logs/stream` endpoint in `internal/daemon/server.go`

During development, the log stream endpoint was accidentally replaced when adding the `/state/config` endpoint:

```go
// BAD — the endpoint was replaced instead of added
mux.HandleFunc("/state/config", s.handleConfigState)
// /logs/stream is now missing!
```

Both endpoints must be registered:

```go
// GOOD — both endpoints registered
mux.HandleFunc("/logs/stream", s.handleLogStream)
mux.HandleFunc("/state/config", s.handleConfigState)
```

Without this, `client.StreamLogs()` receives a 404, falls back to a non-streaming mode, and `daemonSlotService` gets no log entries. The goroutine in the constructor also blocks on `<-ch` waiting for log events that never arrive.

### 3. (Bonus) Parallel edits broke `tui.go` brace structure

When the `tui.go` file was modified concurrently by multiple agents, the brace structure in the `Update` method became inconsistent. This manifested as hard-to-trace `syntax error: unexpected keyword case, expected }` errors at the `switch-message` type switch boundary. Fix: `git checkout` the affected files to HEAD and re-apply targeted changes.

## Why This Works

The `daemonSlotService` adapter bridges the `SlotService` interface to the daemon HTTP API. Without proper construction, the adapter appears to compile but silently returns nil/error for every operation because:

1. `LogChannel()` returns a nil channel → log listener goroutine blocks forever → TUI receives no daemon log events
2. `client.StreamLogs()` gets 404 from missing endpoint → constructor reports log connection failure silently
3. `GetSlotState()` queries healthy daemon with correct port → daemon has no running slots → returns nil → UI shows "Ready"
4. `StartSlotWithMode()` sends HTTP POST → daemon receives valid request → slot actually starts → frontend never calls `GetSlotState` again to verify, because the log/error channel is broken and the event loop hangs

The frontend works correctly with the daemon — it just never gets to the point of checking the result, because the log stream deadlock prevents the event loop from processing subsequent messages.

## Prevention

1. **Always use the constructor function** when a type has complex initialization beyond field assignment. A bare struct literal bypasses all side effects.
2. **When modifying HTTP handler registrations**, double-check that existing routes are not accidentally replaced by new ones. Use `git diff` before committing route changes.
3. **When running multiple parallel agents**, pin files to a single agent at a time, or use `git checkout` to reset before re-applying changes rather than editing in-place against concurrent modifications.
4. **When a type implements an interface and a method returns "not implemented yet"**, that's a signal the adapter is incomplete. The `SlotService` interface had 4 methods returning hardcoded errors — these should have been flagged as incomplete during review.

## Related Issues

- `docs/plans/2026-04-29-001-refactor-c4cd-daemon-first-plan.md`
- `internal/app/daemon_slot_service.go` — the adapter implementation
- `internal/daemon/server.go` — HTTP route registration
- `internal/daemon/client.go` — `StreamLogs()` method (depends on `/logs/stream` endpoint)
