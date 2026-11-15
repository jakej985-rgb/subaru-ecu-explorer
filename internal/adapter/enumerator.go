package adapter

import (
	"context"
	"sort"
)

// Enumerator lists adapter candidates for one transport family without
// opening them. It backs driver discovery and is re-runnable: callers poll
// or re-invoke it to observe hot-plug changes.
type Enumerator interface {
	Enumerate(ctx context.Context) ([]Candidate, error)
}

// ListEnumerator returns a fixed, explicitly configured set of candidates.
// It is the mechanism for user-selected endpoints (network addresses,
// manual device paths) where automatic enumeration is impossible or
// unwanted. The list order is preserved and the result is a copy.
type ListEnumerator struct {
	candidates []Candidate
}

// NewListEnumerator returns an enumerator over candidates.
func NewListEnumerator(candidates ...Candidate) *ListEnumerator {
	out := make([]Candidate, len(candidates))
	copy(out, candidates)
	return &ListEnumerator{candidates: out}
}

// Enumerate returns the configured candidates sorted by kind then ID for
// deterministic output.
func (e *ListEnumerator) Enumerate(_ context.Context) ([]Candidate, error) {
	out := make([]Candidate, len(e.candidates))
	copy(out, e.candidates)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Info.Kind != out[j].Info.Kind {
			return out[i].Info.Kind < out[j].Info.Kind
		}
		return out[i].Info.ID < out[j].Info.ID
	})
	return out, nil
}
