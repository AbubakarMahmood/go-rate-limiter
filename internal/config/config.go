// Package config loads and validates the service configuration from YAML,
// with environment-variable overrides for the settings that differ between
// deployments (PORT, STORE, REDIS_ADDR, REDIS_PASSWORD).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Store backends.
const (
	StoreMemory = "memory"
	StoreRedis  = "redis"
)

// Algorithm names, as used in configuration and the HTTP API.
const (
	AlgorithmTokenBucket   = "token_bucket"
	AlgorithmSlidingWindow = "sliding_window"
	AlgorithmFixedWindow   = "fixed_window"
)

// Algorithms lists every implemented algorithm.
var Algorithms = []string{AlgorithmTokenBucket, AlgorithmSlidingWindow, AlgorithmFixedWindow}

// Duration wraps time.Duration so YAML values like "500ms", "5s" or "1m"
// parse; yaml.v3 has no native support for duration strings.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration must be a string like \"30s\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// Std returns the wrapped time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// Config is the root of the service configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Store      string           `yaml:"store"` // "memory" or "redis"
	Redis      RedisConfig      `yaml:"redis"`
	Algorithms AlgorithmsConfig `yaml:"algorithms"`
	Limits     LimitsConfig     `yaml:"limits"`
	Metrics    MetricsConfig    `yaml:"metrics"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         int      `yaml:"port"`
	ReadTimeout  Duration `yaml:"read_timeout"`
	WriteTimeout Duration `yaml:"write_timeout"`
	IdleTimeout  Duration `yaml:"idle_timeout"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addresses []string `yaml:"addresses"`
	Password  string   `yaml:"password"`
	DB        int      `yaml:"db"`
	PoolSize  int      `yaml:"pool_size"`
}

// AlgorithmsConfig selects the algorithm used when a request names none.
type AlgorithmsConfig struct {
	Default string `yaml:"default"`
}

// LimitsConfig holds the default limit plus optional named tiers
// (e.g. "free", "premium") that requests can select.
type LimitsConfig struct {
	Default LimitConfig            `yaml:"default"`
	Tiers   map[string]LimitConfig `yaml:"tiers"`
}

// LimitConfig is one rate-limit definition.
type LimitConfig struct {
	Requests int      `yaml:"requests"` // permits per window
	Window   Duration `yaml:"window"`
	Burst    int      `yaml:"burst"` // token-bucket capacity; defaults to Requests
}

// MetricsConfig controls the Prometheus endpoint.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// Load reads filename, applies environment overrides, fills defaults and
// validates the result.
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filename, err)
	}

	if err := finalize(config); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	return config, nil
}

// Default returns the built-in configuration with environment overrides
// applied, for running without a config file.
func Default() (*Config, error) {
	config := &Config{}
	if err := finalize(config); err != nil {
		return nil, err
	}
	return config, nil
}

func finalize(config *Config) error {
	if err := applyEnv(config); err != nil {
		return err
	}
	applyDefaults(config)
	return validate(config)
}

func applyEnv(config *Config) error {
	if port := os.Getenv("PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid PORT %q: %w", port, err)
		}
		config.Server.Port = p
	}
	if store := os.Getenv("STORE"); store != "" {
		config.Store = store
	}
	if addrs := os.Getenv("REDIS_ADDR"); addrs != "" {
		config.Redis.Addresses = strings.Split(addrs, ",")
	}
	if password, ok := os.LookupEnv("REDIS_PASSWORD"); ok {
		config.Redis.Password = password
	}
	return nil
}

func applyDefaults(config *Config) {
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.ReadTimeout == 0 {
		config.Server.ReadTimeout = Duration(5 * time.Second)
	}
	if config.Server.WriteTimeout == 0 {
		config.Server.WriteTimeout = Duration(10 * time.Second)
	}
	if config.Server.IdleTimeout == 0 {
		config.Server.IdleTimeout = Duration(120 * time.Second)
	}
	if config.Store == "" {
		config.Store = StoreMemory
	}
	if len(config.Redis.Addresses) == 0 {
		config.Redis.Addresses = []string{"localhost:6379"}
	}
	if config.Redis.PoolSize == 0 {
		config.Redis.PoolSize = 100
	}
	if config.Algorithms.Default == "" {
		config.Algorithms.Default = AlgorithmTokenBucket
	}
	if config.Limits.Default.Requests == 0 {
		config.Limits.Default.Requests = 100
	}
	if config.Limits.Default.Window == 0 {
		config.Limits.Default.Window = Duration(time.Minute)
	}
	if config.Metrics.Path == "" {
		config.Metrics.Path = "/metrics"
	}
}

func validate(config *Config) error {
	if config.Server.Port < 1 || config.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range", config.Server.Port)
	}
	if config.Store != StoreMemory && config.Store != StoreRedis {
		return fmt.Errorf("store must be %q or %q, got %q", StoreMemory, StoreRedis, config.Store)
	}

	valid := false
	for _, a := range Algorithms {
		if config.Algorithms.Default == a {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("algorithms.default must be one of %s, got %q",
			strings.Join(Algorithms, ", "), config.Algorithms.Default)
	}

	if err := validateLimit("limits.default", config.Limits.Default); err != nil {
		return err
	}
	for name, tier := range config.Limits.Tiers {
		if name == "default" {
			return fmt.Errorf("limits.tiers must not define a tier named %q; use limits.default", name)
		}
		if err := validateLimit("limits.tiers."+name, tier); err != nil {
			return err
		}
	}
	return nil
}

func validateLimit(name string, limit LimitConfig) error {
	if limit.Requests <= 0 {
		return fmt.Errorf("%s.requests must be positive, got %d", name, limit.Requests)
	}
	if limit.Window <= 0 {
		return fmt.Errorf("%s.window must be positive, got %s", name, limit.Window.Std())
	}
	if limit.Burst < 0 {
		return fmt.Errorf("%s.burst must not be negative, got %d", name, limit.Burst)
	}
	return nil
}
