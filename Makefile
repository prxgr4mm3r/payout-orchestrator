DB_URL=postgres://postgres:postgres@localhost:5432/payout?sslmode=disable

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
	DB_URL=$(DB_URL) go run ./cmd/api