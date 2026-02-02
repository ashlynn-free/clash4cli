[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繫體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**Klien Clash lintas platform terbaik untuk CLI.**

![](./images/screenshot.png)

## 🤩 Fitur utama:
* Tanpa setup, langsung bisa dipakai.
* **Dukungan subscription modern**
* Didukung oleh kernel [mihomo](https://github.com/MetaCubeX/mihomo)
* **Tanpa root**
* Gunakan `c4cgo` untuk menjalankan perintah apa pun via proxy
* Lintas platform untuk Linux dan macOS
## 📦 Instalasi
Unduh binari dari [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases), lalu jalankan.


## 🧭 Penggunaan dasar

### Menjalankan dashboard
```bash
./c4c
```

Di UI, tambahkan/pilih subscription dan pilih node untuk terhubung.

- Port mixed default: `17890` (bisa diubah di Settings)
- Core berjalan di background; keluar dari `c4c` **tidak** menghentikannya
- Untuk berhenti: buka Nodes dan tekan `Enter` sampai `Disconnect`

### `c4cgo`

`c4cgo` otomatis menggunakan lingkungan proxy yang dibuat oleh c4c. Fitur ini berbasis pada [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

Contoh: jalankan `curl` via proxy

```bash
./c4cgo curl ipinfo.io
```

Contoh output:

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

## 🗂️ Direktori data

Default: `~/.clash4cli/` (termasuk `config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/`, dll.).
