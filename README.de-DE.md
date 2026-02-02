[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**Der beste plattformübergreifende Clash-Client für die CLI.**

![](./images/screenshot.png)

## 🤩 Hauptfunktionen:
* Keine Einrichtung, sofort einsatzbereit.
* **Moderne Subscription-Unterstützung**
* Angetrieben vom [mihomo](https://github.com/MetaCubeX/mihomo)-Kernel
* **Ohne Root**
* Mit `c4cgo` beliebige Befehle über Proxy ausführen
* Cross-Platform für Linux und macOS
## 📦 Installation
Lade die Binärdateien von [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases) herunter und führe sie einfach aus.


## 🧭 Grundlegende Nutzung

### Dashboard starten
```bash
./c4c
```

In der UI Subscription hinzufügen/auswählen und einen Node zum Verbinden wählen.

- Standard Mixed-Port: `17890` (in Settings konfigurierbar)
- Der Core läuft standardmäßig im Hintergrund; das Beenden von `c4c` stoppt ihn **nicht**
- Zum Stoppen: gehe zu Nodes und drücke `Enter`, bis `Disconnect` erscheint

### `c4cgo`

`c4cgo` nutzt automatisch die von c4c erstellte Proxy-Umgebung. Dieses Feature basiert auf [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

Beispiel: `curl` über Proxy ausführen

```bash
./c4cgo curl ipinfo.io
```

Beispielausgabe:

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

## 🗂️ Datenverzeichnis

Standard: `~/.clash4cli/` (enthält `config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/`, usw.).
