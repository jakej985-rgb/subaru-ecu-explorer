# Arduino Mega Go Driver

`internal/adapter/arduino` implements the `arduino-serial` adapter kind:
the Go-side driver for the Arduino Mega 2560 acquisition firmware described
in `ARDUINO_SERIAL_PROTOCOL.md` and `ARDUINO_INTERFACE_ARCHITECTURE.md`.

## Components

- **`Driver`** (`driver.go`) — implements `adapter.Driver`. `Discover`
  enumerates USB serial devices via `serialenum`; `Open` opens the port via
  `serialport.Open` (injectable `Opener` for tests), performs the versioned
  handshake, and returns a `Session`.
- **`Session`** — implements `adapter.Session`. `Read`/`Write`/`SetDeadline`/
  `Close` operate on the raw serial stream; `Frames()` returns the frame
  reader for protocol work. `Info()`, `Capabilities()`, `Health()` expose
  adapter identity, negotiated firmware identity, and connection state.
  `Capabilities().ReadOnly` is always `true` — the firmware advertises no
  vehicle-I/O capability and the driver adds none.
- **`Reader`** (`stream.go`) — resynchronizing frame reader over a
  transport. Discards garbage before the next magic prefix, validates
  CRC/type/version via `DecodeFrame`, and propagates transport errors
  (`ErrTimeout`, `ErrClosed`, ...) unchanged. `timeout` is a per-read idle
  bound, not a total frame deadline.
- **`frame.go`** — the shared versioned codec (magic `0xA5 0x5A`, version 1,
  CRC-16/CCITT-FALSE, 512-byte payload cap), byte-compatible with the
  firmware and replayed against committed hex fixtures.

## Handshake

On `Open` the driver:

1. Writes one `CAPABILITY` request frame (`TypeCapability`, empty payload).
2. Reads frames until a `HELLO` or `CAPABILITY` frame arrives or
   `handshakeTimeout` (default 3 s) expires. Heartbeats/status/other frames
   arriving first are skipped.
3. Decodes the identity payload and rejects protocol-version mismatches
   with `ErrProtocol` (incompatible firmware). A handshake timeout yields
   `ErrTimeout`; either failure closes the port.

The firmware answers the request with a `CAPABILITY` frame; a freshly
reset board may answer with `HELLO` first — both carry the same identity
payload.

## Candidate selection

- `Info.Path` (or `Metadata["path"]`) selects the device, e.g.
  `/dev/serial/by-id/usb-Arduino__www.arduino.cc__0043_...` for a stable
  identity across replug.
- `Metadata["baud"]` overrides the driver default (115200 8N1).
- A candidate without a path fails with `ErrNotFound`; the driver never
  guesses a device.

## Safety boundary

- The session's byte stream is unfiltered: `Read`/`Write` expose raw serial.
  Application code must use `Frames()` plus the `internal/safety` gate for
  any operation-level work; the driver's only protocol action is the
  read-only `CAPABILITY` request.
- The firmware allowlist contains only `OP_SELF_TEST`; there is no
  vehicle-transmit path in the current build (`VEHICLE_KLINE_ENABLED 0`).
- USB connection success says nothing about vehicle compatibility — it only
  means the firmware handshake completed.
- Invalid frames are rejected by the codec/reader; unknown frame types and
  oversized payloads never reach application code.

## Testing

`driver_test.go` covers: HELLO handshake, CAPABILITY reply, noise/heartbeat
resync before identity, handshake timeout, protocol-version mismatch, bad
identity payload, framed round-trip through the session, missing-path
rejection, and discovery kind stamping — all against an in-memory fake
transport. Committed hex fixtures in `tests/fixtures/arduino-serial/` are
decoded by `frame_fixture_test.go`.

## Limitations

- No heartbeat watchdog or automatic reconnect yet — `Health()` reports
  `ready` for the life of the session; health monitoring is a
  `Manager.Monitor` concern (see `ADAPTER_ARCHITECTURE.md`).
- No acquisition producer exists in firmware v0.1; `DATA` frames today only
  carry `OP_SELF_TEST` replies. Periodic/event sample capabilities remain
  unadvertised until a producer lands.
- Bench validation against physical hardware is tracked separately (#66);
  until then this driver is validated against simulation and fixtures only.
