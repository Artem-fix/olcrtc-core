# Траблшутинг

## Диагностика

### Включение debug-логов

```bash
olcrtc-core ... -log debug -dev
```

В режиме `debug` выводятся:
- Все входящие/исходящие соединения
- Прогресс handshake
- Статистика mux-потоков (байты sent/recv)
- Ошибки RTP/DataChannel

### Проверка связи с провайдером

```bash
# Проверить доступность API Telemost
curl -sv https://telemost.yandex.ru/ 2>&1 | grep -E "Connected|SSL|HTTP"

# Проверить STUN (требуется для WebRTC ICE)
stunclient stun.l.google.com 19302
```

---

## Типичные ошибки

### `provider "telemost" not found`

**Причина:** провайдер не зарегистрирован в бинаре.

**Решение:** убедитесь, что в `main.go` или `init()` импортирован пакет провайдера:
```go
import _ "github.com/openlibrecommunity/olcrtc-core/provider/telemost"
```

---

### `join room: context deadline exceeded`

**Причина:** нет доступа к API провайдера.

**Проверьте:**
1. Интернет на сервере: `curl https://telemost.yandex.ru/`
2. DNS: `nslookup telemost.yandex.ru`
3. Если нужен прокси: добавьте `-socks <proxy_host>:<port>`

---

### `dial transport: context deadline exceeded`

**Причина:** WebRTC ICE не установил соединение.

**Проверьте:**
1. STUN: `stunclient stun.l.google.com 19302`
2. Firewall: UDP-порт 3478 и диапазон эфемерных портов (32768–60999)
3. Попробуйте другой транспорт: `-transport seichannel`

---

### `handshake failed: decrypt server hello: aead open`

**Причина:** несовпадение PSK между клиентом и сервером.

```bash
# Проверьте длину ключа (должно быть 64)
echo -n "your_key_here" | wc -c

# Сравните ключи на обеих машинах (первые/последние 8 символов)
echo -n "your_key_here" | cut -c1-8
echo -n "your_key_here" | rev | cut -c1-8 | rev
```

---

### `mux server session: EOF`

**Причина:** клиент закрыл соединение до открытия mux-сессии.

Включите debug-логи и найдите предшествующую ошибку — она укажет на реальную причину.

---

### `socks5 accept: use of closed network connection`

Это **информационное** сообщение, не ошибка. Появляется при штатном завершении — listener закрывается корректно.

---

### `route: registry: "direct" not found`

**Причина:** диспетчер не нашёл зарегистрированный хендлер.

**Решение:** убедитесь, что серверный код регистрирует хендлер `"direct"` до старта сессии.

---

## Производительность

### Низкая скорость на `videochannel`

- Ограничение: ~3 Мбит/с из-за размера luma-буфера 320×240.
- **Решение:** переключитесь на `datachannel` для максимальной скорости.

### Высокая задержка

```bash
# Измерить добавленную задержку
time curl --socks5 127.0.0.1:1080 https://ifconfig.me
```

Нормальная добавленная задержка через DataChannel: **+20–80 мс** к базовой RTT.

Если задержка >500 мс:
1. Проверьте регион провайдера (ICE выбирает ближайший STUN/TURN)
2. Попробуйте другой провайдер

### Утечки горутин (для разработчиков)

```bash
# Тесты с race detector
go test -race ./...

# pprof (если включён в debug-сборке)
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

---

## FAQ

**Q: Можно ли запустить несколько клиентов на одном устройстве?**

```bash
olcrtc-core -listen 127.0.0.1:1080 -room ROOM1 -key KEY ...
olcrtc-core -listen 127.0.0.1:1081 -room ROOM2 -key KEY ...
```

---

**Q: Как ротировать ключ без даунтайма?**

1. Запустите новый сервер с новым ключом и новой комнатой.
2. Обновите подписку с новым URI.
3. После переключения всех клиентов остановите старый сервер.

---

**Q: Поддерживается ли UDP-трафик?**

В текущей версии ядро туннелирует только TCP через SOCKS5. UDP-проксирование планируется в следующих версиях.

---

**Q: Работает ли на Android/iOS?**

Да, через `golang.org/x/mobile`. Мобильное API экспортируется из пакета `mobile/`. AAR для Android собирается через `mage mobile`.

---

**Q: Как убедиться, что трафик идёт через сервер?**

```bash
# IP должен совпадать с IP сервера
curl --socks5 127.0.0.1:1080 https://ifconfig.me

# Проверка через DNS leak test
curl --socks5 127.0.0.1:1080 https://bash.ws/dnsleak/test/1234
```

---

**Q: Что делать если провайдер изменил API?**

Провайдер-адаптеры находятся в `core/provider/<name>/`. При изменении API достаточно обновить только адаптер. Следите за [issues](https://github.com/openlibrecommunity/olcrtc/issues).

---

*[← Вернуться к мануалу](manual.md)*
