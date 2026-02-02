[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**Le meilleur client Clash multiplateforme pour CLI.**

![](./images/screenshot.png)

## 🤩 Fonctionnalités principales :
* Aucun réglage, prêt à l’emploi.
* **Support moderne des abonnements**
* Propulsé par le noyau [mihomo](https://github.com/MetaCubeX/mihomo)
* **Sans root**
* Utilisez `c4cgo` pour exécuter n’importe quelle commande via proxy
* Multiplateforme pour Linux et macOS
## 📦 Installation
Téléchargez les binaires depuis [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases), puis exécutez-les.


## 🧭 Utilisation de base

### Démarrer le tableau de bord
```bash
./c4c
```

Dans l’UI, ajoutez/sélectionnez un abonnement et choisissez un nœud pour vous connecter.

- Port mixed par défaut : `17890` (configurable dans Settings)
- Le core s’exécute en arrière-plan ; quitter `c4c` **ne** l’arrête pas
- Pour arrêter : allez dans Nodes et appuyez sur `Enter` jusqu’à `Disconnect`

### `c4cgo`

`c4cgo` utilise automatiquement l’environnement proxy créé par c4c. Cette fonctionnalité est basée sur [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

Exemple : exécuter `curl` via proxy

```bash
./c4cgo curl ipinfo.io
```

Exemple de sortie :

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

## 🗂️ Répertoire de données

Par défaut : `~/.clash4cli/` (inclut `config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/`, etc.).
