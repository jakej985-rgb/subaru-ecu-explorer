package protocol

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

// ByteTransport is the minimal transport the session needs: byte stream,
// deadlines, close. adapter.Transport satisfies it; fakes do too.
type ByteTransport interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	SetDeadline(t time.Time) error
	Close() error
}

var _ ByteTransport = adapter.Transport(nil)

// Default exchange bounds. They are intentionally conservative: protocol
// sessions are request/response, not streams.
const (
	DefaultResponseTimeout = 5 * time.Second
	DefaultIdleTimeout     = 150 * time.Millisecond
	DefaultMaxBytes        = 4096
)

// Options tunes Session exchange behavior.
type Options struct {
	// ResponseTimeout bounds the entire exchange (request write through
	// last answer). Zero uses DefaultResponseTimeout.
	ResponseTimeout time.Duration
	// IdleTimeout ends the receive loop this long after the most recent
	// byte, once at least one ECU->host message has been collected.
	// Zero uses DefaultIdleTimeout.
	IdleTimeout time.Duration
	// MaxBytes caps total bytes read per exchange. Zero uses
	// DefaultMaxBytes; the request's own MaxBytes overrides.
	MaxBytes int
	// Complete, when non-nil, ends the receive loop early once it returns
	// true for the messages collected so far. Default: first ECU->host
	// message completes the exchange.
	Complete func([]Message) bool
}

// Session runs request/response exchanges for one protocol Codec over one
// ByteTransport. It is transport- and hardware-agnostic: any adapter
// session or in-memory fake can back it.
type Session struct {
	codec Codec
	tr    ByteTransport
	opts  Options
}

// NewSession binds a codec to a transport.
func NewSession(codec Codec, tr ByteTransport, opts Options) *Session {
	if opts.ResponseTimeout <= 0 {
		opts.ResponseTimeout = DefaultResponseTimeout
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	return &Session{codec: codec, tr: tr, opts: opts}
}

// Codec returns the session's codec.
func (s *Session) Codec() Codec { return s.codec }

// Transact performs one request/response exchange.
//
// The request is marshaled and written, then messages are read and
// unmarshaled until the completion policy fires, the idle timeout elapses
// after the first answer, the response timeout expires, or the byte cap is
// hit. All observed messages — including noise and negative responses —
// are preserved in Response.Messages for evidence.
//
// Read-only note: Transact sends whatever the codec marshals. Callers are
// responsible for restricting requests to read-only services; the safety
// package's operation gate is the enforcement point above this layer.
func (s *Session) Transact(ctx context.Context, req Request) (Response, error) {
	if s.tr == nil {
		return Response{}, ErrNoTransport
	}
	if err := ValidateRequest(req); err != nil {
		return Response{}, err
	}
	start := time.Now()
	resp := Response{Request: req, Status: StatusEmpty}

	wire, err := s.codec.Marshal(req)
	if err != nil {
		resp.Status = StatusUnsupported
		resp.Error = err
		return resp, err
	}

	responseTimeout := s.opts.ResponseTimeout
	if req.Timeout > 0 && req.Timeout < responseTimeout {
		responseTimeout = req.Timeout
	}
	maxBytes := s.opts.MaxBytes
	if req.MaxBytes > 0 && req.MaxBytes < maxBytes {
		maxBytes = req.MaxBytes
	}

	deadline := start.Add(responseTimeout)
	if err := s.tr.SetDeadline(deadline); err != nil {
		return resp, fmt.Errorf("protocol: set deadline: %w", err)
	}
	if _, err := s.tr.Write(wire); err != nil {
		resp.Status = StatusMalformed
		resp.Error = err
		resp.Elapsed = time.Since(start)
		return resp, fmt.Errorf("protocol: write request: %w", err)
	}
	resp.Messages = append(resp.Messages, Message{
		Direction: DirectionHostToEcu,
		Timestamp: start,
		Raw:       wire,
		Payload:   append([]byte(nil), req.Payload...),
		Address:   req.Address,
	})

	buf := make([]byte, 0, 256)
	tmp := make([]byte, 512)
	var lastByteAt time.Time
	idle := s.opts.IdleTimeout

	for {
		if err := ctx.Err(); err != nil {
			resp.Elapsed = time.Since(start)
			return resp, err
		}
		// Drain complete messages from the buffer first.
		progress := false
		for {
			msg, consumed, err := s.codec.Unmarshal(buf)
			if errors.Is(err, ErrIncomplete) {
				break
			}
			if err != nil {
				resp.Status = StatusMalformed
				resp.Error = err
				resp.Elapsed = time.Since(start)
				return resp, err
			}
			if msg.Direction == 0 {
				msg.Direction = DirectionEcuToHost
			}
			if msg.Timestamp.IsZero() {
				msg.Timestamp = time.Now()
			}
			resp.Messages = append(resp.Messages, msg)
			resp.Answers = append(resp.Answers, msg)
			buf = buf[consumed:]
			progress = true
			lastByteAt = time.Now()
			if resp.Status == StatusEmpty {
				resp.Status = StatusOK
			}
			if s.opts.Complete != nil && s.opts.Complete(resp.Answers) {
				resp.Elapsed = time.Since(start)
				return resp, nil
			}
		}
		if len(resp.Answers) > 0 && time.Since(lastByteAt) >= idle {
			resp.Elapsed = time.Since(start)
			return resp, nil
		}
		if len(buf) >= maxBytes {
			resp.Status = StatusMalformed
			resp.Error = fmt.Errorf("%w: received byte cap %d exceeded", ErrMalformed, maxBytes)
			resp.Elapsed = time.Since(start)
			return resp, resp.Error
		}
		if time.Now().After(deadline) {
			resp.Status = StatusTimeout
			resp.Error = ErrTimeout
			resp.Elapsed = time.Since(start)
			return resp, ErrTimeout
		}
		// Idle deadline: once we have an answer, a stalled read should not
		// block until the full response timeout. The context deadline, when
		// earlier, clamps the read so cancellation is honored promptly.
		readDeadline := deadline
		if len(resp.Answers) > 0 {
			if t := lastByteAt.Add(idle); t.Before(readDeadline) {
				readDeadline = t
			}
		}
		if dl, ok := ctx.Deadline(); ok && dl.Before(readDeadline) {
			readDeadline = dl
		}
		_ = s.tr.SetDeadline(readDeadline)
		n, err := s.tr.Read(tmp)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				resp.Elapsed = time.Since(start)
				return resp, cerr
			}
			if errors.Is(err, adapter.ErrTimeout) || isTimeout(err) {
				if len(resp.Answers) > 0 {
					resp.Elapsed = time.Since(start)
					return resp, nil
				}
				resp.Status = StatusTimeout
				resp.Error = ErrTimeout
				resp.Elapsed = time.Since(start)
				return resp, ErrTimeout
			}
			resp.Status = StatusMalformed
			resp.Error = err
			resp.Elapsed = time.Since(start)
			return resp, err
		}
		if n > 0 {
			if len(buf)+n > maxBytes {
				n = maxBytes - len(buf)
			}
			buf = append(buf, tmp[:n]...)
			progress = true
		}
		_ = progress
	}
}

// isTimeout reports whether err is a deadline-style timeout from any
// transport implementation.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	var te interface{ Timeout() bool }
	if errors.As(err, &te) && te.Timeout() {
		return true
	}
	return false
}
