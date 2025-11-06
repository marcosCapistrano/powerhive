# syntax=docker/dockerfile:1

FROM golang:1.25 AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./ # optional, but ignore if missing
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/powerhive ./cmd/automation

FROM debian:bookworm-slim AS runtime

ENV TZ=UTC
WORKDIR /app

RUN useradd --create-home --uid 10001 appuser && \
	mkdir -p /app/data && \
	chown -R appuser:appuser /app

COPY --from=build /app/bin/powerhive /app/powerhive
COPY config.json /app/config.json

USER appuser
VOLUME ["/app/data"]

ENTRYPOINT ["/app/powerhive"]
