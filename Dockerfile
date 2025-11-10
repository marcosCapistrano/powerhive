# syntax=docker/dockerfile:1

FROM golang:1.24 AS build

WORKDIR /app

# Download dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /app/bin/powerhive ./cmd/automation

FROM debian:bookworm-slim AS runtime

ENV TZ=UTC
WORKDIR /app

# Install wget for healthcheck, create user and directories
RUN apt-get update && \
    apt-get install -y --no-install-recommends wget ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --create-home --uid 10001 appuser && \
    mkdir -p /app/data && \
    chown -R appuser:appuser /app

COPY --from=build /app/bin/powerhive /app/powerhive
COPY config.json /app/config.json

USER appuser
VOLUME ["/app/data"]
EXPOSE 8080

# Health check: verify the HTTP server responds
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/bin/sh", "-c", "wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1"]

ENTRYPOINT ["/app/powerhive"]
