.PHONY: fmt test build migrate run dev
fmt:
	gofmt -w $$(find . -name '*.go')
test:
	go test ./...
build:
	go build ./cmd/server ./cmd/migrate
migrate:
	go run ./cmd/migrate
run:
	go run ./cmd/server
dev:
	go tool air -c .air.toml
