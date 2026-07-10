COMPOSE  := docker compose -f examples/ecommerce/docker-compose.yml
DDB_PORT ?= 8000
ENDPOINT := http://localhost:$(DDB_PORT)

.PHONY: test demo up down integration generate

test:
	go test ./...
	go test -race ./runtime/...

up:
	DDB_PORT=$(DDB_PORT) $(COMPOSE) up -d --wait

down:
	DDB_PORT=$(DDB_PORT) $(COMPOSE) down

demo: up
	DDB_ENDPOINT=$(ENDPOINT) go run ./examples/ecommerce

integration: up
	DDB_TEST_ENDPOINT=$(ENDPOINT) go test -count=1 -tags=integration ./examples/...

generate:
	go run ./cmd/ddbgen generate ./examples/ecommerce
	go run ./cmd/ddbgen docs ./examples/ecommerce
	go run ./cmd/ddbgen infra --format cfn --out examples/ecommerce/infra ./examples/ecommerce
	go run ./cmd/ddbgen infra --format tf --out examples/ecommerce/infra ./examples/ecommerce
