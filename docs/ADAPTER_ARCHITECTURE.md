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
- `ConnectWithPolicy` — retries transient open failures (missing device,
  I/O errors) per `ReconnectPolicy`; permanent errors (`ErrBusy`,
  `ErrUnsupported`, `ErrPermission`, `ErrProtocol`) and context
  cancellation abort immediately.
- `Shutdown` — closes every active session, races in-flight `Open` calls
  safely (a session opened during shutdown is closed before it is ever
  exposed), and blocks all future connects with `ErrClosed`. Idempotent.

## Lifecycle & health monitoring

Ownership is enforced by a per-device reservation that exists from before
`Driver.Open` is called until after `Session.Close` returns — physical
access can never overlap, even for concurrent `Connect` calls. Only
`stateActive` reservations are visible to callers.

`Monitor` (`internal/adapter/monitor.go`) performs periodic health passes:

- `Check(ctx)` evaluates every connected session — via `Session.Health()`
  or an optional `ProbeFunc` liveness check — and returns `HealthEvent`s
  for state transitions (`unknown → ready → degraded → fault → closed`).
- Sessions reporting `HealthFault` (or failing the probe) are
  disconnected automatically when constructed with
  `disconnectOnFault=true` — the safe default for unattended operation.
- `Run(ctx)` drives `Check` on an interval until cancelled.

Disconnect and cancellation leave no active I/O: `Disconnect` releases
the reservation only after `Close` returns, and closed transports reject
further reads/writes (`ErrClosed`/EOF). Verified by tests covering
concurrent connects, shutdown-during-connect, transient/permanent
reconnect behavior, monitor transitions, and probe-failure disconnects
(`internal/adapter/lifecycle_test.go`).

## Arduino serial framing (`internal/adapter/arduino`)

The Arduino Mega path uses a versioned, bounded USB-serial frame — this is
**transport framing only**. It carries no vehicle-protocol semantics and
never decodes ECU/SSM messages.

- Frame: magic `0xA5 0x5A`, version byte, type, sequence, payload length,
  payload, CRC-16/CCITT-FALSE. Max payload 512 bytes; `DecodeFrame` returns
  `ErrTruncated`/`ErrBadMagic`/`ErrBadVersion`/`ErrBadCRC`/
  `ErrPayloadTooBig`/`ErrInvalidType`.
- Frame types: `Hello` (0x01), `Data` (0x02), `Ack` (0x03), `Error`
  (0x04), `Capability` (0x05). `Encode` and `DecodeFrame` reject any
  other type.
- Hello payload: protocol version, capability bitmask, firmware version and
  board strings (each ≤ 32 bytes, enforced on both encode and decode).
  **Only the payload codec is implemented.** The handshake state machine
  (host acknowledgement, protocol-version compatibility checks, rejecting
  unknown capability bits) is not implemented yet; it lands with the serial
  driver.
- Vehicle-facing data moves only inside `DATA` payloads as opaque bytes;
  interpretation is the protocol layer's job (later issues).

## The three paths

| Path | Kind | Transport | Status |
|---|---|---|---|
| ELM327-compatible OBD-II | `elm327-odbii` | serial/USB/Bluetooth RFCOMM | contract only — driver in later issue |
| Subaru SSM interface | `subaru-ssm` | serial | contract only — driver in later issue |
| Arduino Mega 2560 R3 | `arduino-serial` | USB serial, versioned frames | framing + Hello payload codec implemented; handshake, serial driver + firmware in issues #64/#65 |

## Testing

- `internal/adapter/fake` — in-memory `Transport`, scriptable `Driver`,
  `Session`; used by manager tests and available to later layers.
- `manager_test.go` — multi-driver discovery, exclusive ownership
  (`ErrBusy`), unknown kind/candidate errors, health lifecycle, discover
  error isolation, connect cancellation.
- `arduino/frame_test.go` — encode/decode round trip, truncation, bad
  magic/version/CRC, invalid frame type, oversized payload, back-to-back
  stream parsing, Hello payload round trip and string bounds.

## Limitations & open questions

- No concrete ELM327 or SSM drivers yet — only contracts and fakes.
- `Manager` keys ownership on `Info.ID`; drivers must guarantee ID
  stability per physical device or document collisions.
- Health is session-reported plus optional probe; `Monitor.Run` must be
  started by the service to get periodic checks (not automatic).
- Arduino frame `DATA` payloads are opaque; vehicle-protocol decoding is
  deliberately out of scope here.
- Browser-facing REST/WebSocket APIs are not part of this issue.
