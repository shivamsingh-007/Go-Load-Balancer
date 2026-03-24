package healthcheck

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/load-balancer/pkg/backendpool"
)

type Checker struct {
	pool             *backendpool.Pool
	client           *http.Client
	interval         time.Duration
	failureThreshold int
	successThreshold int
	logger           *slog.Logger

	failureCount map[string]int
	successCount map[string]int
}

func New(
	pool *backendpool.Pool,
	timeout time.Duration,
	interval time.Duration,
	failureThreshold int,
	successThreshold int,
	logger *slog.Logger,
) *Checker {
	return &Checker{
		pool:             pool,
		client:           &http.Client{Timeout: timeout},
		interval:         interval,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		logger:           logger,
		failureCount:     make(map[string]int),
		successCount:     make(map[string]int),
	}
}

func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.checkAll()
			}
		}
	}()
}

func (c *Checker) checkAll() {
	backends := c.pool.List()
	for _, b := range backends {
		url := fmt.Sprintf("http://%s:%d%s", b.Host, b.Port, b.HealthPath)
		resp, err := c.client.Get(url)
		healthy := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		if resp != nil {
			_ = resp.Body.Close()
		}

		if healthy {
			c.failureCount[b.ID] = 0
			c.successCount[b.ID]++
			if !b.Healthy && c.successCount[b.ID] >= c.successThreshold {
				c.pool.MarkHealthy(b.ID, true)
				c.logger.Info("backend recovered", "backend_id", b.ID)
			}
		} else {
			c.successCount[b.ID] = 0
			c.failureCount[b.ID]++
			if b.Healthy && c.failureCount[b.ID] >= c.failureThreshold {
				c.pool.MarkHealthy(b.ID, false)
				c.logger.Warn("backend marked unhealthy", "backend_id", b.ID)
			}
		}
	}
}
