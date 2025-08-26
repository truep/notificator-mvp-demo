# Notification Service MVP

Высокопроизводительный сервис уведомлений реального времени на Go + Redis Streams с поддержкой WebSocket, TTL, подтверждений и кластеризации.

## Описание

Сервис предоставляет надежную платформу для доставки уведомлений в реальном времени через WebSocket с использованием Redis Streams в качестве брокера сообщений. Поддерживает горизонтальное масштабирование, распределенные блокировки и автоматическую очистку данных.

### Ключевые возможности

- 🚀 **Доставка в реальном времени** через WebSocket
- 📦 **Redis Streams** для надежных очередей сообщений  
- ⏰ **TTL управление** (15 минут для полезной нагрузки)
- ✅ **Подтверждения** и отслеживание статуса прочтения
- 🔄 **Consumer Groups** для гарантированной доставки
- 🌐 **Кластеризация** с поддержкой нескольких pod'ов
- 🧹 **Автоочистка** просроченных уведомлений
- 📊 **Мониторинг** через Prometheus метрики
- 🎛️ **Admin API** для управления и мониторинга
- 🖥️ **Web UI** для демонстрации и тестирования

### Архитектура

Сервис построен на принципах Clean Architecture с четким разделением слоев:

- **Domain**: Бизнес-модели и интерфейсы
- **Service**: Бизнес-логика  
- **Repository**: Доступ к данным
- **Handler**: HTTP/WebSocket обработчики
- **Workers**: Фоновые процессы

## Быстрый старт

### Docker Compose (рекомендуется)

```bash
# Клонирование репозитория
git clone <repository-url>
cd notification-mvp

# Запуск всех сервисов
make docker-up

# Открытие Web UI
open http://localhost:8080
```

### Локальная разработка

```bash
# 1. Запуск Redis
make redis-local

# 2. Запуск сервера
make run

# 3. В другом терминале - тестовый клиент
make client

# 4. В третьем терминале - отправка уведомления
make sender
```

## API

### HTTP API

```bash
# Отправка уведомления
curl -X POST http://localhost:8080/api/v1/notify \
  -H "Content-Type: application/json" \
  -d '{
    "target": [{"id": 1, "login": "alice"}],
    "message": "Привет из сервиса уведомлений!",
    "source": "demo-app"
  }'
```

### WebSocket API

```javascript
// Подключение
const ws = new WebSocket('ws://localhost:8080/ws?user_id=1&login=alice');

// Получение уведомления
ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  if (message.type === 'notification.push') {
    console.log('Новое уведомление:', message.data.message);
  }
};
```

## Мониторинг

- **Web UI**: http://localhost:8080
- **Health Check**: http://localhost:8080/health  
- **Metrics**: http://localhost:8080/metrics
- **Admin API**: http://localhost:8080/api/v1/admin/*

## Документация

📚 **Подробная документация доступна на двух языках:**

- 🇬🇧 **English**: [docs/en/README.md](./docs/en/README.md)
- 🇷🇺 **Русский**: [docs/ru/README.md](./docs/ru/README.md)

Документация включает:
- Детальную архитектуру с диаграммами Mermaid
- Полные API контракты и примеры
- Схему данных Redis 
- Руководства по интеграции для Backend и Frontend
- Примеры использования
- Техническую документацию

## Требования

- Go 1.25+
- Redis 7+
- Docker & Docker Compose (для контейнеризации)
- Make

## Технологический стек

- **Backend**: Go с Clean Architecture
- **Message Broker**: Redis Streams + Consumer Groups
- **WebSocket**: gorilla/websocket
- **Monitoring**: Prometheus метрики
- **Storage**: Redis (streams, hash, TTL)
- **Deployment**: Docker + Docker Compose

## Конфигурация

| Переменная       | По умолчанию     | Описание                 |
| ---------------- | ---------------- | ------------------------ |
| `SERVER_ADDR`    | `:8080`          | Адрес HTTP сервера       |
| `REDIS_ADDR`     | `localhost:6379` | Адрес Redis сервера      |
| `REDIS_PASSWORD` | ``               | Пароль Redis             |
| `POD_ID`         | `hostname`       | ID pod для кластеризации |

## Команды Make

```bash
make help           # Справка по командам
make build          # Сборка всех бинарных файлов
make run            # Запуск сервера
make client         # Запуск тестового клиента
make sender         # Отправка тестового уведомления
make docker-up      # Запуск в Docker
make docker-down    # Остановка Docker сервисов
make redis-local    # Запуск локального Redis
```

## Статус проекта

⚠️ **MVP версия** - готов для демонстрации и тестирования базовой функциональности.

### Ограничения MVP

- Нет аутентификации пользователей
- Нет rate limiting
- Базовая обработка ошибок
- Нет персистентности за пределами TTL

## Участие в разработке

1. Следуйте принципам Clean Architecture
2. Добавляйте тесты для новой функциональности  
3. Обновляйте документацию при изменениях API
4. Используйте осмысленные commit сообщения

## Лицензия

MIT License - см. [LICENSE](LICENSE) файл.

---

**Для получения полной документации перейдите в:**
- [📖 English Documentation](./docs/en/README.md)
- [📖 Русская документация](./docs/ru/README.md)
