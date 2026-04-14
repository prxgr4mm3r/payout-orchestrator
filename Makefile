ifneq (,$(wildcard ./.env))
include .env
export
endif

up:
	docker compose up -d

down:
	docker compose down

migrate-up:
	migrate -path ./migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path ./migrations -database "$(DB_URL)" down 1

sqlc:
	cd internal/db && sqlc generate

run-api:
	go run ./cmd/api