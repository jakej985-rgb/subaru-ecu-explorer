# ELM327 Interface

`internal/protocol/elm327` implements ELM327 command serialization and
response parsing as a `protocol.Codec` (issue #30, Phase 3.2). It covers
initialization, AT commands, OBD mode requests, echo/header/space modes,
adapter variants, and ELM error statuses — adapter-independent and
fixture-tested.

## Wire model

- Requests marshal to text: `AT<cmd>\r` for adapter commands, uppercase
  hex + `\r` for OBD modes (`Mode(0x01, 0x0C)` → `010C\r`).
- Responses are `>`-terminated blocks; `Codec.Unmarshal` returns
  `ErrIncomplete` until a prompt arrives, then parses one block into a
  `protocol.Message` with the raw block preserved in `Raw`.
- `Codec.Headers` selects header-mode line parsing (`ATH1` output:
  `<can-id> <count> <data...>`). Production init uses `ATH0`; the flag
  exists for evidence capture and variant testing.
- The codec remembers the last marshaled command so echoed command lines
  (`ATE1`) are suppressed during parsing.

## Response classification (`Metadata["kind"]`)

| kind | Meaning |
|---|---|
| `data` | positive response; `Payload` = decoded data bytes |
| `ok` | AT acknowledgement (`OK`) |
| `error` | ELM status (`UNABLE TO CONNECT`, `CAN ERROR`, `BUS ERROR`, `BUS INIT: ERROR`, `STOPPED`, `NO DATA`, `?`, `ERROR`) — `ErrorOf` yields `*NegativeResponseError` with a stable code |
| `status` | informational (`BUS INIT: OK`, `SEARCHING...` alone) |
| `banner` | adapter text (ATI/ATZ identification) |
| `malformed` | undecodable hex or header-count mismatch |
| `prompt`/`echo-only` | empty or echo-only blocks |

In-band `SEARCHING...` and `BUS INIT...` lines are recorded in
`statusLines` without corrupting data parsing; ISO-TP `...` continuation
markers are skipped.

## Initialization

`Initialize(ctx, session, onMsg)` runs the conservative read-only
sequence `ATZ → ATE0 → ATH0 → ATS0 → ATSP0` (reset, echo off, headers
off, spaces off, automatic protocol), aborting on the first error.
`Identify` adds `ATI` (and optional `ATDP`) returning `AdapterInfo`:
raw banner, parsed version, `Clone` heuristic flag, reported protocol.

The clone heuristic only *suspects* — it never certifies compatibility;
capability claims still require behavioral evidence.

## Timeouts

Session-level bounds come from `protocol.Session` (`ResponseTimeout`,
idle timeout, byte cap). `elm327.CommandTimeout` (2 s) is the suggested
per-request `Request.Timeout` for AT commands; OBD requests that search
for ECUs need longer and should set `Request.Timeout` explicitly.

## Transcript fixtures

`tests/fixtures/elm327/*.txt` are raw wire captures replayed through
`fake.Transport` + `protocol.Session`:

- `mode01-data`, `echo-on`, `headers-on`, `multiline-searching` — data paths
- `unable-to-connect`, `can-error`, `bus-init-error`, `stopped`,
  `no-data`, `unknown-cmd`, `searching-no-data` — error paths
- `malformed-hex` — undecodable data
- `at-identity`, `at-ok` — adapter command responses

## Safety boundary

This codec serializes whatever `Request` it is given — including OBD
modes that could write. The `internal/safety` operation gate remains the
enforcement point; callers must restrict requests to the read-only
allowlist (`identify`, `read-data`, `read-dtc`, `read-measurement`).

## Limitations

- Parser assumes `\r`-terminated lines inside `>`-terminated blocks; some
  clones emit `\n` — `FieldsFunc` accepts both.
- Header-mode validation requires `<count>` to equal the number of
  following byte pairs on the line; exotic formatting fails closed as
  `malformed`.
- No ISO-TP reassembly across multiple ELM responses yet — multi-frame
  answers arrive as separate `data` messages; #31 handles OBD-layer
  semantics.
- Clone detection is heuristic only.
