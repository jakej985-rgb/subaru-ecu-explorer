package protocol

import (
	"errors"
	"fmt"
)

// ErrIncomplete signals that buf does not yet contain a complete message.
// It is an internal codec signal, not a protocol failure: readers should
// fetch more bytes and retry. It is never surfaced to callers.
var ErrIncomplete = errors.New("protocol: incomplete message")

// Codec translates between Requests and wire bytes for one diagnostic
// protocol. Implementations must be adapter-independent and safe to test
// against in-memory transports.
//
// Codec implementations must not perform I/O themselves; Session owns the
// transport and feeds bytes to Unmarshal.
type Codec interface {
	// Name identifies the protocol (e.g. "elm327-obd2", "subaru-ssm").
	Name() string

	// Marshal encodes req to wire bytes ready for the transport.
	// It returns ErrUnsupported for requests the codec cannot express.
	Marshal(req Request) ([]byte, error)

	// Unmarshal extracts the next complete message from buf.
	//
	// Returns (msg, consumed, nil) on success; consumed is the number of
	// buf bytes belonging to the message. Returns ErrIncomplete when buf
	// does not yet hold a full message. Returns a wrapped ErrMalformed for
	// bytes that can never form a valid message; the caller decides whether
	// to resynchronize.
	Unmarshal(buf []byte) (Message, int, error)
}

// FrameCodec is an optional Codec extension for stream protocols that
// resynchronize on garbage: Resync returns the offset of the next
// plausible message start in buf, or -1 if none exists yet.
type FrameCodec interface {
	Codec
	// Resync scans buf for the next plausible message boundary.
	// Returning -1 means keep buffering.
	Resync(buf []byte) int
}

// CodecFunc adapts plain functions to a Codec (useful for tests).
type CodecFunc struct {
	NameFunc    func() string
	MarshalFunc func(Request) ([]byte, error)
	UnmarshalFn func([]byte) (Message, int, error)
}

// Name implements Codec.
func (c CodecFunc) Name() string { return c.NameFunc() }

// Marshal implements Codec.
func (c CodecFunc) Marshal(req Request) ([]byte, error) { return c.MarshalFunc(req) }

// Unmarshal implements Codec.
func (c CodecFunc) Unmarshal(buf []byte) (Message, int, error) {
	return c.UnmarshalFn(buf)
}

// ValidateRequest performs codec-agnostic request sanity checks shared by
// all protocol implementations. It does not restrict addressing: non-zero
// addresses are allowed at this layer — safety gating is the caller's job.
func ValidateRequest(req Request) error {
	if req.Timeout < 0 || req.MaxBytes < 0 {
		return fmt.Errorf("%w: negative timeout/maxbytes", ErrMalformed)
	}
	return nil
}
