package backendpool

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/example/load-balancer/pkg/config"
	"github.com/example/load-balancer/pkg/strategies"
)

type Backend struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Weight     int    `json:"weight"`
	HealthPath string `json:"health_path"`
	Healthy    bool   `json:"healthy"`

	activeConns atomic.Int64
}

type BackendSnapshot struct {
	ID          string `json:"id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Weight      int    `json:"weight"`
	HealthPath  string `json:"health_path"`
	Healthy     bool   `json:"healthy"`
	ActiveConns int64  `json:"active_connections"`
}

func (b *Backend) Address() string {
	return fmt.Sprintf("%s:%d", b.Host, b.Port)
}

func (b *Backend) URL() string {
	return "http://" + b.Address()
}

func (b *Backend) ActiveConns() int64 {
	return b.activeConns.Load()
}

type Pool struct {
	mu       sync.RWMutex
	backends map[string]*Backend
	order    []string
	strategy strategies.Strategy
}

func New(backends []config.BackendConfig, strategy strategies.Strategy) (*Pool, error) {
	p := &Pool{
		backends: make(map[string]*Backend, len(backends)),
		order:    make([]string, 0, len(backends)),
		strategy: strategy,
	}
	for _, b := range backends {
		if err := p.AddBackend(b); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *Pool) AddBackend(cfg config.BackendConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.backends[cfg.ID]; ok {
		return fmt.Errorf("backend %s already exists", cfg.ID)
	}
	if cfg.Weight <= 0 {
		cfg.Weight = 1
	}
	if cfg.HealthPath == "" {
		cfg.HealthPath = "/health"
	}
	p.backends[cfg.ID] = &Backend{
		ID:         cfg.ID,
		Host:       cfg.Host,
		Port:       cfg.Port,
		Weight:     cfg.Weight,
		HealthPath: cfg.HealthPath,
		Healthy:    true,
	}
	p.order = append(p.order, cfg.ID)
	sort.Strings(p.order)
	return nil
}

func (p *Pool) RemoveBackend(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.backends[id]; !ok {
		return fmt.Errorf("backend %s not found", id)
	}
	delete(p.backends, id)
	for i := range p.order {
		if p.order[i] == id {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
	return nil
}

func (p *Pool) SetStrategy(s strategies.Strategy) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.strategy = s
}

func (p *Pool) StrategyName() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.strategy == nil {
		return ""
	}
	return p.strategy.Name()
}

func (p *Pool) SelectBackend(clientIP string, exclude map[string]bool) (*Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.strategy == nil {
		return nil, errors.New("strategy not configured")
	}

	candidates := make([]strategies.Candidate, 0, len(p.order))
	for _, id := range p.order {
		b := p.backends[id]
		if !b.Healthy {
			continue
		}
		if exclude != nil && exclude[b.ID] {
			continue
		}
		candidates = append(candidates, strategies.Candidate{
			ID:          b.ID,
			Weight:      b.Weight,
			ActiveConns: b.ActiveConns(),
		})
	}
	if len(candidates) == 0 {
		return nil, errors.New("no healthy backends")
	}

	id, err := p.strategy.Next(clientIP, candidates)
	if err != nil {
		return nil, err
	}
	backend, ok := p.backends[id]
	if !ok {
		return nil, fmt.Errorf("selected backend %s does not exist", id)
	}
	return backend, nil
}

func (p *Pool) MarkHealthy(id string, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if b, ok := p.backends[id]; ok {
		b.Healthy = healthy
	}
}

func (p *Pool) IncActive(id string) {
	p.mu.RLock()
	b, ok := p.backends[id]
	p.mu.RUnlock()
	if ok {
		b.activeConns.Add(1)
	}
}

func (p *Pool) DecActive(id string) {
	p.mu.RLock()
	b, ok := p.backends[id]
	p.mu.RUnlock()
	if ok {
		b.activeConns.Add(-1)
	}
}

func (p *Pool) List() []BackendSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]BackendSnapshot, 0, len(p.order))
	for _, id := range p.order {
		b := p.backends[id]
		out = append(out, BackendSnapshot{
			ID:          b.ID,
			Host:        b.Host,
			Port:        b.Port,
			Weight:      b.Weight,
			HealthPath:  b.HealthPath,
			Healthy:     b.Healthy,
			ActiveConns: b.ActiveConns(),
		})
	}
	return out
}

func (p *Pool) Get(id string) (*Backend, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	b, ok := p.backends[id]
	return b, ok
}
