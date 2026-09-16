# go_pet_project

Backend portfolio project: a task manager with users, auth, reminders, recurring tasks, internal notifications, Kafka-based outbox delivery, and optional Telegram delivery.

The project is written as a modular Go service to demonstrate clean boundaries, explicit infrastructure adapters, and practical backend workflows without presenting the code as production-ready infrastructure.

## 1. Project overview

`go_pet_project` is a Go backend for personal/team tasks:

- users can register, login, refresh tokens, and manage profile data;
- authenticated users can create and manage tasks;
- tasks can have reminders;
- recurring task series can generate future task occurrences;
- due reminders create notification events through a transactional outbox;
- notifier consumes Kafka events and stores internal notifications;
- Telegram integration can link a chat and deliver notification messages.

## 2. Features

- Email/username registration with password hashing.
- JWT access tokens and refresh token rotation.
- Task CRUD with permission checks.
- Reminder scheduling for absolute and before-deadline reminders.
- Recurring task generation.
- Internal notification list/read state.
- Transactional outbox with Kafka publishing.
- Kafka consumer with manual offset commits.
- Optional Telegram bot delivery.
- Prometheus metrics endpoints.
- Graceful shutdown for API, scheduler, and notifier processes.

## 3. Architecture

The project follows a hexagonal style: domain and application code do not depend on HTTP, GORM, Kafka, or Telegram packages.

Typical HTTP flow:

```text
HTTP adapter
-> inbound port
-> application service
-> domain
-> outbound port
-> PostgreSQL adapter
```

Notification pipeline:

```text
Reminder Scheduler
-> PostgreSQL
-> Outbox
-> Kafka
-> Notifier
-> Internal Notification / Telegram
```

Runtime processes:

- `cmd/api`: HTTP API, migrations, auth, task/reminder/user/notification routes.
- `cmd/scheduler`: reminder scheduler, recurrence scheduler, outbox relay.
- `cmd/notifier`: Kafka consumer, internal notification processing, optional Telegram poller/delivery worker.
- `cmd/dev`: local helper that starts infrastructure and local Go processes.

## 4. Project structure

```text
cmd/
  api/         HTTP API process
  scheduler/   background schedulers and outbox relay
  notifier/    Kafka consumer and Telegram delivery
  dev/         local development launcher

internal/
  auth/
  user/
  task/
  reminder/
  recurrence/
  notification/
  outbox/
  permission/
  platform/    config, database, logging, metrics, server

migrations/    explicit SQL migrations
deploy/        Docker Compose, Prometheus, Grafana config
tests/         integration test helpers/suites
```

## 5. Tech stack

- Go
- PostgreSQL
- GORM
- Kafka via `franz-go`
- Docker Compose
- Prometheus metrics
- Telegram Bot API integration
- GitHub Actions CI

## 6. Request flow example

Creating a reminder through the API:

```text
POST /api/v1/tasks/{taskID}/reminders
-> auth middleware validates JWT
-> reminder HTTP handler parses request
-> ReminderService checks task access and reminder rules
-> ReminderRepository stores reminder in PostgreSQL
```

Processing a due reminder:

```text
Reminder runner claims due rows with SELECT ... FOR UPDATE SKIP LOCKED
-> loads related task
-> creates notification.requested.v1 outbox event
-> marks reminder as sent
-> outbox relay publishes event to Kafka
-> notifier consumes event
-> NotificationService creates internal notification
-> Telegram delivery is queued when the user has linked Telegram
```

## 7. Local run

Create a `.env` file or export the required values. For API/dev mode, `JWT_SECRET` must be at least 32 characters.

```bash
make run
```

This starts local infrastructure through Docker Compose, waits for PostgreSQL and Kafka, runs migrations, and starts API, scheduler, and notifier as local Go processes.

Useful direct commands:

```bash
go run ./cmd/api
go run ./cmd/scheduler
go run ./cmd/notifier
```

## 8. Docker run

```bash
export JWT_SECRET="change-me-to-at-least-32-characters"
make compose-up
```

API defaults to `http://localhost:8080`.

```bash
make compose-down
```

## 9. Tests / quality checks

```bash
make fmt
make vet
make test
make race
make lint
make build
```

CI runs gofmt check, `go vet`, `golangci-lint`, tests, race tests, binary builds, and Docker build.

## 10. Main engineering decisions

- Interfaces are placed near the consumer side to keep dependencies directed inward.
- Domain/application layers do not import HTTP, GORM, Kafka, or Telegram adapters.
- SQL migrations are explicit instead of relying on auto-migration.
- Request and worker paths propagate `context.Context`.
- Refresh tokens are rotated instead of reused indefinitely.
- Reminder notifications use a transactional outbox.
- Background workers claim rows with `SELECT ... FOR UPDATE SKIP LOCKED`.
- API, scheduler, notifier, and metrics servers use graceful shutdown.
- The project includes unit tests and integration-style tests for important persistence and workflow behavior.

## 11. Known limitations

- This is an educational portfolio backend project, not a production-ready system.
- The outbox publisher currently performs Kafka publish while handling claimed DB records inside the claim transaction.
- Telegram delivery can perform an external network call while processing a delivery transaction.
- At production scale, these worker flows should move toward lease/claim state with short database transactions and external work outside those transactions.
