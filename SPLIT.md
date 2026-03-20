# Daemon+Client 架构重构方案

## Context

当前 c4c 是单体架构：TUI 进程直接持有 `SlotManager`，管理 mihomo 代理进程。退出 TUI 就丢失控制。重构为 daemon+client 架构后：
- 代理进程由 daemon 持久管理，TUI 退出不影响代理
- 统一 HTTP REST API，TUI/c4cgo/未来 Web UI 共用同一套接口
- daemon 额外内存开销约 5-10 MB（mihomo 进程本身不变）

## 架构总览

```
c4cd (daemon)                    c4c (TUI client)         c4cgo (CLI client)
┌──────────────────┐            ┌──────────────┐         ┌──────────────┐
│ SlotManager      │◀── HTTP ──▶│ daemon.Client│         │ daemon.Client│
│ Controller ×10   │   REST     │ Bubble Tea   │         │ (或 proxy.lock)│
│ HTTP Server      │   Unix     └──────────────┘         └──────────────┘
│ Log Broadcaster  │   Socket
└──────────────────┘
```

## Phase 0: 提取 SlotService 接口（无行为变更）

**目的**：在不改变任何行为的前提下，让 TUI 依赖接口而非具体类型，为后续替换做准备。

### 0.1 新建接口文件

**新建** `internal/proxy/slot_service.go`

```go
type SlotService interface {
    StartSlot(slotID int, subID string, nodes []subscription.Node, selected *subscription.Node) error
    StopSlot(slotID int) error
    SetSlotMode(slotID int, mode string) error
    CycleSlotMode(slotID int) (string, error)
    GetSlotState(slotID int) *SlotState
    ActiveSlots() []*SlotState
    ActiveSlotCount() int
    NodeSlot(nodeName string) (int, bool)
    GetTrafficForSlot(slotID int) (int64, int64, error)
    GetCoreVersion(slotID int) (string, error)
    GetRulesCount(slotID int) (int, error)
    GetStartedAt(slotID int) (string, error)
    Slot0Port() int
    StopAll()
    LogChannel() <-chan LogEntry
}
```

`*SlotManager` 已经满足此接口，无需额外代码。

### 0.2 TUI 改用接口

**修改** `internal/tui/tui.go`:
- L58: `slotMgr *proxy.SlotManager` → `slotMgr proxy.SlotService`
- L95: 构造函数参数类型同步改

**修改** `internal/tui/tabs/settings.go`:
- L75: `slotMgr *proxy.SlotManager` → `slotMgr proxy.SlotService`
- L136: 构造函数参数类型同步改

### 0.3 初始化逻辑上移到 app.go

**修改** `internal/app/app.go`:
- 在 `New()` 中调用 `slotMgr.SetSaveHistoryFunc(...)` 和 `slotMgr.SyncFromLockFile()`（原来在 tui.go 里）
- `Run()` 中把 `slotMgr` 作为 `proxy.SlotService` 传给 `tui.New()`

---

## Phase 1: Daemon 二进制 + HTTP API Server

### 1.1 daemon 入口

**新建** `cmd/c4cd/main.go`

功能：
- 加载 `config.Config`，创建 `SlotManager`
- 调用 `SyncFromLockFile()` 恢复运行中的代理
- 设置 `SetSaveHistoryFunc` 直接写 config
- 在 Unix socket (`~/.clash4cli/daemon.sock`) 上启动 HTTP server
- 写 PID 文件 (`~/.clash4cli/daemon.pid`)
- SIGTERM/SIGINT 优雅退出（StopAll、删 socket、删 PID 文件）

### 1.2 HTTP API 设计

**新建** `internal/daemon/server.go` — Server 结构体 + 路由注册
**新建** `internal/daemon/handlers.go` — 各端点 handler 实现
**新建** `internal/daemon/types.go` — 请求/响应 JSON 类型

Unix socket 隐式鉴权（文件系统权限），无需 token。

错误响应格式：
```json
{"error": "human-readable message"}
```

**Slot 管理：**

| 方法 | 路径 | 请求体 | 响应 |
|------|------|--------|------|
| `GET` | `/api/v1/slots` | - | `{"slots": [SlotState...]}` |
| `GET` | `/api/v1/slots/:id` | - | `SlotState` 或 `null` |
| `POST` | `/api/v1/slots/:id/start` | `{"sub_id", "nodes", "selected_node"}` | `204` |
| `POST` | `/api/v1/slots/:id/stop` | - | `204` |
| `PUT` | `/api/v1/slots/:id/mode` | `{"mode": "rule\|global"}` | `{"mode": "..."}` |
| `POST` | `/api/v1/slots/:id/cycle` | - | `{"mode": "...", "stopped": bool}` |
| `POST` | `/api/v1/slots/stop-all` | - | `204` |

**Slot 信息：**

| 方法 | 路径 | 响应 |
|------|------|------|
| `GET` | `/api/v1/slots/:id/traffic` | `{"upload": int64, "download": int64}` |
| `GET` | `/api/v1/slots/:id/version` | `{"version": "..."}` |
| `GET` | `/api/v1/slots/:id/rules` | `{"count": int}` |
| `GET` | `/api/v1/slots/:id/started-at` | `{"started_at": "RFC3339"}` |

**系统：**

| 方法 | 路径 | 响应 |
|------|------|------|
| `GET` | `/api/v1/status` | `{"active_count", "slot0_port", "daemon_version"}` |
| `GET` | `/api/v1/node-slot/:name` | `{"slot_id": int, "found": bool}` |
| `GET` | `/api/v1/logs` | SSE 流: `data: {"level", "message"}\n\n` |

### 1.3 日志广播器

**新建** `internal/daemon/log_broadcaster.go`

```go
type logBroadcaster struct {
    mu      sync.RWMutex
    clients map[int]chan proxy.LogEntry
    nextID  int
}

func newLogBroadcaster(logCh <-chan proxy.LogEntry) *logBroadcaster
func (b *logBroadcaster) Subscribe() (id int, ch <-chan proxy.LogEntry)
func (b *logBroadcaster) Unsubscribe(id int)
```

从 `SlotManager.LogChannel()` 读取日志，扇出到所有 SSE 客户端。断连时通过 context 取消自动移除。

### 1.4 Server 实现

```go
type Server struct {
    slotMgr      *proxy.SlotManager
    cfg          *config.Config
    listener     net.Listener
    socketPath   string
    version      string
    logBroadcast *logBroadcaster
}

func NewServer(slotMgr *proxy.SlotManager, cfg *config.Config, socketPath, version string) *Server
func (s *Server) ListenAndServe() error
func (s *Server) Shutdown(ctx context.Context) error
```

使用标准库 `net/http` + `http.ServeMux`，路径参数用 `strings.TrimPrefix` 解析。

### 1.5 路径辅助函数

**修改** `internal/config/paths.go` — 新增：
- `GetDaemonSocketPath()` → `~/.clash4cli/daemon.sock`
- `GetDaemonPIDPath()` → `~/.clash4cli/daemon.pid`

---

## Phase 2: Client 库

**新建** `internal/daemon/client.go`

实现 `proxy.SlotService` 接口，通过 HTTP 调用 daemon。使用标准库 `net/http` + Unix socket transport：

```go
type Client struct {
    httpClient *http.Client
    socketPath string
}

func NewClient(socketPath string) *Client
func (c *Client) Ping() error  // GET /api/v1/status
```

Unix socket transport:
```go
transport := &http.Transport{
    DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
        return net.Dial("unix", socketPath)
    },
}
httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
```

所有请求发往 `http://daemon/api/v1/...`（host 在 Unix socket 下无意义）。

每个 `SlotService` 方法对应一个 HTTP 请求（见 Phase 1.2 表格）。

**新建** `internal/daemon/client_sse.go`

`LogChannel()` 实现：SSE 客户端，连接 `/api/v1/logs`，解码日志推入 channel。断连指数退避重连，channel buffer 100。

---

## Phase 3: 自动拉起 daemon

**新建** `internal/daemon/spawn.go`

```go
func EnsureDaemon(socketPath string) (*Client, error)
```

流程：
1. 尝试连接 socket → `Ping()` 成功则返回 Client
2. 连接失败 → 找 `c4cd` 二进制（与 `c4c` 同目录，或 `$PATH`）
3. `exec.Command` 后台启动，`SysProcAttr.Setsid = true`，stdout/stderr 重定向到 `daemon.log`
4. 轮询 socket（最多 3 秒，100ms 间隔）等待就绪
5. 超时返回错误

**僵尸 socket 检测**：
- 读 `daemon.pid`，检查进程是否存活
- 进程不存在 → 删除 `daemon.sock` 和 `daemon.pid` → 重新启动

---

## Phase 4: TUI 重构

**修改** `internal/app/app.go`:
- `slotMgr *proxy.SlotManager` → `daemonClient *daemon.Client`
- `New()` 中调用 `daemon.EnsureDaemon()` 获取 client
- 移除 `SetSaveHistoryFunc` 和 `SyncFromLockFile`（daemon 自己处理）
- `ProxyProvider` 闭包改用 `daemonClient`：
  ```go
  subManager.SetProxyProvider(func() (*url.URL, error) {
      state := daemonClient.GetSlotState(0)
      if state == nil {
          slots := daemonClient.ActiveSlots()
          if len(slots) > 0 { state = slots[0] }
      }
      if state == nil || state.Port <= 0 {
          return nil, nil
      }
      return &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Port))}, nil
  })
  ```
- `Cleanup()` 只关闭 client，**不** 停止 daemon

**`tui.go` 无需逐行改动**：因为 Phase 0 已切到接口，`slotMgr` 字段类型是 `proxy.SlotService`，传入 `*daemon.Client` 即可。以下 ~30 个调用点自动走 HTTP：

- `slotMgr.NodeSlot(...)`, `GetSlotState(...)`, `StartSlot(...)`, `CycleSlotMode(...)`
- `StopSlot(...)`, `ActiveSlots()`, `GetTrafficForSlot(...)`, `ActiveSlotCount()`
- `Slot0Port()`, `LogChannel()`, `StopAll()`

---

## Phase 5: 状态持久化职责划分

| 文件 | 写入方 | 说明 |
|------|--------|------|
| `config.yaml` | TUI（UI 设置）+ daemon（slot history） | 分段写，不冲突 |
| `proxy.lock` | daemon（SlotManager） | 不变 |
| `daemon.sock` | daemon | 新增 |
| `daemon.pid` | daemon | 新增 |

Daemon 中 `SetSaveHistoryFunc` 直接调 `config.Save()` 持久化 slot history。TUI 只写 UI 相关设置（语言、端口、显示模式等）。两者写 config.yaml 的 key 不重叠，不会冲突。

---

## Phase 6: c4cgo 改动

**基本无需改动**。c4cgo 当前读 `proxy.lock` + 直接调 mihomo API，这些机制不变。

**可选优化**（后续迭代）：c4cgo 也通过 daemon API (`GET /api/v1/slots`) 获取 slot 列表，减少对 lock 文件的依赖。

---

## Phase 7: 构建系统

**修改** `Makefile`:

```makefile
DAEMON_NAME := c4cd

build:
    # ...existing c4c + c4cgo...
    CGO_ENABLED=$(CGO_ENABLED) $(GOENV) go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(DAEMON_NAME) ./cmd/c4cd

# build-target 中同步增加 c4cd 交叉编译：
    GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GOENV) go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(OUTPUT_DIR)/$(DAEMON_NAME)$(EXT) ./cmd/c4cd

clean:
    @rm -f $(BINARY_NAME) $(RGO_NAME) $(DAEMON_NAME)

install: build
    sudo cp $(DAEMON_NAME) /usr/local/bin/
```

---

## 新增/修改文件汇总

**新建（~12 个文件）：**

| 文件 | 用途 |
|------|------|
| `cmd/c4cd/main.go` | daemon 入口 |
| `internal/proxy/slot_service.go` | SlotService 接口 |
| `internal/daemon/server.go` | HTTP server |
| `internal/daemon/handlers.go` | API handlers |
| `internal/daemon/types.go` | JSON 类型定义 |
| `internal/daemon/log_broadcaster.go` | 日志广播 |
| `internal/daemon/client.go` | HTTP client（实现 SlotService） |
| `internal/daemon/client_sse.go` | SSE 日志客户端 |
| `internal/daemon/spawn.go` | 自动拉起逻辑 |
| `internal/daemon/server_test.go` | server 测试 |
| `internal/daemon/client_test.go` | client 测试 |
| `internal/daemon/spawn_test.go` | spawn 测试 |

**修改（~5 个文件）：**

| 文件 | 改动 |
|------|------|
| `internal/tui/tui.go` | 字段类型改为接口，移除 history 回调 |
| `internal/tui/tabs/settings.go` | 字段类型改为接口 |
| `internal/app/app.go` | 用 daemon.Client 替代 SlotManager |
| `internal/config/paths.go` | 新增 socket/PID 路径函数 |
| `Makefile` | 新增 c4cd 构建目标 |

---

## 实施顺序

```
Phase 0 → Phase 1 + Phase 7 → Phase 2 → Phase 3 → Phase 4 + Phase 5
```

Phase 0 是纯重构，可以先合并确保不破坏现有功能。后续 Phase 可以按顺序推进。

---

## 验证方案

1. **Phase 0 后**：`make test` 全通过，`make build && ./c4c` 正常运行
2. **Phase 1 后**：`make build`，手动启动 `./c4cd`，用 curl 验证：
   ```bash
   curl --unix-socket ~/.clash4cli/daemon.sock http://daemon/api/v1/status
   ```
3. **Phase 3 后**：启动 `./c4c`，确认自动拉起 daemon，所有 TUI 功能正常
4. **全部完成后**：
   - 退出 TUI → 代理仍在运行 → 重新打开 TUI → 恢复状态
   - `c4cgo` 仍可正常使用
   - `make test` 全通过

---

## 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| HTTP 延迟 | Unix socket 延迟 <1ms，不影响体验 |
| daemon 崩溃丢失 mihomo 进程 | `SyncFromLockFile()` 可重新接管，已有机制 |
| config.yaml 写冲突 | TUI 和 daemon 写不同 key，不会互相覆盖 |
| 僵尸 socket 文件 | PID 文件检测 + 自动清理 |
| 用户未构建 c4cd | 可选：EnsureDaemon 失败时回退到本地 SlotManager（兼容模式） |
