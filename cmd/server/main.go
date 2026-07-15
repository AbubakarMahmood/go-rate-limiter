// Command server runs the rate-limiter HTTP service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/config"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/handlers"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/metrics"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	log.Printf("configuration: store=%s default_algorithm=%s limit=%d/%s tiers=%d",
		cfg.Store, cfg.Algorithms.Default,
		cfg.Limits.Default.Requests, cfg.Limits.Default.Window.Std(), len(cfg.Limits.Tiers))

	st, err := newStore(cfg)
	if err != nil {
		log.Fatalf("store initialization failed: %v", err)
	}

	limiters := buildLimiters(cfg, st)
	m := metrics.New(prometheus.DefaultRegisterer)
	handler := handlers.NewRateLimitHandler(limiters, st, m, cfg.Algorithms.Default)

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	v1 := router.Group("/v1")
	{
		v1.POST("/check", handler.Check)
		v1.GET("/status/:key", handler.GetStatus)
		v1.POST("/reset/:key", handler.Reset)
	}
	router.GET("/health", handler.Health)
	if cfg.Metrics.Enabled {
		router.GET(cfg.Metrics.Path, gin.WrapH(promhttp.Handler()))
		log.Printf("metrics enabled at %s", cfg.Metrics.Path)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout.Std(),
		WriteTimeout: cfg.Server.WriteTimeout.Std(),
		IdleTimeout:  cfg.Server.IdleTimeout.Std(),
	}

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Printf("store close: %v", err)
	}
	log.Println("stopped")
}

// loadConfig loads CONFIG_FILE if set (failing hard when it is unreadable or
// invalid), falls back to ./config.yaml when present, and otherwise runs on
// built-in defaults. Environment overrides apply in every case.
func loadConfig() (*config.Config, error) {
	if file := os.Getenv("CONFIG_FILE"); file != "" {
		return config.Load(file)
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return config.Load("config.yaml")
	}
	log.Println("no config file found, using defaults")
	return config.Default()
}

func newStore(cfg *config.Config) (limiter.Store, error) {
	switch cfg.Store {
	case config.StoreRedis:
		return store.NewRedisStore(store.RedisConfig{
			Addresses: cfg.Redis.Addresses,
			Password:  cfg.Redis.Password,
			DB:        cfg.Redis.DB,
			PoolSize:  cfg.Redis.PoolSize,
		})
	default:
		return store.NewMemoryStore(), nil
	}
}

// buildLimiters constructs one limiter per algorithm and tier, all sharing
// the same store.
func buildLimiters(cfg *config.Config, st limiter.Store) map[string]map[string]limiter.RateLimiter {
	tiers := map[string]config.LimitConfig{handlers.DefaultTier: cfg.Limits.Default}
	for name, tier := range cfg.Limits.Tiers {
		tiers[name] = tier
	}

	limiters := make(map[string]map[string]limiter.RateLimiter, len(config.Algorithms))
	for _, algorithm := range config.Algorithms {
		limiters[algorithm] = make(map[string]limiter.RateLimiter, len(tiers))
		for name, tier := range tiers {
			c := limiter.Config{Limit: tier.Requests, Window: tier.Window.Std(), Burst: tier.Burst}
			switch algorithm {
			case config.AlgorithmTokenBucket:
				limiters[algorithm][name] = algorithms.NewTokenBucket(st, c)
			case config.AlgorithmSlidingWindow:
				limiters[algorithm][name] = algorithms.NewSlidingWindowCounter(st, c)
			case config.AlgorithmFixedWindow:
				limiters[algorithm][name] = algorithms.NewFixedWindowCounter(st, c)
			}
		}
	}
	return limiters
}
