# Настройка olcrtc-core

## Флаги командной строки

### Общие параметры

| Флаг | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `-mode` | string | `server` | Роль узла: `client` или `server` |
| `-provider` | string | — | Провайдер WebRTC: `telemost`, `jazz`, `wbstream` |
| `-key` | string | — | 64-символьный hex ключ (PSK). **Обязателен** |
| `-transport` | string | `datachannel` | Транспорт: `datachannel`, `videochannel`, `seichannel`, `vp8channel` |
| `-name` | string | `olcrtc-core` | Отображаемое имя пира в комнате провайдера |
| `-log` | string | `info` | Уровень логов: `debug`, `info`, `warn`, `error` |
| `-dev` | bool | `false` | Режим разработки: читаемые логи вместо JSON |
| `-socks` | string | — | SOCKS5-прокси для API-запросов к провайдеру: `host:port` |
| `-dns` | string | — | Кастомный DNS-резолвер: `host:port` (например `1.1.1.1:53`) |
| `-handshake-timeout` | duration | `15s` | Максимальное время на криптографическое рукопожатие |
| `-dial-timeout` | duration | `30s` | Максимальное время на установку транспортного соединения |
| `-reconnect` | duration | `0` | Задержка перед переподключением. `0` — отключено |

### Серверные параметры

| Флаг | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `-forward` | string | — | Адрес для форвардинга трафика: `host:port`. **Обязателен** для сервера |
| `-room` | string | — | ID комнаты. Если не указан — сервер создаёт новую комнату |

### Клиентские параметры

| Флаг | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `-listen` | string | `127.0.0.1:1080` | Локальный адрес для SOCKS5-сервера |
| `-room` | string | — | ID комнаты, к которой подключается клиент. **Обязателен** |

---

## Переменные окружения

| Переменная | Соответствует флагу |
|------------|---------------------|
| `OLCRTC_MODE` | `-mode` |
| `OLCRTC_PROVIDER` | `-provider` |
| `OLCRTC_KEY` | `-key` |
| `OLCRTC_ROOM_ID` | `-room` |
| `OLCRTC_TRANSPORT` | `-transport` |
| `OLCRTC_LISTEN` | `-listen` |
| `OLCRTC_FORWARD` | `-forward` |
| `OLCRTC_DNS` | `-dns` |
| `OLCRTC_SOCKS_PROXY` | `-socks` |
| `OLCRTC_DEBUG` | `-dev` (значение `true`/`false`) |
| `OLCRTC_LOG_LEVEL` | `-log` |
| `OLCRTC_RECONNECT` | `-reconnect` |

> Флаги CLI имеют приоритет над переменными окружения.

---

## Провайдеры

### `telemost` — Яндекс Телемост

Наиболее стабильный провайдер. Поддерживает DataChannel, VideoChannel и SEI.

```bash
-provider telemost
```

### `jazz` — VK Jazz

Хорошая скорость. Поддерживает DataChannel и VideoChannel.

```bash
-provider jazz
```

### `wbstream` — Wildberries Stream

Альтернативный провайдер. Поддерживает DataChannel.

```bash
-provider wbstream
```

---

## Транспорты

### `datachannel` (рекомендуется)

Использует WebRTC DataChannel напрямую. Максимальная скорость, минимальная задержка.

### `videochannel`

Данные кодируются в luma-плоскость синтетических H.264-кадров 320×240. Поток выглядит как видеозвонок.

### `seichannel`

Данные встраиваются в SEI NAL-юниты H.264. Самый незаметный транспорт, скорость ~240 Кбит/с.

### `vp8channel`

Данные встраиваются в VP8-битстрим. Компромисс между заметностью и скоростью.

---

## Примеры конфигураций

### Сервер — продакшн

```bash
olcrtc-core \
  -mode server \
  -provider telemost \
  -key "${OLCRTC_KEY}" \
  -transport datachannel \
  -forward 0.0.0.0:0 \
  -name prod-server-01 \
  -dns 1.1.1.1:53 \
  -handshake-timeout 20s \
  -dial-timeout 45s \
  -reconnect 30s \
  -log info
```

### Клиент — через корпоративный SOCKS5

```bash
olcrtc-core \
  -mode client \
  -provider telemost \
  -room XXXX-YYYY-ZZZZ \
  -key "${OLCRTC_KEY}" \
  -transport datachannel \
  -listen 127.0.0.1:1080 \
  -socks corporate-proxy.example.com:1080 \
  -reconnect 10s \
  -log debug
```

### Клиент — максимальная скрытность (SEI)

```bash
olcrtc-core \
  -mode client \
  -provider telemost \
  -room XXXX-YYYY-ZZZZ \
  -key "${OLCRTC_KEY}" \
  -transport seichannel \
  -listen 127.0.0.1:1080 \
  -log warn
```

---

## Таблица совместимости провайдер × транспорт

| | `datachannel` | `videochannel` | `seichannel` | `vp8channel` |
|---|:---:|:---:|:---:|:---:|
| `telemost` | ✅ | ✅ | ✅ | ✅ |
| `jazz` | ✅ | ✅ | ⚠️ | ✅ |
| `wbstream` | ✅ | ⚠️ | ❌ | ⚠️ |

- ✅ Полная поддержка
- ⚠️ Нестабильно или ограниченно
- ❌ Не поддерживается

*[← Вернуться к мануалу](manual.md)*
