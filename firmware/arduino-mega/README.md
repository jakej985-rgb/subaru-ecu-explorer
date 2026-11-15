# Arduino Mega Diagnostic Firmware

Read-only acquisition firmware for the Mega 2560 R3 interface (issue #64).
Wire format is defined in `docs/hardware/ARDUINO_SERIAL_PROTOCOL.md` and
must stay byte-compatible with `internal/adapter/arduino/frame.go`.

## Build

Requires the Arduino AVR core (Arduino IDE 1.8+/2.x or `arduino-cli`).

```sh
# arduino-cli example
arduino-cli core install arduino:avr
arduino-cli compile --fqbn arduino:avr:mega firmware/arduino-mega
arduino-cli upload  --fqbn arduino:avr:mega -p /dev/ttyACM0 firmware/arduino-mega
```

Board: **Arduino Mega or Mega 2560** (`ATmega2560`). Serial is native USB
CDC at 115200 8N1 (`/dev/ttyACM*` on Linux).

## Safety properties

- Downlink ops are allowlisted: `OP_SELF_TEST` only. Everything else gets
  `ERR_UNSUPPORTED_OP` + ACK; **zero bytes reach any vehicle line**.
- `VEHICLE_KLINE_ENABLED` is `0` and no vehicle transmit path exists in
  this build.
- Bounded TX buffer (drops new frames on overflow rather than blocking),
  bounded RX frame (512 B payload max), parser resync on bad magic/CRC,
  500 ms RX stall timeout, 8-consecutive-error tracking in heartbeat,
  1 s hardware watchdog.
- Stateless reconnect: the host resynchronizes by magic scan; sequence
  numbers expose loss. Firmware sends HELLO + STATUS on every boot.

## Files

- `diag_iface.ino` — state machine, frame builders, RX parser.
- `protocol.h` / `protocol.cpp` — constants + CRC-16/CCITT-FALSE shared
  with the Go codec.

## Protocol summary

Firmware → host: `HELLO` (boot), `STATUS` (state changes), `HEARTBEAT`
(1 s), `CAPABILITY` (on host request), `DATA` (self-test result),
`ERROR` (rejections/faults), `ACK` (downlink seq echo).

Host → firmware: `ACK`, `DATA` (op-coded commands; allowlist enforced),
`CAPABILITY` (capability request). Other types ignored.
