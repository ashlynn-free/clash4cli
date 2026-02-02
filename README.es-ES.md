[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**El mejor cliente Clash multiplataforma para CLI.**

![](./images/screenshot.png)

## 🤩 Características principales:
* Sin configuración, listo para usar.
* **Soporte moderno de suscripciones**
* Impulsado por el kernel [mihomo](https://github.com/MetaCubeX/mihomo)
* **Sin root**
* Usa `c4cgo` para ejecutar cualquier comando vía proxy
* Multiplataforma para Linux y macOS
## 📦 Instalación
Descarga los binarios desde [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases) y ejecútalos.


## 🧭 Uso básico

### Iniciar el panel
```bash
./c4c
```

En la UI, añade/selecciona una suscripción y elige un nodo para conectar.

- Puerto mixed por defecto: `17890` (configurable en Settings)
- El core se ejecuta en segundo plano; salir de `c4c` **no** lo detiene
- Para detener: ve a Nodes y pulsa `Enter` hasta `Disconnect`

### `c4cgo`

`c4cgo` utiliza automáticamente el entorno de proxy creado por c4c. Esta función se basa en [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

Ejemplo: ejecutar `curl` vía proxy

```bash
./c4cgo curl ipinfo.io
```

Salida de ejemplo:

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

## 🗂️ Directorio de datos

Por defecto: `~/.clash4cli/` (incluye `config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/`, etc.).
