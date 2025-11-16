SHELL := /bin/sh

.PHONY: build
build:
	go build -o bin/pr-reviewer ./cmd/server

.PHONY: gen
gen:
	go tool oapi-codegen -config internal/api/oapi.cfg.yaml openapi.yaml

.PHONY: run
run:
	go run ./cmd/server

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: up
up:
	docker compose up --build

.PHONY: down
down:
	docker compose down -v
