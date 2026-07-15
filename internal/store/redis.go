package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
	"github.com/redis/go-redis/v9"
)

// RedisStore is a Redis-backed limiter.Store for distributed deployments.
// Every decision executes as a single Lua script, so concurrent checks from
// any number of application instances serialize on the Redis server and can
// never over-admit. Scripts read the Redis server clock (TIME), which keeps
// instances with skewed local clocks consistent with each other.
type RedisStore struct {
	client redis.UniversalClient
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Addresses []string // one address for a single instance, several for cluster mode
	Password  string
	DB        int // ignored in cluster mode
	PoolSize  int
}

// NewRedisStore connects to Redis (or a Redis Cluster when more than one
// address is given) and verifies the connection.
func NewRedisStore(config RedisConfig) (*RedisStore, error) {
	var client redis.UniversalClient
	if len(config.Addresses) == 1 {
		client = redis.NewClient(&redis.Options{
			Addr:     config.Addresses[0],
			Password: config.Password,
			DB:       config.DB,
			PoolSize: config.PoolSize,
		})
	} else {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    config.Addresses,
			Password: config.Password,
			PoolSize: config.PoolSize,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// incrWindowScript implements fixed- and sliding-window admission in one
// atomic step. State is a hash keyed by window start (unix microseconds);
// only the current and previous windows are ever read, and stale fields are
// pruned on write, so each key holds at most a handful of fields.
//
// KEYS[1] counter key
// ARGV[1] window length, microseconds
// ARGV[2] permits requested (0 = read-only peek)
// ARGV[3] limit
// ARGV[4] 1 = weigh the previous window (sliding), 0 = ignore it (fixed)
// ARGV[5] TTL, seconds
//
// Returns {allowed, current, previous, windowStartMicros, nowMicros}.
var incrWindowScript = redis.NewScript(`
local window = tonumber(ARGV[1])
local n = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local weigh_prev = tonumber(ARGV[4]) == 1
local ttl = tonumber(ARGV[5])

local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
local cur_start = now - (now % window)
local prev_start = cur_start - window

local cur = tonumber(redis.call('HGET', KEYS[1], tostring(cur_start)) or '0')
local prev = 0
if weigh_prev then
	prev = tonumber(redis.call('HGET', KEYS[1], tostring(prev_start)) or '0')
end

local weighted = cur
if weigh_prev and prev > 0 then
	weighted = cur + prev * (1 - (now - cur_start) / window)
end

local allowed = 0
if weighted + n <= limit then
	allowed = 1
	if n > 0 then
		cur = redis.call('HINCRBY', KEYS[1], tostring(cur_start), n)
		for _, field in ipairs(redis.call('HKEYS', KEYS[1])) do
			if tonumber(field) < prev_start then
				redis.call('HDEL', KEYS[1], field)
			end
		end
		redis.call('EXPIRE', KEYS[1], ttl)
	end
end

return {allowed, cur, prev, tostring(cur_start), tostring(now)}
`)

// takeTokensScript implements token-bucket admission in one atomic step.
// State is the token count plus the last-refill timestamp, stored with
// microsecond precision as fractional unix seconds.
//
// KEYS[1] bucket key
// ARGV[1] capacity
// ARGV[2] refill rate, tokens per second
// ARGV[3] tokens requested (0 = read-only peek)
// ARGV[4] TTL, seconds
//
// Returns {allowed, tokensAfter}. Nothing is written on denial or peek: the
// stored (tokens, ts) pair regenerates the same balance on the next call.
var takeTokensScript = redis.NewScript(`
local capacity = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local n = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local t = redis.call('TIME')
local now = tonumber(t[1]) + tonumber(t[2]) / 1000000

local state = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(state[1])
local ts = tonumber(state[2])
if tokens == nil or ts == nil then
	tokens = capacity
	ts = now
end

local elapsed = now - ts
if elapsed > 0 then
	tokens = tokens + elapsed * refill
	if tokens > capacity then
		tokens = capacity
	end
end

local allowed = 0
if n <= tokens then
	allowed = 1
	if n > 0 then
		tokens = tokens - n
		redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', tostring(now))
		redis.call('EXPIRE', KEYS[1], ttl)
	end
end

return {allowed, tostring(tokens)}
`)

// IncrWindow implements limiter.Store.
func (rs *RedisStore) IncrWindow(ctx context.Context, key string, window time.Duration, n, limit int64, weightPrev bool, ttl time.Duration) (*limiter.WindowResult, error) {
	weigh := 0
	if weightPrev {
		weigh = 1
	}

	raw, err := incrWindowScript.Run(ctx, rs.client, []string{"rl:" + key},
		window.Microseconds(), n, limit, weigh, ttlSeconds(ttl)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis window increment: %w", err)
	}

	reply, ok := raw.([]interface{})
	if !ok || len(reply) != 5 {
		return nil, fmt.Errorf("redis window increment: unexpected reply %T", raw)
	}

	var fields [5]int64
	for i, v := range reply {
		f, err := replyInt(v)
		if err != nil {
			return nil, fmt.Errorf("redis window increment: %w", err)
		}
		fields[i] = f
	}

	return &limiter.WindowResult{
		Allowed:     fields[0] == 1,
		Current:     fields[1],
		Previous:    fields[2],
		WindowStart: time.UnixMicro(fields[3]),
		Now:         time.UnixMicro(fields[4]),
	}, nil
}

// TakeTokens implements limiter.Store.
func (rs *RedisStore) TakeTokens(ctx context.Context, key string, capacity, refillPerSec, n float64, ttl time.Duration) (bool, float64, error) {
	raw, err := takeTokensScript.Run(ctx, rs.client, []string{"rl:" + key},
		formatFloat(capacity), formatFloat(refillPerSec), formatFloat(n), ttlSeconds(ttl)).Result()
	if err != nil {
		return false, 0, fmt.Errorf("redis token take: %w", err)
	}

	reply, ok := raw.([]interface{})
	if !ok || len(reply) != 2 {
		return false, 0, fmt.Errorf("redis token take: unexpected reply %T", raw)
	}

	allowed, err := replyInt(reply[0])
	if err != nil {
		return false, 0, fmt.Errorf("redis token take: %w", err)
	}
	tokens, err := replyFloat(reply[1])
	if err != nil {
		return false, 0, fmt.Errorf("redis token take: %w", err)
	}
	return allowed == 1, tokens, nil
}

// Delete implements limiter.Store.
func (rs *RedisStore) Delete(ctx context.Context, key string) error {
	if err := rs.client.Del(ctx, "rl:"+key).Err(); err != nil {
		return fmt.Errorf("redis delete: %w", err)
	}
	return nil
}

// Ping implements limiter.Store.
func (rs *RedisStore) Ping(ctx context.Context) error {
	return rs.client.Ping(ctx).Err()
}

// Close implements limiter.Store.
func (rs *RedisStore) Close() error {
	return rs.client.Close()
}

// ttlSeconds converts a duration to whole seconds for EXPIRE, rounding up so
// state never expires before the algorithm expects it to.
func ttlSeconds(ttl time.Duration) int64 {
	secs := int64((ttl + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return secs
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func replyInt(v interface{}) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected reply element %T", v)
	}
}

func replyFloat(v interface{}) (float64, error) {
	switch t := v.(type) {
	case string:
		return strconv.ParseFloat(t, 64)
	case int64:
		return float64(t), nil
	default:
		return 0, fmt.Errorf("unexpected reply element %T", v)
	}
}
