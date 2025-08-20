# Многоэтапная сборка для оптимизации размера образа
FROM golang:1.25-alpine AS builder

# Устанавливаем git и ca-certificates для работы с HTTPS
RUN apk add --no-cache git ca-certificates

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложения
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o client ./cmd/client
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sender ./cmd/sender

# Этап для сервера
FROM alpine:latest AS server

RUN apk --no-cache add ca-certificates curl

WORKDIR /root/

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]

# Этап для клиента
FROM alpine:latest AS client

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/client .

CMD ["./client"]

# Этап для отправителя
FROM alpine:latest AS sender

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/sender .

CMD ["./sender"]
