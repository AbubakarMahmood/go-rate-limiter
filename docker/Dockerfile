# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rate-limiter ./cmd/server

# Runtime stage
FROM alpine:3.21

RUN apk --no-cache add ca-certificates \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/rate-limiter .
COPY config.yaml .

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/health" || exit 1

ENTRYPOINT ["/app/rate-limiter"]
