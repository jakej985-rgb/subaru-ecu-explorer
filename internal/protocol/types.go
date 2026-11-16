// Package protocol defines diagnostic protocol sessions, requests,
// responses, raw frames, metadata, timing, addressing, and typed errors
// independent of adapter type. Protocol implementations (OBD-II, SSM,
// acquisition) plug in via the Codec interface and run over any
// adapter.Transport — including in-memory fakes — with no
// hardware-specific imports.
package protocol

import (
	"errors"
	"fmt"
	"time"
)

// Direction identifies who produced a message on the wire.
type Direction int

const (
	// DirectionHostToEcu is a message transmitted by the host.
	DirectionHostToEcu Direction = iota
	// DirectionEcuToHost is a message received from the ECU/module.
	DirectionEcuToHost
)

// String returns a human-readable direction name.
func (d Direction) String() string {
	switch d {
	case DirectionHostToEcu:
		return "host->ecu"
	case DirectionEcuToHost:
		return "ecu->host"
	default:
		return "unknown"
	}
}

// Address identifies the destination/source module for a message.
// The zero Address means "default/broadcast" — the only address a
// read-only session may use until addressing is verified.
type Address struct {
	// Scheme names the addressing space, e.g. "default", "kline",
	// "can-physical", "iso9141".
	Scheme string
	// Value is the scheme-specific address (pin, CAN ID, header byte).
	Value string
}

// IsZero reports whether the address is the default/broadcast address.
func (a Address) IsZero() bool { return a.Scheme == "" && a.Value == "" }

// Message is a single decoded unit of protocol traffic with its raw wire
// bytes preserved for evidence.
type Message struct {
	// Direction is who produced the message.
	Direction Direction
	// Timestamp is when the message was sent (host) or received (ECU).
	Timestamp time.Time
	// Raw is the exact wire encoding of the message.
	Raw []byte
	// Payload is the decoded payload (Raw minus framing/checksum).
	Payload []byte
	// Address is the module address carried by the message, if any.
	Address Address
	// Metadata carries protocol-specific annotations (service, PID,
	// timing marks, decoder notes). Keys are namespaced by the codec.
	Metadata map[string]any
}

// Request is a protocol-level request: a service invocation against an
// optional module address. Encoding to wire bytes is the Codec's job.
type Request struct {
	// Service is the protocol service byte (e.g. an OBD-II mode or SSM
	// command class). Meaning is codec-defined.
	Service byte
	// Payload carries service parameters. May be empty.
	Payload []byte
	// Address selects the target module; zero value = default.
	Address Address
	// Timeout bounds the whole exchange when > 0.
	Timeout time.Duration
	// MaxBytes caps total received bytes for this exchange when > 0.
	MaxBytes int
	// Metadata carries caller annotations echoed into Response.Request.
	Metadata map[string]any
}

// Status is the outcome classification of a Response.
type Status int

const (
	// StatusOK means at least one ECU->host message was received.
	StatusOK Status = iota
	// StatusTimeout means the exchange deadline elapsed with no answer.
	StatusTimeout
	// StatusEmpty means the exchange completed with zero ECU->host
	// messages (idle timeout with nothing received).
	StatusEmpty
	// StatusMalformed means the codec rejected received bytes.
	StatusMalformed
	// StatusUnsupported means the codec does not implement the request.
	StatusUnsupported
)

// String returns a human-readable status name.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusTimeout:
		return "timeout"
	case StatusEmpty:
		return "empty"
	case StatusMalformed:
		return "malformed"
	case StatusUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

// Response is the result of one Request exchange: every message observed
// on the wire, timing, and a typed status.
type Response struct {
	// Request echoes the request that produced this response.
	Request Request
	// Messages contains every message observed during the exchange —
	// both directions — in arrival order.
	Messages []Message
	// Answers contains only the ECU->host messages (a view into Messages).
	Answers []Message
	// Status classifies the outcome.
	Status Status
	// Error carries the typed failure when Status is not OK/Empty.
	Error error
	// Elapsed is the wall-clock duration of the exchange.
	Elapsed time.Duration
}

// NegativeResponseError reports a protocol-level negative response
// (e.g. an OBD negative response code or SSM NAK) decoded by a codec.
type NegativeResponseError struct {
	// Code is the raw negative-response code byte.
	Code byte
	// Detail is an optional human-readable annotation.
	Detail string
}

// Error implements error.
func (e *NegativeResponseError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("negative response 0x%02X: %s", e.Code, e.Detail)
	}
	return fmt.Sprintf("negative response 0x%02X", e.Code)
}

// Typed protocol errors. Wrapping with %w preserves classification.
var (
	// ErrTimeout means no complete answer arrived before the deadline.
	ErrTimeout = errors.New("protocol: exchange timed out")
	// ErrMalformed means received bytes failed codec validation.
	ErrMalformed = errors.New("protocol: malformed message")
	// ErrClosed means the underlying transport/session is closed.
	ErrClosed = errors.New("protocol: transport closed")
	// ErrUnsupported means the codec does not implement the request.
	ErrUnsupported = errors.New("protocol: unsupported request")
	// ErrNoTransport means the session has no transport bound.
	ErrNoTransport = errors.New("protocol: no transport bound")
)
