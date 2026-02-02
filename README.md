# 🚀 Clash for CLI

![](./images/screenshot.png)

> 一个跨平台的终端代理管理器：订阅 ➜ 选节点 ➜ 一键启动本地代理（SOCKS5 + HTTP），再用 `c4cgo` 让任意命令走代理。
>
> 本仓库仅用于发布与文档（不包含源代码）。

## 🌍 Platforms

- macOS（`amd64` Intel / `arm64` Apple Silicon）
- Linux（`amd64` x86_64 / `arm64` aarch64）

## 📦 Installation

### From GitHub Releases


## 🧭 Usage

### `c4c`

启动：

```bash
./c4c
```

在界面里添加/选择订阅并选中节点即可连接（本地默认监听混合端口 `17890`，可在 Settings 里修改）。

说明：`c4c` 启动的核心默认以后台方式运行，退出 `c4c` 不会自动停止核心。如需停止，请在 Nodes 页按 `Enter` 切换到 `Disconnect`。

### `c4cgo`

`c4cgo` 会读取 `~/.clash4cli/proxy.lock`，自动使用当前代理端口。

示例：`curl` 走代理

```bash
./c4cgo curl ipinfo.io
```

示例输出：

```text
[c4cgo] Using proxy at 127.0.0.1:17890 (node: 🇸🇬SG丨新加坡02)
[c4cgo] Mode: proxychains4
[c4cgo] Proxy: socks5://127.0.0.1:17890
[c4cgo] Command: curl
─────────────────────────────────────
{
  "ip": "***.***.***.***",
  "city": "Singapore",
  "region": "Singapore",
  "country": "SG",
  "loc": "1.2897,103.8501",
  "org": "AS38136 Akari Networks",
  "postal": "018989",
  "timezone": "Asia/Singapore",
  "readme": "https://ipinfo.io/missingauth"
}
```

## 🗂️ Data directory

默认：`~/.clash4cli/`（包含 `config.yaml`、`proxy.lock`、`subscriptions/`、`mihomo/` 等）。
