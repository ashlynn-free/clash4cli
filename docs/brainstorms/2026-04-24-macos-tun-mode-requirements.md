---
date: 2026-04-24
topic: macos-tun-mode
---

# macOS TUN Mode

## Problem Frame

clash4cli 的 TUN 模式目前仅支持 Linux（代码中 `runtime.GOOS == "linux"` 硬限制）。但 mihomo 核心本身完全支持 macOS TUN，且 macOS 不需要 root 权限。macOS 用户无法通过 TUI 开启全局透明代理，只能依赖 proxychains（受 SIP 限制）或手动配置系统代理。

## Requirements

**核心功能**
- R1. 移除 TUN 模式的 `runtime.GOOS == "linux"` 限制，允许 macOS 启用 TUN
- R2. macOS 上 TUN 默认使用 `mixed` 栈、启用 `auto-route` 和 `auto-detect-interface`
- R3. macOS 上启用 TUN 时不显示 root 权限警告（与 Linux 不同的提示逻辑，root 检查已在代码中通过 `runtime.GOOS == "linux"` 门控）
- R4. c4cgo 的 proxychains 机制保持独立，不受 TUN 开关影响

**高级选项**
- R5. 用户可在设置中选择 TUN 栈：`mixed`（默认）、`gvisor`、`system`
- R6. 用户可自定义 TUN 相关参数：MTU、设备名称
- R7. 用户可配置 DNS：自定义 DNS 服务器列表、DNS 劫持目标（`dns-hijack`）

**UI**
- R8. macOS 上 TUN 开关在设置中可用（当前灰显禁用）
- R9. macOS 上 Docker TCP 重定向保持禁用（iptables 仅 Linux 可用）

## Success Criteria
- macOS 上可通过 TUI 开启/关闭 TUN 模式
- 开启后所有网络流量透明走代理（全局透明代理）
- 高级选项（栈、MTU、DNS）可配置且生效
- 不影响 c4cgo 的现有 proxychains 功能

## Scope Boundaries
- 不处理 macOS Docker 重定向（iptables 不可用）
- 不自动管理 DNS 配置——DNS 交给订阅配置或用户手动处理
- 不处理 macOS 26.x 特定兼容性问题的 workaround（属于 mihomo 核心范畴）
- 不替代 c4cgo 的 proxychains 功能

## Key Decisions
- **默认栈选 mixed**：mihomo 文档推荐，TCP 走 system、UDP 走 gvisor，性能与兼容性平衡
- **不管理 DNS**：macOS 上 DNS 处理是已知痛点，交给订阅配置或用户手动处理更稳定
- **不需要 root**：与 Linux 不同，macOS 上 mihomo 创建 TUN 设备不需要特权

## Dependencies / Assumptions
- mihomo 核心版本需支持 macOS TUN（release 版本，非 alpha）
- 用户可能需要在 macOS 防火墙中白名单 mihomo

## Outstanding Questions

### Deferred to Planning
- [Affects R5-R7][Technical] 高级选项在 TUI 中的具体布局和交互方式
- [Affects R1][Technical] 确认 `internal/proxy/config.go` 中需要修改的所有位置
- [Affects R6-R7][Needs research] mihomo macOS TUN 的 DNS 配置最佳实践

## Next Steps
→ `/ce:plan` for structured implementation planning
