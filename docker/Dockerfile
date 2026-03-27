# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -buildid= -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o aiproxy ./cmd/server

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata wget su-exec && \
    adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /build/aiproxy /app/aiproxy
COPY --from=builder /build/config/config.example.json /app/config/config.example.json
COPY docker/entrypoint.sh /app/docker-entrypoint.sh

RUN chmod +x /app/docker-entrypoint.sh && \
    mkdir -p /app/data && \
    chown -R appuser:appuser /app

EXPOSE 8080 8081

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

ENV AIPROXY_CONFIG=/app/config/config.json
ENV DATA_DIR=/app/data

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/aiproxy"]