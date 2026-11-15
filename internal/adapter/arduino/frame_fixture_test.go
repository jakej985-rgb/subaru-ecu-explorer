package arduino

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDir = "../../../tests/fixtures/arduino-serial"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("fixture %s is not valid hex: %v", name, err)
	}
	return raw
}

func decodeOne(t *testing.T, name string) (Frame, int) {
	t.Helper()
	raw := readFixture(t, name)
	f, n, err := DecodeFrame(raw)
	if err != nil {
		t.Fatalf("fixture %s failed to decode: %v", name, err)
	}
	if n != len(raw) {
		t.Fatalf("fixture %s: consumed %d of %d bytes", name, n, len(raw))
	}
	return f, n
}

func TestFixtureHello(t *testing.T) {
	f, _ := decodeOne(t, "hello.hex")
	if f.Type != TypeHello || f.Seq != 0 {
		t.Fatalf("unexpected hello frame %+v", f)
	}
	h, err := DecodeHello(f.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if h.FirmwareVersion != "0.1.0" || h.Board != "mega2560" || h.ProtocolVersion != 1 {
		t.Fatalf("unexpected hello %+v", h)
	}
	if h.Capabilities != CapSelfTest {
		t.Fatalf("unexpected caps 0x%x", h.Capabilities)
	}
}

func TestFixtureHeartbeat(t *testing.T) {
	f, _ := decodeOne(t, "heartbeat.hex")
	if f.Type != TypeHeartbeat || f.Seq != 1 {
		t.Fatalf("unexpected heartbeat %+v", f)
	}
	if len(f.Payload) != 7 {
		t.Fatalf("heartbeat payload size %d", len(f.Payload))
	}
}

func TestFixtureStatus(t *testing.T) {
	f, _ := decodeOne(t, "status.hex")
	if f.Type != TypeStatus || f.Seq != 2 {
		t.Fatalf("unexpected status %+v", f)
	}
	if f.Payload[0] != 0x01 {
		t.Fatalf("unexpected status state 0x%02x", f.Payload[0])
	}
	if int(f.Payload[1]) != len(f.Payload)-2 {
		t.Fatalf("status detail_len %d, actual %d", f.Payload[1], len(f.Payload)-2)
	}
}

func TestFixtureDataBatch(t *testing.T) {
	f, _ := decodeOne(t, "data-batch.hex")
	if f.Type != TypeData || f.Seq != 3 {
		t.Fatalf("unexpected data %+v", f)
	}
	if len(f.Payload) != 5 {
		t.Fatalf("data payload size %d", len(f.Payload))
	}
}

func TestFixtureError(t *testing.T) {
	f, _ := decodeOne(t, "error.hex")
	if f.Type != TypeError || f.Seq != 4 {
		t.Fatalf("unexpected error %+v", f)
	}
	if f.Payload[0] != 0x10 {
		t.Fatalf("unexpected error code 0x%02x", f.Payload[0])
	}
	if int(f.Payload[1]) != len(f.Payload)-2 {
		t.Fatalf("error detail_len %d, actual %d", f.Payload[1], len(f.Payload)-2)
	}
}

func TestFixtureCapability(t *testing.T) {
	f, _ := decodeOne(t, "capability.hex")
	if f.Type != TypeCapability || f.Seq != 5 {
		t.Fatalf("unexpected capability %+v", f)
	}
	h, err := DecodeHello(f.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if h.FirmwareVersion != "0.1.0" {
		t.Fatalf("unexpected caps hello %+v", h)
	}
}

func TestFixtureCorruptCRC(t *testing.T) {
	raw := readFixture(t, "corrupt-crc.hex")
	if _, _, err := DecodeFrame(raw); err == nil {
		t.Fatal("expected CRC failure on corrupt fixture")
	}
}

func TestFixtureResync(t *testing.T) {
	raw := readFixture(t, "resync.hex")
	// Skip garbage bytes until magic, then decode the valid frame.
	off := -1
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] == 0xA5 && raw[i+1] == 0x5A {
			off = i
			break
		}
	}
	if off < 0 {
		t.Fatal("no magic found in resync fixture")
	}
	f, n, err := DecodeFrame(raw[off:])
	if err != nil {
		t.Fatalf("resync decode failed: %v", err)
	}
	if f.Type != TypeHeartbeat || f.Seq != 7 {
		t.Fatalf("unexpected resynced frame %+v", f)
	}
	if n != len(raw)-off {
		t.Fatalf("resync consumed %d of %d", n, len(raw)-off)
	}
}
