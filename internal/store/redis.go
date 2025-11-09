package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
	"github.com/redis/go-redis/v9"
)

// RedisStore implements a Redis-backed store for distributed rate limiting
// Uses Lua scripts for atomic operations
// Supports Redis Cluster for horizontal scaling
type RedisStore struct {
	client redis.UniversalClient
	ctx    context.Context
	ttl    time.Duration // TTL for keys to prevent memory leaks
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addresses []string
	Password  string
	DB        int
	PoolSize  int
	TTL       time.Duration
}

// NewRedisStore creates a new Redis store
func NewRedisStore(config RedisConfig) (*RedisStore, error) {
	var client redis.UniversalClient

	if len(config.Addresses) == 1 {
		// Single instance
		client = redis.NewClient(&redis.Options{
			Addr:     config.Addresses[0],
			Password: config.Password,
			DB:       config.DB,
			PoolSize: config.PoolSize,
		})
	} else {
		// Redis Cluster
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    config.Addresses,
			Password: config.Password,
			PoolSize: config.PoolSize,
		})
	}

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	ttl := config.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour // Default TTL
	}

	return &RedisStore{
		client: client,
		ctx:    ctx,
		ttl:    ttl,
	}, nil
}

// Lua script for atomic increment with expiry
var incrementScript = redis.NewScript(`
	local key = KEYS[1]
	local window = ARGV[1]
	local ttl = tonumber(ARGV[2])

	local field = window
	local count = redis.call('HINCRBY', key, field, 1)

	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

// Increment increments the counter for a key at a specific window
func (rs *RedisStore) Increment(key string, window time.Time) (int64, error) {
	windowKey := fmt.Sprintf("window:%s", key)
	windowStr := strconv.FormatInt(window.Unix(), 10)

	result, err := incrementScript.Run(
		rs.ctx,
		rs.client,
		[]string{windowKey},
		windowStr,
		int(rs.ttl.Seconds()),
	).Result()

	if err != nil {
		return 0, fmt.Errorf("increment failed: %w", err)
	}

	count, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}

	return count, nil
}

// GetWindows returns all windows for a key within a time range
func (rs *RedisStore) GetWindows(key string, from, to time.Time) ([]limiter.Window, error) {
	windowKey := fmt.Sprintf("window:%s", key)

	// Get all fields and values from the hash
	result, err := rs.client.HGetAll(rs.ctx, windowKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get windows: %w", err)
	}

	windows := make([]limiter.Window, 0)
	for field, value := range result {
		timestamp, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			continue
		}

		t := time.Unix(timestamp, 0)
		if (t.Equal(from) || t.After(from)) && (t.Equal(to) || t.Before(to)) {
			count, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}

			windows = append(windows, limiter.Window{
				Timestamp: t,
				Count:     count,
			})
		}
	}

	return windows, nil
}

// SetTokens sets the token count and last refill time for token bucket
func (rs *RedisStore) SetTokens(key string, tokens float64, lastRefill time.Time) error {
	tokenKey := fmt.Sprintf("tokens:%s", key)

	pipe := rs.client.Pipeline()
	pipe.HSet(rs.ctx, tokenKey, "tokens", tokens)
	pipe.HSet(rs.ctx, tokenKey, "last_refill", lastRefill.Unix())
	pipe.Expire(rs.ctx, tokenKey, rs.ttl)

	_, err := pipe.Exec(rs.ctx)
	if err != nil {
		return fmt.Errorf("failed to set tokens: %w", err)
	}

	return nil
}

// GetTokens gets the token count and last refill time for token bucket
func (rs *RedisStore) GetTokens(key string) (tokens float64, lastRefill time.Time, err error) {
	tokenKey := fmt.Sprintf("tokens:%s", key)

	result, err := rs.client.HGetAll(rs.ctx, tokenKey).Result()
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get tokens: %w", err)
	}

	if len(result) == 0 {
		return 0, time.Time{}, nil
	}

	tokensStr, ok := result["tokens"]
	if ok {
		tokens, _ = strconv.ParseFloat(tokensStr, 64)
	}

	lastRefillStr, ok := result["last_refill"]
	if ok {
		lastRefillUnix, _ := strconv.ParseInt(lastRefillStr, 10, 64)
		lastRefill = time.Unix(lastRefillUnix, 0)
	}

	return tokens, lastRefill, nil
}

// Delete removes all data for a key
func (rs *RedisStore) Delete(key string) error {
	windowKey := fmt.Sprintf("window:%s", key)
	tokenKey := fmt.Sprintf("tokens:%s", key)

	pipe := rs.client.Pipeline()
	pipe.Del(rs.ctx, windowKey)
	pipe.Del(rs.ctx, tokenKey)

	_, err := pipe.Exec(rs.ctx)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (rs *RedisStore) Close() error {
	return rs.client.Close()
}
