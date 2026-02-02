[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**最佳跨平台 CLI Clash 客戶端。**

![](./images/screenshot.png)

## 🤩 主要特色：
* 免設定，開箱即用。
* **現代訂閱支援**
* 由 [mihomo](https://github.com/MetaCubeX/mihomo) 核心驅動
* **無需 Root**
* 使用 `c4cgo` 讓任意指令走代理
* 同時支援 Linux 與 macOS 的跨平台運行
## 📦 安裝
從 [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases) 下載二進位檔，然後直接執行即可。


## 🧭 基本用法

### 啟動面板
```bash
./c4c
```

在介面裡新增/選擇訂閱並選擇一個節點進行連線。

- 預設混合埠：`17890`（可在 Settings 修改）
- 核心預設背景執行：離開 `c4c` 並不會停止核心
- 停止方式：回到 Nodes 按 `Enter` 直到 `Disconnect`

### `c4cgo`

`c4cgo` 會自動使用由 c4c 建立的代理網路環境。此功能基於 [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

範例：透過代理執行 `curl`

```bash
./c4cgo curl ipinfo.io
```

範例輸出：

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

## 🗂️ 資料目錄

預設：`~/.clash4cli/`（包含 `config.yaml`、`proxy.lock`、`subscriptions/`、`mihomo/` 等）。
