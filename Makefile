SHELL := /bin/sh

.DEFAULT_GOAL := help

GOLANGCI_LINT_IMAGE ?= golangci/golangci-lint:v2.13.2

DEV_COMPOSE := -f compose.dev.yaml

.PHONY: help build up down logs ps prod prod-down vet lint test integration branch feature bugfix chore cert-init bootstrap

help:
	@printf '%s\n' \
		'Использование: make <цель>' \
		'' \
		'Цели:' \
		'  help             			Показать список доступных команд' \
		'  build     					Собрать Docker-образы' \
		'  up               			Запустить стек локально' \
		'  down             			Остановить локальный стек' \
		'  logs             			Следить за логами контейнеров' \
		'  ps               			Показать состояние контейнеров' \
		'  prod             			Запустить стек на сервере (без проброса порта БД)' \
		'  prod-down        			Остановить стек на сервере' \
		'  cert-init        			Выпустить TLS-сертификат Let'"'"'s Encrypt для домена' \
		'  bootstrap        			Создать компанию и её администратора в прод-стеке (COMPANY, EMAIL, ADMIN_NAME, PASSWORD)' \
		'  vet              			Проверить код через go vet' \
		'  lint             			Проверить Go-код через golangci-lint в Docker' \
		'  test             			Запустить Go-тесты (-race -cover)' \
		'  integration                    Запустить Go-тесты на одноразовой PostgreSQL через BE3_TEST_DSN' \
		'  make branch TYPE=x NAME=y    Cоздать x/y' \
		'  make feature NAME=login      Cоздать feature/login' \
		'  make bugfix NAME=api         Cоздать bugfix/api' \
		'  make chore NAME=deps         Cоздать chore/deps'


build:
	docker compose build

up:
	docker compose $(DEV_COMPOSE) up --build -d

down:
	docker compose $(DEV_COMPOSE) down

logs:
	docker compose logs -f

ps:
	docker compose ps

prod:
	docker compose up --build -d

prod-down:
	docker compose down

cert-init:
	./deploy/certbot/init-letsencrypt.sh

bootstrap:
	@test -n "$(COMPANY)" || (printf '%s\n' 'COMPANY is required' && exit 1)
	@test -n "$(EMAIL)" || (printf '%s\n' 'EMAIL is required' && exit 1)
	@test -n "$(ADMIN_NAME)" || (printf '%s\n' 'ADMIN_NAME is required' && exit 1)
	@test -n "$(PASSWORD)" || (printf '%s\n' 'PASSWORD is required' && exit 1)
	docker compose exec \
		-e BOOTSTRAP_COMPANY_NAME="$(COMPANY)" \
		-e BOOTSTRAP_ADMIN_EMAIL="$(EMAIL)" \
		-e BOOTSTRAP_ADMIN_NAME="$(ADMIN_NAME)" \
		-e BOOTSTRAP_ADMIN_PASSWORD="$(PASSWORD)" \
		api /app/bootstrap

vet:
	cd backend/api-service && go vet ./...

lint:
	docker run --rm -v "$(CURDIR):/app" -w /app/backend/api-service $(GOLANGCI_LINT_IMAGE) golangci-lint run --config /app/.golangci.yml

test:
	cd backend/api-service && go test ./... -race -cover

integration:
	@test -n "$(BE3_TEST_DSN)" || (printf '%s\n' 'BE3_TEST_DSN must point to a disposable PostgreSQL database' && exit 1)
	cd backend/api-service && BE3_TEST_DSN="$(BE3_TEST_DSN)" go test ./... -race -count=1

branch:
	@test -n "$(TYPE)" || (printf '%s\n' 'TYPE is required' && exit 1)
	@test -n "$(NAME)" || (printf '%s\n' 'NAME is required' && exit 1)
	@printf '%s' "$(NAME)" | grep -Eq '^[a-z0-9][a-z0-9._-]*$$' || (printf '%s\n' 'NAME must use lowercase letters, numbers, dots, dashes, or underscores' && exit 1)
	git switch -c "$(TYPE)/$(NAME)"

feature:
	@$(MAKE) branch TYPE=feature NAME="$(NAME)"

bugfix:
	@$(MAKE) branch TYPE=bugfix NAME="$(NAME)"

chore:
	@$(MAKE) branch TYPE=chore NAME="$(NAME)"
