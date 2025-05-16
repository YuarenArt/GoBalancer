# Этап 1: Сборка приложения
FROM golang:1.24 AS builder

WORKDIR /app

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /gobalancer ./cmd/server/main.go

FROM alpine:3.18

WORKDIR /app

COPY --from=builder /gobalancer /app/gobalancer

RUN apk add --no-cache ca-certificates

RUN mkdir -p /app/logs

EXPOSE 8080

# Запускаем приложение
CMD ["/app/gobalancer"]