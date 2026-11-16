package obd2

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol/elm327"
)

// Client runs OBD-II queries over an ELM327-backed protocol session.
// It is the typed front-end to the raw codec: callers get Measurements,
// never raw hex.
type Client struct {
	s     *protocol.Session
	codec *elm327.Codec
	pst   time.Duration // per-query timeout override
}

// NewClient binds an OBD-II client to a session whose codec is elm327.
// The codec must be *elm327.Codec; other codecs are rejected at query
// time with ErrUnsupported.
func NewClient(s *protocol.Session) (*Client, error) {
	c, ok := s.Codec().(*elm327.Codec)
	if !ok {
		return nil, fmt.Errorf("%w: obd2 client requires elm327 codec, got %s",
			ErrUnsupportedMode, s.Codec().Name())
	}
	return &Client{s: s, codec: c}, nil
}

// Query performs one mode/PID request and decodes the response.
//
// The request is the documented read-only OBD-II form: mode + PID only.
// Write-capable modes (0x02 clear, 0x04/0x08 controls, 0x09 write) are
// not expressible through this API — they are absent from the registry
// and rejected as ErrUnsupportedMode before any bytes are sent.
func (c *Client) Query(ctx context.Context, mode, pid byte) (Measurement, error) {
	def, err := Lookup(mode, pid)
	if err != nil {
		return Measurement{}, err
	}
	req := elm327.Mode(mode, pid)
	if mode == 0x03 {
		// Mode 03 carries no PID on the wire; 0x00 is only a registry key.
		req = elm327.Mode(mode)
	}
	if c.pst > 0 {
		req.Timeout = c.pst
	}
	resp, err := c.s.Transact(ctx, req)
	if err != nil {
		return Measurement{}, err
	}
	// Find the first data answer; ELM error statuses map to typed errors.
	var payload []byte
	var lines []string
	for _, m := range resp.Answers {
		if e := elm327.ErrorOf(m); e != nil {
			return Measurement{}, mapElmError(e)
		}
		if elm327.IsData(m) && len(m.Payload) > 0 {
			payload = m.Payload
			if _, hdr := m.Metadata["canIDs"]; !hdr {
				lines, _ = m.Metadata["lines"].([]string)
			}
			break
		}
	}
	if payload == nil {
		return Measurement{}, ErrNoData
	}
	// Strip the mode/PID response header: byte0 = 0x40|mode, byte1 = pid.
	// Mode 03 has no PID byte, and every response line repeats the 0x43
	// header, so strip it per line before joining DTC pairs.
	data := payload
	if mode == 0x03 {
		if len(lines) == 0 {
			lines = []string{fmt.Sprintf("%X", payload)}
		}
		data, err = stripMode03Lines(lines)
		if err != nil {
			return Measurement{}, err
		}
	} else {
		if len(data) < 2 || data[0] != 0x40|mode || data[1] != pid {
			return Measurement{}, fmt.Errorf("%w: response header % x does not match mode 0x%02X PID 0x%02X",
				ErrMalformedData, data, mode, pid)
		}
		data = data[2:]
	}
	if def.extraSkip > 0 {
		if len(data) < def.extraSkip {
			return Measurement{}, fmt.Errorf("%w: mode 0x%02X PID 0x%02X missing %d count bytes",
				ErrShortPayload, mode, pid, def.extraSkip)
		}
		data = data[def.extraSkip:]
	}
	return Decode(mode, pid, data)
}

// stripMode03Lines validates and removes the 0x43 header from each ELM
// data line, concatenating the remaining DTC bytes.
func stripMode03Lines(lines []string) ([]byte, error) {
	var out []byte
	for _, ln := range lines {
		bs, err := hex.DecodeString(strings.Join(strings.Fields(ln), ""))
		if err != nil {
			return nil, fmt.Errorf("%w: mode03 line %q", ErrMalformedData, ln)
		}
		if len(bs) < 1 || bs[0] != 0x43 {
			return nil, fmt.Errorf("%w: unexpected mode03 header % x", ErrMalformedData, bs)
		}
		out = append(out, bs[1:]...)
	}
	return out, nil
}

// mapElmError converts an elm327 ErrorOf result into an obd2-typed error.
// Every ELM error status means the ECU did not return usable data.
func mapElmError(e error) error {
	var ne *protocol.NegativeResponseError
	if errors.As(e, &ne) {
		return fmt.Errorf("%w: %s", ErrNoData, ne.Detail)
	}
	return e
}

// Convenience typed queries for the PIDs the target needs.

// RPM returns engine speed in rpm.
func (c *Client) RPM(ctx context.Context) (Measurement, error) { return c.Query(ctx, 0x01, 0x0C) }

// CoolantTemp returns coolant temperature in degC.
func (c *Client) CoolantTemp(ctx context.Context) (Measurement, error) {
	return c.Query(ctx, 0x01, 0x05)
}

// VehicleSpeed returns speed in km/h.
func (c *Client) VehicleSpeed(ctx context.Context) (Measurement, error) {
	return c.Query(ctx, 0x01, 0x0D)
}

// ThrottlePosition returns throttle opening in percent.
func (c *Client) ThrottlePosition(ctx context.Context) (Measurement, error) {
	return c.Query(ctx, 0x01, 0x11)
}

// SupportedPIDs returns the mode-01 supported PID list.
func (c *Client) SupportedPIDs(ctx context.Context) (Measurement, error) {
	return c.Query(ctx, 0x01, 0x00)
}

// VIN returns the 17-character VIN (mode 09 PID 0x02).
func (c *Client) VIN(ctx context.Context) (Measurement, error) { return c.Query(ctx, 0x09, 0x02) }

// ECUName returns the ECU name string (mode 09 PID 0x0A) when supported.
func (c *Client) ECUName(ctx context.Context) (Measurement, error) { return c.Query(ctx, 0x09, 0x0A) }

// StoredDTCs returns confirmed DTCs (mode 03).
func (c *Client) StoredDTCs(ctx context.Context) (Measurement, error) {
	return c.Query(ctx, 0x03, 0x00)
}
