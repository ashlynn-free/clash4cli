# 🚀 clash4cli（rocket4cli）
![](./images/screenshot.png)
> 🧩 一个**无需 Root** 的终端代理管理器（TUI）。像 ShellCrash 一样方便，但在**用户态**运行：订阅 ➜ 选节点 ➜ 一键启动本地代理（SOCKS5 + HTTP）➜ `c4cgo` 让任意命令走代理。

---

## ✨ 你会喜欢它的原因

- 🖥️ **全屏 TUI**：`Tab` 切换页面，操作像在用一个“终端 App”
- 📮 **订阅管理**：添加/更新/删除订阅，自动解析节点
- 📡 **节点列表**：只显示“当前订阅”的节点；对着节点按 `Enter` 在 `Disconnect / Rule / Global` 间切换
- 🔌 **本地代理服务**：启动 `mihomo`，提供一个“混合端口”（同一端口支持 SOCKS5 + HTTP）
- 🌍 **中英双语**：`en_us` / `zh_cn`
- 🧼 **干净**：所有数据都在 `~/.clash4cli/`

---

## ⚡ 3 分钟上手（TL;DR）

1️⃣ 编译并启动：

```bash
make build
./c4c
```

2️⃣ 在 TUI 里：

- `Tab` 切到 **Subscriptions** ➜ `a` ➜ 粘贴订阅 URL ➜ `Enter` ➜ 输入名称 ➜ `Enter`
- 如果 URL 在服务器上被阻断：用 `z` 添加“手动订阅”，输入名称后粘贴浏览器打开的订阅内容（原始 base64 / YAML / JSON 均可），按 `Ctrl+S` 保存
- 在 **Subscriptions** 里选中订阅 ➜ `Enter` 设为当前订阅（前面会出现 ✅）
- 切回 **Nodes**（`Tab`）➜ 选中节点 ➜ 连按 `Enter` 在 `Disconnect / Rule / Global` 间切换（`Disconnect` 会停止代理）

3️⃣ 让命令走代理（推荐）：

```bash
./c4cgo curl https://ip.sb
```

---

## 📦 安装

### ✅ 方式 A：源码编译（推荐）

```bash
make build
./c4c
```

编译后会得到：

- 🧠 `c4c`：TUI 主程序
- 🏃 `c4cgo`：让命令走代理的小工具（读取 `proxy.lock`）

### 📦 方式 B：使用打包产物（tar.gz）

如果你拿到 `make package` / `make release` 生成的归档：

```bash
tar -xzf clash4cli-<os>-<arch>-<version>.tar.gz
cd clash4cli-<os>-<arch>-<version>
./c4c
```

### 🔧 方式 C：安装到 PATH（需要 sudo）

```bash
make install
```

---

## 🧭 使用指南

### 🟢 启动 / 退出

- 启动：`c4c`
- 退出：`q` 或 `Ctrl+C`（会自动停止代理进程）

### 🔌 启停代理（mihomo）

- 对着节点按 `Enter`：在 `Disconnect` / `Rule` / `Global` 间切换（`Disconnect` 会停止代理）
- 代理启动后，本地会监听一个“混合端口”（默认 `17890`）
  - HTTP 代理：`http://127.0.0.1:17890`
  - SOCKS5 代理：`socks5://127.0.0.1:17890`

### 🏃 `c4cgo`：让命令走代理（强烈推荐）

`c4cgo` 会读取 `~/.clash4cli/proxy.lock`，自动获得当前代理端口并执行命令。

```bash
# 让 curl 走代理
c4cgo curl https://ip.sb

# 安静模式：只输出命令结果
c4cgo -q curl -s https://ip.sb

# 指定端口（不读 proxy.lock）
c4cgo --port 17890 curl https://ip.sb
```

🧠 运行模式（`--mode`）：

- 🤖 `auto`（默认）：优先尝试 `proxychains4`，不行再退回环境变量
- 🧷 `proxychains`：强制使用 `proxychains4`
- 🌱 `env`：强制使用环境变量（HTTP_PROXY/ALL_PROXY）

> 🍎 macOS 提醒：系统自带的可执行文件（例如 `/usr/bin/curl`）可能被 SIP 保护，`proxychains` 注入会被忽略。此时 `c4cgo` 会自动回退到 `env`；你也可以显式指定：`c4cgo --mode env ...`

---

## ⌨️ 快捷键一览（TUI）

### 🌐 全局

| Keys | 功能 |
|---|---|
| `Tab` | 切换标签页 |
| `q` / `Ctrl+C` | 退出 |

### 📡 Nodes（节点）

| Keys | 功能 |
|---|---|
| `↑/↓` 或 `j/k` | 移动 |
| `Enter` | 切换模式（Disconnect / Rule / Global） |
| `t` | 测速当前订阅节点 |
| `r` | 刷新节点列表 |

### 📮 Subscriptions（订阅）

| Keys | 功能 |
|---|---|
| `Enter` | 设为当前订阅（前面显示 ✅；Nodes 只显示该订阅的节点） |
| `a` | 添加订阅（先输入 URL，再输入名称） |
| `z` | 手动订阅：输入名称后粘贴订阅内容（`Ctrl+S` 保存） |
| `u` | 更新选中订阅 |
| `U` | 更新全部订阅 |
| `d` | 删除选中订阅 |
| `r` | 刷新订阅列表 |
| `Esc` | 取消输入 |

### ⚙️ Settings（设置）

| Keys | 功能 |
|---|---|
| `↑/↓` 或 `j/k` | 切换字段 |
| `←/→` | 切换选项（如语言） |
| `Ctrl+S` | 保存 |

### 🧾 Logs（日志）

| Keys | 功能 |
|---|---|
| `↑/↓` 或 `j/k` | 滚动 |
| `g` / `G` | 顶部 / 底部 |
| `a` | 自动滚动开关 |
| `c` | 清空 |

---

## 🗂️ 配置与目录

默认工作目录：`~/.clash4cli/`

目录结构（示例）：

```text
~/.clash4cli/
├── config.yaml          # 配置（端口/语言/测速并发等）
├── proxy.lock           # 当前代理信息（给 c4cgo 读取）
├── subscriptions/       # 订阅落盘（yaml）
├── mihomo/              # mihomo 工作目录（缓存/配置等）
└── logs/                # 应用日志
```

`config.yaml` 示例：

```yaml
proxy:
  mixed_port: 17890
language: en_us # 或 zh_cn
speedtest:
  concurrent: 10
  timeout: 5s
ui:
  server_display: show # show|blur|off（Nodes 页的 Server 列）
```

---

## 🧩 依赖说明（mihomo / proxychains4）

### 🧠 mihomo（必需）

`c4c` 启动代理时会寻找 `mihomo`：

1. ✅ 系统 `PATH` 里的 `mihomo`
2. ✅ 与可执行文件同目录的 `mihomo`
3. ✅ `third_party/mihomo/` 下的 `mihomo-<os>-<arch>`（可通过 `make mihomo` / `make mihomo_all` / `make deps` 拉取）

### 🧷 proxychains4（可选）

`c4cgo` 会自动探测 `proxychains4` + 动态库并在可用时优先使用；否则会用环境变量方式。

如需为当前平台拉取/构建：

```bash
make proxychains
```

如需一键拉取所有依赖（mihomo + proxychains4），用于跨平台打包：

```bash
make deps
```

版本固定在 `Makefile` 的 `PROXYCHAINS_NG_VERSION`，需要升级时再改这个值即可。

如需拉取/构建所有目前支持的平台（darwin amd64/arm64 + linux amd64）：

```bash
make proxychains_all
```

> 提示：在 macOS 上构建 `linux/amd64` 版本需要 Docker（脚本会自动尝试使用 Docker），否则请在对应的 Linux 机器上执行。Linux 版本默认会用 `centos:7` 作为构建基底（更老的 glibc 基线），以便在大多数 Linux 发行版上可用。

---

## 🧯 常见问题（FAQ）

### ❓ `c4cgo` 提示找不到 `proxy.lock`

✅ 你需要先：

- 启动 `c4c`
- 选择一个节点（`Enter`）
- 确保模式不是 `Disconnect`（对着节点按 `Enter` 切换）

### ❓ 启动代理时报 `mihomo binary not found`

✅ 确保：

- `mihomo` 在 `PATH` 中，或
- 放在 `third_party/mihomo/`，或
- 与 `c4c` 同目录

### ❓ 端口被占用（比如 `17890`）

✅ 进入 `Settings`（`Tab` 切换到 Settings），修改 `Mixed Port` 并保存（`Ctrl+S`）。

### ❓ 订阅显示 0 节点

✅ 可能是网络不可达或订阅内容不兼容。你可以：

- 在 `Subscriptions` 里对该订阅按 `u` 重新更新
- 检查订阅 URL 是否能直连访问（本工具拉取订阅时不会走系统代理）

---

## 🛠️ 开发者指北

```bash
make run      # 开发模式运行
make test     # 运行测试
make package  # 当前平台打包
make packages # 全平台打包
make deps     # 拉取/构建所有依赖（用于跨平台打包）
```

### 🌿 分支约定

- `dev`：日常开发分支（建议所有 PR 都合到 `dev`）
- `main`：发布分支（推送到 `main` 会触发 GitHub Actions 自动构建并发布 GitHub Release）

---

## 📚 支持的订阅格式

- ✅ Clash YAML
- ✅ Base64 编码订阅（SS / VMess / VLESS / Trojan / Hysteria2）
- ✅ SIP008

---

## 📄 License

MIT（本项目源代码）。打包产物中可能包含第三方组件（如 `mihomo` / `proxychains-ng`），其许可证以 `THIRD_PARTY_NOTICES.md` 为准。
