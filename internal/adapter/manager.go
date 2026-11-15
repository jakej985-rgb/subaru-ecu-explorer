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
	shutdown bool
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
	// done is closed when an in-flight transition (opening or closing)
	// completes, so Shutdown can wait for it.
	done chan struct{}
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
			// Keep partial results: a composite driver may return healthy
			// candidates alongside a failing sub-driver's error.
			errs = append(errs, fmt.Errorf("%s discover: %w", d.Kind(), err))
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
	return m.connect(ctx, candidate)
}

// ReconnectPolicy controls ConnectWithPolicy retries. A zero value uses
// DefaultReconnectPolicy.
type ReconnectPolicy struct {
	// MaxAttempts is the total number of Open attempts (>=1).
	MaxAttempts int
	// Backoff is the delay between attempts.
	Backoff time.Duration
}

// DefaultReconnectPolicy retries transient open failures three times with
// a short backoff.
var DefaultReconnectPolicy = ReconnectPolicy{MaxAttempts: 3, Backoff: 250 * time.Millisecond}

// permanent reports whether err must not be retried.
func permanent(err error) bool {
	return errors.Is(err, ErrBusy) ||
		errors.Is(err, ErrUnsupported) ||
		errors.Is(err, ErrPermission) ||
		errors.Is(err, ErrProtocol) ||
		errors.Is(err, ErrClosed)
}

// ConnectWithPolicy connects like Connect but retries transient failures
// (missing device, I/O errors) according to policy. Context cancellation
// and permanent errors abort immediately.
func (m *Manager) ConnectWithPolicy(ctx context.Context, candidate Candidate, policy ReconnectPolicy) (Session, error) {
	if policy.MaxAttempts <= 0 {
		policy = DefaultReconnectPolicy
	}
	var err error
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(policy.Backoff):
			}
		}
		var s Session
		s, err = m.connect(ctx, candidate)
		if err == nil {
			return s, nil
		}
		if permanent(err) || ctx.Err() != nil {
			return nil, err
		}
	}
	return nil, err
}

func (m *Manager) connect(ctx context.Context, candidate Candidate) (Session, error) {
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return nil, ErrClosed
	}
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
	res := &managed{kind: candidate.Info.Kind, state: stateOpening, done: make(chan struct{})}
	m.sessions[candidate.Info.ID] = res
	m.mu.Unlock()

	s, err := d.Open(ctx, candidate)

	m.mu.Lock()
	if err != nil {
		delete(m.sessions, candidate.Info.ID)
		close(res.done)
		m.mu.Unlock()
		return nil, err
	}
	if m.shutdown {
		// Shutdown raced the open: close immediately, never expose it. The
		// reservation is held until Close returns so Shutdown can wait.
		m.mu.Unlock()
		_ = s.Close()
		m.mu.Lock()
		delete(m.sessions, candidate.Info.ID)
		close(res.done)
		m.mu.Unlock()
		return nil, ErrClosed
	}
	res.session = s
	res.state = stateActive
	close(res.done)
	m.mu.Unlock()
	return s, nil
}

// Shutdown closes every active session and prevents new connections. It is
// safe to call multiple times and from any goroutine; session close errors
// are joined into the result. Shutdown also waits for in-flight opens
// (which close their session once Open returns) and in-flight disconnects.
// If ctx ends first, Shutdown returns ctx.Err(); those operations still
// finish and close their adapters on their own. After Shutdown, Connect
// fails with ErrClosed.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return nil
	}
	m.shutdown = true
	sessions := make([]Session, 0, len(m.sessions))
	var pending []chan struct{}
	for id, man := range m.sessions {
		if man.state == stateActive && man.session != nil {
			sessions = append(sessions, man.session)
			delete(m.sessions, id)
			continue
		}
		// Opening or closing: the owning goroutine releases it.
		pending = append(pending, man.done)
	}
	m.mu.Unlock()

	var errs []error
	for _, s := range sessions {
		// Always close every captured session: their reservations are
		// already gone, so skipping one would strand an open adapter.
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, ch := range pending {
		select {
		case <-ch:
		case <-ctx.Done():
		}
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// disconnectSession disconnects id only if it is still owned by exactly s.
// A nil s matches any active session.
func (m *Manager) disconnectSession(id string, s Session) error {
	m.mu.Lock()
	man, ok := m.sessions[id]
	if !ok || man.state != stateActive || (s != nil && man.session != s) {
		m.mu.Unlock()
		return nil
	}
	man.state = stateClosing
	man.done = make(chan struct{})
	m.mu.Unlock()

	err := man.session.Close()

	m.mu.Lock()
	delete(m.sessions, id)
	close(man.done)
	m.mu.Unlock()
	return err
}

// Disconnect closes the session for id, if any, and releases ownership only
// after Close returns. It is safe to call for unknown, still-opening, or
// already-disconnected IDs (those are no-ops).
func (m *Manager) Disconnect(id string) error {
	return m.disconnectSession(id, nil)
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
