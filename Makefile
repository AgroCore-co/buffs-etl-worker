.PHONY: dev build run test lint clean docker-build docker-up docker-down

# ── Variáveis ────────────────────────────────────────────────────────────────
APP_NAME := etl-buffs
BUILD_DIR := ./bin
MAIN := ./cmd/server

# ── Desenvolvimento ──────────────────────────────────────────────────────────

## dev: Roda o servidor em modo desenvolvimento com hot-reload
dev:
	@echo "🚀 Iniciando em modo desenvolvimento..."
	go run $(MAIN)

## build: Compila o binário otimizado
build:
	@echo "🔨 Compilando $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -a -installsuffix cgo -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)
	@echo "✅ Binário em $(BUILD_DIR)/$(APP_NAME)"

## run: Compila e roda o binário
run: build
	$(BUILD_DIR)/$(APP_NAME)

## test: Roda todos os testes
test:
	go test ./... -v -race -cover

## test-coverage: Gera relatório de cobertura
test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Relatório em coverage.html"

## lint: Roda linter (requer golangci-lint)
lint:
	golangci-lint run ./...

## clean: Remove artefatos de build
clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html

# ── Docker ───────────────────────────────────────────────────────────────────

## docker-build: Build da imagem Docker
docker-build:
	docker build -t $(APP_NAME):latest .

## docker-up: Sobe o stack completo (ETL)
docker-up:
	docker-compose up -d

## docker-down: Para todo o stack
docker-down:
	docker-compose down

## docker-logs: Mostra logs do ETL
docker-logs:
	docker-compose logs -f etl-buffs

# ── Utilitários ──────────────────────────────────────────────────────────────

## deps: Instala/atualiza dependências
deps:
	go mod tidy
	go mod download

## fmt: Formata o código
fmt:
	gofmt -w .
	goimports -w .

## help: Mostra esta mensagem de ajuda
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
