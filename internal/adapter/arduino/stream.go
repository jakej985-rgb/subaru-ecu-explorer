package arduino

import (
	"errors"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

// Reader extracts frames from an adapter.Transport, resynchronizing on
// corrupt data by scanning for the frame magic. It is safe for one
// goroutine; the transport itself allows one reader.
type Reader struct {
	tr  adapter.Transport
	buf []byte
}

// NewReader returns a frame reader over tr.
func NewReader(tr adapter.Transport) *Reader {
	return &Reader{tr: tr}
}

// indexMagic returns the offset of the next magic prefix in buf, or -1.
func indexMagic(buf []byte) int {
	for i := 0; i+1 < len(buf); i++ {
		if buf[i] == magic[0] && buf[i+1] == magic[1] {
			return i
		}
	}
	return -1
}

// ReadFrame returns the next complete valid frame. timeout bounds each
// underlying read (idle timeout, not a total deadline); transport errors
// (ErrTimeout, ErrClosed, ErrPermission, ...) propagate unchanged.
func (r *Reader) ReadFrame(timeout time.Duration) (Frame, error) {
	tmp := make([]byte, 256)
	for {
		// Resync: discard everything before the next magic prefix.
		if idx := indexMagic(r.buf); idx > 0 {
			r.buf = r.buf[idx:]
		} else if idx < 0 {
			r.buf = r.buf[:0]
		}
		if len(r.buf) >= headerSize+crcSize {
			f, n, err := DecodeFrame(r.buf)
			switch {
			case err == nil:
				r.buf = r.buf[n:]
				return f, nil
			case errors.Is(err, ErrTruncated):
				// Incomplete frame: fall through to read more.
			default:
				// Corrupt frame: drop one byte and rescan for magic.
				r.buf = r.buf[1:]
				continue
			}
		}
		if timeout > 0 {
			_ = r.tr.SetDeadline(time.Now().Add(timeout))
		}
		n, err := r.tr.Read(tmp)
		if err != nil {
			return Frame{}, err
		}
		if n > 0 {
			r.buf = append(r.buf, tmp[:n]...)
		}
	}
}
