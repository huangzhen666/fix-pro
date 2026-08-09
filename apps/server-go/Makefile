.PHONY: fmt test build migrate run
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
