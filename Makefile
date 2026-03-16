.PHONY: help build up down logs ps clean test lint

help:
	@echo "Available commands:"
	@echo "  make build     - Build all Docker images"
	@echo "  make up        - Start all services"
	@echo "  make down      - Stop all services"
	@echo "  make logs      - View logs"
	@echo "  make ps        - List running containers"
	@echo "  make clean     - Remove containers, volumes, and images"
	@echo "  make test      - Run tests"
	@echo "  make login     - Login to DHI registry"

LOG_OPTS=--follow --tail=100

build:
	docker compose build --no-cache

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs $(LOG_OPTS)

logs-api:
	docker compose logs $(LOG_OPTS) api

logs-nginx:
	docker compose logs $(LOG_OPTS) nginx

ps:
	docker compose ps

clean:
	docker compose down -v --rmi local

restart:
	docker compose restart

pull:
	docker compose pull

login:
	@echo "Login to DHI registry"
	@docker login dhi.io

test:
	docker compose exec api go test -v ./...

shell-api:
	docker compose exec api sh

shell-postgres:
	docker compose exec postgres psql -U $(DB_USER) -d $(DB_NAME)

health:
	@echo "Checking services health..."
	@curl -sf http://localhost/health || echo "Nginx: UNHEALTHY"
	@curl -sf http://localhost/api/v1 || echo "API: UNHEALTHY"
