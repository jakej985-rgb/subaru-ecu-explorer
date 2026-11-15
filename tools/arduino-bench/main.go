// Command arduino-bench runs the repeatable serial-layer bench checks for
// the Arduino Mega acquisition firmware against a real serial port. It is
// the executable half of docs/hardware/ARDUINO_BENCH_VALIDATION.md.
//
// Usage:
//
//	go run ./tools/arduino-bench -port /dev/ttyACM0 [-baud 115200] [-tests all|handshake,selftest,...] [-count 50] [-timeout 5s]
//
// Every check prints "PASS name ..." or "FAIL name ..." with a UTC
// timestamp; tee the output to a log for the bench results record.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/arduino"
)

var (
	port    = flag.String("port", "", "serial device path, e.g. /dev/ttyACM0 (required)")
	baud    = flag.Uint("baud", 115200, "serial baud rate")
	tests   = flag.String("tests", "all", "comma-separated test names or 'all'")
	count   = flag.Int("count", 50, "iterations for the throughput test")
	timeout = flag.Duration("timeout", 5*time.Second, "per-check timeout")
)

var failures int

type bench struct {
	s *arduino.Session
}

func stamp(format string, args ...interface{}) {
	fmt.Printf("[%s] %s\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), fmt.Sprintf(format, args...))
}

func pass(name, detail string) { stamp("PASS %s %s", name, detail) }
func fail(name, detail string) {
	stamp("FAIL %s %s", name, detail)
	failures++
}

// openSession connects via the production driver path.
func openSession() *bench {
	if *port == "" {
		fmt.Fprintln(os.Stderr, "-port is required")
		os.Exit(2)
	}
	d := arduino.NewDriver(adapter.KindArduino, uint32(*baud))
	c := adapter.Candidate{Info: adapter.Info{ID: "bench/" + *port, Kind: adapter.KindArduino, Path: *port}}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	s, err := d.Open(ctx, c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open failed: %v\n", err)
		os.Exit(1)
	}
	return &bench{s: s.(*arduino.Session)}
}

// sendData writes a DATA frame carrying payload and returns the first
// reply frame of type want, skipping ACKs/STATUS/HEARTBEAT. Non-fatal
// frames are consumed until the deadline.
func (b *bench) roundTrip(want arduino.FrameType, payload []byte) (arduino.Frame, error) {
	enc, err := arduino.Encode(arduino.Frame{Type: arduino.TypeData, Seq: 0, Payload: payload})
	if err != nil {
		return arduino.Frame{}, err
	}
	if _, err := b.s.Write(enc); err != nil {
		return arduino.Frame{}, err
	}
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		f, err := b.s.Frames().ReadFrame(500 * time.Millisecond)
		if err != nil {
			continue // idle timeout: keep waiting for the wanted frame
		}
		if f.Type == want {
			return f, nil
		}
	}
	return arduino.Frame{}, fmt.Errorf("no frame type 0x%02x within %s", byte(want), *timeout)
}

// expectErrCode sends a DATA frame with op payload and returns the first
// ERROR frame's code byte.
func (b *bench) expectErrCode(payload []byte) (byte, error) {
	f, err := b.roundTrip(arduino.TypeError, payload)
	if err != nil {
		return 0, err
	}
	if len(f.Payload) < 1 {
		return 0, fmt.Errorf("error frame with empty payload")
	}
	return f.Payload[0], nil
}

func main() {
	flag.Parse()
	b := openSession()
	defer b.s.Close()

	h := b.s.Hello()
	if h.ProtocolVersion != arduino.FrameVersion {
		fail("handshake", fmt.Sprintf("protocol %d, host %d", h.ProtocolVersion, arduino.FrameVersion))
	} else if !b.s.Capabilities().ReadOnly {
		fail("capabilities", "session not read-only")
	} else {
		pass("handshake", fmt.Sprintf("fw=%s board=%s proto=%d caps=0x%08x",
			h.FirmwareVersion, h.Board, h.ProtocolVersion, h.Capabilities))
	}

	want := map[string]bool{}
	for _, t := range strings.Split(*tests, ",") {
		want[strings.TrimSpace(t)] = true
	}
	run := func(name string, fn func()) {
		if want["all"] || want[name] {
			fn()
		}
	}

	run("selftest", func() {
		f, err := b.roundTrip(arduino.TypeData, []byte{arduino.OpSelfTest})
		if err != nil {
			fail("selftest", err.Error())
			return
		}
		if len(f.Payload) != 4 {
			fail("selftest", fmt.Sprintf("reply payload %d bytes, want 4", len(f.Payload)))
			return
		}
		if f.Payload[3] != arduino.StReady {
			fail("selftest", fmt.Sprintf("state 0x%02x, want ST_READY", f.Payload[3]))
			return
		}
		pass("selftest", fmt.Sprintf("room=%d crcErr=%d state=%d",
			f.Payload[0], int(f.Payload[1])<<8|int(f.Payload[2]), f.Payload[3]))
	})

	run("reject", func() {
		bad := false
		for _, op := range []byte{0x7F, 0x00, 0x02} {
			code, err := b.expectErrCode([]byte{op})
			if err != nil {
				fail("reject", fmt.Sprintf("op 0x%02x: %v", op, err))
				bad = true
				break
			}
			if code != arduino.ErrUnsupportedOp {
				fail("reject", fmt.Sprintf("op 0x%02x -> error 0x%02x, want 0x%02x", op, code, arduino.ErrUnsupportedOp))
				bad = true
				break
			}
		}
		if bad {
			return
		}
		// Empty DATA payload must be rejected as bad payload.
		code, err := b.expectErrCode(nil)
		if err != nil {
			fail("reject", "empty payload: "+err.Error())
			return
		}
		if code != arduino.ErrBadPayload {
			fail("reject", fmt.Sprintf("empty payload -> error 0x%02x, want 0x%02x", code, arduino.ErrBadPayload))
			return
		}
		pass("reject", "unsupported ops and empty payload rejected")
	})

	run("heartbeat", func() {
		deadline := time.Now().Add(3 * time.Second)
		seen := false
		for time.Now().Before(deadline) && !seen {
			f, err := b.s.Frames().ReadFrame(500 * time.Millisecond)
			if err == nil && f.Type == arduino.TypeHeartbeat {
				seen = len(f.Payload) == 7
			}
		}
		if !seen {
			fail("heartbeat", "no 7-byte heartbeat within 3s")
			return
		}
		pass("heartbeat", "periodic heartbeat observed")
	})

	run("corrupt", func() {
		// Inject garbage mid-stream; the parser must resync and the
		// firmware must keep answering valid requests.
		if _, err := b.s.Write([]byte{0x00, 0x11, 0x22, 0xA5}); err != nil {
			fail("corrupt", err.Error())
			return
		}
		if _, err := b.roundTrip(arduino.TypeData, []byte{arduino.OpSelfTest}); err != nil {
			fail("corrupt", "no response after garbage injection: "+err.Error())
			return
		}
		pass("corrupt", "parser resynced after garbage")
	})

	run("capability", func() {
		enc, _ := arduino.Encode(arduino.Frame{Type: arduino.TypeCapability})
		if _, err := b.s.Write(enc); err != nil {
			fail("capability", err.Error())
			return
		}
		deadline := time.Now().Add(*timeout)
		ok := false
		for time.Now().Before(deadline) && !ok {
			f, err := b.s.Frames().ReadFrame(500 * time.Millisecond)
			if err != nil {
				continue
			}
			if f.Type == arduino.TypeCapability {
				hh, err := arduino.DecodeHello(f.Payload)
				ok = err == nil && hh.FirmwareVersion == h.FirmwareVersion
			}
		}
		if !ok {
			fail("capability", "no matching CAPABILITY frame")
			return
		}
		pass("capability", "capability request answered")
	})

	run("throughput", func() {
		start := time.Now()
		for i := 0; i < *count; i++ {
			enc, _ := arduino.Encode(arduino.Frame{Type: arduino.TypeData, Payload: []byte{arduino.OpSelfTest}})
			if _, err := b.s.Write(enc); err != nil {
				fail("throughput", err.Error())
				return
			}
			deadline := time.Now().Add(2 * time.Second)
			answered := false
			for time.Now().Before(deadline) && !answered {
				f, err := b.s.Frames().ReadFrame(300 * time.Millisecond)
				if err == nil && f.Type == arduino.TypeData {
					answered = true
				}
			}
			if !answered {
				fail("throughput", fmt.Sprintf("iteration %d unanswered", i))
				return
			}
		}
		el := time.Since(start)
		pass("throughput", fmt.Sprintf("%d self-tests in %s (%.1f/s)", *count, el.Round(time.Millisecond), float64(*count)/el.Seconds()))
	})

	run("reconnect", func() {
		if err := b.s.Close(); err != nil {
			fail("reconnect", err.Error())
			return
		}
		b2 := openSession()
		defer b2.s.Close()
		if b2.s.Hello().FirmwareVersion != h.FirmwareVersion {
			fail("reconnect", "identity changed after reconnect")
			return
		}
		pass("reconnect", "close/reopen handshake clean")
	})

	if failures > 0 {
		stamp("done %d check(s) FAILED", failures)
		os.Exit(1)
	}
	stamp("done all selected tests passed")
}
