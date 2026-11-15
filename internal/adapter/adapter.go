// Package adapter defines the hardware adapter contracts shared by the
// ELM327-compatible OBD-II, Subaru SSM, and Arduino acquisition paths.
//
// Drivers own physical transport details; protocol semantics (AT commands,
// SSM messages, ECU decoding) live in later layers and never leak into these
// interfaces. The local Go service owns every connection — browser clients
// never touch device transports.
package adapter

import (
	"context"
	"errors"
	"io"
	"time"
)

// Kind identifies an adapter implementation path.
type Kind string

const (
	// KindELM327 is a generic ELM327-compatible OBD-II adapter.
	KindELM327 Kind = "elm327-odbii"
	// KindSubaruSSM is a Subaru-specific SSM interface.
	KindSubaruSSM Kind = "subaru-ssm"
	// KindArduino is the custom Arduino Mega acquisition interface over a
	// versioned USB serial transport.
	KindArduino Kind = "arduino-serial"
)

// Capabilities describes what an adapter path supports. ReadOnly reflects the
// project's read-only operating boundary, not the hardware's raw ability.
type Capabilities struct {
	ReadOnly  bool     `json:"readOnly"`
	Transport string   `json:"transport"`
	Protocols []string `json:"protocols,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

// HealthState is the adapter health lifecycle state.
type HealthState string

const (
	HealthUnknown    HealthState = "unknown"
	HealthConnecting HealthState = "connecting"
	HealthReady      HealthState = "ready"
	HealthDegraded   HealthState = "degraded"
	HealthFault      HealthState = "fault"
	HealthClosed     HealthState = "closed"
)

// Health reports adapter health with the time of the last state change.
type Health struct {
	State  HealthState `json:"state"`
	Detail string      `json:"detail,omitempty"`
	Since  time.Time   `json:"since"`
}

// Info is the stable adapter identity. ID must be stable across reconnects
// where the transport provides one (device path, serial, IP:port).
type Info struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// Candidate is one discovered, not-yet-open adapter instance.
type Candidate struct {
	Info     Info              `json:"info"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Session is an open, exclusively owned adapter connection. It embeds a
// byte-stream Transport; framing and protocol semantics are the caller's
// concern, not the transport's.
type Session interface {
	// Transport is the raw byte stream for this session.
	Transport
	// Info returns the adapter identity negotiated at open time.
	Info() Info
	// Capabilities returns the adapter's declared capabilities.
	Capabilities() Capabilities
	// Health returns the current health state.
	Health() Health
}

// Transport is a bidirectional byte stream with deadlines. Implementations
// must be safe for one reader and one writer concurrently.
type Transport interface {
	io.Reader
	io.Writer
	// SetDeadline bounds future Read/Write calls.
	SetDeadline(t time.Time) error
	// Close releases the physical transport. It must be idempotent.
	Close() error
}

// Driver is one adapter implementation path (ELM327, SSM, Arduino, ...).
// Drivers are interchangeable behind this contract and must not embed
// vehicle-specific protocol or decoder logic.
type Driver interface {
	// Kind returns the adapter kind this driver serves.
	Kind() Kind
	// Discover enumerates currently available candidates. It must not open
	// connections; an empty result with nil error means none found.
	Discover(ctx context.Context) ([]Candidate, error)
	// Open establishes a session for candidate. The caller (Manager) owns the
	// returned session and guarantees exclusive access to the physical device.
	Open(ctx context.Context, candidate Candidate) (Session, error)
}

// Sentinel errors. Drivers should wrap these where applicable so callers can
// match with errors.Is.
var (
	// ErrNotFound indicates the candidate is no longer present.
	ErrNotFound = errors.New("adapter: candidate not found")
	// ErrBusy indicates the physical device is already owned by a session.
	ErrBusy = errors.New("adapter: device already in use")
	// ErrClosed indicates I/O on a closed session.
	ErrClosed = errors.New("adapter: session closed")
	// ErrTimeout indicates a deadline-bound operation timed out.
	ErrTimeout = errors.New("adapter: i/o timeout")
	// ErrUnsupported indicates the driver does not support the operation.
	ErrUnsupported = errors.New("adapter: unsupported operation")
	// ErrPermission indicates the host denied access to the device (for
	// example missing read/write permission on a serial node).
	ErrPermission = errors.New("adapter: permission denied")
	// ErrProtocol indicates malformed adapter framing or an unexpected
	// handshake response.
	ErrProtocol = errors.New("adapter: protocol error")
)
