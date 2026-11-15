package adapter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Manager owns physical adapter connections for the local Go service. It
// registers drivers, discovers candidates across all of them, and hands out
// exclusively owned sessions. No two sessions may share one physical device.
type Manager struct {
	mu       sync.Mutex
	drivers  map[Kind]Driver
	sessions map[string]*managed // key: candidate Info.ID
}

type sessionState uint8

const (
	stateOpening sessionState = iota
	stateActive
	stateClosing
)

// managed is a per-device reservation. It exists from before Driver.Open is
// called until after Session.Close returns, so physical access never
// overlaps. Only stateActive entries are exposed to callers.
type managed struct {
	session Session
	kind    Kind
	state   sessionState
}

// NewManager returns an empty adapter manager.
func NewManager() *Manager {
	return &Manager{
		drivers:  map[Kind]Driver{},
		sessions: map[string]*managed{},
	}
}

// RegisterDriver adds a driver. Re-registering the same kind replaces it.
func (m *Manager) RegisterDriver(d Driver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drivers[d.Kind()] = d
}

// Kinds returns the registered adapter kinds in sorted order.
func (m *Manager) Kinds() []Kind {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Kind, 0, len(m.drivers))
	for k := range m.drivers {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Discover enumerates candidates from every registered driver. Candidates
// from different kinds may share IDs; callers select by Info.Kind + Info.ID.
func (m *Manager) Discover(ctx context.Context) ([]Candidate, error) {
	m.mu.Lock()
	drivers := make([]Driver, 0, len(m.drivers))
	for _, d := range m.drivers {
		drivers = append(drivers, d)
	}
	m.mu.Unlock()

	var out []Candidate
	var errs []error
	for _, d := range drivers {
		cands, err := d.Discover(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s discover: %w", d.Kind(), err))
			continue
		}
		for _, c := range cands {
			if c.Info.Kind == "" {
				c.Info.Kind = d.Kind()
			}
			out = append(out, c)
		}
	}
	return out, errors.Join(errs...)
}

// Connect opens an exclusively owned session for candidate. It fails with
// ErrBusy if the candidate ID is already connected through this manager.
func (m *Manager) Connect(ctx context.Context, candidate Candidate) (Session, error) {
	m.mu.Lock()
	d, ok := m.drivers[candidate.Info.Kind]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: no driver for kind %q", ErrUnsupported, candidate.Info.Kind)
	}
	// Any reservation (opening, active, or closing) makes the device busy.
	if _, busy := m.sessions[candidate.Info.ID]; busy {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrBusy, candidate.Info.ID)
	}
	res := &managed{kind: candidate.Info.Kind, state: stateOpening}
	m.sessions[candidate.Info.ID] = res
	m.mu.Unlock()

	s, err := d.Open(ctx, candidate)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		delete(m.sessions, candidate.Info.ID)
		return nil, err
	}
	res.session = s
	res.state = stateActive
	return s, nil
}

// Disconnect closes the session for id, if any, and releases ownership only
// after Close returns. It is safe to call for unknown, still-opening, or
// already-disconnected IDs (those are no-ops).
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	man, ok := m.sessions[id]
	if !ok || man.state != stateActive {
		m.mu.Unlock()
		return nil
	}
	man.state = stateClosing
	m.mu.Unlock()

	err := man.session.Close()

	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	return err
}

// Session returns the active session for id, or nil.
func (m *Manager) Session(id string) Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if man, ok := m.sessions[id]; ok && man.state == stateActive {
		return man.session
	}
	return nil
}

// ConnectedIDs returns the IDs of all active sessions in sorted order.
func (m *Manager) ConnectedIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.sessions))
	for id, man := range m.sessions {
		if man.state == stateActive {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Health returns the health of the session for id, or a closed/unknown health
// when no session exists.
func (m *Manager) Health(id string) Health {
	if s := m.Session(id); s != nil {
		return s.Health()
	}
	return Health{State: HealthClosed, Since: time.Now()}
}
