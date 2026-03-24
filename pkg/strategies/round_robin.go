package strategies

import (
	"errors"
	"sync"
)

// RoundRobinStrategy uses smooth weighted round robin for fair weighted distribution.
type RoundRobinStrategy struct {
	mu      sync.Mutex
	current map[string]int
}

func NewRoundRobin() *RoundRobinStrategy {
	return &RoundRobinStrategy{current: make(map[string]int)}
}

func (s *RoundRobinStrategy) Name() string {
	return "round_robin"
}

func (s *RoundRobinStrategy) Next(_ string, candidates []Candidate) (string, error) {
	if len(candidates) == 0 {
		return "", errors.New("no candidates")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	totalWeight := 0
	best := -1
	for i := range candidates {
		w := normalizeWeight(candidates[i].Weight)
		totalWeight += w
		s.current[candidates[i].ID] += w
		if best == -1 || s.current[candidates[i].ID] > s.current[candidates[best].ID] {
			best = i
		}
	}

	selected := candidates[best].ID
	s.current[selected] -= totalWeight
	return selected, nil
}
