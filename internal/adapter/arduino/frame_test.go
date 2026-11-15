package arduino

import (
	"bytes"
	"errors"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	f := Frame{Type: TypeData, Seq: 0x1234, Payload: []byte{1, 2, 3, 4}}
	enc, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	dec, n, err := DecodeFrame(enc)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(enc) {
		t.Fatalf("consumed %d of %d bytes", n, len(enc))
	}
	if dec.Type != f.Type || dec.Seq != f.Seq || !bytes.Equal(dec.Payload, f.Payload) {
		t.Fatalf("round trip mismatch: %+v", dec)
	}
}

func TestDecodeTruncated(t *testing.T) {
	f := Frame{Type: TypeData, Seq: 1, Payload: []byte("hello")}
	enc, _ := Encode(f)
	for _, size := range []int{0, 4, headerSize + crcSize - 1, len(enc) - 1} {
		if _, _, err := DecodeFrame(enc[:size]); !errors.Is(err, ErrTruncated) {
			t.Fatalf("size %d: expected ErrTruncated, got %v", size, err)
		}
	}
}

func TestDecodeBadMagic(t *testing.T) {
	enc, _ := Encode(Frame{Type: TypeAck, Seq: 0})
	enc[0] = 0x00
	if _, _, err := DecodeFrame(enc); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}

func TestDecodeBadCRC(t *testing.T) {
	enc, _ := Encode(Frame{Type: TypeData, Seq: 7, Payload: []byte{9}})
	enc[len(enc)-1] ^= 0xFF
	if _, _, err := DecodeFrame(enc); !errors.Is(err, ErrBadCRC) {
		t.Fatalf("expected ErrBadCRC, got %v", err)
	}
}

func TestDecodeBadVersion(t *testing.T) {
	enc, _ := Encode(Frame{Type: TypeData})
	enc[2] = 0xFF
	if _, _, err := DecodeFrame(enc); !errors.Is(err, ErrBadVersion) {
		t.Fatalf("expected ErrBadVersion, got %v", err)
	}
}

func TestEncodeRejectsOversizedPayload(t *testing.T) {
	if _, err := Encode(Frame{Type: TypeData, Payload: make([]byte, maxPayloadSize+1)}); !errors.Is(err, ErrPayloadTooBig) {
		t.Fatalf("expected ErrPayloadTooBig, got %v", err)
	}
}

func TestStreamOfFrames(t *testing.T) {
	var stream []byte
	for i := 0; i < 3; i++ {
		enc, err := Encode(Frame{Type: TypeData, Seq: uint16(i), Payload: []byte{byte(i)}})
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, enc...)
	}
	// Parse back-to-back frames from one buffer.
	rest := stream
	for i := 0; i < 3; i++ {
		f, n, err := DecodeFrame(rest)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if f.Seq != uint16(i) {
			t.Fatalf("frame %d: seq %d", i, f.Seq)
		}
		rest = rest[n:]
	}
	if len(rest) != 0 {
		t.Fatalf("trailing bytes: %d", len(rest))
	}
}

func TestHelloRoundTrip(t *testing.T) {
	h := Hello{FirmwareVersion: "0.3.1", Board: "mega2560", ProtocolVersion: 1, Capabilities: CapPeriodicSamples | CapSelfTest}
	payload, err := h.EncodeHello()
	if err != nil {
		t.Fatal(err)
	}
	dec, err := DecodeHello(payload)
	if err != nil {
		t.Fatal(err)
	}
	if dec != h {
		t.Fatalf("hello mismatch: %+v vs %+v", dec, h)
	}
}

func TestFrameTypeValidation(t *testing.T) {
	for _, ft := range []FrameType{TypeHello, TypeData, TypeAck, TypeError, TypeCapability, TypeHeartbeat, TypeStatus} {
		enc, err := Encode(Frame{Type: ft})
		if err != nil {
			t.Fatalf("type 0x%02x: %v", byte(ft), err)
		}
		if _, _, err := DecodeFrame(enc); err != nil {
			t.Fatalf("type 0x%02x decode: %v", byte(ft), err)
		}
	}
	for _, ft := range []FrameType{0x00, 0x08, 0xFF} {
		if _, err := Encode(Frame{Type: ft}); !errors.Is(err, ErrInvalidType) {
			t.Fatalf("type 0x%02x: expected ErrInvalidType, got %v", byte(ft), err)
		}
	}
	// CRC-valid frame with an undefined type must be rejected on decode.
	enc, _ := Encode(Frame{Type: TypeData})
	enc[3] = 0x00
	crc := crc16(enc[:headerSize])
	enc[headerSize] = byte(crc >> 8)
	enc[headerSize+1] = byte(crc)
	if _, _, err := DecodeFrame(enc); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("expected ErrInvalidType on decode, got %v", err)
	}
}

func TestHelloEmptyStringsRoundTrip(t *testing.T) {
	h := Hello{ProtocolVersion: 1}
	payload, err := h.EncodeHello()
	if err != nil {
		t.Fatal(err)
	}
	dec, err := DecodeHello(payload)
	if err != nil {
		t.Fatal(err)
	}
	if dec != h {
		t.Fatalf("hello mismatch: %+v vs %+v", dec, h)
	}
}

func helloPayload(fwLen, boardLen int) []byte {
	p := []byte{0, 1, 0, 0, 0, 0, byte(fwLen)}
	p = append(p, bytes.Repeat([]byte{'f'}, fwLen)...)
	p = append(p, byte(boardLen))
	return append(p, bytes.Repeat([]byte{'b'}, boardLen)...)
}

func TestDecodeHelloStringBounds(t *testing.T) {
	if _, err := DecodeHello(helloPayload(32, 32)); err != nil {
		t.Fatalf("32-byte fields should decode: %v", err)
	}
	if _, err := DecodeHello(helloPayload(33, 0)); !errors.Is(err, ErrHelloField) {
		t.Fatalf("fw 33: expected ErrHelloField, got %v", err)
	}
	if _, err := DecodeHello(helloPayload(0, 33)); !errors.Is(err, ErrHelloField) {
		t.Fatalf("board 33: expected ErrHelloField, got %v", err)
	}
}

func TestDecodeHelloTruncated(t *testing.T) {
	if _, err := DecodeHello([]byte{0, 1, 0, 0, 0, 1}); !errors.Is(err, ErrTruncated) {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}
