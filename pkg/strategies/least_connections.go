package strategies

import "errors"

// LeastConnectionsStrategy picks backend with lowest activeConns / weight score.
type LeastConnectionsStrategy struct{}

func NewLeastConnections() *LeastConnectionsStrategy {
	return &LeastConnectionsStrategy{}
}

func (s *LeastConnectionsStrategy) Name() string {
	return "least_connections"
}

func (s *LeastConnectionsStrategy) Next(_ string, candidates []Candidate) (string, error) {
	if len(candidates) == 0 {
		return "", errors.New("no candidates")
	}

	best := candidates[0]
	bestScore := float64(best.ActiveConns) / float64(normalizeWeight(best.Weight))
	for _, c := range candidates[1:] {
		score := float64(c.ActiveConns) / float64(normalizeWeight(c.Weight))
		if score < bestScore {
			best = c
			bestScore = score
		}
	}
	return best.ID, nil
}
