package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	UptimeSeconds      int64             `json:"uptime_seconds"`
	RequestsPerSecond  uint64            `json:"requests_per_second"`
	TotalRequests      uint64            `json:"total_requests"`
	TotalErrors        uint64            `json:"total_errors"`
	TotalTimeouts      uint64            `json:"total_timeouts"`
	BackendActiveConns map[string]int64  `json:"backend_active_connections"`
	BackendRequests    map[string]uint64 `json:"backend_requests"`
	StatusCounts       map[int]uint64    `json:"status_counts"`
}

type Registry struct {
	startedAt time.Time

	totalRequests atomic.Uint64
	totalErrors   atomic.Uint64
	totalTimeouts atomic.Uint64

	perSecondCounter atomic.Uint64
	rps              atomic.Uint64

	mu           sync.RWMutex
	statusCounts map[int]uint64
	activeByBE   map[string]int64
	requestsByBE map[string]uint64
}

func NewRegistry() *Registry {
	return &Registry{
		startedAt:    time.Now(),
		statusCounts: make(map[int]uint64),
		activeByBE:   make(map[string]int64),
		requestsByBE: make(map[string]uint64),
	}
}

func (r *Registry) Start(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				r.rps.Store(r.perSecondCounter.Swap(0))
			}
		}
	}()
}

func (r *Registry) IncRequest() {
	r.totalRequests.Add(1)
	r.perSecondCounter.Add(1)
}

func (r *Registry) IncError() {
	r.totalErrors.Add(1)
}

func (r *Registry) IncTimeout() {
	r.totalTimeouts.Add(1)
}

func (r *Registry) ObserveStatus(code int) {
	r.mu.Lock()
	r.statusCounts[code]++
	r.mu.Unlock()
}

func (r *Registry) IncBackendActive(id string) {
	r.mu.Lock()
	r.activeByBE[id]++
	r.mu.Unlock()
}

func (r *Registry) DecBackendActive(id string) {
	r.mu.Lock()
	if r.activeByBE[id] > 0 {
		r.activeByBE[id]--
	}
	r.mu.Unlock()
}

func (r *Registry) IncBackendRequest(id string) {
	r.mu.Lock()
	r.requestsByBE[id]++
	r.mu.Unlock()
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	active := make(map[string]int64, len(r.activeByBE))
	for k, v := range r.activeByBE {
		active[k] = v
	}
	req := make(map[string]uint64, len(r.requestsByBE))
	for k, v := range r.requestsByBE {
		req[k] = v
	}
	status := make(map[int]uint64, len(r.statusCounts))
	for k, v := range r.statusCounts {
		status[k] = v
	}

	return Snapshot{
		UptimeSeconds:      int64(time.Since(r.startedAt).Seconds()),
		RequestsPerSecond:  r.rps.Load(),
		TotalRequests:      r.totalRequests.Load(),
		TotalErrors:        r.totalErrors.Load(),
		TotalTimeouts:      r.totalTimeouts.Load(),
		BackendActiveConns: active,
		BackendRequests:    req,
		StatusCounts:       status,
	}
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Snapshot())
	})
}
