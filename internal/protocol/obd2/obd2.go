// Package obd2 implements documented generic OBD-II (SAE J1979) services
// and PIDs needed by the target vehicle: typed decoding definitions,
// units, scaling, and explicit unsupported-PID handling. It consumes
// decoded data bytes produced by lower layers (e.g. elm327) and never
// fabricates values: unknown PIDs, short payloads, and ECU "no data"
// responses are explicit errors.
package obd2

import (
	"errors"
	"fmt"
)

// Typed errors for OBD-II layer outcomes.
var (
	// ErrUnsupportedMode means the service mode is not implemented.
	ErrUnsupportedMode = errors.New("obd2: unsupported mode")
	// ErrUnsupportedPID means the PID is not in the supported registry.
	ErrUnsupportedPID = errors.New("obd2: unsupported PID")
	// ErrShortPayload means the ECU returned fewer bytes than the PID
	// definition requires.
	ErrShortPayload = errors.New("obd2: short payload")
	// ErrNoData means the ECU/adapter reported no data for the request
	// (ELM "NO DATA"/"STOPPED" or equivalent).
	ErrNoData = errors.New("obd2: no data")
	// ErrMalformedData means the payload failed decode validation.
	ErrMalformedData = errors.New("obd2: malformed data")
)

// Measurement is one decoded OBD-II value with provenance metadata.
type Measurement struct {
	// Mode is the service mode (e.g. 0x01 current data).
	Mode byte
	// PID is the parameter ID within the mode.
	PID byte
	// Name is the human-readable parameter name.
	Name string
	// Raw is the exact response payload bytes (evidence).
	Raw []byte
	// Value is the decoded, scaled value: float64 for numeric PIDs,
	// string for ASCII PIDs, []Flag for bitfield PIDs, []DTC for mode 03.
	Value any
	// Unit is the engineering unit ("", "km/h", "rpm", "degC", ...).
	Unit string
	// Bytes is the expected payload length for fixed-size PIDs.
	Bytes int
}

// Flag is one decoded bit in a bitfield PID.
type Flag struct {
	// Bit is the bit position within the payload (MSB-first per J1979).
	Bit int
	// Name describes the flag's meaning.
	Name string
	// Set reports whether the bit was set.
	Set bool
}

// DTC is a decoded diagnostic trouble code.
type DTC struct {
	// Code is the human-readable code, e.g. "P0300".
	Code string
	// Raw is the two-byte DTC as received.
	Raw [2]byte
	// Type classifies the code system (powertrain/chassis/body/network).
	Type string
	// Pending indicates a pending (non-confirmed) fault.
	Pending bool
	// Confirmed indicates a confirmed fault.
	Confirmed bool
	// TestFailed mirrors the J1979 test-failed bit.
	TestFailed bool
	// Permanent is set for permanent codes (mode 0A only; always false here).
	Permanent bool
}

// pidDef describes one supported PID.
type pidDef struct {
	mode  byte
	pid   byte
	name  string
	bytes int // expected payload length; 0 = variable
	unit  string
	// decode converts payload to Value; receives the full payload.
	decode func([]byte) (any, error)
	// flags, when non-nil, decodes payload bits into []Flag.
	flags map[int]string
	// extraSkip is the number of bytes between the mode/PID response
	// header and the actual data. Mode 09 responses carry a one-byte
	// item count before the data payload.
	extraSkip int
}

func u8(b []byte) byte { return b[0] }

// bitFlags decodes payload bits MSB-first into []Flag using names.
func bitFlags(payload []byte, names map[int]string) ([]Flag, error) {
	out := make([]Flag, 0, len(names))
	for bit, name := range names {
		byteIdx := bit / 8
		bitIdx := 7 - (bit % 8) // J1979: bit 7 of byte 0 is "bit 1"
		if byteIdx >= len(payload) {
			return nil, fmt.Errorf("%w: bit %d beyond payload", ErrShortPayload, bit)
		}
		out = append(out, Flag{Bit: bit, Name: name, Set: payload[byteIdx]&(1<<bitIdx) != 0})
	}
	return out, nil
}

// stft decodes a fuel-trim byte: (A - 128) * 100 / 128 percent.
func stft(b []byte) (any, error) {
	return float64(int(b[0])-128) * 100.0 / 128.0, nil
}

// rpm2 decodes a two-byte RPM value: (A*256+B)/4.
func rpm2(b []byte) (any, error) {
	return float64(int(b[0])<<8|int(b[1])) / 4.0, nil
}

// u16raw decodes a two-byte unsigned value.
func u16raw(b []byte) (any, error) {
	return float64(int(b[0])<<8 | int(b[1])), nil
}

// asciiDecode decodes an ASCII payload, trimming trailing NULs/spaces.
func asciiDecode(b []byte) (any, error) {
	end := len(b)
	for end > 0 && (b[end-1] == 0 || b[end-1] == ' ') {
		end--
	}
	for i := 0; i < end; i++ {
		if b[i] < 0x20 || b[i] > 0x7E {
			return nil, fmt.Errorf("%w: non-ASCII byte 0x%02X", ErrMalformedData, b[i])
		}
	}
	return string(b[:end]), nil
}

// pid01Supported is the mode 01 PID 0x00 supported-PIDs bitmask:
// four payload bytes, bit N (MSB-first) means PID N supported.
func supportedPIDs(payload []byte, base byte) ([]byte, error) {
	var out []byte
	for i := 0; i < 32; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if byteIdx < len(payload) && payload[byteIdx]&(1<<bitIdx) != 0 {
			out = append(out, base+byte(i))
		}
	}
	return out, nil
}

// supportedPIDs20 handles the extended 0x20-0x3F range reported by
// PID 0x20 (not implemented; registry is honest about coverage).

// registry holds the documented generic PIDs implemented for the target.
// Only standard SAE J1979 mode 01 PIDs relevant to a 1999 EJ25 plus
// mode 09 identification and mode 03 stored DTCs are defined.
var registry = map[[2]byte]pidDef{
	// ---- Mode 01: current powertrain data ----
	{0x01, 0x00}: {mode: 0x01, pid: 0x00, name: "supported-pids-01-20", bytes: 4, unit: "",
		decode: func(b []byte) (any, error) { return supportedPIDs(b, 0x01) }},
	{0x01, 0x04}: {mode: 0x01, pid: 0x04, name: "calculated-load", bytes: 1, unit: "%",
		decode: func(b []byte) (any, error) { return float64(u8(b)) * 100.0 / 255.0, nil }},
	{0x01, 0x05}: {mode: 0x01, pid: 0x05, name: "coolant-temp", bytes: 1, unit: "degC",
		decode: func(b []byte) (any, error) { return float64(int(u8(b)) - 40), nil }},
	{0x01, 0x06}: {mode: 0x01, pid: 0x06, name: "short-term-fuel-trim-bank1", bytes: 1, unit: "%",
		decode: stft},
	{0x01, 0x07}: {mode: 0x01, pid: 0x07, name: "long-term-fuel-trim-bank1", bytes: 1, unit: "%",
		decode: stft},
	{0x01, 0x0B}: {mode: 0x01, pid: 0x0B, name: "intake-manifold-absolute-pressure", bytes: 1, unit: "kPa",
		decode: func(b []byte) (any, error) { return float64(u8(b)), nil }},
	{0x01, 0x0C}: {mode: 0x01, pid: 0x0C, name: "engine-rpm", bytes: 2, unit: "rpm",
		decode: rpm2},
	{0x01, 0x0D}: {mode: 0x01, pid: 0x0D, name: "vehicle-speed", bytes: 1, unit: "km/h",
		decode: func(b []byte) (any, error) { return float64(u8(b)), nil }},
	{0x01, 0x0F}: {mode: 0x01, pid: 0x0F, name: "intake-air-temp", bytes: 1, unit: "degC",
		decode: func(b []byte) (any, error) { return float64(int(u8(b)) - 40), nil }},
	{0x01, 0x10}: {mode: 0x01, pid: 0x10, name: "mass-air-flow-rate", bytes: 2, unit: "g/s",
		decode: func(b []byte) (any, error) {
			return float64(int(b[0])<<8|int(b[1])) / 100.0, nil
		}},
	{0x01, 0x11}: {mode: 0x01, pid: 0x11, name: "throttle-position", bytes: 1, unit: "%",
		decode: func(b []byte) (any, error) { return float64(u8(b)) * 100.0 / 255.0, nil }},
	{0x01, 0x1F}: {mode: 0x01, pid: 0x1F, name: "run-time-since-engine-start", bytes: 2, unit: "s",
		decode: u16raw},
	{0x01, 0x21}: {mode: 0x01, pid: 0x21, name: "distance-traveled-with-mil-on", bytes: 2, unit: "km",
		decode: u16raw},
	{0x01, 0x2F}: {mode: 0x01, pid: 0x2F, name: "fuel-tank-level-input", bytes: 1, unit: "%",
		decode: func(b []byte) (any, error) { return float64(u8(b)) * 100.0 / 255.0, nil }},
	{0x01, 0x31}: {mode: 0x01, pid: 0x31, name: "distance-since-codes-cleared", bytes: 2, unit: "km",
		decode: u16raw},

	// ---- Mode 03: stored (confirmed) DTCs ----
	// Payload: repeating 2-byte DTCs. Mode 03 requests carry no PID.
	{0x03, 0x00}: {mode: 0x03, pid: 0x00, name: "stored-dtcs", bytes: 0, unit: "",
		decode: decodeDTCs},

	// ---- Mode 09: vehicle information ----
	{0x09, 0x02}: {mode: 0x09, pid: 0x02, name: "vin", bytes: 0, unit: "", extraSkip: 1,
		decode: asciiDecode},
	{0x09, 0x04}: {mode: 0x09, pid: 0x04, name: "calibration-id", bytes: 0, unit: "", extraSkip: 1,
		decode: asciiDecode},
	{0x09, 0x0A}: {mode: 0x09, pid: 0x0A, name: "ecu-name", bytes: 0, unit: "", extraSkip: 1,
		decode: asciiDecode},
}

// supportedModes lists modes the registry can decode at all.
var supportedModes = map[byte]bool{0x01: true, 0x03: true, 0x09: true}

// decodeDTCs converts mode 03 payload pairs into []DTC.
func decodeDTCs(b []byte) (any, error) {
	if len(b) == 0 {
		return []DTC{}, nil
	}
	if len(b)%2 != 0 {
		return nil, fmt.Errorf("%w: odd DTC payload %d bytes", ErrMalformedData, len(b))
	}
	out := make([]DTC, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		hi, lo := b[i], b[i+1]
		if hi == 0 && lo == 0 {
			continue // padding
		}
		// J1979 DTC: two bytes -> system letter + 4 hex digits.
		// byte1 bits 7-6 = type, bits 5-4 = second char class...
		typeBits := hi >> 6
		sysLetter := string("PCBU"[typeBits])
		// Second character encodes failure type.
		failType := (hi >> 4) & 0x03
		second := string("0123"[failType])
		code := fmt.Sprintf("%s%s%X%02X", sysLetter, second, hi&0x0F, lo)
		// Mode 03 = confirmed DTCs: confirmed bit set by definition.
		out = append(out, DTC{
			Code:      code,
			Raw:       [2]byte{hi, lo},
			Type:      dtcSystemName(typeBits),
			Confirmed: true,
			// Mode 03 carries no per-DTC status; TestFailed stays false.
		})
	}
	return out, nil
}

func dtcSystemName(t byte) string {
	switch t {
	case 0:
		return "powertrain"
	case 1:
		return "chassis"
	case 2:
		return "body"
	case 3:
		return "network"
	}
	return "unknown"
}

// Lookup returns the pidDef for a mode/PID, or an error.
func Lookup(mode, pid byte) (pidDef, error) {
	if !supportedModes[mode] {
		return pidDef{}, fmt.Errorf("%w: mode 0x%02X", ErrUnsupportedMode, mode)
	}
	def, ok := registry[[2]byte{mode, pid}]
	if !ok {
		return pidDef{}, fmt.Errorf("%w: mode 0x%02X PID 0x%02X", ErrUnsupportedPID, mode, pid)
	}
	return def, nil
}

// Supported reports whether a mode/PID is in the registry.
func Supported(mode, pid byte) bool {
	_, err := Lookup(mode, pid)
	return err == nil
}

// Decode validates and decodes a response payload for mode/PID into a
// Measurement. It never fabricates: short payloads and decode failures
// are errors.
func Decode(mode, pid byte, payload []byte) (Measurement, error) {
	def, err := Lookup(mode, pid)
	if err != nil {
		return Measurement{}, err
	}
	if def.bytes > 0 && len(payload) < def.bytes {
		return Measurement{}, fmt.Errorf("%w: mode 0x%02X PID 0x%02X got %d bytes, want %d",
			ErrShortPayload, mode, pid, len(payload), def.bytes)
	}
	val, err := def.decode(payload)
	if err != nil {
		return Measurement{}, err
	}
	return Measurement{
		Mode: mode, PID: pid, Name: def.name,
		Raw:   append([]byte(nil), payload...),
		Value: val, Unit: def.unit, Bytes: def.bytes,
	}, nil
}

// PIDs lists all registered PIDs for a mode (for capability reporting).
func PIDs(mode byte) []pidDef {
	var out []pidDef
	for _, d := range registry {
		if d.mode == mode {
			out = append(out, d)
		}
	}
	return out
}
