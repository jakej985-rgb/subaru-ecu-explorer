// Package tcpdev implements an adapter.Driver and Transport for network
// (TCP) adapter endpoints, e.g. WiFi ELM327 adapters at host:port.
// Endpoints are always explicitly configured — there is no network
// scanning, consistent with the no-arbitrary-probing safety rule.
package tcpdev

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

// Driver serves TCP candidates produced by an adapter.Enumerator (typically
// adapter.NewListEnumerator for explicit endpoints).
type Driver struct {
	kind adapter.Kind
	enum adapter.Enumerator
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// NewDriver returns a TCP driver for kind backed by enum.
func NewDriver(kind adapter.Kind, enum adapter.Enumerator) *Driver {
	return &Driver{kind: kind, enum: enum}
}

func (d *Driver) Kind() adapter.Kind { return d.kind }

func (d *Driver) Discover(ctx context.Context) ([]adapter.Candidate, error) {
	cands, err := d.enum.Enumerate(ctx)
	if err != nil {
		return cands, err
	}
	for i := range cands {
		if cands[i].Info.Kind == "" {
			cands[i].Info.Kind = d.kind
		}
	}
	return cands, nil
}

func (d *Driver) Open(ctx context.Context, c adapter.Candidate) (adapter.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addr := c.Info.Path
	if addr == "" {
		addr = c.Metadata["address"]
	}
	if addr == "" {
		return nil, fmt.Errorf("%w: candidate %s has no address", adapter.ErrNotFound, c.Info.ID)
	}
	dial := d.dial
	if dial == nil {
		dialer := net.Dialer{}
		dial = dialer.DialContext
	}
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if c.Info.ID == "" {
		c.Info.ID = addr
	}
	if c.Info.Kind == "" {
		c.Info.Kind = d.kind
	}
	return &session{info: c.Info, conn: conn, since: time.Now()}, nil
}

type session struct {
	info  adapter.Info
	conn  net.Conn
	since time.Time
}

func (s *session) Read(p []byte) (int, error)    { return s.conn.Read(p) }
func (s *session) Write(p []byte) (int, error)   { return s.conn.Write(p) }
func (s *session) SetDeadline(t time.Time) error { return s.conn.SetDeadline(t) }
func (s *session) Close() error                  { return s.conn.Close() }
func (s *session) Info() adapter.Info            { return s.info }
func (s *session) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{ReadOnly: true, Transport: "tcp", Notes: "explicitly configured endpoint"}
}
func (s *session) Health() adapter.Health {
	return adapter.Health{State: adapter.HealthReady, Since: s.since}
}
