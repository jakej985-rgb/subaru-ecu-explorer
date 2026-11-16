# Protocol Engine Abstractions

`internal/protocol` defines diagnostic protocol sessions, requests,
responses, raw frames, metadata, timing, addressing, and typed errors
**independent of adapter type** (issue #29, Phase 3.1).

Protocol implementations — OBD-II/ELM327 (#30, #31), Subaru SSM (#32),
ECU identification (#33) — plug in through the `Codec` interface and run
over any byte transport, including in-memory fakes. No hardware-specific
imports are required to test a protocol.

## Core types

| Type | Purpose |
|---|---|
| `Address` | Module addressing (`Scheme` + `Value`); zero value = default/broadcast — the only address a read-only session should use until addressing is verified |
| `Message` | One unit of wire traffic: `Direction`, `Timestamp`, `Raw` bytes, decoded `Payload`, `Address`, `Metadata`. Raw bytes are always preserved for evidence |
| `Request` | Service invocation: `Service` byte, `Payload`, `Address`, `Timeout`, `MaxBytes`, `Metadata` |
| `Response` | Exchange result: `Messages` (everything observed, both directions, in order), `Answers` (ECU→host view), `Status`, `Error`, `Elapsed` |
| `Status` | `ok` / `timeout` / `empty` / `malformed` / `unsupported` |

## Codec interface

```go
type Codec interface {
    Name() string
    Marshal(Request) ([]byte, error)              // request -> wire bytes
    Unmarshal([]byte) (Message, int, error)       // bytes -> next message
}
```

- `Unmarshal` returns `ErrIncomplete` when the buffer lacks a full message
  (readers keep buffering) and a wrapped `ErrMalformed` for bytes that can
  never parse.
- Optional `FrameCodec.Resync` lets stream protocols report the next
  plausible message boundary for garbage resynchronization.
- `CodecFunc` adapts plain functions — used by the test suite.

## Session

`NewSession(codec, transport, options)` binds a codec to any
`ByteTransport` (`adapter.Transport` satisfies it). `Transact(ctx, req)`:

1. Validates and marshals the request; `ErrUnsupported` if the codec
   can't express it.
2. Writes the wire bytes, recording the host→ECU message.
3. Reads and unmarshals messages until a completion policy fires:
   - default: first ECU→host message, then idle-timeout drain
   - `Options.Complete` predicate for multi-frame answers
   - idle timeout ends the exchange shortly after the last byte
4. Bounds: `ResponseTimeout` (whole exchange), request `Timeout`
   override, `MaxBytes` cap, context cancellation (deadline-clamped
   reads).

Every observed message — noise, negative responses, partial traffic — is
preserved in `Response.Messages`; nothing is silently dropped.

## Typed errors

`ErrTimeout`, `ErrMalformed`, `ErrClosed`, `ErrUnsupported`,
`ErrNoTransport`, plus `NegativeResponseError{Code, Detail}` for
protocol-level NAKs decoded by codecs.

## Safety boundary

`Transact` sends whatever the codec marshals — this layer is
transport/protocol plumbing, not policy. The `internal/safety` operation
gate (`identify`, `read-data`, `read-dtc`, `read-measurement`) remains the
enforcement point above it. Protocol codecs must not add write-capable
services without an explicit safety review.

## Tests

`session_test.go` runs the whole contract against
`adapter/fake.Transport` — round-trip with multi-message answers, idle and
response timeouts, request-level timeout override, malformed codec errors,
unsupported marshal, early-completion predicate, byte cap, context
cancellation, negative-response typing, and the `ByteTransport`
conformance check. No hardware imports anywhere in the test path.

## Limitations / unresolved

- No concrete codec ships yet — OBD-II (#30/#31) and SSM (#32) implement
  `Codec` next.
- `Unmarshal` resynchronization is codec-defined; `FrameCodec.Resync`
  exists but no production codec uses it yet.
- Addressing is modeled but unused until a protocol needs it.
