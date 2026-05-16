# Быстрый старт

## Требования

| Компонент | Минимум |
|-----------|---------|
| Go | 1.25+ |
| ОС сервера | Linux amd64 / arm64 |
| ОС клиента | Linux / macOS / Windows |
| Порты (исходящие) | HTTPS (443), STUN (3478/udp) |

---

## 1. Сборка из исходников

```bash
git clone https://github.com/Artem-fix/olcrtc-core
cd olcrtc-core

# Зависимости
go mod download

# Сборка CLI
go build -trimpath -ldflags="-s -w" -o olcrtc-core ./cmd/olcrtc-core

# Кросс-компиляция для Linux (с macOS/Windows)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o olcrtc-core-linux-amd64 ./cmd/olcrtc-core
```

---

## 2. Генерация ключа

Обе стороны (клиент и сервер) должны иметь одинаковый 256-битный ключ в hex-кодировке (64 символа).

```bash
# Через openssl
openssl rand -hex 32
```

Пример ключа:
a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4

> ⚠️ Никогда не передавайте ключ по незашифрованным каналам. Используйте SSH, Signal или другой E2E-зашифрованный мессенджер.

---

## 3. Запуск сервера

```bash
./olcrtc-core \
  -mode server \
  -provider telemost \
  -key a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4 \
  -forward 0.0.0.0:0 \
  -name my-server \
  -log info
```

После старта сервер выведет в лог **Room ID**:

```json
{
  "level": "info",
  "logger": "session.server",
  "msg": "server ready",
  "room_id": "XXXX-YYYY-ZZZZ"
}
```

Скопируйте `room_id` — он нужен клиенту.

---

## 4. Запуск клиента

```bash
./olcrtc-core \
  -mode client \
  -provider telemost \
  -room XXXX-YYYY-ZZZZ \
  -key a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4 \
  -listen 127.0.0.1:1080 \
  -log info
```

После подключения:

```json
{"level":"info","logger":"session.client","msg":"mux session established"}
{"level":"info","logger":"session.client","msg":"listening","addr":"127.0.0.1:1080"}
```

---

## 5. Проверка

```bash
# Проверка через curl с SOCKS5-прокси
curl --socks5 127.0.0.1:1080 https://ifconfig.me

# Или через переменные окружения
export ALL_PROXY=socks5://127.0.0.1:1080
curl https://ifconfig.me
```

Должен вернуться IP-адрес вашего сервера, а не вашей локальной машины.

---

## 6. Docker (сервер)

```bash
# Сборка образа
docker build -t olcrtc-core:latest .

# Запуск
docker run -d \
  --name olcrtc-server \
  --restart unless-stopped \
  -e OLCRTC_PROVIDER=telemost \
  -e OLCRTC_KEY=a3f8c2d1e4b7a09f6c5d2e1f8b3a4c7d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4 \
  olcrtc-core:latest
```

---

## 7. Автозапуск через systemd

```ini
# /etc/systemd/system/olcrtc-core.service
[Unit]
Description=olcrtc-core tunnel server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/olcrtc-core \
  -mode server \
  -provider telemost \
  -key YOUR_KEY_HERE \
  -forward 0.0.0.0:0 \
  -log info
Restart=on-failure
RestartSec=10s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now olcrtc-core
sudo journalctl -u olcrtc-core -f
```

---
*[← Вернуться к мануалу](manual.md)*
