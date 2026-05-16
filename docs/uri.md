# URI-формат olcrtc-core

## Назначение

URI позволяет передавать параметры подключения в компактном виде — для ручного ввода, QR-кода или вставки в подписку. Клиент разбирает URI и автоматически конфигурирует сессию без ручного указания флагов.

---

## Синтаксис
olcrtc:// <provider> /<room_id>?key=<hex_key>[&transport= <kind> ][&name=<display_name>][&socks= <proxy> ]

### Компоненты

| Компонент | Обязателен | Описание |
|-----------|-----------|----------|
| `olcrtc://` | ✅ | Схема URI |
| `<provider>` | ✅ | Имя провайдера: `telemost`, `jazz`, `wbstream` |
| `<room_id>` | ✅ | ID комнаты, полученный от сервера |
| `key` | ✅ | 64-символьный hex PSK |
| `transport` | ❌ | Транспорт. По умолчанию: `datachannel` |
| `name` | ❌ | Отображаемое имя. По умолчанию: `olcrtc-core` |
| `socks` | ❌ | SOCKS5-прокси для API-запросов: `host:port` |

---

## Примеры

### Минимальный URI
olcrtc://telemost/CONF-2026-ABCD?key=a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4

### Полный URI с параметрами
olcrtc://telemost/CONF-2026-ABCD?key=a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4&transport=videochannel&name=alice&socks=10.0.0.1 :1080

### Jazz через SEI
olcrtc://jazz/JZ-ROOM-XY99?key=b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2&transport=seichannel

---

## Правила кодирования

- `room_id` может содержать `-` и буквенно-цифровые символы. URL-кодирование не требуется.
- `key` — строго 64 hex-символа (`a–f`, `0–9`), регистр не важен, рекомендуется нижний.
- `name` — URL-кодируется стандартным образом (`%20` для пробела).
- `socks` — формат `host:port`.

---

## Генерация URI на сервере

После запуска сервер выводит URI в лог:

```json
{
  "level": "info",
  "logger": "session.server",
  "msg": "server ready",
  "room_id": "CONF-2026-ABCD",
  "uri": "olcrtc://telemost/CONF-2026-ABCD?key=<key_placeholder>"
}
```

> `<key_placeholder>` — напоминание подставить ключ вручную. Сервер не логирует ключ в открытом виде.

---

## QR-код

```bash
# Через qrencode (Linux)
qrencode -t UTF8 "olcrtc://telemost/CONF-2026-ABCD?key=..."

# Через Python
python3 -c "import qrcode; qrcode.make('olcrtc://telemost/CONF-2026-ABCD?key=...').show()"
```