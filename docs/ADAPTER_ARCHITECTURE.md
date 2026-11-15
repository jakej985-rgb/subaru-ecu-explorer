# Adapter Architecture

Phase 2.1 — the shared hardware adapter contracts in `internal/adapter`.
Covers issue #23: adapter discovery, connection, transport I/O, identity,
capabilities, health, and lifecycle for all three hardware paths.

## Design rules

- **The local Go service owns every physical connection.** Browser clients
  reach adapters only through the service's REST/WebSocket APIs — never the
  device transport directly.
- **Drivers are interchangeable.** `adapter.Driver` is the only seam between
  the service and hardware. ELM327, Subaru SSM, and Arduino implementations
  sit behind it; nothing above the driver layer knows which path is active.
- **Drivers own transport, not protocol.** AT commands, SSM message formats,
  ECU decoding, and vehicle-specific semantics live in later layers. A
  driver exposes a byte-stream `Transport` plus identity, capabilities, and
  health — nothing more.
- **USB connectivity is not vehicle compatibility.** A discovered device
  proves only that a transport exists. `Capabilities.Protocols` records what
  the adapter *claims*; actual vehicle-protocol support is established later
  through the Phase 1 resolver and Phase 2 protocol work — never assumed
  from detection.
- **Read-only is the operating boundary.** `Capabilities.ReadOnly` reflects
  the project's safety posture. Early drivers must not expose actuator,
  write, or security-bypass operations through the transport contract.

## Contracts (`internal/adapter/adapter.go`)

| Type | Role |
|---|---|
| `Kind` | `elm327-odbii`, `subaru-ssm`, `arduino-serial` — one per implementation path |
| `Driver` | `Kind()`, `Discover(ctx)`, `Open(ctx, candidate)` — discover without opening; open returns an exclusively owned `Session` |
| `Session` | `Transport` + `Info()`, `Capabilities()`, `Health()` |
| `Transport` | `io.Reader`/`io.Writer` + `SetDeadline`, idempotent `Close`; one reader + one writer concurrently |
| `Candidate` | discovered, not-yet-open device (`Info` + free-form metadata) |
| `Info` | stable identity: `ID`, `Kind`, `Name`, `Path` |
| `Capabilities` | `ReadOnly`, `Transport`, `Protocols`, `Notes` |
| `Health`/`HealthState` | `unknown → connecting → ready`, plus `degraded`, `fault`, `closed`; carries `Since` timestamp and detail |

Sentinel errors (`ErrNotFound`, `ErrBusy`, `ErrClosed`, `ErrTimeout`,
`ErrUnsupported`, `ErrProtocol`) are wrapped by drivers so callers match
with `errors.Is`.

## Manager (`internal/adapter/manager.go`)

`Manager` is the service-side owner of all connections:

- `RegisterDriver` / `Kinds` — driver registry, one driver per kind.
- `Discover` — aggregates candidates across drivers; a failing driver
  contributes an error but does not block healthy drivers
  (`errors.Join`).
- `Connect` — opens a session with exclusive ownership. Fails with
  `ErrUnsupported` for unregistered kinds, `ErrNotFound` propagated from
  the driver, and `ErrBusy` if the device ID is already connected —
  including a race-safe re-check after `Open` returns.
- `Disconnect` — idempotent close + ownership release.
- `Session` / `ConnectedIDs` / `Health` — introspection for APIs and
  health monitoring.

## Arduino serial framing (`internal/adapter/arduino`)

The Arduino Mega path uses a versioned, bounded USB-serial frame — this is
**transport framing only**. It carries no vehicle-protocol semantics and
never decodes ECU/SSM messages.

- Frame: magic `0xA5`, version byte, type, sequence, payload length,
  payload, CRC-16/CCITT. Max payload 512 bytes; `DecodeFrame` returns
  `ErrTruncated`/`ErrBadMagic`/`ErrBadVersion`/`ErrBadCRC`/
  `ErrPayloadTooBig`.
- Frame types: `HELLO`, `HELLO_ACK`, `CAPS`, `CAPS_ACK`, `DATA`,
  `ACK`, `NACK`, `RESET`.
- Capability negotiation: firmware sends `HELLO` (firmware version, board,
  protocol version, capability bitmask); the host replies `HELLO_ACK` with
  the protocol version it supports and its accepted capabilities, then
  `CAPS`/`CAPS_ACK` exchange settles the final set. Mismatched protocol
  versions and unknown capability bits are rejected explicitly — the
  transport never silently degrades.
- Vehicle-facing data moves only inside `DATA` payloads as opaque bytes;
  interpretation is the protocol layer's job (later issues).

## The three paths

| Path | Kind | Transport | Status |
|---|---|---|---|
| ELM327-compatible OBD-II | `elm327-odbii` | serial/USB/Bluetooth RFCOMM | contract only — driver in later issue |
| Subaru SSM interface | `subaru-ssm` | serial | contract only — driver in later issue |
| Arduino Mega 2560 R3 | `arduino-serial` | USB serial, versioned frames | framing + negotiation implemented; serial driver + firmware in issues #64/#65 |

## Testing

- `internal/adapter/fake` — in-memory `Transport`, scriptable `Driver`,
  `Session`; used by manager tests and available to later layers.
- `manager_test.go` — multi-driver discovery, exclusive ownership
  (`ErrBusy`), unknown kind/candidate errors, health lifecycle, discover
  error isolation, connect cancellation.
- `arduino/frame_test.go` — encode/decode round trip, truncation, bad
  magic/version/CRC, oversized payload, back-to-back stream parsing,
  HELLO negotiation round trip.

## Limitations & open questions

- No concrete ELM327 or SSM drivers yet — only contracts and fakes.
- `Manager` keys ownership on `Info.ID`; drivers must guarantee ID
  stability per physical device or document collisions.
- Health is session-reported; no supervisor/monitor loop yet (connection
  lifecycle & health monitoring is issue #27).
- Arduino frame `DATA` payloads are opaque; vehicle-protocol decoding is
  deliberately out of scope here.
- Browser-facing REST/WebSocket APIs are not part of this issue.
