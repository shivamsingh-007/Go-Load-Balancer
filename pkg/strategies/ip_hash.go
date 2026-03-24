package strategies

import (
	"errors"
	"hash/fnv"
)

// IPHashStrategy maps client IP to backend deterministically using weighted buckets.
type IPHashStrategy struct{}

func NewIPHash() *IPHashStrategy {
	return &IPHashStrategy{}
}

func (s *IPHashStrategy) Name() string {
	return "ip_hash"
}

func (s *IPHashStrategy) Next(clientIP string, candidates []Candidate) (string, error) {
	if len(candidates) == 0 {
		return "", errors.New("no candidates")
	}
	if clientIP == "" {
		clientIP = "0.0.0.0"
	}

	totalWeight := 0
	for _, c := range candidates {
		totalWeight += normalizeWeight(c.Weight)
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(clientIP))
	bucket := int(h.Sum32() % uint32(totalWeight))

	running := 0
	for _, c := range candidates {
		running += normalizeWeight(c.Weight)
		if bucket < running {
			return c.ID, nil
		}
	}

	return candidates[len(candidates)-1].ID, nil
}
