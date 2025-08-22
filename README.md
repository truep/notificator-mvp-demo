# Notification MVP Service (Go + Redis Streams)

## Обзор
MVP сервиса уведомлений на Go, использующего Redis Streams для доставки в реальном времени через WebSocket с поддержкой:
- Групповой публикации уведомлений по персональным стримам
- TTL 15 минут для payload в Redis
- Отправка новым и последним 100 событиям при подключении
- ACK помечает `read`, данные не удаляются из стрима
- WebUI для демонстрации (http://localhost:8080)

## Архитектура
- HTTP API: `POST /api/v1/notify`
- WebSocket: `GET /ws?user_id=&login=`
- Redis:
  - `stream:<user_id>-<login>` — XADD MAXLEN=100, хранит `nid`
  - `notification:<uuid>` — payload, TTL=15m
  - `notification_state:<user>` — HSET `<uuid>` = `read`
- Consumer Group: `notifications` (per-user stream)

## Поток данных
```
[ Клиент (POST /notify) ]
    |
    v
[ API-сервис ]
    |---> SETEX notification:<uuid> (TTL=15m)
    '---> XADD stream:<user> notification_id=<uuid> (MAXLEN=100)
    
[ Redis ]
  ├── notification:<uuid> (payload, TTL=15m)
  ├── stream:<user> [ {nid}, ... ]
  └── notification_state:<user> [uuid → read]

[ WS-сервис ]
   ├─ Registry: user → [ws1, ws2...]
   ├─ Consumer (XREADGROUP)
   ├─ Initial sync: XRANGE last 100 + HMGET read
   └─ Dispatcher (payload + send → WS)
```

## Контракты
### HTTP: POST /api/v1/notify
Request:
```json
{
  "target": [
    {"id": 1, "login": "admin_user"},
    {"id": 2, "login": "post7"}
  ],
  "message": "Оператор…",
  "created_at": "2023-10-27T10:00:00Z",
  "source": "chat-control-api"
}
```
Response (202):
```json
{
  "result": [
    {"target": {"id":1,"login":"admin_user"}, "notification_id": "uuid1"},
    {"target": {"id":2,"login":"post7"},       "notification_id": "uuid2"}
  ]
}
```

### WebSocket: GET /ws?user_id=&login=
Сообщение от сервера:
```json
{
  "type": "notification.push",
  "data": {
    "notification_id": "uuid",
    "stream_id": "1658926500000-0",
    "message": "…",
    "created_at": "2024-01-01T12:00:00Z",
    "source": "chat-api",
    "status": "unread | auto_cleared",
    "read": false
  }
}
```
ACK от клиента:
```json
{
  "type": "notification.read",
  "data": {"notification_id":"uuid","stream_id":"1658926500000-0"}
}
```
ACK от сервера:
```json
{"type":"notification.read.ack","data":{"notification_id":"uuid","stream_id":"1658926500000-0"}}
```

### Admin API
- GET `/api/v1/admin/clients`
- GET `/api/v1/admin/users`
- GET `/api/v1/admin/pending` — теперь включает `read` (если payload есть)
- GET `/api/v1/admin/history?user_id=&login=` — XRANGE последних 100 с `read/unread`

Пример ответа `/api/v1/admin/pending`:
```json
{
  "pending_notifications": {
    "1-alice": [
      {"id": "1234-0", "payload": {"notification_id": "uuid", "message": "Hello", "created_at": "2024-01-01T12:00:00Z"}, "read": false}
    ]
  },
  "total_pending": 1,
  "users_with_pending": 1
}
```

Пример ответа `/api/v1/admin/history`:
```json
{
  "user": {"id": 1, "login": "alice"},
  "history": [
    {"id": "1234-0", "payload": {"notification_id":"uuid","message":"Hi"}, "read": true, "status": "unread"}
  ],
  "count": 1
}
```

## Веб-интерфейс (http://localhost:8080)
Функции:
- Список подключенных клиентов (автообновление 3s)
- Отправка уведомлений: manual, всем подключенным, выбор нескольких
- Pending уведомления (автообновление 5s)
- История пользователя (последние 100) с бейджами READ/UNREAD
- Авто-ACK только для непрочитанных

Демо сценарии:
1) Несколько клиентов — отправка всем подключенным
2) Pending — для оффлайн пользователя; появится после повторной отправки
3) Real-time мониторинг — автообновляемые счетчики и списки

## Сборка и запуск
С Docker Compose:
```bash
make docker-up
# Открыть UI
xdg-open http://localhost:8080 || open http://localhost:8080
```
Локально:
```bash
make redis-local
make run        # сервер
make client     # ws клиент
make sender     # отправитель
```

## Конфигурация
- `SERVER_ADDR` (default `:8080`)
- `REDIS_ADDR` (default `localhost:6379`)
- `REDIS_PASSWORD`

## Технические детали
- Redis Streams + Consumer Groups, ConsumerID = `user:<id>`
- XADD MAXLEN=100, payload TTL = 15m
- Initial sync: XRANGE 100 + HMGET read
- ACK: XACK + HSET `notification_state:<user>` `<uuid>` = `read`

## Ограничения
- Без аутентификации/метрик и т.п. (MVP)

## Структура
```
cmd/     # server, client, sender
internal/# domain, repository, service, handler, websocket, worker
```
