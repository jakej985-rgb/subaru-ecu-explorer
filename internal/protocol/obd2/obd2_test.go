package obd2_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol/elm327"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol/obd2"
)

// feedAfterWrite feeds resp once any request bytes hit the transport.
func feedAfterWrite(tr *fake.Transport, resp string) {
	go func() {
		for len(tr.Written()) == 0 {
			time.Sleep(2 * time.Millisecond)
		}
		time.Sleep(5 * time.Millisecond)
		tr.Feed([]byte(resp))
	}()
}

func newClient(t *testing.T, resp string) *obd2.Client {
	t.Helper()
	tr := fake.NewTransport()
	s := protocol.NewSession(elm327.New(), tr, protocol.Options{
		ResponseTimeout: 2 * time.Second, IdleTimeout: 30 * time.Millisecond,
	})
	feedAfterWrite(tr, resp)
	c, err := obd2.NewClient(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDecodeRPM(t *testing.T) {
	m, err := obd2.Decode(0x01, 0x0C, []byte{0x1A, 0xF8})
	if err != nil {
		t.Fatal(err)
	}
	if m.Value.(float64) != 1726 {
		t.Fatalf("rpm %v", m.Value)
	}
	if m.Unit != "rpm" || m.Name != "engine-rpm" {
		t.Fatalf("meta %q %q", m.Name, m.Unit)
	}
}

func TestDecodeCoolantTemp(t *testing.T) {
	m, err := obd2.Decode(0x01, 0x05, []byte{0x80}) // 128-40=88? no: 0x80=128 → 88°C
	if err != nil {
		t.Fatal(err)
	}
	if m.Value.(float64) != 88 {
		t.Fatalf("temp %v", m.Value)
	}
}

func TestDecodeSupportedPIDs(t *testing.T) {
	// Bits for PIDs 0x01? mask: byte0=0xBE → bits... craft: support 0x0C,0x0D
	// bit i set means PID (i+1)... bit positions: PID-1. PID 0x0C→bit11,
	// 0x0D→bit12. byte1 bits 3,4 (MSB-first): byte1=0x18.
	m, err := obd2.Decode(0x01, 0x00, []byte{0x00, 0x18, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	pids, ok := m.Value.([]byte)
	if !ok {
		t.Fatalf("value type %T", m.Value)
	}
	found := map[byte]bool{}
	for _, p := range pids {
		found[p] = true
	}
	if !found[0x0C] || !found[0x0D] || len(pids) != 2 {
		t.Fatalf("pids %v", pids)
	}
}

func TestDecodeDTCs(t *testing.T) {
	m, err := obd2.Decode(0x03, 0x00, []byte{0x03, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	dtcs, ok := m.Value.([]obd2.DTC)
	if !ok || len(dtcs) != 1 {
		t.Fatalf("dtcs %#v", m.Value)
	}
	if dtcs[0].Code != "P0300" {
		t.Fatalf("code %q", dtcs[0].Code)
	}
	if !dtcs[0].Confirmed {
		t.Fatal("mode03 DTC should be confirmed")
	}
}

func TestDecodeVIN(t *testing.T) {
	vin := []byte("1G1BL52P7TR000001")
	m, err := obd2.Decode(0x09, 0x02, vin)
	if err != nil {
		t.Fatal(err)
	}
	if m.Value.(string) != "1G1BL52P7TR000001" {
		t.Fatalf("vin %q", m.Value)
	}
}

func TestUnsupportedPID(t *testing.T) {
	_, err := obd2.Decode(0x01, 0xFF, []byte{1})
	if !errors.Is(err, obd2.ErrUnsupportedPID) {
		t.Fatalf("expected ErrUnsupportedPID, got %v", err)
	}
}

func TestUnsupportedMode(t *testing.T) {
	// Mode 02 (clear DTCs) must be rejected before decode.
	_, err := obd2.Decode(0x02, 0x00, nil)
	if !errors.Is(err, obd2.ErrUnsupportedMode) {
		t.Fatalf("expected ErrUnsupportedMode, got %v", err)
	}
}

func TestShortPayload(t *testing.T) {
	_, err := obd2.Decode(0x01, 0x0C, []byte{0x1A}) // RPM needs 2 bytes
	if !errors.Is(err, obd2.ErrShortPayload) {
		t.Fatalf("expected ErrShortPayload, got %v", err)
	}
}

func TestClientQueryRPM(t *testing.T) {
	// ELM data payload includes 41 0C header then 1A F8.
	c := newClient(t, "41 0C 1A F8\r>")
	m, err := c.RPM(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Value.(float64) != 1726 {
		t.Fatalf("rpm %v", m.Value)
	}
}

func TestClientQueryVIN(t *testing.T) {
	// Multi-line VIN response with ISO-TP continuation marker.
	resp := "49 02 01 31 47 31 42 4C 35 32 50 37 54 52\r...\r30 30 30 30 30 31\r>"
	c := newClient(t, resp)
	m, err := c.VIN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Value.(string) != "1G1BL52P7TR000001" {
		t.Fatalf("vin %q", m.Value)
	}
}

func TestClientQueryDTCs(t *testing.T) {
	c := newClient(t, "43 30 00\r>")
	m, err := c.StoredDTCs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dtcs := m.Value.([]obd2.DTC)
	if len(dtcs) != 1 || dtcs[0].Code != "P3000" {
		t.Fatalf("dtcs %#v", dtcs)
	}
}

func TestClientQueryDTCsMultiLine(t *testing.T) {
	c := newClient(t, "43 01 33\r43 02 10\r>")
	m, err := c.StoredDTCs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dtcs := m.Value.([]obd2.DTC)
	if len(dtcs) != 2 || dtcs[0].Code != "P0133" || dtcs[1].Code != "P0210" {
		t.Fatalf("dtcs %#v", dtcs)
	}
}

func TestClientNoData(t *testing.T) {
	c := newClient(t, "NO DATA\r>")
	_, err := c.RPM(context.Background())
	if !errors.Is(err, obd2.ErrNoData) {
		t.Fatalf("expected ErrNoData, got %v", err)
	}
}

func TestClientUnsupportedBeforeIO(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(elm327.New(), tr, protocol.Options{})
	c, err := obd2.NewClient(s)
	if err != nil {
		t.Fatal(err)
	}
	// Mode 02 clear-DTCs is not in the registry: rejected before any write.
	_, err = c.Query(context.Background(), 0x02, 0x00)
	if !errors.Is(err, obd2.ErrUnsupportedMode) {
		t.Fatalf("expected ErrUnsupportedMode, got %v", err)
	}
	if len(tr.Written()) != 0 {
		t.Fatal("unsupported request must not write to transport")
	}
}

func TestClientHeaderMismatch(t *testing.T) {
	// Response header doesn't match the request — must not decode blindly.
	c := newClient(t, "41 0D 32\r>") // asks RPM, gets speed response
	_, err := c.RPM(context.Background())
	if !errors.Is(err, obd2.ErrMalformedData) {
		t.Fatalf("expected ErrMalformedData, got %v", err)
	}
}
