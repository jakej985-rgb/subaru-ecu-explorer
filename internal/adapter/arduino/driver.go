package arduino

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/serialenum"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/serialport"
)

// Opener opens a serial path as a transport. serialport.Open is the
// production implementation; tests inject fakes.
type Opener func(ctx context.Context, path string, baud uint32) (adapter.Transport, error)

// Driver is the adapter.Driver for the Arduino Mega acquisition interface
// (kind arduino-serial). It discovers USB serial devices, opens the
// versioned framed transport, and performs the HELLO/CAPABILITY
// handshake before exposing a Session.
type Driver struct {
	kind             adapter.Kind
	baud             uint32
	opener           Opener
	enum             adapter.Enumerator
	handshakeTimeout time.Duration
	perReadTimeout   time.Duration
}

// NewDriver returns a production Arduino driver: USB serial discovery via
// serialenum and transport via serialport.Open.
func NewDriver(kind adapter.Kind, baud uint32) *Driver {
	return &Driver{
		kind: kind,
		baud: baud,
		opener: func(_ context.Context, path string, baud uint32) (adapter.Transport, error) {
			return serialport.Open(path, baud)
		},
		enum:             serialenum.New(""),
		handshakeTimeout: 3 * time.Second,
		perReadTimeout:   500 * time.Millisecond,
	}
}

// NewDriverWith returns a driver with injected transport opener and
// enumerator (tests, simulation).
func NewDriverWith(kind adapter.Kind, baud uint32, opener Opener, enum adapter.Enumerator) *Driver {
	d := NewDriver(kind, baud)
	if opener != nil {
		d.opener = opener
	}
	if enum != nil {
		d.enum = enum
	}
	return d
}

func (d *Driver) Kind() adapter.Kind { return d.kind }

func (d *Driver) Discover(ctx context.Context) ([]adapter.Candidate, error) {
	if d.enum == nil {
		return nil, nil
	}
	cands, err := d.enum.Enumerate(ctx)
	if err != nil {
		return cands, err
	}
	for i := range cands {
		cands[i].Info.Kind = d.kind
	}
	return cands, nil
}

func (d *Driver) candidatePath(c adapter.Candidate) string {
	if c.Info.Path != "" {
		return c.Info.Path
	}
	return c.Metadata["path"]
}

func (d *Driver) candidateBaud(c adapter.Candidate) uint32 {
	if b := c.Metadata["baud"]; b != "" {
		var v uint32
		if _, err := fmt.Sscanf(b, "%d", &v); err == nil && v != 0 {
			return v
		}
	}
	return d.baud
}

// Open connects to the candidate, performs the capability handshake, and
// returns a session. The handshake requests capabilities and accepts the
// first HELLO or CAPABILITY frame; protocol-version mismatches and
// handshake timeouts fail with ErrProtocol/ErrTimeout and close the port.
func (d *Driver) Open(ctx context.Context, c adapter.Candidate) (adapter.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := d.candidatePath(c)
	if path == "" {
		return nil, fmt.Errorf("%w: candidate %s has no path", adapter.ErrNotFound, c.Info.ID)
	}
	tr, err := d.opener(ctx, path, d.candidateBaud(c))
	if err != nil {
		return nil, err
	}

	s := &Session{
		info:  c.Info,
		tr:    tr,
		rd:    NewReader(tr),
		since: time.Now(),
	}
	if s.info.Kind == "" {
		s.info.Kind = d.kind
	}

	if err := s.handshake(ctx, d); err != nil {
		_ = tr.Close()
		return nil, err
	}
	return s, nil
}

// handshake requests capabilities and waits for the firmware identity.
func (s *Session) handshake(ctx context.Context, d *Driver) error {
	req, err := Encode(Frame{Type: TypeCapability, Seq: 0})
	if err != nil {
		return err
	}
	if _, err := s.tr.Write(req); err != nil {
		return err
	}
	deadline := time.Now().Add(d.handshakeTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: no hello/capability frame within %s", adapter.ErrTimeout, d.handshakeTimeout)
		}
		f, err := s.rd.ReadFrame(d.perReadTimeout)
		if err != nil {
			if errors.Is(err, adapter.ErrTimeout) {
				continue
			}
			return err
		}
		if f.Type != TypeHello && f.Type != TypeCapability {
			continue // heartbeats/status before identity are fine
		}
		h, err := DecodeHello(f.Payload)
		if err != nil {
			return fmt.Errorf("%w: %v", adapter.ErrProtocol, err)
		}
		if h.ProtocolVersion != FrameVersion {
			return fmt.Errorf("%w: firmware protocol %d, host %d", adapter.ErrProtocol, h.ProtocolVersion, FrameVersion)
		}
		s.hello = h
		return nil
	}
}

// Session is an open Arduino acquisition session. Transport I/O is the raw
// serial stream; use Frames() for the versioned frame protocol.
type Session struct {
	info  adapter.Info
	tr    adapter.Transport
	rd    *Reader
	hello Hello
	since time.Time
}

// Frames returns the resyncing frame reader for this session.
func (s *Session) Frames() *Reader { return s.rd }

// Hello returns the negotiated firmware identity.
func (s *Session) Hello() Hello { return s.hello }

// Read drains bytes buffered by the frame reader (e.g. handshake
// read-ahead) before reading the transport, so the raw stream stays intact.
func (s *Session) Read(p []byte) (int, error) {
	if len(s.rd.buf) > 0 {
		n := copy(p, s.rd.buf)
		s.rd.buf = s.rd.buf[n:]
		return n, nil
	}
	return s.tr.Read(p)
}
func (s *Session) Write(p []byte) (int, error)   { return s.tr.Write(p) }
func (s *Session) SetDeadline(t time.Time) error { return s.tr.SetDeadline(t) }
func (s *Session) Close() error                  { return s.tr.Close() }
func (s *Session) Info() adapter.Info            { return s.info }
func (s *Session) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		ReadOnly:  true,
		Transport: "usb-serial",
		Protocols: []string{"diag-acquisition"},
		Notes:     "fw " + s.hello.FirmwareVersion + " on " + s.hello.Board,
	}
}
func (s *Session) Health() adapter.Health {
	return adapter.Health{State: adapter.HealthReady, Detail: "fw " + s.hello.FirmwareVersion, Since: s.since}
}
