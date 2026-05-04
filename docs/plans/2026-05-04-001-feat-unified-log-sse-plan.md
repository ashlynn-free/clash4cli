---
title: "feat: Unify log types and publish daemon logs via SSE"
type: feat
status: active
date: 2026-05-04
origin: docs/brainstorms/2026-05-04-unified-log-sse-requirements.md
---

# feat: Unify log types and publish daemon logs via SSE

## Overview

Unify the two separate log entry types (`logger.Entry` and `proxy.LogEntry`) into a single `logger.Entry` type used across the entire project. Extend the SSE `/logs/stream` endpoint to carry both daemon application logs and slot/mihomo logs. Slot controllers publish through the `logger` package instead of owning independent log channels.

## Problem Frame

Current state has three fragmented log paths:
- `logger.Entry` (int level + Time) — app-level logs, file + internal channel, only for c4c TUI locally
- `proxy.LogEntry` (string level, no Time) — slot/mihomo logs, via `SlotManager.LogChannel()` → SSE
- `/logs/stream` SSE endpoint only publishes `proxy.LogEntry`; c4cd's own `logger.Info/Warn/Error` calls never reach SSE clients

## Requirements Trace

- R1. `logger.Entry` as the sole log entry type; delete `proxy.LogEntry`
- R2. SSE `log` event carries `time` (RFC3339Nano), `level`, `message`, plus daemon-injected `revision`
- R3. Source distinction via message prefix (`[daemon]`, `[slotN]`)
- R4. `logger` supports daemon and client initialization paths; daemon path registers SSE publisher
- R5. c4cd initializes logger in daemon mode; c4c in client mode
- R6. Daemon-mode logger writes to file + pushes to internal channel + calls SSE publisher
- R7. Slot controller calls `logger.Info/Warn/Error` directly with `[slotN]` prefix; no independent `logCh`
- R8. Remove `SlotManager.forwardLogs()`, `SlotController.LogChannel()`, `SlotManager.LogChannel()`
- R9. `/logs/stream` carries both daemon and slot logs as `event: log` with unified format
- R10. No history on SSE connect (already satisfied)
- R11. `logStreamHub` types change from `proxy.LogEntry` to `logger.Entry`
- R12. `daemon.Client.StreamLogs()` returns `<-chan logger.Entry`
- R13. `daemonSlotService` exposes `LogChannel() <-chan logger.Entry`
- R14. TUI log panel shows daemon/slot logs (via SSE) + local logs (via `logger.Channel()`)
- R15. Log panel no longer handles `proxy.LogEntry`

## Scope Boundaries

- Log file writing behavior unchanged (filename, format, path)
- No new log levels
- SSE endpoint path unchanged (`/logs/stream`)
- `c4cgo` functionality unaffected

## Context & Research

### Relevant Code and Patterns

- `internal/logger/logger.go` — singleton `Logger` with `sync.Once`, `Log()` writes file + non-blocking channel push
- `internal/proxy/controller.go:84-88` — `proxy.LogEntry` definition (to be removed)
- `internal/proxy/controller.go:546-561` — `readLogs()` pipes mihomo stdout → `c.logCh`
- `internal/proxy/slot_manager.go:631-643` — `forwardLogs()` fanout with `[slotN]` prefix
- `internal/daemon/log_stream.go:14-49` — `logStreamHub` fanout broadcaster (subscribe/publish)
- `internal/daemon/log_stream.go:51-60` — `startLogFanout()` goroutine bridging `SlotManager.LogChannel()` → hub
- `internal/daemon/log_stream.go:62-116` — `handleLogStream` SSE handler
- `internal/daemon/client.go:98-144` — `StreamLogs()` SSE client
- `internal/app/daemon_slot_service.go:15-48` — bridges daemon SSE → local channel for TUI
- `internal/tui/tui.go:1186-1207` — `listenSlotLogs()` + `listenAppLogs()` tea.Cmd listeners
- `internal/tui/tui.go:610-622` — Update handler for `proxy.LogEntry` and `logger.Entry` messages
- `internal/tui/tabs/logs.go:77-82` — `proxy.LogEntry` case in LogsTab.Update

### All `proxy.LogEntry` Reference Sites (complete list)

| File | Lines | Action |
|------|-------|--------|
| `internal/proxy/controller.go` | 85-88, 95, 541-544, 553 | Remove type, field, method, rewrite readLogs |
| `internal/proxy/slot_controller.go` | 27, 128-131 | Remove channel creation, delegation method |
| `internal/proxy/slot_manager.go` | 39, 51, 70-73, 631-643 | Remove field, init, method, forwardLogs |
| `internal/proxy/slot_service.go` | 12 | Remove from interface |
| `internal/daemon/log_stream.go` | 17, 21, 24, 26, 40 | Change to `logger.Entry` |
| `internal/daemon/types.go` | 29-32 | Remove `LogEvent` type |
| `internal/daemon/client.go` | 98, 115, 140 | Change return type and channel type |
| `internal/app/daemon_slot_service.go` | 19, 24, 46 | Change channel type |
| `internal/tui/tui.go` | 610-616 | Remove `case proxy.LogEntry:` |
| `internal/tui/tabs/logs.go` | 77-82 | Remove `case proxy.LogEntry:` |

### Institutional Learnings

No relevant `docs/solutions/` entries for logging.

## Key Technical Decisions

- **`SetPublisher` over `InitDaemon`**: Keep `logger.Init()` signature unchanged for backward compatibility. Add `logger.SetPublisher(func(Entry))` — daemon calls it after init, client never does. Simpler than dueling `sync.Once` init paths.

- **Remove `LogChannel()` from `SlotService`**: The daemon-side `SlotManager` no longer exposes a log channel (slot logs go through `logger`). The client-side `daemonSlotService` exposes `LogChannel()` as a concrete method, not through the interface. The TUI receives the log channel as a separate constructor parameter.

- **`readLogs()` calls `logger.Info` directly**: Replace `c.logCh <- LogEntry{Level: "info", Message: line}` with `logger.Info("[slot%d] %s", slotID, line)`. Mihomo's own log level parsing is deferred (all lines currently mapped to "info" anyway).

- **Two TUI listeners preserved with wrapper type**: `listenSlotLogs` (daemon SSE → `DaemonLogMsg{logger.Entry}`) and `listenAppLogs` (local `logger.Channel()` → `logger.Entry`) remain separate. A thin `DaemonLogMsg` wrapper struct distinguishes daemon-sourced entries from local ones in the Update handler, ensuring correct listener re-arming.

- **`revision` stays daemon-injected**: Not a field on `logger.Entry`. Injected by `handleLogStream` at publish time from `Server.revision`.

## Implementation Units

- [ ] **Unit 1: Extend logger package with SSE publisher**

**Goal:** Add optional publisher callback to the logger so daemon-mode processes can forward every log entry to SSE.

**Requirements:** R4, R5, R6

**Dependencies:** None

**Files:**
- Modify: `internal/logger/logger.go`
- Test: `internal/logger/logger_test.go` (new)

**Approach:**
- Add `Publisher` type: `type Publisher func(Entry)`
- Add `publisher Publisher` field to `Logger` struct
- Add `SetPublisher(p Publisher)` function — sets `defaultLogger.publisher` if logger is initialized
- Add `ClearPublisher()` function — sets `defaultLogger.publisher` to nil; called during daemon shutdown to release hub references
- In `Log()`, after `writeToFile` and channel push, launch `defaultLogger.publisher(entry)` in a goroutine when non-nil, wrapped with `defer/recover` to prevent publisher panics from crashing the caller
- Publisher implementations must be safe for concurrent use (the hub's `publish()` acquires a mutex and uses non-blocking channel sends)

**Patterns to follow:**
- Existing `SetLevel()` in `logger.go:99-105` — same pattern of nil-guard + mutex-protected setter
- Existing `Log()` in `logger.go:116-143` — three-phase write (file → channel → publisher)

**Test scenarios:**
- Happy path: `SetPublisher(fn)` → `Info("test")` → publisher receives `Entry{Level: LevelInfo, Message: "test"}`
- Happy path: `Init()` without `SetPublisher` → `Info("test")` → no panic, file written, channel receives entry
- Edge case: `SetPublisher(nil)` after a non-nil publisher was set → subsequent logs don't call the old publisher
- Edge case: `SetPublisher` before `Init` → no-op (no logger to attach to)

**Verification:**
- `logger_test.go` passes with `go test ./internal/logger/`
- Existing behavior for client mode (no SetPublisher call) is unchanged

---

- [ ] **Unit 2: Adapt proxy layer — remove old log infrastructure**

**Goal:** Delete `proxy.LogEntry` type, remove all per-controller log channels, and route all slot/mihomo logs through `logger.Info/Warn/Error`.

**Requirements:** R1, R7, R8

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/proxy/controller.go`
- Modify: `internal/proxy/slot_controller.go`
- Modify: `internal/proxy/slot_manager.go`
- Modify: `internal/proxy/slot_service.go`
- Test: `internal/proxy/slot_controller_test.go`
- Test: `internal/proxy/slot_manager_test.go`

**Approach:**
- Remove `LogEntry` struct definition from `controller.go:84-88`
- Remove `logCh` field from `Controller` struct
- Remove `Controller.LogChannel()` method
- Rewrite `Controller.readLogs()`: replace `c.logCh <- LogEntry{...}` with `logger.Info("[slot%d] %s", slotID, line)` — add `slotID` parameter
- Rewrite all call sites that write to `c.logCh` or `sm.logCh` to call `logger.Info/Warn/Error` instead: 23 sites in `controller.go`, 6 sites in `slot_controller.go`, and 2 sites in `slot_manager.go`. These are lifecycle events (start, stop, error, mode change). Messages get `[slotN]` prefix via the controller's slotID.
- Remove `SlotController.LogChannel()` delegation method
- Remove `SlotManager.logCh` field and `SlotManager.LogChannel()` method
- Remove `SlotManager.forwardLogs()` — no longer needed since slot controllers publish directly to logger
- Remove `LogChannel()` from `SlotService` interface in `slot_service.go`

**Patterns to follow:**
- Controller lifecycle log call sites already call `c.logCh <- LogEntry{Level: "warn", Message: ...}` — replace with `logger.Warn("[slot%d] ...", c.slotID, ...)`
- The `readLogs()` scanner loop stays; only the channel write changes to a `logger.Info` call

**Test scenarios:**
- Happy path: `Controller.StartSlotWithMode()` with valid config → `logger.Channel()` receives lifecycle log entries with `[slotN]` prefix
- Happy path: mihomo stdout produces a line → `logger.Channel()` receives `[slotN] <line>` as INFO
- Edge case: mihomo stderr produces a line → logged as WARN level
- Edge case: logger channel full → log dropped (non-blocking), no goroutine leak
- Verification: `proxy.LogEntry` type no longer referenced anywhere in `internal/proxy/`

**Verification:**
- `go build ./internal/proxy/` succeeds
- `go test ./internal/proxy/` passes (updated tests)
- Grep for `proxy.LogEntry` across codebase returns zero results in non-docs files
- Note: `internal/tui/` and `internal/app/` will fail to compile after this unit because `SlotService.LogChannel()` is removed and `proxy.LogEntry` no longer exists. This is expected — resolved in Units 4-5.

---

- [ ] **Unit 3: Update daemon SSE hub and server wiring**

**Goal:** Change the SSE hub to publish `logger.Entry`, update SSE data format to include timestamp, and wire the logger publisher to the hub.

**Requirements:** R2, R6, R9, R10, R11

**Dependencies:** Unit 1, Unit 2

**Files:**
- Modify: `internal/daemon/log_stream.go`
- Modify: `internal/daemon/server.go`
- Modify: `internal/daemon/types.go`
- Test: `internal/daemon/server_test.go`

**Approach:**
- In `log_stream.go`: change all `proxy.LogEntry` references to `logger.Entry` — `subscribe()` signature, `publish()` signature, subscriber map type, channel types
- In `handleLogStream`: update the `send("log", ...)` payload to include `"time": entry.Time.Format(time.RFC3339Nano)`. Level is already `entry.Level.String()` ("INFO"/"WARN"/"ERROR"/"DEBUG")
- In `server.go`: remove `startLogFanout()` method — the goroutine that bridged `SlotManager.LogChannel()` → hub is no longer needed
- In `server.go`: add `initLogPublisher()` method that calls `logger.SetPublisher(func(entry logger.Entry) { s.logHub.publish(entry) })` — called during `Start()` after `logger.Init()`
- In `server.go`: add `logger.ClearPublisher()` call in `Stop()` to release hub references on shutdown
- In `server.go`: remove `mux.HandleFunc("/logs/stream", ...)` (endpoint stays, just no code change needed for the route itself — but verify handler still works)
- In `types.go`: remove `LogEvent` struct (no longer used — was only referenced by the old architecture)

**Patterns to follow:**
- Existing `handleLogStream` SSE event loop structure (lines 99-115) stays — only the data payload changes
- Existing `s.revision.Load()` injection pattern stays

**Test scenarios:**
- Happy path: SSE client connects → receives `event: ready` with revision and api_version → receives `event: log` with `{time, level, message, revision}` for daemon lifecycle logs
- Happy path: SSE client receives slot log with `[slot0]` prefix in message
- Edge case: SSE client connects before any slots start → only receives daemon-level logs
- Edge case: Multiple SSE clients → both receive the same log entries (fanout)
- Error path: Client disconnects → subscriber cleaned up, no goroutine leak

**Verification:**
- `go build ./internal/daemon/` succeeds
- `go test ./internal/daemon/` passes
- Manual: start c4cd, `curl -N http://127.0.0.1:47189/logs/stream` shows daemon startup logs with timestamps

---

- [ ] **Unit 4: Update daemon SSE client**

**Goal:** Wire `logger.SetPublisher` in c4cd's main, update `daemon.Client.StreamLogs()` return type.

**Requirements:** R5, R12

**Dependencies:** Unit 3

**Files:**
- Modify: `cmd/c4cd/main.go`
- Modify: `internal/daemon/client.go`

**Approach:**
- In `cmd/c4cd/main.go`: after `logger.Init()`, call `logger.SetPublisher(...)` — but the server/hub isn't created yet. Move `logger.SetPublisher` call to inside `server.Start()` (done in Unit 3). c4cd's main.go doesn't need changes for this wiring.
- Wait — re-examine. c4cd creates `daemon.NewServer()` then calls `server.Start(ctx)`. Unit 3 puts `SetPublisher` in `Start()`. So c4cd main.go only needs a comment change at most. No code changes required.
- Actually: double-check that `logger.Init()` is called in `cmd/c4cd/main.go:46` before `daemon.NewServer()`. Yes. Good.
- In `client.go`: change `StreamLogs()` return type from `<-chan proxy.LogEntry` to `<-chan logger.Entry`
- Update SSE parsing goroutine to construct `logger.Entry` with parsed `time` field and `logger.Level` from the string level
- SSE data now includes `"time"` field — parse it with `time.Parse(time.RFC3339Nano, ...)`
- SSE data `"level"` is now `logger.Level.String()` format — parse with a switch/helper

**Patterns to follow:**
- Existing `StreamLogs()` SSE parsing loop in `client.go:119-143` — same scanner/bufio pattern

**Test scenarios:**
- Happy path: `StreamLogs(ctx)` → channel receives `logger.Entry` with Time, Level, Message populated
- Happy path: SSE `ready` event parsed and skipped (not a log event)
- Edge case: SSE connection drops → channel closed, stop function works
- Edge case: Malformed JSON in `data:` line → skipped silently

**Verification:**
- `go build ./cmd/c4cd/` succeeds
- `go build ./internal/daemon/` succeeds
- `go test ./internal/daemon/` passes

---

- [ ] **Unit 5: Update TUI client side**

**Goal:** Update `daemonSlotService` and TUI model to use unified `logger.Entry` type, remove `proxy.LogEntry` handling.

**Requirements:** R13, R14, R15

**Dependencies:** Unit 4

**Files:**
- Modify: `internal/app/daemon_slot_service.go`
- Modify: `internal/app/app.go`
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/tabs/logs.go`

**Approach:**
- `daemon_slot_service.go`:
  - Remove `logCh` field, `stopLog` field, and the SSE bridge goroutine (it was a 1:1 pass-through from `client.StreamLogs()` to `svc.logCh` with no transformation)
  - `LogChannel()` is no longer needed — the TUI consumes directly from the SSE channel
- `app.go`:
  - Call `client.StreamLogs(context.Background())` directly and pass the returned `<-chan logger.Entry` to `tui.New()`
  - Store the `stopLog` cleanup function on `App` and call it in `App.Cleanup()`
- `tui.go`:
  - Add `daemonLogCh <-chan logger.Entry` field to Model
  - Add `DaemonLogMsg` wrapper struct: `type DaemonLogMsg struct { Entry logger.Entry }` — implements `tea.Msg`
  - Update `New()` constructor signature: add `daemonLogCh <-chan logger.Entry` as the 7th parameter
  - `listenSlotLogs()`: read from `m.daemonLogCh`, wrap each entry as `DaemonLogMsg{entry}` before returning
  - Remove `case proxy.LogEntry:` from `Update()` handler
  - Add `case DaemonLogMsg:` to `Update()` handler — forwards to `m.logsTab.AddLog(msg.Entry.Level.String(), msg.Entry.Message)` and re-arms `m.listenSlotLogs()`
  - Keep `case logger.Entry:` handler for local app logs — re-arms `m.listenAppLogs()`
- `tabs/logs.go`:
  - Remove `case proxy.LogEntry:` from `Update()` handler (lines 77-82)
  - `logger.Entry` case is not needed in LogsTab directly — it's handled in the TUI model's Update which calls `m.logsTab.AddLog(msg.Level.String(), msg.Message)`
  - Wait — LogsTab also has a `case proxy.LogEntry:` handler. Remove it. The TUI model already intercepts `proxy.LogEntry` before forwarding to LogsTab. After unification, the TUI model handles `logger.Entry` from both sources.

**Patterns to follow:**
- Existing `listenSlotLogs()` pattern in `tui.go:1189-1197`
- Existing `listenAppLogs()` pattern in `tui.go:1199-1209`

**Test scenarios:**
- Happy path: TUI starts → `logger.Entry` from daemon SSE appears in log panel
- Happy path: TUI starts → `logger.Entry` from local logger appears in log panel
- Happy path: Both daemon and local logs interleaved in log panel
- Edge case: Daemon SSE connection fails → local logs still appear
- Edge case: Log panel at max capacity (1000 entries) → oldest entries trimmed

**Verification:**
- `go build ./cmd/c4c/` succeeds
- `go build ./internal/app/` succeeds
- `go build ./internal/tui/` succeeds
- Manual: start c4cd, then c4c, press `4` for logs tab → see daemon startup logs

---

- [ ] **Unit 6: Cleanup and verification**

**Goal:** Remove all remaining `proxy.LogEntry` references, update tests, verify full build and test suite.

**Requirements:** R1 (complete removal), success criteria

**Dependencies:** Unit 5

**Files:**
- Modify: `internal/daemon/types.go` (remove `LogEvent` — if not done in Unit 3)
- Modify: `internal/proxy/slot_controller_test.go`
- Modify: `internal/proxy/slot_manager_test.go`
- Modify: `internal/daemon/server_test.go`
- Modify: `internal/daemon/lifecycle_test.go`

**Approach:**
- Grep for `proxy.LogEntry` across entire codebase (excluding docs/) — all remaining references must be fixed or removed
- Grep for `LogEntry` in `internal/proxy/` — the type should be gone
- Update test files: replace any `proxy.LogEntry{}` literals with `logger.Entry{}`
- For tests that verify log channel behavior: update to read from `logger.Channel()` instead
- Run `make test` and fix any failures
- Manual smoke test: `c4cd` running + `c4c` connected → logs tab shows daemon + slot logs

**Test scenarios:**
- Verification: `grep -r "proxy.LogEntry" --include="*.go" .` returns zero results
- Verification: `make test` passes with all tests
- Verification: `go build ./...` succeeds for all packages

**Verification:**
- `make test` passes
- Zero `proxy.LogEntry` references in Go source files

## System-Wide Impact

- **Interaction graph:** `logger.Log()` → publisher callback → `logStreamHub.publish()` → SSE subscribers. Slot controllers no longer own channels. TUI receives unified type from two channels (local + remote).

- **Error propagation:** Logger publisher failures are silent (fire-and-forget). SSE write failures trigger client disconnect → subscriber cleanup. No error flows backward from SSE to logger.

- **State lifecycle risks:** `logger.SetPublisher` is called once during server startup. If called multiple times (e.g., server restart), it overwrites the previous publisher — old hub subscribers may leak if not properly cleaned up.

- **API surface parity:** The `SlotService` interface loses `LogChannel()`. Only `daemonSlotService` (client-side) exposes it as a concrete method. No other `SlotService` implementations are affected.

- **Integration coverage:** The SSE ↔ TUI path (`daemon.Client.StreamLogs()` → `daemonSlotService` bridge goroutine → TUI channel → Update handler → LogsTab) spans 5 components. Unit tests cover each individually; manual testing covers the full chain.

- **Unchanged invariants:** Log file format, log file path, SSE endpoint path (`/logs/stream`), `c4cgo` binary behavior, and `logger.Channel()` API are all preserved.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `SlotService` interface change breaks unknown external consumers | `SlotService` is internal-only (no external SDK consumers). Grep confirms all implementors are in `internal/`. |
| Race between `logger.SetPublisher` and early log calls in daemon startup | Publisher is set in `Server.Start()` before any slots are started or requests served. Early `logger.Info` calls (config loading) go to file only, which is acceptable. |
| ~30 call sites in `controller.go` need rewriting to use `logger` | Each call site is a simple replacement: `c.logCh <- LogEntry{Level: "warn", Message: "..."}` → `logger.Warn("[slot%d] ...", c.slotID, ...)`. Systematic grep-and-replace. |

## Sources & References

- **Origin document:** [docs/brainstorms/2026-05-04-unified-log-sse-requirements.md](../brainstorms/2026-05-04-unified-log-sse-requirements.md)
- Related code: `internal/logger/logger.go`, `internal/proxy/controller.go`, `internal/daemon/log_stream.go`
- Related plan: [docs/plans/2026-04-29-001-refactor-c4cd-daemon-first-plan.md](2026-04-29-001-refactor-c4cd-daemon-first-plan.md)
