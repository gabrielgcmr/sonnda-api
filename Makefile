# Makefile
# ==============================================================================
# 🛠️ CONFIGURAÇÕES E VARIÁVEIS
# ==============================================================================
APP_NAME := sonnda
MAIN     := ./cmd/api
VERSION ?= 1.0.0
LDFLAGS := -s -w -X github.com/gabrielgcmr/sonnda/cmd/api.version=$(VERSION)
SQLC_SPEC := internal/infrastructure/database/postgres/sqlc/sqlc.yaml

# ==============================================================================
# 🎯 TARGETS PRINCIPAIS
# ==============================================================================
.PHONY: all dev dev-air build clean generate test help openapi-export tools-air

all: build

# Roda apenas o backend (sem Air)
dev:
	go run $(MAIN)

# Roda backend com hot reload via Air
dev-air:
	go run github.com/air-verse/air@latest -c .air.toml

build:
	go build -o bin/$(APP_NAME) -ldflags "$(LDFLAGS)" $(MAIN)

# Limpeza (Compatível com Linux/WSL)
clean:
	@echo "🧹 Limpando binários e cache..."
	rm -rf bin

test:
	go test ./... -v

# ==============================================================================
# 🔄 WATCHERS E PROCESSOS INTERNOS
# ==============================================================================
.PHONY: air-run

air-run:
	go run github.com/air-verse/air@latest -c .air.toml

# Instala o Air localmente em ./bin (para uso em dev / CI)
tools-air:
	@mkdir -p bin
	GOBIN=$(CURDIR)/bin go install github.com/air-verse/air@latest

# ==============================================================================
# 🐘 DATABASE
# ==============================================================================
.PHONY: sqlc sqlc-check 

sqlc:
	go tool sqlc generate -f $(SQLC_SPEC)

sqlc-check:
	go tool sqlc compile -f $(SQLC_SPEC)

# ==============================================================================
# 🧬 CODEGEN
# ==============================================================================
openapi-export:
	go run ./cmd/openapi-export -output artifacts/openapi.json -version $(VERSION)

generate: sqlc

# ==============================================================================
# 🐘 DOCKER
# ==============================================================================
.PHONY: docker-up docker-down

docker-up:
	docker compose up -d

docker-down:
	docker compose down	

# ==============================================================================
# ℹ️ AJUDA
# ==============================================================================
help:
	@echo "Comandos disponíveis:"
	@echo "  dev         - Inicia apenas o Backend (sem Air)"
	@echo "  dev-air     - Inicia apenas o Backend (com Air)"
	@echo "  build       - Gera o binário de produção"
	@echo "  clean       - Limpa pastas geradas"
	@echo "  generate    - Gera o SQLC"
	@echo "  openapi-export - Exporta o OpenAPI dinamico do Huma para artifacts/openapi.json"
	@echo "  tools-air   - Instala o Air em ./bin"
	@echo "  docker-up   - Sobe o docker"
	@echo "  docker-down - Derruba o docker"

# ==============================================================================
# 📚 OPENAPI
# ==============================================================================
