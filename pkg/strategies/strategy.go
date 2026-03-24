package strategies

import "fmt"

// Candidate is a lightweight backend view used by routing strategies.
type Candidate struct {
	ID          string
	Weight      int
	ActiveConns int64
}

// Strategy picks a backend ID from a set of healthy candidates.
type Strategy interface {
	Name() string
	Next(clientIP string, candidates []Candidate) (string, error)
}

func normalizeWeight(w int) int {
	if w <= 0 {
		return 1
	}
	return w
}

func New(name string) (Strategy, error) {
	switch name {
	case "round_robin", "weighted_round_robin":
		return NewRoundRobin(), nil
	case "least_connections", "least_busy":
		return NewLeastConnections(), nil
	case "ip_hash":
		return NewIPHash(), nil
	default:
		return nil, fmt.Errorf("unsupported strategy %q", name)
	}
}
