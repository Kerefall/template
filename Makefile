-include .env
export

.PHONY: generate migrate migrate-down run test test-integration lint fmt check-layers

# OpenAPI-код и моки (mocks_test.go рядом с кодом, коммитятся).
generate:
	go tool oapi-codegen \
		-config api/oapi-codegen.yaml \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/service

# Все тесты с -race; покрытие считается по usecase и domain и печатается в конце.
test:
	go test -race -count=1 \
		-coverprofile=coverage.out \
		-coverpkg=./internal/usecase/...,./internal/domain/... \
		./...
	@go tool cover -func=coverage.out | tail -n 1

# Интеграционные тесты репозитория: нужна база (tripgoctl environment start) и make migrate.
test-integration:
	go test -race -count=1 -tags=integration ./internal/repository/...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

# Домен не должен тянуть инфраструктуру.
check-layers:
	@! go list -deps ./internal/domain/... | grep -E 'jackc/pgx|go-chi|net/http|opentelemetry' \
		&& echo "domain: ok" || (echo "НАРУШЕНИЕ: domain импортирует инфраструктуру" && exit 1)
