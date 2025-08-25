.PHONY: help build run test clean docker-build docker-up docker-down dev-setup

# Переменные
APP_NAME = notification-mvp
SERVER_BIN = bin/server
CLIENT_BIN = bin/client
SENDER_BIN = bin/sender

# Цвета для вывода
GREEN = \033[32m
YELLOW = \033[33m
RED = \033[31m
NC = \033[0m # No Color

help: ## Показать справку
	@echo "$(GREEN)Доступные команды:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'

build: ## Собрать все бинарные файлы
	@echo "$(GREEN)Сборка приложений...$(NC)"
	@mkdir -p bin
	@go build -o $(SERVER_BIN) ./cmd/server
	@go build -o $(CLIENT_BIN) ./cmd/client
	@go build -o $(SENDER_BIN) ./cmd/sender
	@echo "$(GREEN)Сборка завершена!$(NC)"

run: build ## Запустить сервер
	@echo "$(GREEN)Запуск сервера...$(NC)"
	@./$(SERVER_BIN)

client: build ## Запустить тестовый клиент
	@echo "$(GREEN)Запуск клиента...$(NC)"
	@./$(CLIENT_BIN) -user=$(DEV_USER_ID) -login=$(DEV_LOGIN)

client-auto: build ## Запустить клиент с автоподтверждением
	@echo "$(GREEN)Запуск клиента с автоподтверждением...$(NC)"
	@./$(CLIENT_BIN) -user=$(DEV_USER_ID) -login=$(DEV_LOGIN) -auto

sender: build ## Отправить тестовое уведомление
	@echo "$(GREEN)Отправка тестового уведомления...$(NC)"
	@./$(SENDER_BIN) -user=$(DEV_USER_ID) -login=$(DEV_LOGIN) -message="Test notification from Makefile"

sender-multiple: build ## Отправить несколько уведомлений
	@echo "$(GREEN)Отправка множественных уведомлений...$(NC)"
	@./$(SENDER_BIN) -user=$(DEV_USER_ID) -login=$(DEV_LOGIN) -count=5 -interval=2s -message="Multiple notification test"

test: ## Запустить тесты (в данной реализации тестов нет)
	@echo "$(YELLOW)Тесты не реализованы в MVP версии$(NC)"

clean: ## Очистить бинарные файлы
	@echo "$(GREEN)Очистка...$(NC)"
	@rm -rf bin/
	@echo "$(GREEN)Очистка завершена!$(NC)"

docker-build: ## Собрать Docker образы
	@echo "$(GREEN)Сборка Docker образов...$(NC)"
	@docker-compose build
	@echo "$(GREEN)Docker образы собраны!$(NC)"

docker-up: ## Запустить все сервисы в Docker
	@echo "$(GREEN)Запуск сервисов в Docker...$(NC)"
	@docker-compose up -d
	@echo "$(GREEN)Сервисы запущены!$(NC)"
	@echo "$(YELLOW)Сервер доступен по адресу: http://localhost:8080$(NC)"
	@echo "$(YELLOW)Веб-клиент доступен по адресу: http://localhost:8080$(NC)"

docker-down: ## Остановить все сервисы Docker
	@echo "$(GREEN)Остановка сервисов...$(NC)"
	@docker-compose down
	@echo "$(GREEN)Сервисы остановлены!$(NC)"

docker-logs: ## Показать логи сервисов
	@docker-compose logs -f

docker-redis: ## Подключиться к Redis CLI
	@docker-compose exec redis redis-cli

dev-setup: ## Настроить окружение для разработки
	@echo "$(GREEN)Настройка окружения для разработки...$(NC)"
	@echo "$(YELLOW)Убедитесь что у вас установлены:$(NC)"
	@echo "  - Go 1.25+"
	@echo "  - Docker и Docker Compose"
	@echo "  - Make"
	@echo ""
	@echo "$(GREEN)Для запуска локально:$(NC)"
	@echo "  1. Запустите Redis: docker run -d -p 6379:6379 redis:7-alpine"
	@echo "  2. Запустите сервер: make run"
	@echo "  3. В другом терминале запустите клиент: make client"
	@echo "  4. В третьем терминале отправьте уведомление: make sender"
	@echo ""
	@echo "$(GREEN)Для запуска в Docker:$(NC)"
	@echo "  1. make docker-up"
	@echo "  2. Откройте http://localhost:8080 в браузере"

demo: ## Демонстрация работы сервиса
	@echo "$(GREEN)Демонстрация работы сервиса...$(NC)"
	@echo "$(YELLOW)Запуск в фоне...$(NC)"
	@SERVER_ADDR=:8081 ./$(SERVER_BIN) & echo $$! > .server_demo_pid
	@sleep 3
	@echo "$(GREEN)Отправка уведомления...$(NC)"
	@./$(SENDER_BIN) -addr=http://localhost:8081 -user=1 -login=demo_user -message="Demo notification"
	@sleep 2
	@echo "$(GREEN)Остановка сервера...$(NC)"
	@kill `cat .server_demo_pid` || true; rm -f .server_demo_pid

# Специальные цели для разработки
redis-local: ## Запустить Redis локально в Docker
	@echo "$(GREEN)Запуск Redis...$(NC)"
	@docker start redis-local >/dev/null 2>&1 || docker run -d --name redis-local -p 6379:6379 redis:7-alpine
	@echo "$(GREEN)Redis запущен на порту 6379$(NC)"

redis-stop: ## Остановить локальный Redis
	@echo "$(GREEN)Остановка Redis...$(NC)"
	@docker stop redis-local || true
	@echo "$(GREEN)Redis остановлен$(NC)"

# Загрузка переменных из .env файла
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Устанавливаем значения по умолчанию если переменные не определены
DEV_USER_ID ?= 1
DEV_LOGIN ?= test_user
