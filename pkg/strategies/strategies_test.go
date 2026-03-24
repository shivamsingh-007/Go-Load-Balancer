package strategies

import "testing"

func TestRoundRobinWeightedDistribution(t *testing.T) {
	rr := NewRoundRobin()
	candidates := []Candidate{
		{ID: "a", Weight: 1},
		{ID: "b", Weight: 2},
	}

	count := map[string]int{}
	for i := 0; i < 300; i++ {
		id, err := rr.Next("", candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count[id]++
	}

	if count["b"] <= count["a"] {
		t.Fatalf("expected weighted backend b to receive more traffic, got a=%d b=%d", count["a"], count["b"])
	}
}

func TestLeastConnections(t *testing.T) {
	lc := NewLeastConnections()
	candidates := []Candidate{
		{ID: "a", Weight: 1, ActiveConns: 10},
		{ID: "b", Weight: 1, ActiveConns: 2},
	}

	id, err := lc.Next("", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "b" {
		t.Fatalf("expected b, got %s", id)
	}
}

func TestIPHashDeterministic(t *testing.T) {
	ipHash := NewIPHash()
	candidates := []Candidate{
		{ID: "a", Weight: 1},
		{ID: "b", Weight: 1},
		{ID: "c", Weight: 1},
	}

	first, err := ipHash.Next("203.0.113.10", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 10; i++ {
		next, err := ipHash.Next("203.0.113.10", candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if next != first {
			t.Fatalf("expected deterministic result %s, got %s", first, next)
		}
	}
}
