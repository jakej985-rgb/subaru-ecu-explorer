# Arduino Serial Protocol (P2.9)

Versioned USB-serial protocol between the Arduino Mega firmware and the
Go service. Transport framing is implemented in
`internal/adapter/arduino/frame.go` (Go) and
`firmware/arduino-mega/protocol.h` (firmware); the two must stay
byte-compatible. `tests/fixtures/arduino-serial/` pins the format.

## Frame format

All multi-byte fields are big-endian.

| Offset | Size | Field |
|-------:|-----:|-------|
| 0 | 2 | magic `0xA5 0x5A` |
| 2 | 1 | protocol version (= 1) |
| 3 | 1 | frame type |
| 4 | 2 | sequence number |
| 6 | 2 | payload length (≤ 512) |
| 8 | N | payload |
| 8+N | 2 | CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF) over bytes 0..8+N−1 |

Serial: USB CDC, 115200 8N1. The Go service owns the connection
exclusively (`internal/adapter` Manager).

## Frame types

| Type | Value | Direction | Payload |
|------|------:|-----------|---------|
| HELLO | 0x01 | FW→host | identity + capabilities (below) |
| DATA | 0x02 | both | acquisition sample batch (FW) or op command (host) |
| ACK | 0x03 | both | 2-byte echo of the acknowledged frame's seq |
| ERROR | 0x04 | FW→host | `code u8 | detail_len u8 | detail[≤48]` |
| CAPABILITY | 0x05 | FW→host | same layout as HELLO payload |
| HEARTBEAT | 0x06 | FW→host | `uptime_ms u32 | status u8 | rx_crc_errors u16` |
| STATUS | 0x07 | FW→host | `state u8 | detail_len u8 | detail[≤32]` |

HELLO/CAPABILITY payload: `protoVer u16 | caps u32 | fwLen u8 |
fwVersion[fwLen] | boardLen u8 | board[boardLen]` (strings ≤32 B).

Capability bits: `CAP_PERIODIC_SAMPLES` (1<<0), `CAP_EVENT_SAMPLES`
(1<<1), `CAP_SELF_TEST` (1<<2). v0.1 firmware advertises
PERIODIC_SAMPLES + SELF_TEST; no vehicle-I/O capability bits exist.

## Status states & error codes

States: `ST_BOOT 0x00`, `ST_READY 0x01`, `ST_ERROR 0x02`,
`ST_VEH_DISABLED 0x03` (vehicle transmit not fitted/approved).

Errors: `ERR_UNSUPPORTED_OP 0x10`, `ERR_BAD_PAYLOAD 0x11`,
`ERR_INTERNAL 0x12`, `ERR_VEHICLE_IO 0x20`.

## Downlink operations (host → firmware, FRM_DATA payload[0])

| Op | Code | Behavior |
|----|-----:|----------|
| OP_SELF_TEST | 0x01 | Returns FRM_DATA: `[tx_headroom u8][rx_crc_errors u16][state u8]` — no vehicle I/O |

**Everything else is rejected** with `ERR_UNSUPPORTED_OP` + ACK. There is
no vehicle-facing transmit path in this firmware build
(`VEHICLE_KLINE_ENABLED 0`); vehicle operations are added only after the
protocol and wiring are explicitly approved.

## Resync, timeouts, reconnect

- Parser scans for magic; bad magic/CRC/version/oversize payload drops
  the byte and resyncs. RX stall >500 ms resets the parser.
- Heartbeat every 1000 ms carries uptime, state, and cumulative CRC
  errors — the host detects silence as disconnect.
- Sequence numbers are per-direction and wrap at 16 bits; loss is
  detectable, ordering is not guaranteed across reconnects.
- Firmware is stateless across USB reconnect: HELLO + STATUS(BOOT/READY)
  are sent on every boot; no session state survives a reset except
  counters.

## Fixtures

`tests/fixtures/arduino-serial/*.hex` contain hex-encoded frames
produced by the Go codec and consumed by `frame_fixture_test.go`:
hello, heartbeat, status, data batch, error, capability, plus corrupt
(CRC-flipped) and resync (garbage-prefixed) streams.
