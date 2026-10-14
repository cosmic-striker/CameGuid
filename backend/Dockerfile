# Multi-target Dockerfile for all three Go services
FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
# Create a minimal go.sum to allow the build to proceed
RUN go mod download 2>/dev/null || true
COPY . .
RUN go mod tidy

# Build each service
FROM builder AS build-api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api ./cmd/api

FROM builder AS build-scanner
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /scanner ./cmd/scanner

FROM builder AS build-relay
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /relay ./cmd/relay

# --- Runtime Images ---

FROM alpine:3.20 AS api
RUN apk add --no-cache ca-certificates wget tzdata
COPY --from=build-api /api /usr/local/bin/api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1
CMD ["api"]

FROM alpine:3.20 AS scanner
RUN apk add --no-cache ca-certificates nmap nmap-scripts tzdata
COPY --from=build-scanner /scanner /usr/local/bin/scanner
EXPOSE 8081
CMD ["scanner"]

FROM alpine:3.20 AS relay
RUN apk add --no-cache ca-certificates ffmpeg wget tzdata
COPY --from=build-relay /relay /usr/local/bin/relay
RUN mkdir -p /tmp/streams /tmp/snapshots
EXPOSE 8082
CMD ["relay"]
