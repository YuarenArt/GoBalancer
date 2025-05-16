
# GoBalancer

**GoBalancer** — это простой и эффективный HTTP-балансировщик нагрузки, написанный на Go. Он поддерживает выбор бэкендов по алгоритму "наименьшее количество соединений" (least connections), проверку здоровья бэкендов (health checks) и ограничение скорости запросов (rate limiting) с использованием алгоритма Token Bucket.

## Требования
Для работы с проектом вам понадобятся:
- **Go** версии 1.24.
- **Docker** и **Docker Compose** для развертывания через контейнеры.
- **Make** для использования `Makefile` (опционально, но упрощает сборку и запуск).

## Установка и сборка

### 1. Клонирование репозитория
Склонируйте репозиторий на свою машину:
```bash
git clone https://github.com/YuarenArt/GoBalancer.git
cd GoBalancer
```

### 2. Установка зависимостей
Убедитесь, что у вас установлен Go, и загрузите зависимости проекта:
```bash
make deps
```
Или, если вы не используете `Makefile`:
```bash
go mod tidy
go mod download
```

### 3. Сборка проекта
Соберите бинарный файл `gobalancer`:
```bash
make build
```
Или вручную:
```bash
go build -ldflags="-s -w" -o gobalancer cmd/server/main.go
```

### Запуск через Docker
1. Убедитесь, что у вас установлены Docker и Docker Compose.
2. Соберите и запустите контейнеры:
   ```bash
   make docker-build
   make docker-up
   ```
   Это создаст два Docker-образа: `gobalancer-balancer` (основной сервис) и `gobalancer-mock` (мок-сервисы для бэкендов), а затем запустит их через `docker-compose`.

3. Проверьте, что сервисы работают:
   ```bash
   curl http://localhost:8080/
   ```
   Вы также можете проверить состояние бэкендов:
   ```bash
   curl http://localhost:8081/health
   curl http://localhost:8082/health
   ```

4. Просмотрите логи:
   ```bash
   make docker-logs
   ```

5. Остановите контейнеры, когда закончите:
   ```bash
   make docker-down
   ```

## Конфигурация
GoBalancer поддерживает конфигурацию через:
- **Файл `config.yaml`** (в корневой директории).
- **Переменные окружения**.
- **Флаги командной строки**.

### Пример `config.yaml`
```yaml
http_port: "8080"
backends:
  - "http://localhost:8081"
  - "http://localhost:8082"
log_type: "slog"
log_to_file: "false"
log_file: "logs/balancer.log"
rate_limit:
  enabled: "false"
  default_rate: "10"
  default_burst: "20"
  type: "token_bucket"
  ticker_duration: "1s"
balancer:
  type: "least_conn"
```

