// Package arduino implements the versioned USB serial frame transport for the
// Arduino Mega acquisition interface. It carries bounded acquisition messages
// only; vehicle-specific decoding stays in the Go service above this layer.
package arduino

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Frame layout (all multi-byte fields big-endian):
//
//	offset  size  field
//	0       2     magic 0xA5 0x5A
//	2       1     protocol version (currently 1)
//	3       1     frame type
//	4       2     sequence number
//	6       2     payload length
//	8       N     payload
//	8+N     2     CRC-16/CCITT over bytes 0..8+N-1
const (
	FrameVersion   = 1
	headerSize     = 8
	crcSize        = 2
	maxPayloadSize = 512
)

var magic = [2]byte{0xA5, 0x5A}

// FrameType identifies the payload semantics.
type FrameType byte

const (
	TypeHello      FrameType = 0x01 // firmware hello / capability negotiation
	TypeData       FrameType = 0x02 // acquisition sample batch
	TypeAck        FrameType = 0x03 // acknowledgement
	TypeError      FrameType = 0x04 // firmware-reported error
	TypeCapability FrameType = 0x05 // capability report
	TypeHeartbeat  FrameType = 0x06 // periodic liveness tick
	TypeStatus     FrameType = 0x07 // state transition report
)

// Frame is one transport unit.
type Frame struct {
	Type    FrameType
	Seq     uint16
	Payload []byte
}

// Errors returned by the codec.
var (
	ErrShortBuffer   = errors.New("arduino: frame shorter than header")
	ErrBadMagic      = errors.New("arduino: bad frame magic")
	ErrBadVersion    = errors.New("arduino: unsupported protocol version")
	ErrPayloadTooBig = errors.New("arduino: payload exceeds maximum")
	ErrBadCRC        = errors.New("arduino: CRC mismatch")
	ErrTruncated     = errors.New("arduino: incomplete frame")
	ErrInvalidType   = errors.New("arduino: invalid frame type")
	ErrHelloField    = errors.New("arduino: hello string field too long")
)

// maxHelloString bounds each Hello string field.
const maxHelloString = 32

func validType(t FrameType) bool {
	switch t {
	case TypeHello, TypeData, TypeAck, TypeError, TypeCapability, TypeHeartbeat, TypeStatus:
		return true
	}
	return false
}

// Encode serializes a frame. It returns an error for invalid types or
// oversized payloads.
func Encode(f Frame) ([]byte, error) {
	if !validType(f.Type) {
		return nil, fmt.Errorf("%w: 0x%02x", ErrInvalidType, byte(f.Type))
	}
	if len(f.Payload) > maxPayloadSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrPayloadTooBig, len(f.Payload), maxPayloadSize)
	}
	buf := make([]byte, headerSize+len(f.Payload)+crcSize)
	copy(buf[0:2], magic[:])
	buf[2] = FrameVersion
	buf[3] = byte(f.Type)
	binary.BigEndian.PutUint16(buf[4:6], f.Seq)
	binary.BigEndian.PutUint16(buf[6:8], uint16(len(f.Payload)))
	copy(buf[8:], f.Payload)
	crc := crc16(buf[:headerSize+len(f.Payload)])
	binary.BigEndian.PutUint16(buf[headerSize+len(f.Payload):], crc)
	return buf, nil
}

// DecodeFrame parses one frame from the front of buf. It returns the frame,
// the number of bytes consumed, and an error. ErrTruncated means more bytes
// are needed; other errors mean the stream is corrupt and should be resynced.
func DecodeFrame(buf []byte) (Frame, int, error) {
	if len(buf) < headerSize+crcSize {
		return Frame{}, 0, ErrTruncated
	}
	if buf[0] != magic[0] || buf[1] != magic[1] {
		return Frame{}, 0, ErrBadMagic
	}
	if buf[2] != FrameVersion {
		return Frame{}, 0, fmt.Errorf("%w: %d", ErrBadVersion, buf[2])
	}
	payloadLen := int(binary.BigEndian.Uint16(buf[6:8]))
	if payloadLen > maxPayloadSize {
		return Frame{}, 0, fmt.Errorf("%w: %d", ErrPayloadTooBig, payloadLen)
	}
	total := headerSize + payloadLen + crcSize
	if len(buf) < total {
		return Frame{}, 0, ErrTruncated
	}
	body := buf[:headerSize+payloadLen]
	want := binary.BigEndian.Uint16(buf[headerSize+payloadLen : total])
	if crc16(body) != want {
		return Frame{}, 0, ErrBadCRC
	}
	if !validType(FrameType(buf[3])) {
		return Frame{}, 0, fmt.Errorf("%w: 0x%02x", ErrInvalidType, buf[3])
	}
	payload := make([]byte, payloadLen)
	copy(payload, buf[headerSize:headerSize+payloadLen])
	return Frame{
		Type:    FrameType(buf[3]),
		Seq:     binary.BigEndian.Uint16(buf[4:6]),
		Payload: payload,
	}, total, nil
}

// crc16 implements CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF).
func crc16(b []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, c := range b {
		crc ^= uint16(c) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// Capability bits advertised by firmware in hello/capability messages.
// These describe acquisition features only — never vehicle write/actuator
// functions, which are out of scope for this project.
const (
	CapPeriodicSamples uint32 = 1 << 0 // firmware can stream timed samples
	CapEventSamples    uint32 = 1 << 1 // firmware can emit event-triggered samples
	CapSelfTest        uint32 = 1 << 2 // firmware supports a read-only self-test
)

// Hello is the firmware handshake payload: versioned capability negotiation.
type Hello struct {
	FirmwareVersion string
	Board           string
	ProtocolVersion uint16
	Capabilities    uint32
}

// EncodeHello serializes a Hello into a frame payload.
func (h Hello) EncodeHello() ([]byte, error) {
	if len(h.FirmwareVersion) > maxHelloString || len(h.Board) > maxHelloString {
		return nil, ErrHelloField
	}
	buf := make([]byte, 2+2+4+1+len(h.FirmwareVersion)+1+len(h.Board))
	off := 0
	binary.BigEndian.PutUint16(buf[off:], h.ProtocolVersion)
	off += 2
	binary.BigEndian.PutUint32(buf[off:], h.Capabilities)
	off += 4
	buf[off] = byte(len(h.FirmwareVersion))
	off++
	copy(buf[off:], h.FirmwareVersion)
	off += len(h.FirmwareVersion)
	buf[off] = byte(len(h.Board))
	off++
	copy(buf[off:], h.Board)
	off += len(h.Board)
	return buf[:off], nil
}

// DecodeHello parses a hello payload. It rejects truncated or oversized
// string fields.
func DecodeHello(payload []byte) (Hello, error) {
	var h Hello
	if len(payload) < 8 {
		return h, ErrTruncated
	}
	off := 0
	h.ProtocolVersion = binary.BigEndian.Uint16(payload[off:])
	off += 2
	h.Capabilities = binary.BigEndian.Uint32(payload[off:])
	off += 4
	fwLen := int(payload[off])
	off++
	if fwLen > maxHelloString {
		return Hello{}, ErrHelloField
	}
	if off+fwLen+1 > len(payload) {
		return h, ErrTruncated
	}
	h.FirmwareVersion = string(payload[off : off+fwLen])
	off += fwLen
	boardLen := int(payload[off])
	off++
	if boardLen > maxHelloString {
		return Hello{}, ErrHelloField
	}
	if off+boardLen > len(payload) {
		return h, ErrTruncated
	}
	h.Board = string(payload[off : off+boardLen])
	return h, nil
}
