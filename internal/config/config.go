package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig         `yaml:"server"`
	Redis      RedisConfig          `yaml:"redis"`
	Algorithms AlgorithmsConfig     `yaml:"algorithms"`
	Limits     LimitsConfig         `yaml:"limits"`
	Metrics    MetricsConfig        `yaml:"metrics"`
	Store      string               `yaml:"store"` // "memory" or "redis"
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addresses []string      `yaml:"addresses"`
	Password  string        `yaml:"password"`
	DB        int           `yaml:"db"`
	PoolSize  int           `yaml:"pool_size"`
	TTL       time.Duration `yaml:"ttl"`
}

// AlgorithmsConfig holds algorithm configuration
type AlgorithmsConfig struct {
	Default string `yaml:"default"` // "token_bucket", "sliding_window", "fixed_window"
}

// LimitsConfig holds rate limiting configuration
type LimitsConfig struct {
	Default LimitConfig            `yaml:"default"`
	Tiers   map[string]LimitConfig `yaml:"tiers"`
}

// LimitConfig represents a rate limit configuration
type LimitConfig struct {
	Requests int           `yaml:"requests"` // Max requests
	Window   time.Duration `yaml:"window"`   // Time window
	Burst    int           `yaml:"burst"`    // Burst capacity (for token bucket)
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
	Port    int    `yaml:"port"`
}

// Load loads configuration from a YAML file
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.ReadTimeout == 0 {
		config.Server.ReadTimeout = 5 * time.Second
	}
	if config.Server.WriteTimeout == 0 {
		config.Server.WriteTimeout = 10 * time.Second
	}
	if config.Server.IdleTimeout == 0 {
		config.Server.IdleTimeout = 120 * time.Second
	}
	if config.Algorithms.Default == "" {
		config.Algorithms.Default = "token_bucket"
	}
	if config.Limits.Default.Requests == 0 {
		config.Limits.Default.Requests = 100
	}
	if config.Limits.Default.Window == 0 {
		config.Limits.Default.Window = 1 * time.Minute
	}
	if config.Store == "" {
		config.Store = "memory"
	}
	if config.Metrics.Path == "" {
		config.Metrics.Path = "/metrics"
	}
	if config.Metrics.Port == 0 {
		config.Metrics.Port = config.Server.Port
	}
	if config.Redis.PoolSize == 0 {
		config.Redis.PoolSize = 100
	}
	if config.Redis.TTL == 0 {
		config.Redis.TTL = 24 * time.Hour
	}

	return &config, nil
}

// LoadOrDefault loads configuration from file or returns default config
func LoadOrDefault(filename string) *Config {
	config, err := Load(filename)
	if err != nil {
		return DefaultConfig()
	}
	return config
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Redis: RedisConfig{
			Addresses: []string{"localhost:6379"},
			Password:  "",
			DB:        0,
			PoolSize:  100,
			TTL:       24 * time.Hour,
		},
		Algorithms: AlgorithmsConfig{
			Default: "token_bucket",
		},
		Limits: LimitsConfig{
			Default: LimitConfig{
				Requests: 100,
				Window:   1 * time.Minute,
				Burst:    120,
			},
			Tiers: map[string]LimitConfig{
				"free": {
					Requests: 100,
					Window:   1 * time.Hour,
					Burst:    120,
				},
				"premium": {
					Requests: 10000,
					Window:   1 * time.Hour,
					Burst:    12000,
				},
			},
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
			Port:    8080,
		},
		Store: "memory",
	}
}
