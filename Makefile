.PHONY: fmt vet test race lint run compose-up compose-down build

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

run:
	go run ./cmd/dev

compose-up:
	docker compose -f deploy/compose.yaml up --build

compose-down:
	docker compose -f deploy/compose.yaml down

build:
	go build ./cmd/api
	go build ./cmd/scheduler
	go build ./cmd/notifier
