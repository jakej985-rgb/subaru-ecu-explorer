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

func TestDecodeHelloTruncated(t *testing.T) {
	if _, err := DecodeHello([]byte{0, 1, 0, 0, 0, 1}); !errors.Is(err, ErrTruncated) {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}
