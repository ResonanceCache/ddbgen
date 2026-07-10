COMPOSE := docker compose -f examples/ecommerce/docker-compose.yml

.PHONY: test demo up down integration generate

test:
	go test ./...
	go test -race ./runtime/...

up:
	$(COMPOSE) up -d --wait

down:
	$(COMPOSE) down

demo: up
	go run ./examples/ecommerce

integration: up
	DDB_TEST_ENDPOINT=http://localhost:8000 go test -tags=integration ./examples/...

generate:
	go run ./cmd/ddbgen generate ./examples/ecommerce
	go run ./cmd/ddbgen docs ./examples/ecommerce
	go run ./cmd/ddbgen infra --format cfn --out examples/ecommerce/infra ./examples/ecommerce
	go run ./cmd/ddbgen infra --format tf --out examples/ecommerce/infra ./examples/ecommerce
