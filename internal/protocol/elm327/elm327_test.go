package elm327_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol/elm327"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "elm327", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// exchange runs one Transact against a fake transport pre-fed with the
// fixture response.
func exchange(t *testing.T, codec *elm327.Codec, req protocol.Request, resp []byte) protocol.Response {
	t.Helper()
	tr := fake.NewTransport()
	s := protocol.NewSession(codec, tr, protocol.Options{
		ResponseTimeout: 2 * time.Second,
		IdleTimeout:     30 * time.Millisecond,
	})
	go func() {
		time.Sleep(10 * time.Millisecond)
		tr.Feed(resp)
	}()
	r, err := s.Transact(context.Background(), req)
	if err != nil {
		t.Fatalf("transact: %v", err)
	}
	return r
}

func TestMode01DataResponse(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, "mode01-data.txt"))
	if len(r.Answers) != 1 {
		t.Fatalf("answers %d", len(r.Answers))
	}
	m := r.Answers[0]
	if elm327.KindOf(m) != "data" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
	want := []byte{0x41, 0x0C, 0x1A, 0xF8}
	if string(m.Payload) != string(want) {
		t.Fatalf("payload %x, want %x", m.Payload, want)
	}
	if string(m.Raw) != "41 0C 1A F8\r>" {
		t.Fatalf("raw %q", m.Raw)
	}
}

func TestEchoOnSuppressed(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, "echo-on.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "data" {
		t.Fatalf("kind %q (echo not suppressed?)", elm327.KindOf(m))
	}
	if string(m.Payload) != string([]byte{0x41, 0x0C, 0x1A, 0xF8}) {
		t.Fatalf("payload %x", m.Payload)
	}
}

func TestHeadersOn(t *testing.T) {
	codec := elm327.New()
	codec.Headers = true
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, "headers-on.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "data" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
	// Header fields (7E8 04) must be stripped, not decoded as data.
	if string(m.Payload) != string([]byte{0x41, 0x0C, 0x1A, 0xF8}) {
		t.Fatalf("payload %x", m.Payload)
	}
	ids, _ := m.Metadata["canIDs"].([]string)
	if len(ids) != 1 || ids[0] != "7E8" {
		t.Fatalf("canIDs %v", m.Metadata["canIDs"])
	}
}

func TestMultilineSearching(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, "multiline-searching.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "data" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
	if string(m.Payload) != string([]byte{0x41, 0x0C, 0x1A, 0xF8}) {
		t.Fatalf("payload %x", m.Payload)
	}
	sl, _ := m.Metadata["statusLines"].([]string)
	if len(sl) != 1 || sl[0] != "SEARCHING..." {
		t.Fatalf("statusLines %v", m.Metadata["statusLines"])
	}
}

func TestErrorStatuses(t *testing.T) {
	cases := []struct {
		fixture string
		code    byte
		status  string
	}{
		{"unable-to-connect.txt", 0x01, "UNABLE TO CONNECT"},
		{"can-error.txt", 0x02, "CAN ERROR"},
		{"bus-init-error.txt", 0x06, "BUS INIT: ERROR"},
		{"stopped.txt", 0x07, "STOPPED"},
		{"no-data.txt", 0x08, "NO DATA"},
		{"unknown-cmd.txt", 0x0A, "?"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			codec := elm327.New()
			r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, tc.fixture))
			m := r.Answers[0]
			if elm327.KindOf(m) != "error" {
				t.Fatalf("kind %q", elm327.KindOf(m))
			}
			err := elm327.ErrorOf(m)
			var ne *protocol.NegativeResponseError
			if !errors.As(err, &ne) || ne.Code != tc.code {
				t.Fatalf("error %v, want code 0x%02x", err, tc.code)
			}
		})
	}
}

func TestSearchingNoData(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, "searching-no-data.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "error" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
}

func TestMalformedHex(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), fixture(t, "malformed-hex.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "malformed" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
	if elm327.ErrorOf(m) == nil {
		t.Fatal("expected typed error")
	}
}

func TestATIdentityBanner(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.AT("I"), fixture(t, "at-identity.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "banner" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
	if string(m.Payload) != "ELM327 v1.5" {
		t.Fatalf("payload %q", m.Payload)
	}
}

func TestATOk(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.AT("E0"), fixture(t, "at-ok.txt"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "ok" {
		t.Fatalf("kind %q", elm327.KindOf(m))
	}
	if elm327.ErrorOf(m) != nil {
		t.Fatal("OK should not be an error")
	}
}

func TestMarshalFormats(t *testing.T) {
	codec := elm327.New()
	b, err := codec.Marshal(elm327.Mode(0x01, 0x0C))
	if err != nil || string(b) != "010C\r" {
		t.Fatalf("mode marshal %q %v", b, err)
	}
	b, err = codec.Marshal(elm327.AT("Z"))
	if err != nil || string(b) != "ATZ\r" {
		t.Fatalf("AT marshal %q %v", b, err)
	}
	b, err = codec.Marshal(protocol.Request{Service: 0x09, Payload: []byte{0x02, 0xF1}})
	if err != nil || string(b) != "0902F1\r" {
		t.Fatalf("mode9 marshal %q %v", b, err)
	}
}

func TestSpacesOffData(t *testing.T) {
	codec := elm327.New()
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), []byte("410C1AF8\r>"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "data" || string(m.Payload) != string([]byte{0x41, 0x0C, 0x1A, 0xF8}) {
		t.Fatalf("kind %q payload %x", elm327.KindOf(m), m.Payload)
	}
}

func TestHeadersSpacesOff(t *testing.T) {
	codec := elm327.New()
	codec.Headers = true
	r := exchange(t, codec, elm327.Mode(0x01, 0x0C), []byte("7E804410C1AF8\r>"))
	m := r.Answers[0]
	if elm327.KindOf(m) != "data" || string(m.Payload) != string([]byte{0x41, 0x0C, 0x1A, 0xF8}) {
		t.Fatalf("kind %q payload %x", elm327.KindOf(m), m.Payload)
	}
}

func TestHeadersModeBanner(t *testing.T) {
	codec := elm327.New()
	codec.Headers = true
	r := exchange(t, codec, elm327.AT("I"), []byte("ELM327 v1.5\r>"))
	if k := elm327.KindOf(r.Answers[0]); k != "banner" {
		t.Fatalf("kind %q", k)
	}
}

func TestMarshalModeEqualsPID(t *testing.T) {
	codec := elm327.New()
	for _, req := range []protocol.Request{
		elm327.Mode(0x01, 0x01),
		{Service: 0x01, Payload: []byte{0x01}},
	} {
		b, err := codec.Marshal(req)
		if err != nil || string(b) != "0101\r" {
			t.Fatalf("marshal %q %v", b, err)
		}
	}
}

func TestIdentifyErrorPropagates(t *testing.T) {
	codec := elm327.New()
	tr := fake.NewTransport()
	s := protocol.NewSession(codec, tr, protocol.Options{
		ResponseTimeout: 2 * time.Second, IdleTimeout: 20 * time.Millisecond,
	})
	resps := make([][]byte, 0, len(elm327.InitSteps)+1)
	resps = append(resps, []byte("ELM327 v1.5\r>"))
	for range elm327.InitSteps[1:] {
		resps = append(resps, []byte("OK\r>"))
	}
	resps = append(resps, []byte("?\r>"))
	feedOnWrite(tr, resps...)
	if _, err := elm327.Identify(context.Background(), s, false); err == nil {
		t.Fatal("expected ATI error")
	}
}

func TestUnmarshalIncomplete(t *testing.T) {
	codec := elm327.New()
	_, _, err := codec.Unmarshal([]byte("41 0C 1A"))
	if !errors.Is(err, protocol.ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete, got %v", err)
	}
}

// feedOnWrite feeds each response only after new request bytes appear on
// the transport, emulating a real adapter that never answers early.
func feedOnWrite(tr *fake.Transport, responses ...[]byte) {
	go func() {
		seen := 0
		for _, resp := range responses {
			for len(tr.Written()) <= seen {
				time.Sleep(2 * time.Millisecond)
			}
			seen = len(tr.Written())
			time.Sleep(5 * time.Millisecond)
			tr.Feed(resp)
		}
	}()
}

func TestInitializeSequence(t *testing.T) {
	codec := elm327.New()
	tr := fake.NewTransport()
	s := protocol.NewSession(codec, tr, protocol.Options{
		ResponseTimeout: 2 * time.Second, IdleTimeout: 20 * time.Millisecond,
	})
	resps := make([][]byte, 0, len(elm327.InitSteps)+1)
	for range elm327.InitSteps {
		resps = append(resps, []byte("OK\r>"))
	}
	resps = append(resps, []byte("ELM327 v1.5\r>"))
	feedOnWrite(tr, resps...)
	info, err := elm327.Identify(context.Background(), s, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "1.5" {
		t.Fatalf("version %q", info.Version)
	}
	if info.Clone {
		t.Fatal("v1.5 should not be flagged clone")
	}
}

func TestInitializeFailureAborts(t *testing.T) {
	codec := elm327.New()
	tr := fake.NewTransport()
	s := protocol.NewSession(codec, tr, protocol.Options{
		ResponseTimeout: 500 * time.Millisecond, IdleTimeout: 20 * time.Millisecond,
	})
	resps := [][]byte{[]byte("OK\r>"), []byte("UNABLE TO CONNECT\r>")}
	feedOnWrite(tr, resps...)
	err := elm327.Initialize(context.Background(), s, nil)
	if err == nil {
		t.Fatal("expected init failure")
	}
}

func TestIdentifyCloneHeuristic(t *testing.T) {
	// Clone heuristic exercised through the Identify path below.
	codec := elm327.New()
	tr := fake.NewTransport()
	s := protocol.NewSession(codec, tr, protocol.Options{
		ResponseTimeout: 2 * time.Second, IdleTimeout: 20 * time.Millisecond,
	})
	resps := make([][]byte, 0, len(elm327.InitSteps)+1)
	for range elm327.InitSteps {
		resps = append(resps, []byte("OK\r>"))
	}
	resps = append(resps, []byte("vLinkerMP v2.1 clone\r>"))
	feedOnWrite(tr, resps...)
	info, err := elm327.Identify(context.Background(), s, false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Clone {
		t.Fatal("clone banner not detected")
	}
}
