FROM golang:1.25-alpine AS build

ARG APP=api

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

FROM alpine:3.22 AS runtime

RUN addgroup -S app && adduser -S app -G app \
    && apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=build /out/app /app/app
COPY --from=build /src/migrations /app/migrations

USER app

EXPOSE 8080 9091

ENTRYPOINT ["/app/app"]
