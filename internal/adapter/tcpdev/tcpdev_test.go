package tcpdev

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

func echoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				line, _ := r.ReadString('\n')
				_, _ = c.Write([]byte("echo:" + line))
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestTCPDriverDiscoverConnectIO(t *testing.T) {
	addr, stop := echoServer(t)
	defer stop()

	cand := adapter.Candidate{
		Info:     adapter.Info{ID: "wifi-elm327", Path: addr},
		Metadata: map[string]string{"address": addr},
	}
	d := NewDriver(adapter.KindELM327, adapter.NewListEnumerator(cand))

	cands, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].Info.Kind != adapter.KindELM327 {
		t.Fatalf("unexpected discovery: %+v", cands)
	}

	s, err := d.Open(context.Background(), cands[0])
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.Write([]byte("ATI\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf[:n]), "echo:ATI") {
		t.Fatalf("unexpected response %q", buf[:n])
	}
	if s.Capabilities().Transport != "tcp" || !s.Capabilities().ReadOnly {
		t.Fatalf("unexpected capabilities %+v", s.Capabilities())
	}
}

func TestTCPDriverMissingEndpoint(t *testing.T) {
	// Grab a port then close it so connects are refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	d := NewDriver(adapter.KindELM327, adapter.NewListEnumerator(adapter.Candidate{
		Info: adapter.Info{ID: "gone", Path: addr},
	}))
	cands, _ := d.Discover(context.Background())
	if _, err := d.Open(context.Background(), cands[0]); err == nil {
		t.Fatal("expected dial failure for missing endpoint")
	}
}

func TestManagerReconnectCycle(t *testing.T) {
	addr, stop := echoServer(t)
	defer stop()

	m := adapter.NewManager()
	m.RegisterDriver(NewDriver(adapter.KindELM327, adapter.NewListEnumerator(adapter.Candidate{
		Info: adapter.Info{ID: "wifi-elm327", Path: addr},
	})))

	cands, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s1, err := m.Connect(context.Background(), cands[0])
	if err != nil {
		t.Fatal(err)
	}
	// Exclusive ownership: second connect to same ID is busy.
	if _, err := m.Connect(context.Background(), cands[0]); !errors.Is(err, adapter.ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	if err := m.Disconnect("wifi-elm327"); err != nil {
		t.Fatal(err)
	}
	// Reconnect after disconnect works.
	s2, err := m.Connect(context.Background(), cands[0])
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	if s1 == s2 {
		t.Fatal("reconnect should produce a new session")
	}
	_ = s2.Close()
}
