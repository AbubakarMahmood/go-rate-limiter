package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood1/go-rate-limiter/internal/config"
	"github.com/AbubakarMahmood1/go-rate-limiter/internal/handlers"
	"github.com/AbubakarMahmood1/go-rate-limiter/internal/metrics"
	"github.com/AbubakarMahmood1/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Load configuration
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "config.yaml"
	}

	cfg := config.LoadOrDefault(configFile)
	log.Printf("Loaded configuration: store=%s, algorithm=%s", cfg.Store, cfg.Algorithms.Default)

	// Initialize store
	var storeInstance limiter.Store
	var err error

	switch cfg.Store {
	case "redis":
		redisConfig := store.RedisConfig{
			Addresses: cfg.Redis.Addresses,
			Password:  cfg.Redis.Password,
			DB:        cfg.Redis.DB,
			PoolSize:  cfg.Redis.PoolSize,
			TTL:       cfg.Redis.TTL,
		}
		storeInstance, err = store.NewRedisStore(redisConfig)
		if err != nil {
			log.Fatalf("Failed to initialize Redis store: %v", err)
		}
		log.Println("Using Redis store")
	default:
		storeInstance = store.NewMemoryStore()
		log.Println("Using in-memory store")
	}

	defer storeInstance.Close()

	// Initialize metrics
	metricsInstance := metrics.NewMetrics()

	// Create rate limiters for each algorithm
	limiters := make(map[string]limiter.RateLimiter)

	// Token Bucket
	limiters["token_bucket"] = algorithms.NewTokenBucket(storeInstance, limiter.Config{
		Limit:  cfg.Limits.Default.Requests,
		Window: cfg.Limits.Default.Window,
		Burst:  cfg.Limits.Default.Burst,
	})

	// Sliding Window Counter
	limiters["sliding_window"] = algorithms.NewSlidingWindowCounter(storeInstance, limiter.Config{
		Limit:  cfg.Limits.Default.Requests,
		Window: cfg.Limits.Default.Window,
	})

	// Fixed Window Counter
	limiters["fixed_window"] = algorithms.NewFixedWindowCounter(storeInstance, limiter.Config{
		Limit:  cfg.Limits.Default.Requests,
		Window: cfg.Limits.Default.Window,
	})

	log.Printf("Initialized %d algorithms", len(limiters))

	// Set Gin mode
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create HTTP router
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Create handlers
	handler := handlers.NewRateLimitHandler(limiters, metricsInstance, cfg.Algorithms.Default)

	// Register routes
	v1 := router.Group("/v1")
	{
		v1.POST("/check", handler.Check)
		v1.GET("/status/:key", handler.GetStatus)
		v1.POST("/reset/:key", handler.Reset)
	}

	router.GET("/health", handler.Health)

	// Metrics endpoint
	if cfg.Metrics.Enabled {
		router.GET(cfg.Metrics.Path, gin.WrapH(promhttp.Handler()))
		log.Printf("Metrics enabled at %s", cfg.Metrics.Path)
	}

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
