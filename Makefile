-include .env
export

.PHONY: generate migrate migrate-down run test lint

generate:
	go tool oapi-codegen \
		-config api/oapi-codegen.yaml \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/trip-service

test:
	go test -race ./...

lint:
	golangci-lint run ./...
