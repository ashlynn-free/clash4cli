# Docker 劫持（TCP, Linux）技术方案

> 本文描述 clash4cli 的 **Docker “只接管容器访问互联网的 TCP 流量”** 的实现方式。  
> 该功能 **默认关闭**、**Linux 独有**、并且 **需要 root**（修改 `iptables`）。

## 目标与边界

**目标**

- 只接管 Docker 容器的**出站 TCP** 上网流量，使其透明地经过 `mihomo` 代理。
- 不改系统全局代理、不改路由（与 TUN 不同），避免“破坏本机网络环境”。
- 可随时撤销：关闭开关即可移除我们添加的 `iptables` 规则。

**不做（当前版本）**

- 不接管 UDP（例如 DNS/QUIC）；仅 TCP。
- 不处理 IPv6（仅 `iptables`，不写 `ip6tables` 规则）。
- 不接管 host 网络命名空间的流量（只针对 Docker bridge 入站到 host 的包）。
- 不保证覆盖所有容器网络类型（例如 macvlan/host network 可能不匹配我们的接口规则）。

## 总体原理（redir-port + iptables REDIRECT）

clash4cli 依赖 mihomo 的 **`redir-port`** 能力：

1. 容器访问互联网（TCP）时，数据包会从容器 veth 进入宿主机的 Docker bridge（如 `docker0` 或 `br-xxxx`）。
2. clash4cli 在宿主机上配置 `iptables -t nat PREROUTING`，把这些从 Docker bridge 进入的 TCP 流量 **REDIRECT 到本机某个端口**（即 `mihomo` 的 `redir-port`）。
3. `mihomo` 监听该 `redir-port`，通过 `SO_ORIGINAL_DST` 等机制获取原始目标地址，再按 Clash 规则转发到代理节点。

这样做的核心优点是：不需要全局 TUN/路由，只“截获一部分流量”。

## 运行时数据流（clash4cli 内部）

### 1) 配置开关

- 配置项：`proxy.docker_redirect_tcp`（Settings 中的 “Docker 劫持（TCP, Linux）”）
- 仅 Linux 生效；其他平台 UI 也会展示该项，但会显示为不可用（灰显）。

### 2) redir-port 的选择与持久化

当 `proxy.docker_redirect_tcp=true` 且平台为 Linux 时：

- `c4c` 在启动 `mihomo` 前，会为 `redir-port` 选择一个空闲端口（优先用 `mixed_port + 1`，冲突则顺延寻找）。
- 生成的 `mihomo` 配置会包含：

```yaml
redir-port: <port>
```

- 该端口会写入 `~/.clash4cli/proxy.lock`（字段：`redir_port`），供后续“应用 iptables”读取。

### 3) iptables 规则的写入与撤销

真正的“劫持”由 `iptables` 完成，因此需要 root。

- 在 Settings 中点击保存（Linux + root），clash4cli 会读取 `proxy.lock` 里的 `redir_port` 并写入 `iptables`。
- 关闭开关并保存（Linux + root），clash4cli 会移除相关规则并删除自建链。

## iptables 规则设计

实现文件：`internal/netctl/docker_linux.go`

### 规则概览

- 表：`nat`
- 自建链：`CLASH4CLI_DOCKER`
- 入口：`PREROUTING`
- 匹配：入接口为 Docker bridge（`docker0`、以及前缀匹配 `br*`）的 TCP 流量
- 动作：跳转到自建链，再按目标网段决定是否 REDIRECT

### 近似规则（示意）

1) 创建并清空链：

```bash
iptables -t nat -N CLASH4CLI_DOCKER 2>/dev/null || true
iptables -t nat -F CLASH4CLI_DOCKER
```

2) 在链里排除“本地/私网/保留地址”（尽量只影响“互联网”）：

```bash
iptables -t nat -A CLASH4CLI_DOCKER -d 10.0.0.0/8     -j RETURN
iptables -t nat -A CLASH4CLI_DOCKER -d 172.16.0.0/12  -j RETURN
iptables -t nat -A CLASH4CLI_DOCKER -d 192.168.0.0/16 -j RETURN
# 以及 127/8、169.254/16、224/4、240/4 等（详见源码）
```

3) 对剩余 TCP 流量执行 REDIRECT：

```bash
iptables -t nat -A CLASH4CLI_DOCKER -p tcp -j REDIRECT --to-ports <redir-port>
```

4) 在 PREROUTING 注入跳转规则（插入到第 1 条，优先级最高）：

```bash
iptables -t nat -I PREROUTING 1 -i docker0 -p tcp -j CLASH4CLI_DOCKER
iptables -t nat -I PREROUTING 1 -i br+     -p tcp -j CLASH4CLI_DOCKER
```

> `br+` 是 iptables 的接口前缀匹配写法，用于覆盖 `br-xxxx` 等用户自定义网络桥。

### 撤销规则

关闭开关并保存（root）后，会：

- 删除 `PREROUTING` 的跳转规则（循环删除直到不存在）
- `-F` 清空链并 `-X` 删除链

## 关键依赖与前置条件

**必须**

- Linux
- root 权限（或具备等效的 `CAP_NET_ADMIN` 运行能力）
- 系统可用 `iptables`（发行版可能是 `iptables-nft` 包装器；只要 `iptables` 命令可用即可）

**强烈建议**

- `mihomo` 的监听地址不要是 loopback：
  - `allow_lan=true`
  - `bind_address=0.0.0.0`（默认）或绑定到 Docker bridge 的 IP

原因：`PREROUTING` 的 `REDIRECT` 会把目的地址改写为宿主机在该入接口上的地址（例如 `docker0` 的 `172.17.0.1`），如果 `mihomo` 只监听 `127.0.0.1`，容器流量无法连上。

## 使用与操作建议

### 启用流程（推荐）

1. 在 Settings 中打开 `Docker 劫持（TCP, Linux）` 并保存。
2. 点击 “Restart core”（或在 Nodes 里重启/重新连接）让 `mihomo` 重新生成配置并产生 `redir-port`。
3. 用 root 运行 `c4c`（`sudo c4c`）后，再次在 Settings 点击保存以应用 `iptables`。

### 端口变化

`redir-port` 是动态选择的：如果你重启 core、端口冲突导致端口变化，需要（root）再保存一次 Settings 来刷新 `iptables` 的目标端口。

## 排错

1) 检查 `proxy.lock` 是否包含 `redir_port`：

```bash
cat ~/.clash4cli/proxy.lock
```

2) 检查 `iptables` 是否写入成功：

```bash
sudo iptables -t nat -S | rg 'CLASH4CLI_DOCKER|PREROUTING'
```

3) 检查 `mihomo` 是否在监听 `redir-port`（示例）：

```bash
sudo ss -ltnp | rg '<redir-port>'
```

4) 如果容器走的是 `--network host`，本方案不会接管（需要另一个方案）。

## 安全与影响说明

- 本功能会修改宿主机的 `iptables`，可能影响容器出网行为；请谨慎启用，并确保可随时关闭。
- 开启 `allow_lan` 可能会让局域网其他设备访问你的本地代理端口；如有安全顾虑：
  - 通过防火墙限制访问，或
  - 将 `bind_address` 绑定到 Docker bridge IP（但可能无法覆盖所有 `br-*` 网络）。

