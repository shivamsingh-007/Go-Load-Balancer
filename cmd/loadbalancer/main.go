package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/load-balancer/pkg/adminapi"
	"github.com/example/load-balancer/pkg/backendpool"
	"github.com/example/load-balancer/pkg/config"
	"github.com/example/load-balancer/pkg/healthcheck"
	"github.com/example/load-balancer/pkg/metrics"
	"github.com/example/load-balancer/pkg/proxy"
	"github.com/example/load-balancer/pkg/ratelimit"
	"github.com/example/load-balancer/pkg/strategies"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err.Error())
		os.Exit(1)
	}

	strat, err := strategies.New(cfg.Strategy)
	if err != nil {
		logger.Error("invalid strategy", "error", err.Error())
		os.Exit(1)
	}

	pool, err := backendpool.New(cfg.Backends, strat)
	if err != nil {
		logger.Error("failed to initialize backend pool", "error", err.Error())
		os.Exit(1)
	}

	metricRegistry := metrics.NewRegistry()
	metricStop := make(chan struct{})
	metricRegistry.Start(metricStop)

	checker := healthcheck.New(
		pool,
		time.Duration(cfg.HealthCheck.TimeoutMS)*time.Millisecond,
		time.Duration(cfg.HealthCheck.IntervalMS)*time.Millisecond,
		cfg.HealthCheck.FailureThreshold,
		cfg.HealthCheck.SuccessThreshold,
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker.Start(ctx)

	proxyHandler := proxy.NewHandler(
		pool,
		metricRegistry,
		logger,
		time.Duration(cfg.RequestTimeoutMS)*time.Millisecond,
		cfg.RetryCount,
	)

	rateLimiter := ratelimit.New(cfg.RateLimit.Enabled, cfg.RateLimit.RPS, cfg.RateLimit.Burst)
	frontMux := http.NewServeMux()
	frontMux.Handle("/", rateLimiter.Middleware(proxyHandler))

	admin := adminapi.New(pool, metricRegistry)
	adminMux := http.NewServeMux()
	adminMux.Handle("/", admin.Handler())

	frontServer := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           frontMux,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	adminServer := &http.Server{
		Addr:              cfg.AdminAddress,
		Handler:           adminMux,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("load balancer listening", "addr", cfg.ListenAddress, "strategy", cfg.Strategy)
		if err := frontServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("frontend server error", "error", err.Error())
			cancel()
		}
	}()

	go func() {
		logger.Info("admin api listening", "addr", cfg.AdminAddress)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("admin server error", "error", err.Error())
			cancel()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	case <-ctx.Done():
		logger.Warn("context cancelled, shutting down")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()

	if err := frontServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("frontend shutdown failed", "error", err.Error())
	}
	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin shutdown failed", "error", err.Error())
	}

	close(metricStop)
	fmt.Println("shutdown complete")
}
