[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**CLI 向けの最高のクロスプラットフォーム Clash クライアント。**

![](./images/screenshot.png)

## 🤩 主な特徴：
* 設定不要で、すぐに使えます。
* **モダンなサブスクリプション対応**
* [mihomo](https://github.com/MetaCubeX/mihomo) カーネルで動作
* **Root 不要**
* `c4cgo` で任意のコマンドをプロキシ経由で実行
* Linux と macOS の両方に対応
## 📦 インストール
[GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases) からバイナリをダウンロードして、そのまま実行してください。


## 🧭 基本的な使い方

### ダッシュボードを起動
```bash
./c4c
```

UI でサブスクリプションを追加/選択し、接続するノードを選びます。

- 既定の mixed ポート: `17890`（Settings で変更可能）
- コアは既定でバックグラウンド動作のため、`c4c` を終了しても停止しません
- 停止するには: Nodes で `Enter` を押して `Disconnect` まで切り替えます

### `c4cgo`

`c4cgo` は c4c が作成したプロキシ環境を自動的に利用します。この機能は [proxychians-ng](https://github.com/rofl0r/proxychains-ng) に基づいています

例: `curl` をプロキシ経由で実行

```bash
./c4cgo curl ipinfo.io
```

出力例:

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

## 🗂️ データディレクトリ

既定: `~/.clash4cli/`（`config.yaml`、`proxy.lock`、`subscriptions/`、`mihomo/` など）。
