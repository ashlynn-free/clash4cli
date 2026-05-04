---
date: 2026-05-04
topic: unified-log-sse
---

# 统一日志系统 & SSE 日志流

## Problem Frame

当前日志系统存在三套割裂的类型和通道：

- `logger.Entry`（int level + Time）— 应用级日志，写文件 + 内部 channel，仅供 c4c TUI 本地消费
- `proxy.LogEntry`（string level，无 Time）— slot/mihomo 日志，走 `SlotManager.LogChannel()` → SSE
- `/logs/stream` SSE 端点只发布 `proxy.LogEntry`，c4cd 自身 `logger.Info/Warn/Error` 无法到达 SSE 客户端

结果是 c4c 的日志面板需要同时监听两个不同来源、两种不同类型，而 daemon 自身的运行日志对 SSE 客户端不可见。

## Data Flow

```mermaid
flowchart TB
  D[logger.Info/Warn/Error] --> L[logger.Logger]
  M[mihomo stdout/stderr] --> SC[SlotController.readLogs]
  SC --> D
  L --> F[log file]
  L --> C[internal channel]
  L --> P[SSE publisher callback<br/>daemon mode only]
  P --> H[logStreamHub]
  H --> SSE[/logs/stream SSE]
  C --> TUI[LogsTab<br/>local logs]
  SSE --> DSC[daemonSlotService]
  DSC --> TUI
```

## Requirements

**类型统一**
- R1. 以 `logger.Entry`（Time + Level + Message）作为全项目唯一的日志条目类型，删除 `proxy.LogEntry`
- R2. SSE `log` 事件数据包含 `logger.Entry` 的全部字段（`time` RFC3339、`level`、`message`）以及 daemon 服务端注入的 `revision`（uint64 状态计数器，由 `Server.revision` 在发布时注入）

**来源区分**
- R3. 日志来源（daemon / slotN）通过 message 前缀区分，例如 `[daemon] server started`、`[slot0] mihomo connected`，不引入额外的 source 字段

**logger 包改造**
- R4. `logger.Init` 支持区分 daemon 和 client 两种初始化路径，daemon 路径额外注册 SSE 发布回调
- R5. c4cd 启动时以 daemon 模式初始化 logger，c4c 以 client 模式初始化，各自调用不同的 `Init` 变体
- R6. logger 包在 daemon 模式下，每条日志同时：写入日志文件 + 推送到内部 channel + 调用 SSE 发布回调

**slot/mihomo 日志统一**
- R7. slot controller 不再持有独立的 `logCh`，改为直接调用 `logger.Info/Warn/Error`，消息带 `[slotN]` 前缀
- R8. `SlotManager.forwardLogs()` 移除，`SlotController.LogChannel()` 移除，`SlotManager.LogChannel()` 移除

**SSE 端点**
- R9. SSE 端点 `/logs/stream` 同时承载 daemon 日志和 slot/mihomo 日志，均为 `event: log`，数据格式统一为 `logger.Entry`
- R10. SSE 连接建立时不发送历史日志（当前已满足，hub fanout 无缓冲）
- R11. `logStreamHub` 的发布/订阅类型从 `proxy.LogEntry` 改为 `logger.Entry`

**c4c 客户端**
- R12. `daemon.Client.StreamLogs()` 返回 `<-chan logger.Entry`
- R13. `daemonSlotService.LogChannel()` 返回 `<-chan logger.Entry`
- R14. c4c TUI 日志面板展示两类日志：通过 SSE 接收的 daemon/slot 日志 + 本进程 `logger.Channel()` 的本地日志
- R15. 日志面板不再需要处理 `proxy.LogEntry` 类型

## Success Criteria
- c4cd 自身的 `logger.Info/Warn/Error` 调用在 c4c 日志面板中可见
- 日志面板中 daemon、slot、本地日志均有清晰来源标识（前缀）
- `proxy.LogEntry` 类型从代码库中完全移除

## Scope Boundaries
- 不改变日志文件写入行为（文件名、格式、路径不变）
- 不引入新的日志级别
- 不改变 SSE 端点路径（仍为 `/logs/stream`）
- 不影响 `c4cgo` 的功能

## Key Decisions
- **统一到 `logger.Entry`**：删除 `proxy.LogEntry`，全项目使用 `{Time, Level, Message}` 三元组
- **前缀区分来源**：不引入 source 字段，用 `[daemon]` / `[slotN]` 前缀自然区分
- **logger.Init 变体区分环境**：daemon 路径注册 SSE 回调，client 路径不注册
- **slot 日志走 logger 包**：`SlotController` 直接调用 `logger.Info` 等，不再有独立 logCh
- **SSE 不传历史**：维持当前行为，新连接只看到连接后的日志

## Dependencies / Assumptions
- `logger` 包目前不依赖 daemon/proxy，引入 SSE 回调机制不会产生循环依赖
- `proxy` 包新增对 `logger` 的 import 是安全的（logger 是 leaf 包）

## Outstanding Questions

### Deferred to Planning
- [Affects R4][Technical] `logger.Init` 的具体函数签名设计（functional options vs 多个 Init 变体）
- [Affects R7][Technical] `SlotController.readLogs()` 中 mihomo stdout/stderr 的逐行解析如何适配 `logger` 调用
- [Affects R14][Technical] TUI 中本地 log channel 和远程 SSE channel 的合并策略（两个 goroutine 各自监听 vs 统一 fan-in）
