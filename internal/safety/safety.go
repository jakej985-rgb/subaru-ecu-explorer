// Package safety enforces the project's read-only operating boundary at the
// operation layer. A GatedSession is the only supported way to send
// application-level operations through an adapter session: every request
// passes an operation allowlist, a read-only capability check, health
// preconditions, and a rate limit before any byte reaches the transport.
//
// The gate is deliberately above the byte transport: adapter.Session still
// exposes raw Read/Write for protocol implementations, but normal API
// callers must go through Transact so prohibited operations cannot be
// invoked by accident.
package safety

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

// Operation identifies an application-level diagnostic operation.
type Operation string

// Read-only operations permitted by the default policy.
const (
	OpIdentify        Operation = "identify"
	OpReadData        Operation = "read-data"
	OpReadDTC         Operation = "read-dtc"
	OpReadMeasurement Operation = "read-measurement"
)

// Prohibited operations. These are defined so callers and tests can refer
// to them by name; they are never in any allowlist.
const (
	OpWrite          Operation = "write"
	OpClearDTC       Operation = "clear-dtc"
	OpActuatorTest   Operation = "actuator-test"
	OpProgramming    Operation = "programming"
	OpFlash          Operation = "flash"
	OpSecurityAccess Operation = "security-access"
	OpImmobilizer    Operation = "immobilizer"
	OpPinProbe       Operation = "pin-probe"
)

// DefaultAllowlist is the set of operations the project permits.
var DefaultAllowlist = map[Operation]bool{
	OpIdentify:        true,
	OpReadData:        true,
	OpReadDTC:         true,
	OpReadMeasurement: true,
}

// DefaultAllowedHealth are the session health states permitted to transact.
var DefaultAllowedHealth = map[adapter.HealthState]bool{
	adapter.HealthReady: true,
}

// Policy configures a GatedSession.
type Policy struct {
	// Allowlist names the permitted operations. Nil uses DefaultAllowlist.
	Allowlist map[Operation]bool
	// RequireReadOnly rejects sessions that do not declare read-only
	// capability. Defaults to true — this is the project safety boundary.
	RequireReadOnly *bool
	// AllowedHealth lists health states that may transact. Nil uses
	// DefaultAllowedHealth (ready only).
	AllowedHealth map[adapter.HealthState]bool
	// MinInterval is the minimum spacing between Transact calls; 0
	// disables rate limiting.
	MinInterval time.Duration
	// ResponseTimeout bounds the response read after a request is
	// written. <=0 defaults to 2s; the context deadline can shorten it.
	ResponseTimeout time.Duration
	// ResponseIdleTimeout bounds how long to keep reading after the first
	// response bytes arrive. <=0 defaults to 100ms. It lets request/
	// response protocols without framing terminate promptly.
	ResponseIdleTimeout time.Duration
	// MaxResponseBytes caps the accumulated response.
	MaxResponseBytes int
	// Preconditions are extra gates evaluated before every Transact; the
	// first error aborts the operation.
	Preconditions []Precondition
}

// Precondition is an extra gate evaluated before each Transact.
type Precondition func(ctx context.Context, s adapter.Session) error

// Errors returned by the gate.
var (
	// ErrProhibitedOperation means the operation is not in the allowlist.
	ErrProhibitedOperation = errors.New("safety: operation not permitted")
	// ErrReadOnlyRequired means the session does not declare read-only
	// capability while the policy requires it.
	ErrReadOnlyRequired = errors.New("safety: session is not read-only")
	// ErrHealthGate means the session health state is not allowed.
	ErrHealthGate = errors.New("safety: session health does not permit operations")
	// ErrRateLimited means the minimum interval has not elapsed.
	ErrRateLimited = errors.New("safety: rate limit exceeded")
	// ErrPrecondition means a custom precondition failed.
	ErrPrecondition = errors.New("safety: precondition failed")
)

// GatedSession wraps an adapter.Session and is the supported API for
// application-level operations. It intentionally does not re-expose
// unrestricted Write; all requests go through Transact.
type GatedSession struct {
	inner  adapter.Session
	policy Policy

	mu      sync.Mutex
	lastTxn time.Time
}

// Wrap returns a GatedSession enforcing policy over s. A nil policy uses
// defaults.
func Wrap(s adapter.Session, policy *Policy) *GatedSession {
	p := Policy{}
	if policy != nil {
		p = *policy
	}
	if p.Allowlist == nil {
		p.Allowlist = DefaultAllowlist
	}
	if p.RequireReadOnly == nil {
		v := true
		p.RequireReadOnly = &v
	}
	if p.AllowedHealth == nil {
		p.AllowedHealth = DefaultAllowedHealth
	}
	if p.MaxResponseBytes <= 0 {
		p.MaxResponseBytes = 4096
	}
	if p.ResponseTimeout <= 0 {
		p.ResponseTimeout = 2 * time.Second
	}
	if p.ResponseIdleTimeout <= 0 {
		p.ResponseIdleTimeout = 100 * time.Millisecond
	}
	return &GatedSession{inner: s, policy: p}
}

// Inner returns the wrapped session for protocol-layer code that needs raw
// transport access (framing, checksums). Application callers should use
// Transact instead.
func (g *GatedSession) Inner() adapter.Session { return g.inner }

// Authorize reports whether op may run now, without performing I/O.
func (g *GatedSession) Authorize(ctx context.Context, op Operation) error {
	return g.check(ctx, op)
}

func (g *GatedSession) check(ctx context.Context, op Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !g.policy.Allowlist[op] {
		return fmt.Errorf("%w: %q", ErrProhibitedOperation, op)
	}
	if *g.policy.RequireReadOnly && !g.inner.Capabilities().ReadOnly {
		return ErrReadOnlyRequired
	}
	if h := g.inner.Health().State; !g.policy.AllowedHealth[h] {
		return fmt.Errorf("%w: state %s", ErrHealthGate, h)
	}
	for _, pre := range g.policy.Preconditions {
		if err := pre(ctx, g.inner); err != nil {
			return fmt.Errorf("%w: %v", ErrPrecondition, err)
		}
	}
	return nil
}

// Transact performs one gated request/response exchange: it authorizes the
// operation, enforces the rate limit, writes request, then accumulates the
// response until ResponseTimeout, the context deadline, or
// MaxResponseBytes. Prohibited operations fail before any byte is written.
func (g *GatedSession) Transact(ctx context.Context, op Operation, request []byte) ([]byte, error) {
	if err := g.check(ctx, op); err != nil {
		return nil, err
	}

	g.mu.Lock()
	if g.policy.MinInterval > 0 && !g.lastTxn.IsZero() {
		if elapsed := time.Since(g.lastTxn); elapsed < g.policy.MinInterval {
			g.mu.Unlock()
			return nil, fmt.Errorf("%w: %s remaining", ErrRateLimited, g.policy.MinInterval-elapsed)
		}
	}
	g.lastTxn = time.Now()
	g.mu.Unlock()

	// Bound all I/O by the earlier of the context deadline and
	// now+ResponseTimeout, and set it before Write so a stale deadline
	// from a previous Transact cannot reject the request.
	ioDeadline := time.Now().Add(g.policy.ResponseTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(ioDeadline) {
		ioDeadline = dl
	}
	_ = g.inner.SetDeadline(ioDeadline)

	if _, err := g.inner.Write(request); err != nil {
		return nil, err
	}

	var resp []byte
	buf := make([]byte, 512)
	idleDeadline := time.Time{}
	for len(resp) < g.policy.MaxResponseBytes {
		if len(resp) > 0 && idleDeadline.IsZero() {
			idleDeadline = time.Now().Add(g.policy.ResponseIdleTimeout)
			if idleDeadline.After(ioDeadline) {
				idleDeadline = ioDeadline
			}
		}
		deadline := ioDeadline
		if !idleDeadline.IsZero() {
			deadline = idleDeadline
		}
		if !time.Now().Before(deadline) {
			break
		}
		_ = g.inner.SetDeadline(deadline)
		n, err := g.inner.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			if len(resp) >= g.policy.MaxResponseBytes {
				resp = resp[:g.policy.MaxResponseBytes]
				break
			}
		}
		if err != nil {
			// Deadline/timeout after partial or no data is a normal end of
			// response for request/response protocols without framing.
			if errors.Is(err, adapter.ErrTimeout) || isTimeoutErr(err) {
				break
			}
			return resp, err
		}
	}
	return resp, nil
}

func isTimeoutErr(err error) bool {
	var nerr interface{ Timeout() bool }
	return errors.As(err, &nerr) && nerr.Timeout()
}
