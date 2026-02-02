[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繫體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**CLI için en iyi çapraz platform Clash istemcisi.**

![](./images/screenshot.png)

## 🤩 Başlıca özellikler:
* Kurulum yok, kutudan çıktığı gibi.
* **Modern abonelik desteği**
* [mihomo](https://github.com/MetaCubeX/mihomo) çekirdeği ile güçlendirilmiştir
* **Root olmadan**
* `c4cgo` ile herhangi bir komutu proxy üzerinden çalıştırın
* Linux ve macOS için çapraz platform
## 📦 Kurulum
[GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases) üzerinden ikili dosyaları indirin ve çalıştırın.


## 🧭 Temel kullanım

### Kontrol panelini başlat
```bash
./c4c
```

UI içinde bir abonelik ekleyin/seçin ve bağlanmak için bir node seçin.

- Varsayılan mixed port: `17890` (Settings'ten değiştirilebilir)
- Core varsayılan olarak arka planda çalışır; `c4c`'den çıkmak core'u **durdurmaz**
- Durdurmak için: Nodes'a gidin ve `Disconnect` olana kadar `Enter`'a basın

### `c4cgo`

`c4cgo`, c4c tarafından oluşturulan proxy ortamını otomatik olarak kullanır. Bu özellik [proxychians-ng](https://github.com/rofl0r/proxychains-ng) tabanlıdır

Örnek: `curl`'ü proxy üzerinden çalıştır

```bash
./c4cgo curl ipinfo.io
```

Örnek çıktı:

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

## 🗂️ Veri dizini

Varsayılan: `~/.clash4cli/` (`config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/` vb. içerir).
