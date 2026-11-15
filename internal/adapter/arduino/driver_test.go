package arduino_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/arduino"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
)

func helloFrame(t *testing.T, fw string, proto uint16, caps uint32) []byte {
	t.Helper()
	payload, err := (arduino.Hello{
		FirmwareVersion: fw, Board: "mega2560",
		ProtocolVersion: proto, Capabilities: caps,
	}).EncodeHello()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := arduino.Encode(arduino.Frame{Type: arduino.TypeHello, Seq: 0, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func capFrame(t *testing.T) []byte {
	t.Helper()
	payload, err := (arduino.Hello{
		FirmwareVersion: "0.1.0", Board: "mega2560",
		ProtocolVersion: 1, Capabilities: arduino.CapSelfTest,
	}).EncodeHello()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := arduino.Encode(arduino.Frame{Type: arduino.TypeCapability, Seq: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func newDriver(tr *fake.Transport) *arduino.Driver {
	return arduino.NewDriverWith(adapter.KindArduino, 115200,
		func(ctx context.Context, path string, baud uint32) (adapter.Transport, error) {
			return tr, nil
		}, nil)
}

func cand() adapter.Candidate {
	return adapter.Candidate{Info: adapter.Info{ID: "serial/ttyACM0", Kind: adapter.KindArduino, Path: "/dev/ttyACM0"}}
}

func TestHandshakeHello(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	go func() {
		// Firmware sends HELLO shortly after the capability request.
		time.Sleep(20 * time.Millisecond)
		tr.Feed(helloFrame(t, "0.1.0", 1, arduino.CapSelfTest))
	}()
	s, err := d.Open(context.Background(), cand())
	if err != nil {
		t.Fatal(err)
	}
	as := s.(*arduino.Session)
	if as.Hello().FirmwareVersion != "0.1.0" || as.Hello().Board != "mega2560" {
		t.Fatalf("unexpected hello %+v", as.Hello())
	}
	if !s.Capabilities().ReadOnly {
		t.Fatal("session must declare read-only")
	}
	if s.Health().State != adapter.HealthReady {
		t.Fatalf("unexpected health %s", s.Health().State)
	}
	// The capability request frame must have been written to the port.
	if len(tr.Written()) == 0 {
		t.Fatal("expected capability request written")
	}
}

func TestHandshakeCapabilityReply(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	go func() {
		time.Sleep(20 * time.Millisecond)
		tr.Feed(capFrame(t))
	}()
	s, err := d.Open(context.Background(), cand())
	if err != nil {
		t.Fatal(err)
	}
	if s.(*arduino.Session).Hello().Capabilities != arduino.CapSelfTest {
		t.Fatal("unexpected negotiated capabilities")
	}
}

func TestHandshakeSkipsNoiseAndHeartbeat(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	go func() {
		time.Sleep(10 * time.Millisecond)
		// Garbage, then a heartbeat, then the real hello.
		tr.Feed([]byte{0x00, 0xDE, 0xAD})
		hb, _ := arduino.Encode(arduino.Frame{Type: arduino.TypeHeartbeat, Seq: 9, Payload: []byte{1, 2, 3, 4, 5, 6, 7}})
		tr.Feed(hb)
		tr.Feed(helloFrame(t, "0.2.0", 1, arduino.CapSelfTest|arduino.CapPeriodicSamples))
	}()
	s, err := d.Open(context.Background(), cand())
	if err != nil {
		t.Fatal(err)
	}
	if s.(*arduino.Session).Hello().FirmwareVersion != "0.2.0" {
		t.Fatal("expected hello after noise resync")
	}
}

func TestHandshakeTimeout(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	// No frames ever arrive.
	_, err := d.Open(context.Background(), cand())
	if !errors.Is(err, adapter.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestHandshakeProtocolMismatch(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	go func() {
		time.Sleep(20 * time.Millisecond)
		tr.Feed(helloFrame(t, "9.9.9", 99, 0))
	}()
	_, err := d.Open(context.Background(), cand())
	if !errors.Is(err, adapter.ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}

func TestHandshakeBadHelloPayload(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	go func() {
		time.Sleep(20 * time.Millisecond)
		enc, _ := arduino.Encode(arduino.Frame{Type: arduino.TypeHello, Seq: 0, Payload: []byte{1, 2, 3}})
		tr.Feed(enc)
	}()
	_, err := d.Open(context.Background(), cand())
	if !errors.Is(err, adapter.ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}

func TestSessionFramesRoundTrip(t *testing.T) {
	tr := fake.NewTransport()
	d := newDriver(tr)
	go func() {
		time.Sleep(20 * time.Millisecond)
		tr.Feed(helloFrame(t, "0.1.0", 1, arduino.CapSelfTest))
	}()
	s, err := d.Open(context.Background(), cand())
	if err != nil {
		t.Fatal(err)
	}
	as := s.(*arduino.Session)

	// Host sends OP_SELF_TEST data frame; firmware replies.
	opReq, _ := arduino.Encode(arduino.Frame{Type: arduino.TypeData, Seq: 1, Payload: []byte{0x01}})
	if _, err := tr.Write(opReq); err != nil {
		t.Fatal(err)
	}
	respPayload := []byte{200, 0, 0, 1}
	resp, _ := arduino.Encode(arduino.Frame{Type: arduino.TypeData, Seq: 2, Payload: respPayload})
	tr.Feed(resp)

	f, err := as.Frames().ReadFrame(500 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if f.Type != arduino.TypeData || string(f.Payload) != string(respPayload) {
		t.Fatalf("unexpected frame %+v", f)
	}
}

func TestOpenMissingPath(t *testing.T) {
	d := newDriver(fake.NewTransport())
	c := cand()
	c.Info.Path = ""
	c.Metadata = nil
	if _, err := d.Open(context.Background(), c); !errors.Is(err, adapter.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDiscoverStampsKind(t *testing.T) {
	d := arduino.NewDriverWith(adapter.KindArduino, 115200, nil,
		adapter.NewListEnumerator(adapter.Candidate{Info: adapter.Info{ID: "x", Path: "/dev/ttyACM0"}}))
	cands, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].Info.Kind != adapter.KindArduino {
		t.Fatalf("unexpected discovery %+v", cands)
	}
}
