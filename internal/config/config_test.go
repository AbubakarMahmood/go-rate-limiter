package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoad_ParsesDurationsAndTiers(t *testing.T) {
	path := writeConfig(t, `
server:
  port: 9000
  read_timeout: 500ms
store: memory
algorithms:
  default: sliding_window
limits:
  default:
    requests: 42
    window: 90s
  tiers:
    premium:
      requests: 1000
      window: 1h
      burst: 1200
`)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, 500*time.Millisecond, cfg.Server.ReadTimeout.Std())
	assert.Equal(t, AlgorithmSlidingWindow, cfg.Algorithms.Default)
	assert.Equal(t, 42, cfg.Limits.Default.Requests)
	assert.Equal(t, 90*time.Second, cfg.Limits.Default.Window.Std())
	require.Contains(t, cfg.Limits.Tiers, "premium")
	assert.Equal(t, time.Hour, cfg.Limits.Tiers["premium"].Window.Std())
	assert.Equal(t, 1200, cfg.Limits.Tiers["premium"].Burst)

	// Unspecified values fall back to defaults.
	assert.Equal(t, 10*time.Second, cfg.Server.WriteTimeout.Std())
	assert.Equal(t, "/metrics", cfg.Metrics.Path)
}

func TestLoad_RejectsInvalidConfig(t *testing.T) {
	cases := map[string]string{
		"bad duration":       "limits:\n  default:\n    requests: 10\n    window: sixty",
		"numeric duration":   "server:\n  read_timeout: 5",
		"unknown store":      "store: postgres",
		"unknown algorithm":  "algorithms:\n  default: leaky_bucket",
		"negative requests":  "limits:\n  default:\n    requests: -1\n    window: 1m",
		"zero tier window":   "limits:\n  tiers:\n    free:\n      requests: 10\n      window: 0s",
		"reserved tier name": "limits:\n  tiers:\n    default:\n      requests: 10\n      window: 1m",
		"port out of range":  "server:\n  port: 70000",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeConfig(t, content))
			assert.Error(t, err)
		})
	}
}

func TestLoad_MissingFileErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	assert.Error(t, err)
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("STORE", "redis")
	t.Setenv("REDIS_ADDR", "redis-a:6379,redis-b:6379")
	t.Setenv("REDIS_PASSWORD", "secret")

	cfg, err := Default()
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, StoreRedis, cfg.Store)
	assert.Equal(t, []string{"redis-a:6379", "redis-b:6379"}, cfg.Redis.Addresses)
	assert.Equal(t, "secret", cfg.Redis.Password)
}

func TestEnvOverrides_InvalidPort(t *testing.T) {
	t.Setenv("PORT", "eighty")
	_, err := Default()
	assert.Error(t, err)
}

func TestDefault_IsValid(t *testing.T) {
	cfg, err := Default()
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, StoreMemory, cfg.Store)
	assert.Equal(t, AlgorithmTokenBucket, cfg.Algorithms.Default)
	assert.Equal(t, 100, cfg.Limits.Default.Requests)
	assert.Equal(t, time.Minute, cfg.Limits.Default.Window.Std())
}
