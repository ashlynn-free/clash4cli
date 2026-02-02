[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**最佳跨平台 CLI Clash 客户端。**

![](./images/screenshot.png)

## 🤩 主要特性：
* 无需配置，开箱即用。
* **现代订阅支持**
* 由 [mihomo](https://github.com/MetaCubeX/mihomo) 内核驱动
* **无需 Root**
* 使用 `c4cgo` 让任意命令走代理
* 同时支持 Linux 与 macOS 的跨平台运行
## 📦 安装
从 [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases) 下载二进制文件，然后直接运行即可。


## 🧭 基础用法

### 启动面板
```bash
./c4c
```

在界面里添加/选择订阅并选择一个节点进行连接。

- 默认混合端口：`17890`（可在 Settings 修改）
- 核心默认后台运行：退出 `c4c` 并不会停止核心
- 停止方式：回到 Nodes 按 `Enter` 直到 `Disconnect`

### `c4cgo`

`c4cgo` 会自动使用由 c4c 创建的代理网络环境。此功能基于 [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

示例：通过代理运行 `curl`

```bash
./c4cgo curl ipinfo.io
```

示例输出：

```text
[c4cgo] Using proxy at 127.0.0.1:17890 (node: SG node)
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

## 🗂️ 数据目录

默认：`~/.clash4cli/`（包含 `config.yaml`、`proxy.lock`、`subscriptions/`、`mihomo/` 等）。
