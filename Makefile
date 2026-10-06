APP_PORT ?= 8000

.PHONY: run dev build test vet fmt tidy

run: build ## Build and run once
	APP_PORT=$(APP_PORT) ./bin/baaku

dev: ## Live-reload dev server (air)
	APP_PORT=$(APP_PORT) go tool air

build: ## Build bin/baaku and bin/migrate
	go build -o ./bin/baaku ./cmd/baaku
	go build -o ./bin/migrate ./cmd/migrate

test: ## Run tests
	go test ./...

vet: ## go vet + gofmt check
	go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

fmt: ## Format code
	gofmt -w .

tidy: ## Tidy modules
	go mod tidy
