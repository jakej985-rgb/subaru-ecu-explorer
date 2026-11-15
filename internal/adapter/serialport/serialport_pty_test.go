//go:build linux

package serialport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/serialenum"
	"golang.org/x/sys/unix"
)

// openPTY returns a pty master and the slave path, which behaves like a
// real serial tty for termios, poll, exclusivity, and hangup.
func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty support: %v", err)
	}
	fd := int(m.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		_ = m.Close()
		t.Fatalf("unlockpt: %v", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		_ = m.Close()
		t.Fatalf("ptsname: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, fmt.Sprintf("/dev/pts/%d", n)
}

func ptyCandidate(path string) adapter.Candidate {
	return adapter.Candidate{Info: adapter.Info{ID: "serial/pty", Kind: adapter.KindELM327, Path: path}}
}

func openPTYSession(t *testing.T, path string) adapter.Session {
	t.Helper()
	d := NewDriver(adapter.KindELM327, serialenum.New(t.TempDir()), 38400)
	s, err := d.Open(context.Background(), ptyCandidate(path))
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestPTYReadWrite(t *testing.T) {
	m, path := openPTY(t)
	s := openPTYSession(t, path)

	if _, err := m.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, err := s.Read(buf)
	if err != nil || string(buf[:n]) != "hi" {
		t.Fatalf("read = %q, %v", buf[:n], err)
	}

	if n, err := s.Write([]byte("ATZ\r")); err != nil || n != 4 {
		t.Fatalf("write = %d, %v", n, err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(m, got); err != nil || string(got) != "ATZ\r" {
		t.Fatalf("master got %q, %v", got, err)
	}
}

func TestPTYReadDeadline(t *testing.T) {
	_, path := openPTY(t)
	s := openPTYSession(t, path)
	if err := s.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(make([]byte, 1)); !errors.Is(err, adapter.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestPTYCloseWakesIdleRead(t *testing.T) {
	_, path := openPTY(t)
	s := openPTYSession(t, path)
	done := make(chan error, 1)
	go func() {
		_, err := s.Read(make([]byte, 1)) // no deadline
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, adapter.ErrClosed) {
			t.Fatalf("expected ErrClosed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not wake pending read")
	}
}

func TestPTYHangupTerminatesRead(t *testing.T) {
	m, path := openPTY(t)
	s := openPTYSession(t, path)
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(s, make([]byte, 4))
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = m.Close()
	select {
	case err := <-done:
		if !errors.Is(err, adapter.ErrClosed) {
			t.Fatalf("expected ErrClosed after hangup, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hangup did not terminate read")
	}
}

func TestPTYLargeWriteComplete(t *testing.T) {
	m, path := openPTY(t)
	s := openPTYSession(t, path)
	payload := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB
	recv := make(chan []byte, 1)
	go func() {
		got := make([]byte, len(payload))
		_, _ = io.ReadFull(m, got)
		recv <- got
	}()
	if err := s.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	n, err := s.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("write = %d/%d, %v", n, len(payload), err)
	}
	if got := <-recv; !bytes.Equal(got, payload) {
		t.Fatal("payload mismatch")
	}
}

func TestPTYShortWriteReportsError(t *testing.T) {
	_, path := openPTY(t) // master never drains
	s := openPTYSession(t, path)
	if err := s.SetDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 1<<20)
	n, err := s.Write(payload)
	if n == len(payload) || !errors.Is(err, adapter.ErrTimeout) {
		t.Fatalf("expected partial count with ErrTimeout, got %d, %v", n, err)
	}
}

func TestPTYExclusiveOpen(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses TIOCEXCL")
	}
	_, path := openPTY(t)
	_ = openPTYSession(t, path)
	d := NewDriver(adapter.KindELM327, serialenum.New(t.TempDir()), 38400)
	alias := ptyCandidate(path)
	alias.Info.ID = "serial/pty-alias" // different ID, same device
	if _, err := d.Open(context.Background(), alias); !errors.Is(err, adapter.ErrBusy) {
		t.Fatalf("expected ErrBusy for second open, got %v", err)
	}
}

func TestPTYManagerReconnect(t *testing.T) {
	_, path := openPTY(t)
	m := adapter.NewManager()
	m.RegisterDriver(NewDriver(adapter.KindELM327, serialenum.New(t.TempDir()), 38400))
	cand := ptyCandidate(path)

	s1, err := m.Connect(context.Background(), cand)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Connect(context.Background(), cand); !errors.Is(err, adapter.ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	if err := m.Disconnect(cand.Info.ID); err != nil {
		t.Fatal(err)
	}
	s2, err := m.Connect(context.Background(), cand)
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	if s1 == s2 {
		t.Fatal("reconnect should produce a new session")
	}
	_ = m.Disconnect(cand.Info.ID)
}
