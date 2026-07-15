# go-rate-limiter

[![CI](https://github.com/AbubakarMahmood/go-rate-limiter/actions/workflows/ci.yml/badge.svg)](https://github.com/AbubakarMahmood/go-rate-limiter/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A standalone rate-limiting service in Go. Other services call its HTTP API to ask "may this client do this thing right now?" and get an authoritative, atomic answer — with standard `X-RateLimit-*` headers, tiered limits, and Prometheus metrics included.

- **Three algorithms**: token bucket, sliding window counter, fixed window counter — selectable per request.
- **Two storage backends**: in-memory for a single instance, Redis for a fleet. Every decision is a single atomic operation (a per-key critical section in memory, a Lua script in Redis), so concurrent requests can never over-admit — this is tested, not assumed.
- **Tiered limits**: named tiers (e.g. `free`, `premium`) with independent budgets, defined in config and selected per request.
- **Observability**: Prometheus metrics with a provisioned Grafana dashboard in the Compose stack.

## Quick start

```bash
# Run directly (in-memory store, config.yaml optional)
go run ./cmd/server

# Or run the full stack: service + Redis + Prometheus + Grafana
make docker-up
```

Check a request:

```bash
curl -s -X POST http://localhost:8080/v1/check \
  -H 'Content-Type: application/json' \
  -d '{"resource": "api.users.create", "identifier": "user-123"}'
```

```json
{
  "allowed": true,
  "limit": 120,
  "remaining": 119,
  "reset_at": "2026-07-15T17:32:47Z"
}
```

When the budget is spent the service answers `429` with a `Retry-After` header and the same JSON shape plus `retry_after` (seconds, rounded up).

> With the Compose stack the service listens on **:8081**, Prometheus on :9090, Grafana on :3000 (admin/admin).

## API

| Method | Path              | Purpose                                                        |
|--------|-------------------|----------------------------------------------------------------|
| `POST` | `/v1/check`       | Decide and consume: may `identifier` access `resource` now?    |
| `GET`  | `/v1/status/:key` | Current state for `identifier:resource` — never consumes       |
| `POST` | `/v1/reset/:key`  | Clear state for one key (operational/admin use)                |
| `GET`  | `/health`         | `200` when the store answers, `503` otherwise                  |
| `GET`  | `/metrics`        | Prometheus metrics                                             |

### `POST /v1/check`

```json
{
  "resource":   "api.users.create",   // required — what is being accessed
  "identifier": "user-123",           // required — who is accessing it
  "algorithm":  "sliding_window",     // optional — override the default
  "tier":       "premium",            // optional — use a configured tier's limits
  "count":      3                     // optional — consume several permits at once (default 1)
}
```

`count` is all-or-nothing: either all permits are granted or none are, and a denied request consumes nothing. A `count` that exceeds the configured limit can never succeed and is rejected with `400` rather than `429`.

Every decision carries the standard headers:

```http
X-RateLimit-Limit: 120
X-RateLimit-Remaining: 117
X-RateLimit-Reset: 1783532167
Retry-After: 12          (only on 429)
```

### Status and reset

`/v1/status` and `/v1/reset` take the same `identifier:resource` pair as the path key, with `algorithm` and `tier` as query parameters:

```bash
curl -s 'http://localhost:8080/v1/status/user-123:api.users.create?algorithm=token_bucket'
curl -s -X POST 'http://localhost:8080/v1/reset/user-123:api.users.create'
```

Status is a true read: it reports `remaining` without consuming permits.

## Algorithms

| Algorithm        | Behaviour                                                                 | Trade-off                                                       |
|------------------|---------------------------------------------------------------------------|-----------------------------------------------------------------|
| `token_bucket`   | Refills continuously at `requests/window`; bursts up to `burst`           | Best for smoothing traffic while tolerating short spikes        |
| `sliding_window` | Weights the previous window by its remaining overlap: `cur + prev×w`      | Near-accurate limiting with two counters per key                |
| `fixed_window`   | One counter per window                                                     | Cheapest, but admits up to 2× the limit across a window boundary |

The default algorithm is set in config; each request may override it. The three algorithms keep separate state, so switching algorithms never inherits stale counts.

## Configuration

`config.yaml` (all keys optional — built-in defaults apply):

```yaml
server:
  port: 8080
  read_timeout: 5s
  write_timeout: 10s

store: memory            # memory | redis

redis:
  addresses: [localhost:6379]
  pool_size: 100

algorithms:
  default: token_bucket  # token_bucket | sliding_window | fixed_window

limits:
  default:
    requests: 100        # permits per window
    window: 1m
    burst: 120           # token-bucket capacity (defaults to requests)
  tiers:
    premium:
      requests: 10000
      window: 1h
      burst: 12000

metrics:
  enabled: true
  path: /metrics
```

Environment overrides (useful in containers): `PORT`, `STORE`, `REDIS_ADDR` (comma-separated for cluster), `REDIS_PASSWORD`, and `CONFIG_FILE` to point at a different file. A malformed config file fails startup loudly rather than silently falling back to defaults.

## Design notes

**Atomicity lives in the store.** Algorithms are stateless; each decision compiles down to one atomic store operation. The Redis backend runs the entire read-evaluate-increment sequence as a Lua script on the server, so any number of service instances share correct limits without coordination. The in-memory backend takes a per-key mutex — unrelated keys never contend. A concurrency test asserts that 300 parallel requests against a limit of 100 admit *exactly* 100, on both backends.

**Clocks.** Redis decisions use the Redis server clock (`TIME` inside the script), making instances with skewed local clocks consistent with each other.

**TTLs are derived, not configured.** Window state expires after two windows (when it can no longer influence a decision); bucket state expires after exactly the time a full refill would take, so eviction is indistinguishable from refilling. Idle keys cost nothing in either backend.

**Retry math doesn't truncate.** `retry_after` is computed from the actual refill rate or window overlap with sub-second precision, then rounded *up* to whole seconds for the header — a denied client is never told to retry too early.

**Known limits.** The sliding window is the standard weighted approximation (it assumes requests in the previous window were evenly distributed). The in-memory store is per-instance by design — run Redis when there is more than one instance.

## Performance

`make bench` on a Ryzen 7 5800H (Windows, Go 1.26, in-memory store):

| Benchmark                    | token_bucket | sliding_window | fixed_window |
|------------------------------|--------------|----------------|--------------|
| Parallel, 100 keys           | 66 ns/op     | 93 ns/op       | 97 ns/op     |
| Parallel, single hot key     | 139 ns/op    | 234 ns/op      | 220 ns/op    |
| Allocations per decision     | 80 B / 3     | 160 B / 4      | 160 B / 4    |

These measure the decision path (algorithm + store); end-to-end HTTP latency is dominated by the network and, for the Redis backend, one round trip per decision.

There is also a [vegeta](https://github.com/tsenart/vegeta) script for load-testing a running instance: `./scripts/load-test.sh`.

## Development

```bash
make test           # unit + handler tests, race detector
make bench          # algorithm benchmarks
make docker-up      # full stack with provisioned Grafana dashboard
```

The Redis integration tests run automatically when `REDIS_ADDR` is set and skip otherwise; CI runs them against a Redis service container, along with `gofmt`, `go vet`, the race detector, and a Docker image smoke test.

### Project layout

```
cmd/server/          entry point, wiring
internal/algorithms/ token bucket, sliding window, fixed window
internal/store/      memory and Redis backends (atomic ops, Lua scripts)
internal/handlers/   HTTP API
internal/config/     YAML config, env overrides, validation
internal/metrics/    Prometheus collectors
pkg/limiter/         core interfaces shared by the above
docker/              Dockerfile, Compose stack, Prometheus & Grafana provisioning
```

## License

[MIT](LICENSE)
