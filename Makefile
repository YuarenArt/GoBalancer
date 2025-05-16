PROJECT_NAME = GoBalancer
BINARY_NAME = gobalancer
MAIN_PATH = cmd/server/main.go
DOCKERFILE = Dockerfile
DOCKER_COMPOSE_FILE = docker-compose.yml
DOCKER_IMAGE = gobalancer-balancer
DOCKER_MOCK_IMAGE = gobalancer-backend

GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOMOD = $(GOCMD) mod

# Флаги сборки
BUILD_FLAGS = -ldflags="-s -w"

# Цвета для вывода
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
RESET  := $(shell tput -Txterm sgr0)

.PHONY: all
all: build

.PHONY: build
build:
	@echo "$(YELLOW)Building $(PROJECT_NAME)...$(RESET)"
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)Build completed: $(BINARY_NAME)$(RESET)"


.PHONY: clean
clean:
	@echo "$(YELLOW)Cleaning up...$(RESET)"
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	docker rm -f $(DOCKER_IMAGE) $(DOCKER_MOCK_IMAGE) 2>/dev/null || true
	docker rmi $(DOCKER_IMAGE):latest $(DOCKER_MOCK_IMAGE):latest 2>/dev/null || true
	@echo "$(GREEN)Cleanup completed$(RESET)"

.PHONY: test
test:
	@echo "$(YELLOW)Running tests...$(RESET)"
	$(GOTEST) -v ./...
	@echo "$(GREEN)Tests completed$(RESET)"

.PHONY: deps
deps:
	@echo "$(YELLOW)Installing dependencies...$(RESET)"
	$(GOMOD) tidy
	$(GOMOD) download
	@echo "$(GREEN)Dependencies installed$(RESET)"

.PHONY: run
run: build
	@echo "$(YELLOW)Running $(PROJECT_NAME)...$(RESET)"
	./$(BINARY_NAME)
	@echo "$(GREEN)Application stopped$(RESET)"

.PHONY: docker-build
docker-build:
	@echo "$(YELLOW)Building Docker image for $(DOCKER_IMAGE)...$(RESET)"
	docker build -f $(DOCKERFILE) -t $(DOCKER_IMAGE):latest .
	@echo "$(YELLOW)Building Docker image for mock services...$(RESET)"
	docker build -f Dockerfile.backend -t $(DOCKER_MOCK_IMAGE):latest .
	@echo "$(GREEN)Docker images built: $(DOCKER_IMAGE):latest, $(DOCKER_MOCK_IMAGE):latest$(RESET)"

.PHONY: docker-up
docker-up: docker-build
	@echo "$(YELLOW)Starting Docker Compose...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) up -d
	@echo "$(GREEN)Docker Compose started$(RESET)"

.PHONY: docker-down
docker-down:
	@echo "$(YELLOW)Stopping and removing Docker Compose...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) down
	@echo "$(GREEN)Docker Compose stopped and removed$(RESET)"

.PHONY: docker-logs
docker-logs:
	@echo "$(YELLOW)Showing Docker logs...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f --no-log-prefix

.PHONY: docker-restart
docker-restart: docker-down docker-up

.PHONY: help
help:
	@echo "$(YELLOW)Makefile commands for $(PROJECT_NAME):$(RESET)"
	@echo "  $(GREEN)all$(RESET)          - Build the project (default)"
	@echo "  $(GREEN)build$(RESET)       - Build the binary"
	@echo "  $(GREEN)clean$(RESET)       - Clean build artifacts and Docker images"
	@echo "  $(GREEN)test$(RESET)        - Run tests"
	@echo "  $(GREEN)deps$(RESET)        - Install dependencies"
	@echo "  $(GREEN)run$(RESET)         - Run the application locally"
	@echo "  $(GREEN)docker-build$(RESET) - Build Docker images"
	@echo "  $(GREEN)docker-up$(RESET)    - Start Docker Compose"
	@echo "  $(GREEN)docker-down$(RESET)  - Stop and remove Docker Compose"
	@echo "  $(GREEN)docker-logs$(RESET)  - Show Docker logs"
	@echo "  $(GREEN)docker-restart$(RESET) - Restart Docker Compose"
	@echo "  $(GREEN)help$(RESET)        - Show this help"
