.PHONY: run api fmt vet test

run:
	go run ./cmd/dev

api:
	go run ./cmd/api

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

test:
	go test ./...
