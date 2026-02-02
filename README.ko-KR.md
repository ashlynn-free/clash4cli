[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**CLI용 최고의 크로스 플랫폼 Clash 클라이언트.**

![](./images/screenshot.png)

## 🤩 주요 기능:
* 설정 없이 바로 사용.
* **모던 구독 지원**
* [mihomo](https://github.com/MetaCubeX/mihomo) 커널 기반
* **Root 없이 실행**
* `c4cgo`로 어떤 명령이든 프록시로 실행
* Linux와 macOS 모두 지원
## 📦 설치
[GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases)에서 바이너리를 다운로드한 뒤, 바로 실행하세요.


## 🧭 기본 사용법

### 대시보드 실행
```bash
./c4c
```

UI에서 구독을 추가/선택하고 연결할 노드를 선택하세요.

- 기본 mixed 포트: `17890` (Settings에서 변경 가능)
- 코어는 기본적으로 백그라운드로 실행되며, `c4c`를 종료해도 **중지되지 않습니다**
- 중지하려면: Nodes에서 `Enter`를 눌러 `Disconnect`까지 전환

### `c4cgo`

`c4cgo`는 c4c가 만든 프록시 환경을 자동으로 사용합니다. 이 기능은 [proxychians-ng](https://github.com/rofl0r/proxychains-ng)에 기반합니다

예시: 프록시로 `curl` 실행

```bash
./c4cgo curl ipinfo.io
```

예시 출력:

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

## 🗂️ 데이터 디렉터리

기본값: `~/.clash4cli/` (`config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/` 등 포함).
