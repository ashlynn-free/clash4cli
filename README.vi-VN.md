[🇺🇸 English](README.md) | [🇨🇳 简体中文](README.zh-CN.md) | [🇨🇳 繁體中文](README.zh-TW.md) | [🇷🇺 Русский](README.ru-RU.md) | [🇯🇵 日本語](README.ja-JP.md) | [🇰🇷 한국어](README.ko-KR.md) | [🇪🇸 Español](README.es-ES.md) | [🇧🇷 Português (Brasil)](README.pt-BR.md) | [🇫🇷 Français](README.fr-FR.md) | [🇩🇪 Deutsch](README.de-DE.md) | [🇮🇹 Italiano](README.it-IT.md) | [🇮🇩 Bahasa Indonesia](README.id-ID.md) | [🇻🇳 Tiếng Việt](README.vi-VN.md) | [🇹🇷 Türkçe](README.tr-TR.md)

# 🚀 Clash for CLI
**Trình khách Clash đa nền tảng tốt nhất cho CLI.**

![](./images/screenshot.png)

## 🤩 Tính năng chính:
* Không cần thiết lập, dùng ngay.
* **Hỗ trợ subscription hiện đại**
* Được cung cấp bởi kernel [mihomo](https://github.com/MetaCubeX/mihomo)
* **Không cần root**
* Dùng `c4cgo` để chạy bất kỳ lệnh nào qua proxy
* Đa nền tảng cho Linux và macOS
## 📦 Cài đặt
Tải file nhị phân từ [GitHub Releases](https://github.com/ashlynn-free/clash4cli/releases), sau đó chạy ngay.


## 🧭 Cách dùng cơ bản

### Khởi động dashboard
```bash
./c4c
```

Trong UI, thêm/chọn subscription và chọn một node để kết nối.

- Cổng mixed mặc định: `17890` (có thể đổi trong Settings)
- Core chạy nền; thoát `c4c` **không** dừng core
- Để dừng: vào Nodes và bấm `Enter` cho đến `Disconnect`

### `c4cgo`

`c4cgo` tự động dùng môi trường proxy do c4c tạo ra. Tính năng này dựa trên [proxychians-ng](https://github.com/rofl0r/proxychains-ng)

Ví dụ: chạy `curl` qua proxy

```bash
./c4cgo curl ipinfo.io
```

Ví dụ output:

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

## 🗂️ Thư mục dữ liệu

Mặc định: `~/.clash4cli/` (bao gồm `config.yaml`, `proxy.lock`, `subscriptions/`, `mihomo/`, v.v.).
