# Comandos del proyecto. Requiere GNU Make, Docker y Go 1.22+ (tests/lint).
# En Windows, ejecutalo desde Git Bash o WSL (las recetas usan sh).

COMPOSE := docker compose --env-file .env -f deploy/docker-compose.yml
MOCK_COMPOSE := docker compose -f deploy/docker-compose.mock.yml
SPECTRAL_IMAGE := stoplight/spectral:6.14.3
CONTRACT := docs/contracts/benefits-api.v1.yaml
RULESET := docs/contracts/.spectral.yaml

GO_MODULES := pkg gateway \
	services/members-service services/booking-service services/benefits-service \
	services/training-service services/notification-worker

.DEFAULT_GOAL := help
.PHONY: help up down clean logs ps test lint fmt tidy env-check mock-up mock-down mock-logs contract-lint

help: ## Lista los comandos disponibles
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "} {printf "  %-15s %s\n", $$1, $$2}'

# ------------------------------------------------------------------ stack local

env-check:
	@test -f .env || { echo "Falta el archivo .env. Crealo con: cp .env.example .env"; exit 1; }

up: env-check ## Construye y levanta todo el stack; espera a que esté healthy
	$(COMPOSE) up -d --build --wait
	@echo ""
	@echo "Listo:  web http://localhost:5173  ·  gateway http://localhost:8080/api/v1/status"

down: env-check ## Baja el stack (conserva los datos)
	$(COMPOSE) down

clean: env-check ## Baja el stack y BORRA los volúmenes de datos
	$(COMPOSE) down -v

logs: env-check ## Logs en vivo (todos, o uno con: make logs s=api-gateway)
	$(COMPOSE) logs -f $(s)

ps: env-check ## Estado de los contenedores
	$(COMPOSE) ps

# ------------------------------------------------------------------ calidad

test: ## Tests de todos los módulos Go (los de integración usan Docker vía testcontainers)
	@for m in $(GO_MODULES); do \
		echo "==> go test $$m"; \
		(cd $$m && go test ./...) || exit 1; \
	done

lint: ## go vet + golangci-lint (si está instalado) + chequeo de tipos del frontend
	@for m in $(GO_MODULES); do \
		echo "==> go vet $$m"; \
		(cd $$m && go vet ./...) || exit 1; \
	done
	@if command -v golangci-lint >/dev/null 2>&1; then \
		for m in $(GO_MODULES); do \
			echo "==> golangci-lint $$m"; \
			(cd $$m && golangci-lint run ./...) || exit 1; \
		done; \
	else \
		echo "golangci-lint no está instalado: se omite (la CI lo ejecuta)"; \
	fi
	@if [ -d web/node_modules ]; then \
		echo "==> tsc web"; cd web && npm run lint; \
	else \
		echo "web/node_modules no existe: se omite el frontend (instalá con: npm --prefix web ci)"; \
	fi

fmt: ## Formatea el código Go
	gofmt -w $(GO_MODULES)

tidy: ## go mod tidy en todos los módulos
	@for m in $(GO_MODULES); do \
		echo "==> go mod tidy $$m"; \
		(cd $$m && go mod tidy) || exit 1; \
	done

# ------------------------------------------------------------------ contrato de partners

mock-up: ## Levanta solo el mock del contrato de partners en http://localhost:4010
	$(MOCK_COMPOSE) up -d

mock-down: ## Baja el mock
	$(MOCK_COMPOSE) down

mock-logs: ## Muestra los logs del mock (requests y validaciones)
	$(MOCK_COMPOSE) logs -f benefits-mock

contract-lint: ## Valida el contrato con Spectral
	docker run --rm -v "$(CURDIR):/work" -w /work $(SPECTRAL_IMAGE) \
		lint $(CONTRACT) --ruleset $(RULESET) --fail-severity=warn
