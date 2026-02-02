[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**Il miglior client Clash multipiattaforma per CLI.**

![](./images/screenshot.png)

## 🤩 Funzionalità principali:
* Nessuna configurazione, pronto all’uso.
* **Supporto moderno alle subscription**
* Basato sul kernel [mihomo](https://github.com/MetaCubeX/mihomo)
* **Senza root**
* Usa `c4cgo` per eseguire qualsiasi comando via proxy
* Cross-platform per Linux e macOS
## 📦 Installazione
Scarica i binari da [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases), poi eseguili.


## 🧭 Utilizzo di base

### Avviare la dashboard
```bash
./c4c
```

Nella UI, aggiungi/seleziona una subscription e scegli un nodo per connetterti.

- Porta mixed predefinita: `17890` (configurabile in Settings)
- Il core gira in background; uscire da `c4c` **non** lo ferma
- Per fermare: vai su Nodes e premi `Enter` fino a `Disconnect`

### `c4cgo`

`c4cgo` usa automaticamente l’ambiente proxy creato da c4c. Questa funzionalità è basata su [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

Esempio: eseguire `curl` via proxy

```bash
./c4cgo curl ipinfo.io
```

Output di esempio:

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

## 🗂️ Directory dei dati

Predefinito: `~/.clash4cli/` (include `config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/`, ecc.).
