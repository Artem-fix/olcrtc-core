<div align="center">

# olcrtc-core

![License](https://img.shields.io/badge/license-WTFPL-0D1117?style=flat-square&logo=open-source-initiative&logoColor=green&labelColor=0D1117)
![Golang](https://img.shields.io/badge/-Golang-0D1117?style=flat-square&logo=go&logoColor=00A7D0)
![Status](https://img.shields.io/badge/status-beta-0D1117?style=flat-square&logoColor=orange&labelColor=0D1117)

**Производительное ядро для туннелирования трафика через WebRTC-каналы легальных видеоконференц-сервисов**

[Быстрый старт](docs/fast.md) · [Документация](docs/manual.md) · [Настройка](docs/settings.md) · [Безопасность](docs/security.md)

</div>

---

## Что это

`olcrtc-core` — сетевое ядро на Go, которое туннелирует произвольный TCP-трафик через WebRTC-сессии внутри легальных видеоконференц-платформ (Яндекс Телемост, VK Jazz, Wildberries Stream). Эти платформы, как правило, не блокируются даже при жёсткой интернет-фильтрации.

```
[Клиент] ──SOCKS5──▶ [olcrtc-core client]
                              │
                     WebRTC (DataChannel/Video/SEI)
                     через комнату провайдера
                              │
                     [olcrtc-core server] ──TCP──▶ [Интернет]
```

---

## Возможности

- **4 транспорта** — DataChannel, VideoChannel (H.264 luma), SEIChannel (H.264 SEI NAL), VP8Channel
- **3 провайдера** — Telemost, Jazz, Wbstream (расширяемо через реестр)
- **Криптография** — X25519 ECDH + HKDF-SHA256 + XChaCha20-Poly1305 AEAD, forward secrecy
- **Мультиплексирование** — smux v2, несколько TCP-потоков в одном WebRTC-соединении
- **SOCKS5** — встроенный inbound-сервер на стороне клиента
- **Graceful shutdown** — строгое управление жизненным циклом через `context.Context`
- **Производительность** — `sync.Pool`, атомарные счётчики, `uber-go/zap`
- **Переподключение** — автоматический reconnect с настраиваемой задержкой
- **Подписки** — поддержка списков серверов с автообновлением

---

## Быстрый старт

### 1. Сборка

```bash
git clone https://github.com/openlibrecommunity/olcrtc-core
cd olcrtc-core
go build -trimpath -ldflags="-s -w" -o olcrtc-core ./cmd/olcrtc-core
```

### 2. Генерация ключа

```bash
openssl rand -hex 32
# → a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4
```

### 3. Сервер (на VPS)

```bash
./olcrtc-core \
  -mode server \
  -provider telemost \
  -key a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4 \
  -forward 0.0.0.0:0
```

Сервер выведет `room_id` в лог.

### 4. Клиент (локально)

```bash
./olcrtc-core \
  -mode client \
  -provider telemost \
  -room XXXX-YYYY-ZZZZ \
  -key a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4 \
  -listen 127.0.0.1:1080
```

### 5. Проверка

```bash
curl --socks5 127.0.0.1:1080 https://ifconfig.me
# → IP вашего сервера
```

---

## Транспорты

| Транспорт | Механизм | Скорость | Заметность |
|-----------|----------|----------|------------|
| `datachannel` | WebRTC DataChannel | ~10 Мбит/с | средняя |
| `videochannel` | H.264 luma-плоскость | ~3 Мбит/с | низкая |
| `seichannel` | H.264 SEI NAL-юниты | ~240 Кбит/с | очень низкая |
| `vp8channel` | VP8 битстрим | ~2 Мбит/с | низкая |

---

## Совместимость провайдер × транспорт

| | `datachannel` | `videochannel` | `seichannel` | `vp8channel` |
|---|:---:|:---:|:---:|:---:|
| `telemost` | ✅ | ✅ | ✅ | ✅ |
| `jazz` | ✅ | ✅ | ⚠️ | ✅ |
| `wbstream` | ✅ | ⚠️ | ❌ | ⚠️ |

---

## Структура проекта

```
olcrtc-core/
├── cmd/olcrtc-core/     # CLI точка входа
├── core/
│   ├── app/             # Wiring всех компонентов
│   ├── crypto/          # X25519, XChaCha20-Poly1305, HKDF
│   ├── dispatcher/      # Маршрутизация потоков
│   ├── inbound/socks5/  # SOCKS5 inbound сервер
│   ├── lifecycle/       # Start/Stop примитивы
│   ├── log/             # Zap-логгер
│   ├── mux/             # smux мультиплексер
│   ├── protect/         # Защита от перегрузки
│   ├── provider/        # Интерфейс WebRTC-провайдеров
│   ├── registry/        # Типобезопасный реестр компонентов
│   ├── routing/         # Движок маршрутизации
│   ├── session/         # Управление сессией
│   ├── stats/           # Метрики
│   └── transport/       # Транспортный уровень
│       ├── datachannel/
│       ├── videochannel/
│       ├── seichannel/
│       └── vp8channel/
└── docs/                # Документация
```

---

## Сборка

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o build/olcrtc-core-linux-amd64 ./cmd/olcrtc-core

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o build/olcrtc-core-darwin-arm64 ./cmd/olcrtc-core

# Windows amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o build/olcrtc-core-windows-amd64.exe ./cmd/olcrtc-core

# Android AAR (через gomobile)
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
gomobile bind -target=android -androidapi 21 -ldflags="-s -w" -o build/olcrtc.aar ./mobile
```

---

## Тесты

```bash
# Все тесты
go test ./...

# С race detector (рекомендуется)
go test -race ./...

# Конкретный пакет
go test -race -v ./core/crypto/...
```

---

## Docker

```bash
# Сборка образа
docker build -t olcrtc-core:latest .

# Запуск сервера
docker run -d \
  --name olcrtc-server \
  --restart unless-stopped \
  -e OLCRTC_PROVIDER=telemost \
  -e OLCRTC_KEY=YOUR_64_HEX_KEY \
  olcrtc-core:latest
```

---

## Документация

| Файл | Содержимое |
|------|-----------|
| [docs/manual.md](docs/manual.md) | Оглавление и основные понятия |
| [docs/about.md](docs/about.md) | Архитектура, паттерны, сравнение с xray-core |
| [docs/fast.md](docs/fast.md) | Быстрый старт, systemd, Docker |
| [docs/settings.md](docs/settings.md) | Все флаги CLI и переменные окружения |
| [docs/uri.md](docs/uri.md) | URI-формат, QR-коды |
| [docs/sub.md](docs/sub.md) | Подписки, балансировка серверов |
| [docs/security.md](docs/security.md) | Криптомодель, рекомендации по безопасности |
| [docs/troubleshooting.md](docs/troubleshooting.md) | Диагностика, типичные ошибки, FAQ |

---

## Зависимости

| Библиотека | Назначение |
|-----------|-----------|
| `pion/webrtc` | WebRTC стек |
| `xtaci/smux` | Stream multiplexing |
| `xtaci/kcp-go` | KCP транспорт |
| `uber-go/zap` | Структурированное логирование |
| `golang.org/x/crypto` | XChaCha20-Poly1305, HKDF, X25519 |
| `google/uuid` | Генерация идентификаторов |

---

## Лицензия

```
DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
Version 2, December 2004
```

<div align="center">

---

Telegram: [@openlibrecommunity](https://t.me/openlibrecommunity)

</div>