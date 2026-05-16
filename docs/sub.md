# Подписки (Subscription)

## Что такое подписка

Подписка — файл или URL со списком URI для подключения. Используется для:
- централизованного управления несколькими серверами;
- автоматического обновления параметров при смене комнаты/ключа;
- распространения конфигурации среди множества клиентов.

---

## Формат файла подписки

Один URI на строку. Пустые строки и строки, начинающиеся с `#`, игнорируются.
файл подписки olcrtc-core
Обновлено: 16 мая 2026 г.
olcrtc://telemost/CONF-MAIN-001?key=a3f8c2d1...&transport=datachannel&name=main-server olcrtc:// jazz/JZ-BACKUP-002?key=a3f8c2d1...&transport=videochannel&name=backup-server olcrtc://telemost/CONF-SEI-003?key=a3f8c2d1...&transport=seichannel&name=stealth-server

### Правила обработки

1. Каждая строка — валидный `olcrtc://` URI.
2. Клиент выбирает первый доступный сервер из списка (сверху вниз).
3. При недоступности первого сервера переключается на следующий.
4. Если все серверы недоступны — ждёт `reconnect`-задержку и повторяет.

---

## Использование подписки

### Локальный файл

```bash
olcrtc-core -mode client -sub /etc/olcrtc/subscription.txt -listen 127.0.0.1:1080
```

### URL подписки (HTTP/HTTPS)

```bash
olcrtc-core -mode client \
  -sub https://example.com/sub/my-subscription.txt \
  -listen 127.0.0.1:1080 \
  -sub-update 3600s
```

> Подписка загружается при старте и обновляется с интервалом `-sub-update`. Активные соединения при обновлении не разрываются.

---

## Раздача подписки

### Через nginx

```bash
cat > /var/www/sub/olcrtc.txt << 'EOF'
olcrtc://telemost/CONF-2026-ABCD?key=YOUR_KEY_HERE&transport=datachannel
EOF
```

```nginx
location /sub/ {
    root /var/www;
    default_type text/plain;
    add_header Cache-Control "no-store";
}
```

### С базовой аутентификацией

```bash
olcrtc-core -mode client \
  -sub https://user:password@example.com/private/sub.txt \
  -listen 127.0.0.1:1080
```

---

## Формат Base64

Для передачи через мессенджеры URI кодируется в Base64 с префиксом `b64:`:

```bash
# Кодирование
echo -n "olcrtc://telemost/CONF-2026-ABCD?key=..." | base64
```

В файле подписки:
b64 :b2xjcnRjOi8vdGVsZW1vc3QvQ09ORi0yMDI2LUFCQ0Q /a2V5PS4uLg==

Клиент автоматически определяет Base64-строки и декодирует их.

---

## Логика балансировки
Список серверов: [A, B, C]
Попытка 1: А → успех → работаем Разрыв А → Попытка 2: А → неудача → Попытка 3: Б → → работаем Разрыв Б → Попытка 4: Б → неудача → Попытка 5: С → успех → работаем Разрыв С → Попытка 6: А → ...