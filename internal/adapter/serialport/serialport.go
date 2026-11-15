// Package serialport implements an adapter.Driver and Transport for
// USB/serial devices using POSIX termios. It performs no protocol work —
// it moves bytes with deadlines.
package serialport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/serialenum"
	"golang.org/x/sys/unix"
)

// Driver opens serial candidates produced by a serialenum.Enumerator.
type Driver struct {
	kind adapter.Kind
	enum *serialenum.Enumerator
	baud uint32 // default baud when the candidate carries none
}

// NewDriver returns a serial driver for kind. baud is the fallback line
// speed; candidates may override it via Metadata["baud"].
func NewDriver(kind adapter.Kind, enum *serialenum.Enumerator, baud uint32) *Driver {
	if enum == nil {
		enum = serialenum.New("")
	}
	return &Driver{kind: kind, enum: enum, baud: baud}
}

func (d *Driver) Kind() adapter.Kind { return d.kind }

func (d *Driver) Discover(ctx context.Context) ([]adapter.Candidate, error) {
	cands, err := d.enum.Enumerate(ctx)
	if err != nil {
		return cands, err
	}
	for i := range cands {
		cands[i].Info.Kind = d.kind
	}
	return cands, nil
}

func (d *Driver) Open(ctx context.Context, c adapter.Candidate) (adapter.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Info.Kind == "" {
		c.Info.Kind = d.kind
	}
	path := c.Info.Path
	if path == "" {
		return nil, fmt.Errorf("%w: candidate %s has no path", adapter.ErrNotFound, c.Info.ID)
	}
	baud := d.baud
	if b := c.Metadata["baud"]; b != "" {
		var v uint32
		if _, err := fmt.Sscanf(b, "%d", &v); err == nil && v != 0 {
			baud = v
		}
	}
	tr, err := openSerial(path, baud)
	if err != nil {
		return nil, err
	}
	return &session{info: c.Info, tr: tr, since: time.Now()}, nil
}

type session struct {
	info  adapter.Info
	tr    *transport
	since time.Time
}

func (s *session) Read(p []byte) (int, error)    { return s.tr.Read(p) }
func (s *session) Write(p []byte) (int, error)   { return s.tr.Write(p) }
func (s *session) SetDeadline(t time.Time) error { return s.tr.SetDeadline(t) }
func (s *session) Close() error                  { return s.tr.Close() }
func (s *session) Info() adapter.Info            { return s.info }
func (s *session) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{ReadOnly: true, Transport: "serial", Notes: "baud from candidate metadata"}
}
func (s *session) Health() adapter.Health {
	return adapter.Health{State: adapter.HealthReady, Since: s.since}
}

// transport is a serial Transport with poll-based deadlines.
//
// Close wakes in-flight polls through a self-pipe, and life guards the
// descriptor lifetime: I/O holds a read lock across poll+syscall while Close
// takes the write lock before releasing fds, so no operation can touch a
// closed (and possibly reused) descriptor.
type transport struct {
	fd    int
	wakeR int
	wakeW int
	life  sync.RWMutex

	mu       sync.Mutex
	deadline time.Time
	closed   bool
}

const pollTerminal = unix.POLLHUP | unix.POLLERR | unix.POLLNVAL

func openSerial(path string, baud uint32) (*transport, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", adapter.ErrNotFound, path)
		}
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("%w: %s", adapter.ErrPermission, path)
		}
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		switch {
		case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
			return nil, fmt.Errorf("%w: %s", adapter.ErrPermission, path)
		case errors.Is(err, unix.ENOENT):
			return nil, fmt.Errorf("%w: %s", adapter.ErrNotFound, path)
		case errors.Is(err, unix.EBUSY):
			return nil, fmt.Errorf("%w: %s", adapter.ErrBusy, path)
		default:
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
	}
	t := &transport{fd: fd}
	if err := t.configure(baud); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	// Exclusive mode: further opens of this tty (via any alias path) fail
	// with EBUSY, so one physical device never has two owners.
	if err := unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("exclusive open %s: %w", path, err)
	}
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wake pipe: %w", err)
	}
	t.wakeR, t.wakeW = p[0], p[1]
	return t, nil
}

// configure applies 8N1 raw termios at baud.
func (t *transport) configure(baud uint32) error {
	tc, err := unix.IoctlGetTermios(t.fd, unix.TCGETS)
	if err != nil {
		return fmt.Errorf("termios get on fd %d: %w", t.fd, err)
	}
	// cfmakeraw equivalent.
	tc.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	tc.Oflag &^= unix.OPOST
	tc.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	tc.Cflag &^= unix.PARENB | unix.CSTOPB | unix.CSIZE | unix.CRTSCTS | unix.CBAUD
	tc.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL
	if b, ok := baudBits(baud); ok {
		tc.Cflag |= b
		tc.Ispeed = b
		tc.Ospeed = b
	} else {
		return fmt.Errorf("unsupported baud %d", baud)
	}
	tc.Cc[unix.VMIN] = 0
	tc.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(t.fd, unix.TCSETS, tc); err != nil {
		return fmt.Errorf("termios set on fd %d: %w", t.fd, err)
	}
	return nil
}

func baudBits(baud uint32) (uint32, bool) {
	switch baud {
	case 9600:
		return unix.B9600, true
	case 19200:
		return unix.B19200, true
	case 38400:
		return unix.B38400, true
	case 57600:
		return unix.B57600, true
	case 115200:
		return unix.B115200, true
	case 230400:
		return unix.B230400, true
	default:
		return 0, false
	}
}

func (t *transport) SetDeadline(d time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return adapter.ErrClosed
	}
	t.deadline = d
	return nil
}

// wait polls the device for events alongside the close wake pipe. It
// returns the device revents, ErrClosed if Close was signalled, or
// ErrTimeout on deadline expiry. Callers must hold t.life.RLock.
func (t *transport) wait(events int16) (int16, error) {
	for {
		t.mu.Lock()
		d := t.deadline
		closed := t.closed
		t.mu.Unlock()
		if closed {
			return 0, adapter.ErrClosed
		}
		ms := -1 // no deadline: block until ready or woken by Close
		if !d.IsZero() {
			remaining := time.Until(d)
			if remaining <= 0 {
				return 0, adapter.ErrTimeout
			}
			ms = int(remaining / time.Millisecond)
			if ms < 1 {
				ms = 1
			}
		}
		fds := []unix.PollFd{
			{Fd: int32(t.fd), Events: events},
			{Fd: int32(t.wakeR), Events: unix.POLLIN},
		}
		n, err := unix.Poll(fds, ms)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue // recompute remaining time
			}
			return 0, err
		}
		if fds[1].Revents != 0 {
			return 0, adapter.ErrClosed
		}
		if n == 0 {
			return 0, adapter.ErrTimeout
		}
		return fds[0].Revents, nil
	}
}

func (t *transport) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	t.life.RLock()
	defer t.life.RUnlock()
	for {
		rev, err := t.wait(unix.POLLIN)
		if err != nil {
			return 0, err
		}
		n, err := unix.Read(t.fd, p)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				if rev&pollTerminal != 0 {
					return 0, adapter.ErrClosed
				}
				continue
			}
			if errors.Is(err, unix.EIO) {
				return 0, adapter.ErrClosed
			}
			return 0, err
		}
		if n == 0 {
			// With O_NONBLOCK an empty line yields EAGAIN, so a zero-byte
			// read means hangup/EOF. Buffered bytes are returned first.
			return 0, adapter.ErrClosed
		}
		return n, nil
	}
}

// Write writes all of p, polling under backpressure until complete, the
// deadline expires, or the transport fails. A short count always carries a
// non-nil error.
func (t *transport) Write(p []byte) (int, error) {
	t.life.RLock()
	defer t.life.RUnlock()
	total := 0
	for total < len(p) {
		rev, err := t.wait(unix.POLLOUT)
		if err != nil {
			return total, err
		}
		if rev&pollTerminal != 0 {
			return total, adapter.ErrClosed
		}
		n, err := unix.Write(t.fd, p[total:])
		if n > 0 {
			total += n
		}
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				continue
			}
			if errors.Is(err, unix.EIO) {
				return total, adapter.ErrClosed
			}
			return total, err
		}
	}
	return total, nil
}

func (t *transport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()
	// Wake any in-flight poll; the byte is never drained so later polls
	// also return immediately.
	_, _ = unix.Write(t.wakeW, []byte{1})
	// Wait for in-flight I/O to exit before releasing descriptors.
	t.life.Lock()
	defer t.life.Unlock()
	err := unix.Close(t.fd)
	_ = unix.Close(t.wakeR)
	_ = unix.Close(t.wakeW)
	return err
}
