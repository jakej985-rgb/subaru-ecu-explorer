# Arduino Bench Validation

Staged validation of the Arduino Mega 2560 acquisition interface **before
any vehicle connection** (issue #66). This document defines the procedures,
instrument settings, expected results, and the sign-off gate. Results are
recorded in §6; a test passes only when measured results match expected
results or the deviation is documented and accepted.

> **Scope:** bench only. No live vehicle connection is authorized by this
> document. The target ECU is never used as a test load. Vehicle connection
> requires the separate readiness checklist (§7) and explicit sign-off.

## 1. Test articles

| Item | Identifier |
|---|---|
| MCU board | Arduino Mega 2560 R3 (CH340 USB-serial) |
| Firmware | `firmware/arduino-mega`, v0.1.x, `VEHICLE_KLINE_ENABLED 0` |
| Protection/transceiver board | Per `ARDUINO_BOM.md` / `arduino-interface-netlist.csv` |
| K-line transceiver | MC33290 (or documented alternate after bench validation) |
| Host | Go service host running `tools/arduino-bench` |

Record actual firmware version and board revision in §6 before starting.

## 2. Equipment and settings

| Instrument | Setting / range | Purpose |
|---|---|---|
| Bench supply | 12 V ±0.2 V, current-limited ≤1 A | vehicle-side rail simulation |
| Digital multimeter | DC V, mA; continuity | voltages, currents, protection checks |
| Oscilloscope | ≥50 MHz, 10× probes | K-line waveform, transients, logic levels |
| USB current meter | 5 V rail | Mega + transceiver board draw |
| Signal generator / second Arduino | 12 V K-line idle level + ISO 9141 frame | transceiver bench stimulus |
| Host serial log | `tools/arduino-bench` output | frame-level evidence |

## 3. Electrical validation (vehicle-side board, unpowered vehicle)

Perform with the board **not connected to any vehicle**. Vehicle-side
terminals terminated per §3.1 first.

### 3.1 Termination fixture

- K-line terminal → 510 Ω pull-up to +12 V (typical K-line idle bias),
  or per transceiver datasheet test circuit. Document actual values.
- GND terminal → supply return.
- All other connector positions left open except pins tied to the
  protection chain under test.

### 3.2 Tests

| # | Test | Procedure | Expected | Result | Deviation |
|---|---|---|---|---|---|
| E1 | Power regulation | Apply 12 V to protection input; measure Mega 5 V rail and transceiver VCC | 5 V ±5 %, 12 V path fused | | |
| E2 | Current limit | Short protected output through ammeter | Fuse/PTC opens or limits ≤ rated hold current; no PCB damage | | |
| E3 | Reverse polarity | Apply −12 V to vehicle input for 10 s | No current path; no damage; correct on re-apply | | |
| E4 | Transient/TVS | Inject ISO 7637-style pulses per test plan (or documented equivalent) | Clamp within transceiver abs-max; no latch-up | | |
| E5 | K-line idle level | Scope K-line terminal at rest | ~12 V idle (K-line high), no oscillation | | |
| E6 | Transceiver TX/RX | Drive K-line with 12 V ISO 9141 frame from signal source; verify Mega UART sees correct bytes | Bit-exact at 10400/4800 baud per mode | | |
| E7 | Isolation/ground | Measure continuity vehicle-GND ↔ USB host-GND | No unintended ground loop; star ground only | | |
| E8 | Thermal | Board at rated continuous load, 30 min | No hotspot > component rating; no derating surprise | | |

**Blocker rule:** any failure in E1–E5 or E7 halts serial bring-up. Fix or
redesign before proceeding; record deviations explicitly.

## 4. Serial/firmware validation (USB only, vehicle side disconnected)

With the Mega on USB only (vehicle connector unplugged):

```sh
go run ./tools/arduino-bench -port /dev/ttyACM0 -tests all | tee bench-serial.log
```

| # | Check | Tool test | Expected |
|---|---|---|---|
| S1 | Handshake | `handshake` | HELLO/CAPABILITY, proto v1, read-only session |
| S2 | Self-test | `selftest` | 4-byte DATA reply, state = `ST_READY` |
| S3 | Op rejection | `reject` | `0x7F`/`0x00`/`0x02` → `ERR_UNSUPPORTED_OP`; empty payload → `ERR_BAD_PAYLOAD` |
| S4 | Heartbeat | `heartbeat` | 7-byte heartbeat ≤3 s |
| S5 | Corruption | `corrupt` | garbage injection → parser resyncs, self-test still answered |
| S6 | Capability | `capability` | request → `TypeCapability` frame with matching identity |
| S7 | Throughput | `throughput` | all `-count` self-tests answered; record ops/s |
| S8 | Reconnect | `reconnect` | close/reopen handshake clean, identity stable |

Also verify in the log: no `TypeError` with `ERR_VEHICLE_IO` during normal
operation; STATUS transitions observed are `ST_BOOT → ST_READY` only.

| # | Result | Measured | Deviation |
|---|---|---|---|
| S1–S8 | | | |

## 5. Fixture replay (no hardware required)

Regression suite runnable without the board:

```sh
go test ./internal/adapter/arduino/ -run 'Frame|Fixture|Driver' -v
```

Covers codec round-trips, corrupt-CRC/truncated/resync fixtures, and the
simulated-device driver tests (handshake, mismatch, timeout, round-trip).

## 6. Results record

Fill in during execution; attach `bench-serial.log` and instrument captures.

| Test | Date | Operator | Firmware | Result | Evidence |
|---|---|---|---|---|---|
| E1–E8 | | | | | |
| S1–S8 | | | | | |

**Bench validation complete when:** all E and S rows pass or carry an
accepted, documented deviation; log attached; firmware version recorded.

## 7. Vehicle-connection readiness checklist (separate gate)

Vehicle connection is a **separate approval**, not implied by this issue.
Before any vehicle-side connection:

- [ ] E1–E8 and S1–S8 complete with results recorded
- [ ] Factory connector pin map for the **exact** vehicle verified against
      primary documentation (not generic OBD-II diagrams)
- [ ] SSM protocol mode verified for the target ECU (SSM2 K-line vs SSM1
      confirmed; transmission ECU applicability resolved or explicitly
      excluded)
- [ ] Wiring schematic re-reviewed against `ARDUINO_VEHICLE_WIRING.md`
- [ ] Protection chain assembled and E-tests re-run on the final build
- [ ] Read-only operation list re-confirmed (`identify`, `read-data`,
      `read-dtc`, `read-measurement`); no writes/clears/actuators
- [ ] Operator present, engine off, keys controlled, second person briefed
- [ ] Sign-off recorded below by named reviewer

### Sign-off

| Role | Name | Date | Signature/approval ref |
|---|---|---|---|
| Bench validation | | | |
| Electrical/schematic review | | | |
| Vehicle-connection approval | | | |

## 8. Known limitations at time of writing

- Firmware v0.1 has **no vehicle transmit path** (`VEHICLE_KLINE_ENABLED 0`);
  E6 validates the transceiver electrically only — no SSM traffic is sent.
- K-line transceiver (MC33290) is NRND; alternate parts require their own
  bench validation before substitution.
- SSM1 (5 V TTL) path is not populated; pins 12/13 remain unwired.
- No live-vehicle evidence exists yet; all vehicle-specific claims remain
  unresolved in the data definitions.
