package adapter

import (
	"context"
	"sync"
	"time"
)

// ProbeFunc performs an explicit liveness check on a session. A nil probe
// means the monitor trusts Session.Health() alone.
type ProbeFunc func(ctx context.Context, s Session) error

// HealthEvent records a health-state transition observed by a Monitor.
type HealthEvent struct {
	ID     string
	Kind   Kind
	From   HealthState
	To     HealthState
	Detail string
}

// Monitor periodically evaluates session health and reports transitions.
// It never performs I/O itself beyond the optional ProbeFunc; all access
// goes through the owning Manager's sessions.
type Monitor struct {
	mgr               *Manager
	interval          time.Duration
	probe             ProbeFunc
	disconnectOnFault bool

	mu   sync.Mutex
	last map[string]tracked
}

// tracked is the last observed state of one specific session.
type tracked struct {
	session Session
	kind    Kind
	state   HealthState
}

// NewMonitor returns a monitor for mgr. interval <= 0 defaults to 5s.
// disconnectOnFault controls whether a faulted session is disconnected
// automatically (recommended for unattended operation).
func NewMonitor(mgr *Manager, interval time.Duration, probe ProbeFunc, disconnectOnFault bool) *Monitor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Monitor{
		mgr:               mgr,
		interval:          interval,
		probe:             probe,
		disconnectOnFault: disconnectOnFault,
		last:              map[string]tracked{},
	}
}

// Check performs one health pass over all connected sessions and returns
// the transitions observed since the previous check. Sessions reporting
// HealthFault (or failing the probe) are disconnected when the monitor was
// configured with disconnectOnFault.
func (mn *Monitor) Check(ctx context.Context) []HealthEvent {
	var events []HealthEvent
	for _, id := range mn.mgr.ConnectedIDs() {
		s := mn.mgr.Session(id)
		if s == nil {
			continue
		}
		h := s.Health()
		to := h.State
		detail := h.Detail
		if mn.probe != nil {
			if err := mn.probe(ctx, s); err != nil {
				if ctx.Err() != nil {
					// Monitor cancelled, not a device fault.
					return events
				}
				to = HealthFault
				detail = err.Error()
			}
		}
		if to == "" {
			to = HealthUnknown
		}

		kind := s.Info().Kind
		mn.mu.Lock()
		prev, seen := mn.last[id]
		mn.last[id] = tracked{session: s, kind: kind, state: to}
		mn.mu.Unlock()
		from := HealthUnknown
		if seen {
			if prev.session != s {
				// Reconnected under the same ID: the old session closed.
				events = append(events, HealthEvent{ID: id, Kind: prev.kind, From: prev.state, To: HealthClosed, Detail: "session replaced"})
			} else {
				from = prev.state
			}
		}
		if to != from {
			events = append(events, HealthEvent{ID: id, Kind: kind, From: from, To: to, Detail: detail})
		}
		if to == HealthFault && mn.disconnectOnFault {
			_ = mn.mgr.disconnectSession(id, s)
			mn.mu.Lock()
			delete(mn.last, id)
			mn.mu.Unlock()
			events = append(events, HealthEvent{ID: id, Kind: kind, From: to, To: HealthClosed, Detail: "disconnected on fault"})
		}
	}
	// Report and forget sessions that disappeared or were replaced.
	mn.mu.Lock()
	for id, prev := range mn.last {
		if mn.mgr.Session(id) != prev.session {
			delete(mn.last, id)
			events = append(events, HealthEvent{ID: id, Kind: prev.kind, From: prev.state, To: HealthClosed, Detail: "disconnected"})
		}
	}
	mn.mu.Unlock()
	return events
}

// Run loops Check on the monitor interval until ctx is cancelled.
func (mn *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(mn.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			mn.Check(ctx)
		}
	}
}
