// Package fake provides in-memory adapter drivers and transports for tests
// and simulation. Nothing here touches real hardware.
package fake

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

// Transport is an in-memory adapter.Transport. Feed supplies inbound bytes
// consumed by Read; Write appends to a separate outbound buffer inspected via
// Written. Deadlines are honored and return adapter.ErrTimeout. It is safe
// for concurrent use.
type Transport struct {
	mu       sync.Mutex
	cond     *sync.Cond
	in       bytes.Buffer
	out      bytes.Buffer
	closed   bool
	deadline time.Time
	timer    *time.Timer
}

// NewTransport returns an empty in-memory transport.
func NewTransport() *Transport {
	t := &Transport{}
	t.cond = sync.NewCond(&t.mu)
	return t
}

// Feed appends bytes as if the remote side sent them.
func (t *Transport) Feed(b []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.in.Write(b)
	t.cond.Broadcast()
}

// expired reports whether the deadline has passed. Caller holds t.mu.
func (t *Transport) expired() bool {
	return !t.deadline.IsZero() && !time.Now().Before(t.deadline)
}

func (t *Transport) Read(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for {
		// Match net.Conn: an expired deadline fails Read even if data is
		// buffered.
		if t.expired() && !t.closed {
			return 0, adapter.ErrTimeout
		}
		if t.in.Len() > 0 || t.closed {
			break
		}
		t.cond.Wait()
	}
	if t.in.Len() == 0 && t.closed {
		return 0, io.EOF
	}
	return t.in.Read(p)
}

func (t *Transport) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return 0, adapter.ErrClosed
	}
	if t.expired() {
		return 0, adapter.ErrTimeout
	}
	return t.out.Write(p)
}

// SetDeadline bounds future Read/Write calls; a blocked Read wakes with
// adapter.ErrTimeout once the deadline passes. A zero time clears it.
func (t *Transport) SetDeadline(d time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deadline = d
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if !d.IsZero() {
		t.timer = time.AfterFunc(time.Until(d), func() {
			t.mu.Lock()
			t.cond.Broadcast()
			t.mu.Unlock()
		})
	}
	t.cond.Broadcast()
	return nil
}

func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
		t.cond.Broadcast()
	}
	return nil
}

// Written returns bytes written so far (for test assertions).
func (t *Transport) Written() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.out.Bytes()...)
}

// Session is a fake adapter.Session.
type Session struct {
	info   adapter.Info
	caps   adapter.Capabilities
	health adapter.Health
	tr     *Transport
}

// NewSession returns a session wrapping tr.
func NewSession(info adapter.Info, caps adapter.Capabilities, tr *Transport) *Session {
	return &Session{
		info:   info,
		caps:   caps,
		health: adapter.Health{State: adapter.HealthReady, Since: time.Now()},
		tr:     tr,
	}
}

func (s *Session) Read(p []byte) (int, error)  { return s.tr.Read(p) }
func (s *Session) Write(p []byte) (int, error) { return s.tr.Write(p) }
func (s *Session) SetDeadline(t time.Time) error {
	return s.tr.SetDeadline(t)
}
func (s *Session) Close() error {
	s.health.State = adapter.HealthClosed
	s.health.Since = time.Now()
	return s.tr.Close()
}
func (s *Session) Info() adapter.Info                 { return s.info }
func (s *Session) Capabilities() adapter.Capabilities { return s.caps }
func (s *Session) Health() adapter.Health             { return s.health }

// Driver is a scriptable fake adapter.Driver.
type Driver struct {
	kind        adapter.Kind
	candidates  []adapter.Candidate
	discoverErr error
	openErr     error
	sessions    []*Session
}

// NewDriver returns a fake driver for kind with the given candidates.
func NewDriver(kind adapter.Kind, candidates ...adapter.Candidate) *Driver {
	return &Driver{kind: kind, candidates: candidates}
}

// SetDiscoverError makes Discover fail.
func (d *Driver) SetDiscoverError(err error) { d.discoverErr = err }

// SetOpenError makes Open fail.
func (d *Driver) SetOpenError(err error) { d.openErr = err }

// Sessions returns sessions created by Open.
func (d *Driver) Sessions() []*Session { return d.sessions }

func (d *Driver) Kind() adapter.Kind { return d.kind }

func (d *Driver) Discover(ctx context.Context) ([]adapter.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.discoverErr != nil {
		return nil, d.discoverErr
	}
	out := make([]adapter.Candidate, len(d.candidates))
	copy(out, d.candidates)
	return out, nil
}

func (d *Driver) Open(ctx context.Context, c adapter.Candidate) (adapter.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.openErr != nil {
		return nil, d.openErr
	}
	for _, cand := range d.candidates {
		if cand.Info.ID == c.Info.ID {
			info := c.Info
			if info.Kind == "" {
				info.Kind = d.kind
			}
			s := NewSession(info, adapter.Capabilities{
				ReadOnly:  true,
				Transport: "fake",
			}, NewTransport())
			d.sessions = append(d.sessions, s)
			return s, nil
		}
	}
	return nil, adapter.ErrNotFound
}
