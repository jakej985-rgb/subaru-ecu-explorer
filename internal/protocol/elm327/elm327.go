// Package elm327 implements ELM327 command serialization and response
// parsing as a protocol.Codec. It handles adapter initialization, AT
// commands, OBD mode requests, echo/header/space modes, adapter variants,
// and ELM error statuses — all adapter-independent and fixture-tested.
//
// The codec speaks the ELM327 text protocol: requests marshal to
// "AT...\r" or hex mode strings + "\r"; responses are ">"-terminated
// blocks parsed into protocol.Message values with raw text preserved.
package elm327

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol"
)

// Service codes for Request.Service. OBD modes use their real mode
// number; adapter commands use the SvcAT range.
const (
	// SvcAT marks an adapter (AT) command; the command text is Payload.
	SvcAT = 0xE0
	// SvcHeaders requests the current protocol header bytes (ATDP/ATH).
	SvcHeaders = 0xE1
)

// Kind classifies a parsed ELM327 response block.
type Kind int

const (
	// KindData is a positive response carrying decoded data bytes.
	KindData Kind = iota
	// KindOK is an AT-command acknowledgement ("OK").
	KindOK
	// KindError is an ELM-reported error status.
	KindError
	// KindBanner is adapter identification text (e.g. after ATZ/ATI).
	KindBanner
	// KindPrompt is an empty prompt-only response.
	KindPrompt
)

// ELM327 terminal status strings observed on the wire, mapped to codes.
// BUS INIT variants are handled separately (informational, not errors).
var statusCodes = map[string]byte{
	"UNABLE TO CONNECT": 0x01,
	"CAN ERROR":         0x02,
	"BUS ERROR":         0x03,
	"BUS INIT: ERROR":   0x06,
	"STOPPED":           0x07,
	"NO DATA":           0x08,
	"ERROR":             0x09,
	"?":                 0x0A,
}

// Codec is an ELM327 protocol.Codec. It is stateful only for echo
// suppression: it remembers the last command it marshaled so echoed
// command lines can be recognized when ATE1 is in effect. Headers, when
// set, selects header-mode line parsing (ATH1); production init uses
// ATH0 so headers stay off.
type Codec struct {
	lastCmd string
	// Headers selects header-mode parsing: each data line is
	// "<can-id> <byte-count> <data...>". Leave false for ATH0 output.
	Headers bool
}

// New returns an ELM327 codec.
func New() *Codec { return &Codec{} }

// Name implements protocol.Codec.
func (c *Codec) Name() string { return "elm327-obd2" }

// AT builds an adapter-command request (e.g. AT("Z") -> "ATZ\r").
func AT(cmd string) protocol.Request {
	return protocol.Request{Service: SvcAT, Payload: []byte(strings.ToUpper(strings.TrimSpace(cmd)))}
}

// Mode builds an OBD mode request: mode 0x01 PID 0x0C -> "010C\r".
func Mode(mode byte, params ...byte) protocol.Request {
	p := append([]byte{mode}, params...)
	return protocol.Request{Service: mode, Payload: p}
}

// Marshal implements protocol.Codec: AT commands become "AT<cmd>\r";
// mode requests become uppercase hex + "\r".
func (c *Codec) Marshal(req protocol.Request) ([]byte, error) {
	var s string
	switch {
	case req.Service == SvcAT:
		if len(req.Payload) == 0 {
			return nil, fmt.Errorf("%w: empty AT command", protocol.ErrMalformed)
		}
		s = "AT" + strings.ToUpper(string(req.Payload))
	case req.Service == SvcHeaders:
		// ATSH <header> — headers are set via AT commands in practice;
		// this service exists for header-byte requests only.
		return nil, fmt.Errorf("%w: SvcHeaders is not a request service", protocol.ErrUnsupported)
	default:
		// OBD mode request: Service is the mode, Payload[0] must equal it
		// for Mode()-built requests; bare params are also accepted.
		body := req.Payload
		if len(body) == 0 || body[0] != req.Service {
			body = append([]byte{req.Service}, body...)
		}
		var b strings.Builder
		for _, by := range body {
			fmt.Fprintf(&b, "%02X", by)
		}
		s = b.String()
	}
	c.lastCmd = s
	return []byte(s + "\r"), nil
}

// Unmarshal implements protocol.Codec: finds the next ">"-terminated
// block and parses it. Returns ErrIncomplete until a prompt arrives.
func (c *Codec) Unmarshal(buf []byte) (protocol.Message, int, error) {
	idx := -1
	for i, by := range buf {
		if by == '>' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return protocol.Message{}, 0, protocol.ErrIncomplete
	}
	block := string(buf[:idx+1])
	return parseBlock(block, c.lastCmd, c.Headers), idx + 1, nil
}

// parseBlock converts one ">"-terminated ELM327 response into a Message.
// Echoed command lines are dropped when they match the last marshaled
// command; status keywords classify the response; remaining hex lines are
// decoded into Payload.
func parseBlock(block, lastCmd string, hdrMode bool) protocol.Message {
	msg := protocol.Message{
		Direction: protocol.DirectionEcuToHost,
		Raw:       []byte(block),
		Metadata:  map[string]any{},
	}
	// Normalize: the prompt terminator and trailing whitespace are framing.
	inner := strings.TrimSuffix(block, ">")
	inner = strings.TrimRight(inner, "\r\n ")
	if inner == "" {
		msg.Metadata["kind"] = "prompt"
		msg.Payload = []byte{}
		return msg
	}

	lines := strings.FieldsFunc(inner, func(r rune) bool { return r == '\r' || r == '\n' })
	var content []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		content = append(content, ln)
	}
	if len(content) == 0 {
		msg.Metadata["kind"] = "prompt"
		return msg
	}

	// Drop the echoed command line (echo mode ATE1).
	if lastCmd != "" && len(content) > 0 &&
		strings.EqualFold(strings.TrimSpace(content[0]), lastCmd) {
		content = content[1:]
	}
	if len(content) == 0 {
		msg.Metadata["kind"] = "echo-only"
		return msg
	}

	// Terminal error status: an exact error keyword as the last line of
	// the whole block (checked before BUS INIT filtering so that
	// "BUS INIT: ERROR" still classifies as an error).
	if last := strings.ToUpper(content[len(content)-1]); true {
		if code, ok := statusCodes[last]; ok {
			msg.Metadata["kind"] = "error"
			msg.Metadata["status"] = last
			msg.Metadata["statusCode"] = code
			msg.Payload = []byte(last)
			return msg
		}
	}

	// Separate in-band status lines (SEARCHING..., BUS INIT...) from
	// content lines. ELM prints status before or after data.
	var statusLines []string
	var body []string
	for _, ln := range content {
		u := strings.ToUpper(ln)
		if u == "SEARCHING..." || strings.HasPrefix(u, "BUS INIT") {
			statusLines = append(statusLines, u)
			continue
		}
		body = append(body, ln)
	}
	if len(statusLines) > 0 {
		msg.Metadata["statusLines"] = statusLines
	}
	// BUS INIT with no error keyword is informational.
	if len(body) == 0 && len(statusLines) > 0 {
		msg.Metadata["kind"] = "status"
		msg.Payload = []byte(strings.Join(statusLines, " | "))
		return msg
	}

	// "OK" acknowledgement.
	if len(body) == 1 && strings.EqualFold(body[0], "OK") {
		msg.Metadata["kind"] = "ok"
		msg.Payload = []byte{}
		return msg
	}

	// Data response: decode hex lines (header-mode aware), honoring
	// "..." ISO-TP continuation markers.
	var data []byte
	var kept []string
	var headers []string
	for _, ln := range body {
		if ln == "..." {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			continue
		}
		line := ln
		if hdrMode {
			// "<can-id> <count> <data...>": strip the two header fields
			// after validating the count matches the data that follows.
			if len(fields) < 3 {
				msg.Metadata["kind"] = "malformed"
				msg.Metadata["badLine"] = ln
				msg.Payload = []byte(strings.Join(body, " "))
				return msg
			}
			n, err := strconv.ParseUint(fields[1], 16, 8)
			if err != nil || int(n) != len(fields)-2 {
				msg.Metadata["kind"] = "malformed"
				msg.Metadata["badLine"] = ln
				msg.Payload = []byte(strings.Join(body, " "))
				return msg
			}
			headers = append(headers, fields[0])
			line = strings.Join(fields[2:], " ")
		}
		// Distinguish attempted-hex data lines from banner text: a line
		// whose fields are all 2 chars is a malformed data attempt; any
		// other shape is adapter banner/text.
		allPairs := true
		for _, f := range fields {
			if len(f) != 2 {
				allPairs = false
				break
			}
		}
		if !looksHexLine(line) {
			if allPairs {
				msg.Metadata["kind"] = "malformed"
				msg.Metadata["badLine"] = ln
				msg.Payload = []byte(strings.Join(body, " "))
				return msg
			}
			msg.Metadata["kind"] = "banner"
			msg.Metadata["text"] = strings.Join(body, " | ")
			msg.Payload = []byte(msg.Metadata["text"].(string))
			return msg
		}
		kept = append(kept, line)
		bs, err := decodeHexLine(line)
		if err != nil {
			msg.Metadata["kind"] = "malformed"
			msg.Metadata["badLine"] = ln
			msg.Payload = []byte(strings.Join(body, " "))
			return msg
		}
		data = append(data, bs...)
	}
	if len(data) == 0 {
		msg.Metadata["kind"] = "banner"
		msg.Metadata["text"] = strings.Join(body, " | ")
		msg.Payload = []byte(msg.Metadata["text"].(string))
		return msg
	}
	msg.Metadata["kind"] = "data"
	msg.Metadata["lines"] = kept
	if len(headers) > 0 {
		msg.Metadata["canIDs"] = headers
	}
	msg.Payload = data
	return msg
}

// looksHexLine reports whether a line consists solely of hex byte pairs
// (spaces allowed between pairs).
func looksHexLine(s string) bool {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if len(f) != 2 {
			return false
		}
		for _, r := range f {
			if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F' || r >= 'a' && r <= 'f') {
				return false
			}
		}
	}
	return true
}

// decodeHexLine decodes a line of space-separated hex byte pairs.
func decodeHexLine(s string) ([]byte, error) {
	fields := strings.Fields(s)
	out := make([]byte, 0, len(fields))
	for _, f := range fields {
		if len(f) != 2 {
			return nil, fmt.Errorf("bad hex field %q", f)
		}
		var v byte
		if _, err := fmt.Sscanf(f, "%02x", &v); err != nil {
			return nil, fmt.Errorf("bad hex byte %q", f)
		}
		out = append(out, v)
	}
	return out, nil
}

// KindOf extracts the classification metadata from a parsed message.
func KindOf(m protocol.Message) string {
	if k, ok := m.Metadata["kind"].(string); ok {
		return k
	}
	return ""
}

// ErrorOf returns a typed error for error/malformed responses, or nil for
// ok/data/banner/prompt responses.
func ErrorOf(m protocol.Message) error {
	switch KindOf(m) {
	case "error":
		code, _ := m.Metadata["statusCode"].(byte)
		status, _ := m.Metadata["status"].(string)
		return &protocol.NegativeResponseError{Code: code, Detail: status}
	case "malformed":
		return fmt.Errorf("%w: %v", protocol.ErrMalformed, m.Metadata["badLine"])
	}
	return nil
}

// IsData reports whether the message carries decoded OBD data bytes.
func IsData(m protocol.Message) bool { return KindOf(m) == "data" }
