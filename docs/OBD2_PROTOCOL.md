# Generic OBD-II Protocol Support

`internal/protocol/obd2` implements documented SAE J1979 services and PIDs
needed by the target vehicle (issue #31, Phase 3.3): typed decoding
definitions, units, scaling, and explicit unsupported-PID handling. It
sits on top of the ELM327 codec (`internal/protocol/elm327`) and the
adapter-independent `protocol.Session`.

## Design rules

- **Registry-driven**: only documented generic PIDs are implemented. A
  mode/PID absent from the registry returns `ErrUnsupportedMode` /
  `ErrUnsupportedPID` — before any bytes reach the transport.
- **No fabrication**: short payloads → `ErrShortPayload`; ECU "NO DATA"/
  "STOPPED"/ELM errors → `ErrNoData`; header mismatches and bad ASCII →
  `ErrMalformedData`. Unknown values stay errors, never guesses.
- **Read-only expressiveness**: `Client.Query` can only express
  mode+PID requests present in the registry. Write-capable modes
  (0x02 clear-DTC, 0x04/0x08 controls, 0x09 writes, security access)
  are not in the registry and cannot be sent through this API.

## Supported registry

### Mode 0x01 — current powertrain data

| PID | Name | Bytes | Unit | Decode |
|---|---|---|---|---|
| 0x00 | supported-pids-01-20 | 4 | — | bitmask → supported PID list |
| 0x04 | calculated-load | 1 | % | A·100/255 |
| 0x05 | coolant-temp | 1 | °C | A−40 |
| 0x06 | short-term-fuel-trim-bank1 | 1 | % | (A−128)·100/128 |
| 0x07 | long-term-fuel-trim-bank1 | 1 | % | (A−128)·100/128 |
| 0x0B | intake-manifold-absolute-pressure | 1 | kPa | A |
| 0x0C | engine-rpm | 2 | rpm | (A·256+B)/4 |
| 0x0D | vehicle-speed | 1 | km/h | A |
| 0x0F | intake-air-temp | 1 | °C | A−40 |
| 0x10 | mass-air-flow-rate | 2 | g/s | (A·256+B)/100 |
| 0x11 | throttle-position | 1 | % | A·100/255 |
| 0x1F | run-time-since-engine-start | 2 | s | A·256+B |
| 0x21 | distance-traveled-with-mil-on | 2 | km | A·256+B |
| 0x2F | fuel-tank-level-input | 1 | % | A·100/255 |
| 0x31 | distance-since-codes-cleared | 2 | km | A·256+B |

### Mode 0x03 — stored DTCs

Repeating 2-byte DTCs decoded to `DTC{Code, Type, Confirmed, TestFailed}`.
Mode 03 responses carry no PID byte; the `0x43` header is validated and
stripped from every response line before DTC pairs are joined.

### Mode 0x09 — vehicle information

| PID | Name | Decode |
|---|---|---|
| 0x02 | vin | 17-byte ASCII (one count byte skipped after header) |
| 0x04 | calibration-id | ASCII |
| 0x0A | ecu-name | ASCII |

Mode 09 responses carry a one-byte item count between the mode/PID header
and the data (`extraSkip` in the registry).

## Client API

`NewClient(session)` requires an elm327-backed `protocol.Session`.
`Query(ctx, mode, pid)` marshals the request, validates the response
header (`0x40|mode`, PID echo), strips mode-09 count bytes, and returns a
`Measurement{Mode, PID, Name, Raw, Value, Unit}`.

Typed conveniences: `RPM`, `CoolantTemp`, `VehicleSpeed`,
`ThrottlePosition`, `SupportedPIDs`, `VIN`, `ECUName`, `StoredDTCs`.

## Evidence and provenance

`Measurement.Raw` preserves the exact ECU payload bytes; `Value` is the
decoded/scaled result. Nothing is inferred — a PID the ECU does not
support surfaces as `ErrNoData` or a decode error, not a default.

## Tests

`obd2_test.go` covers decode math (RPM, coolant, fuel trims, MAP, MAF,
supported-PID bitmask, DTC→P-code, VIN), all typed error paths
(unsupported mode/PID, short payload, malformed data, header mismatch,
no-data), and end-to-end client queries against `fake.Transport` with
scripted ELM327 transcripts — including proof that unsupported requests
write zero bytes.

## Limitations / unresolved

- Mode 01 PID 0x00 only reports the 0x01–0x20 range; the extended
  0x20–0x3F range (PID 0x20) is not implemented.
- Mode 03 only; pending/permanent DTC modes (0x07/0x0A) deferred to the
  DTC work (#36).
- VIN decode is ASCII extraction only; check-digit/WMI interpretation
  lives in `internal/vehicle` (`DecodeVIN`), not here.
- No ISO-TP reassembly beyond what the ELM327 layer concatenates; very
  large mode-09 payloads depend on adapter behavior.
- Subaru-specific SSM decoding is a separate codec (#32), not generic OBD-II.
