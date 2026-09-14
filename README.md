# Todo Go

Stage 0 foundation for a Go backend using Hexagonal Architecture and package by feature.

## Development

Initial setup:

```bash
cp .env.example .env
go mod download
```

Run the local development stack:

```bash
go run ./cmd/dev
```

or:

```bash
make run
```

The dev launcher checks Docker, starts PostgreSQL through `deploy/compose.yaml`, waits for database readiness, applies pending migrations from `migrations/`, and starts the API process.

The PostgreSQL container and Docker volume are not removed on shutdown, so data persists between development restarts.

Useful commands:

```bash
make api
make fmt
make vet
make test
```

Health endpoints:

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
```
