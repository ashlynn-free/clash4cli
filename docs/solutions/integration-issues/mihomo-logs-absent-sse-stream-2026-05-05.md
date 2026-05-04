---
title: "Mihomo process output not reaching SSE log stream"
date: 2026-05-05
category: integration-issues
module: proxy
problem_type: integration_issue
component: tooling
symptoms:
  - "c4c TUI logs tab shows daemon lifecycle events but no mihomo stdout/stderr"
  - "mihomo output visible in log file but not in SSE /logs/stream"
root_cause: config_error
resolution_type: code_fix
severity: high
tags:
  - sse
  - mihomo
  - stdout-pipe
  - http-timeout
---

# Mihomo process output not reaching SSE log stream

## Problem

After unifying log types (removing `proxy.LogEntry`, routing everything through `logger.Entry`), mihomo's actual process stdout/stderr output was not appearing in the SSE `/logs/stream` endpoint or the TUI logs panel. Only slot lifecycle events (start/stop/mode-change) were visible.

## Symptoms

- TUI log panel shows lifecycle events (`[slot0] mihomo started (PID: 1234)`) but no mihomo runtime output
- Mihomo log file (`mihomo-slot-0.log`) has the expected content
- `grep` of SSE stream shows no `[mihomo]` prefixed entries

## What Didn't Work

1. **Unifying log types alone** — Simply changing `c.logCh <- LogEntry{...}` to `logger.Info(...)` only fixed lifecycle events. Mihomo process output always went directly to a log file via `cmd.Stdout = logFile`, never through a pipe or the logger.

2. **Using `io.Pipe` with `io.MultiWriter`** — First attempt used `io.MultiWriter(logFile, stdoutW)` with a manual `io.Pipe`. This had two bugs:
   - `devNull.Close()` was called **before** `cmd.Start()`, causing `EBADF` on stdin. The child process inherited a closed FD for stdin, which caused `cmd.Start()` to fail silently in non-error-checked paths.
   - Nobody closed the pipe's write end (`stdoutW`) after the child exited, causing the reader goroutine (`pipeMihomoOutput`) to never receive EOF and leak indefinitely.

3. **Setting `cmd.StdoutPipe()` in place of MultiWriter** — Fixed the pipe lifecycle, but without addressing the HTTP timeout, logs only appeared for 5 seconds.

## Solution

Three independent fixes were required, each addressing a different layer:

### Fix 1: Pipe mihomo stdout/stderr through logger in daemon mode

In `internal/proxy/testhooks.go:defaultStartMihomoProcess`, use `cmd.StdoutPipe()` / `cmd.StderrPipe()` when `detachSubprocesses` is false (daemon mode). Each pipe is read by `pipeMihomoToLogAndLogger`, which writes each line to both the log file and `logger.Info("[mihomo] %s", line)`.

```go
// Before: stdout/stderr went directly to log file
cmd.Stdout = logFile
cmd.Stderr = logFile

// After: pipe through logger while still writing to file
stdoutPipe, _ := cmd.StdoutPipe()
stderrPipe, _ := cmd.StderrPipe()
// ... cmd.Start() ...
go pipeMihomoToLogAndLogger(stdoutPipe, logFile)
go pipeMihomoToLogAndLogger(stderrPipe, logFile)
```

Key details:
- `cmd.StdoutPipe()` creates a real OS pipe, sets `cmd.Stdout` to the write end, and returns the read end. Go closes the write end after `Start()`, and the child process inherits the write end. When the child exits, the read end gets EOF naturally — no goroutine leak.
- Pipe ordering: `cmd.Stdin` → set file; `cmd.StdoutPipe()` / `cmd.StderrPipe()` → set pipes. Then `cmd.Start()`. The parent closes `devNull` **after** `Start()`.
- `pipeMihomoToLogAndLogger` writes each line to both the log file and `logger.Info`, then closes both when the pipe reader reaches EOF.

### Fix 2: Remove 5-second HTTP timeout on SSE client

In `internal/daemon/client.go:StreamLogs`, the shared `http.Client` had `Timeout: 5 * time.Second`. This timeout applies to the entire request lifetime, including reading the SSE response body. After 5 seconds, the HTTP client would cancel the connection, killing the SSE stream and closing `daemonLogCh` in the TUI.

```go
// Before: shared client with 5s timeout kills SSE after 5 seconds
resp, err := c.httpClient.Do(req)  // c.httpClient.Timeout = 5s

// After: separate client without timeout for streaming
streamClient := &http.Client{}
resp, err := streamClient.Do(req)
```

The `daemon.Client` keeps its 5-second timeout for request-response API calls (health, runtime, slots). Only SSE streaming uses an independent client without timeout.

### Fix 3: Restore `devNull.Close()` order

The original code pattern was:
```go
cmd.Stdin = devNull
// ... setup ...
if err := cmd.Start(); err != nil { ... }
_ = devNull.Close()  // Close AFTER Start
```

The bug moved the close before `Start()`:
```go
cmd.Stdin = devNull
_ = devNull.Close()  // BUG: closes FD before child inherits it
// ... cmd.Start() will fail with EBADF
```

## Why This Works

- **Fix 1** bridges the gap between mihomo's process-level output and the unified `logger` system. By using OS-level pipes managed by Go's `os/exec`, the child process inherits pipe FDs naturally. When the child writes to stdout/stderr, those bytes flow through the pipe to a scanner that calls `logger.Info`, which then fans out to file, the internal channel, and the SSE publisher callback.
- **Fix 2** addresses the fundamental incompatibility between `http.Client.Timeout` (a wall-clock deadline on the entire request) and SSE streaming (an indefinite-length response body).
- **Fix 3** prevents `EBADF` on stdin by ensuring the `/dev/null` FD is still valid when `cmd.Start()` calls `syscall.Dup()`.

## Prevention

- When spawning child processes with `os/exec`, always close inherited FDs (`devNull`, log file) **after** `cmd.Start()`, not before. The FD must be valid for `syscall.Dup()` during spawn.
- SSE streaming connections must use an HTTP client without `Timeout`. Create a dedicated client or use `context.WithTimeout` on the request context instead.
- When piping child process stdout through a processing goroutine, use `cmd.StdoutPipe()` from `os/exec` rather than manual `io.Pipe`. This avoids goroutine leaks because Go manages the write-end lifecycle.

## Related Issues

- [Related plan: unified log SSE](../brainstorms/2026-05-04-unified-log-sse-requirements.md)
- [Related plan: daemon-first architecture](../plans/2026-04-29-001-refactor-c4cd-daemon-first-plan.md)
