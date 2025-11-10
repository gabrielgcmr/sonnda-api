# Makefile
.PHONY: run build docker-up docker-down docker-restart clean

# Executar localmente
run:
	go run cmd/api/main.go

# Build da aplicação
build:
	go build -o bin/api cmd/api/main.go

# Executar com hot reload (air)
dev:
	air

# Docker
docker-up:
	docker-compose up

docker-db:
	docker-compose up -d db

docker-down:
	docker-compose down

docker-restart: docker-down docker-up

docker-dev:
	docker-compose up -d db
	go run cmd/api/main.go

# Limpeza
clean:
	rm -rf bin/
	docker system prune -f

# Database
db-migrate:
	go run cmd/migrate/main.go

# Testes
test:
	go test ./...

# Ajuda
help:
	@echo "Comandos disponíveis:"
	@echo "  run           - Executar aplicação local"
	@echo "  dev           - Executar com hot reload (air)"
	@echo "  build         - Build da aplicação"
	@echo "  docker-up     - Subir containers Docker"
	@echo "  docker-dev    - Subir containers com rebuild"
	@echo "  docker-down   - Parar containers"
	@echo "  docker-restart- Reiniciar containers"
	@echo "  test          - Executar testes"