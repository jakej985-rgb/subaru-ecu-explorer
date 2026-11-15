# Arduino Mega 2560 Diagnostic Interface — Architecture (P2.7)

Status: **design proposal — not yet reviewed or built.** No vehicle
connection is permitted until the safety review and bench validation
(issues #63/#66) are complete.

## Role of the Mega 2560 R3

The Arduino Mega 2560 R3 (ATmega2560 @ 16 MHz, **5 V logic**, native USB
via ATmega16U2 bridge) is the **prototype controller only**. It is not a
vehicle electrical interface: every vehicle-facing signal passes through
the protection/transceiver stage in this document before reaching any MCU
pin. The board communicates with the host Go service over USB serial
(`/dev/ttyACM*`) using the versioned frame protocol in
`internal/adapter/arduino` — USB connectivity is never treated as proof
of vehicle protocol compatibility.

## Block diagram

```
                    ┌────────────────────────────────────────────┐
  DLC pin 16 (+12V) │  Power path                                │
  (always live) ───►│  fuse → reverse-polarity → TVS → bulk C    │
                    │       → 12V→5V automotive buck ──► 5V rail │
                    └────────────────────────────────────────────┘
  DLC pins 4/5      │  Chassis/signal ground → star ground point  │
  (ground)     ───►│  (single-point bond to power ground)        │
                    └────────────────────────────────────────────┘

  DLC pin 7 (K-line, ISO 9141 / SSM2 candidate)          DLC pin 15 (L-line)
        │                                                        │
        ▼                                                        ▼
  ┌───────────┐   ┌───────────────────────────────────────┐   ┌───────────┐
  │ PTC fuse  │   │  K-line protection: series R + TVS    │   │ (same,    │
  │ + TVS     │──►│  (VRWM ≥16 V)                          │──►│  optional │
  └───────────┘   └──────────────────┬────────────────────┘   │  channel) │
                                     ▼                        └─────┬─────┘
                              ┌─────────────┐                        │
                              │  MC33290    │  ISO 9141 K-line       │
                              │  K-line     │  transceiver           │
                              │  transceiver│  (12 V bus side)       │
                              └──────┬──────┘                        │
                    TXD ──────────────┤RXD                            │
                    (Mega UART1, 5V)  │                               │
                                     ▼                               ▼
                    ┌─────────────────────────────────────────────────┐
                    │  Arduino Mega 2560 R3 (5 V logic)               │
                    │  UART1: pins 18 TX1 / 19 RX1  → K-line          │
                    │  UART2: pins 16 TX2 / 17 RX2  → SSM1 header*    │
                    │  UART3: pins 14 TX3 / 15 RX3  → expansion       │
                    │  USB (ATmega16U2) → host Go service             │
                    └─────────────────────────────────────────────────┘

  * SSM1 uses manufacturer-discretionary pins (12/13 per community
    sources — UNVERIFIED on this vehicle) at 5 V TTL. It routes to a
    separate protected header, never to the K-line stage.
```

## Signal paths and coverage

| Vehicle signal | DLC pin | Path | Status |
|---|---|---|---|
| +12 V battery | 16 | fused, reverse-polarity-protected, TVS-clamped, buck to 5 V | designed |
| Ground | 4/5 | star ground to power-stage ground | designed |
| ISO 9141 K-line / SSM2 | 7 | PTC + TVS + series R → MC33290 → UART1 | designed, unverified on vehicle |
| ISO 9141 L-line | 15 | optional second channel footprint | footprint only |
| SSM1 RX/TX (5 V TTL) | 12/13? | separate protected header → UART2; **not wired in v0.1** | **pin assignment unverified** |
| CAN-H/L | 6/14 | unpopulated footprint (not expected 1999) | expansion only |
| J1850 | 1/2 | not connected | n/a |
| Pin 3 | 3 | **not connected — manufacturer-discretionary, not a ground** | prohibited |
| Pins 8–11 | — | **not connected — unknown functions** | prohibited |

Pins 12/13 remain disconnected in v0.1; the SSM1 header is wired only
after factory pin verification (v0.2).

SSM coverage honesty: K-line (MC33290) covers the SSM2/ISO-9141 physical
layer only. **SSM1's 5 V TTL two-wire scheme is a separate path** on
unverified pins and requires its own level/protection review before any
connection. No claim is made that this design covers every SSM mode.

## Power architecture

- Vehicle +12 V enters through a 2 A automotive blade fuse, then a
  reverse-polarity stage (Schottky diode or P-MOSFET ideal diode), then a
  TVS clamp rated for load-dump transients, then bulk capacitance.
- A 12 V→5 V automotive-rated buck regulator (≥1 A) feeds the Mega's
  5 V pin and the MC33290 VDD. The Mega's VIN pin is not used for vehicle
  power (VIN expects 7–12 V; the regulated 5 V rail is cleaner).
- Feeding the 5 V pin does **not** disconnect the Mega R3's USB VBUS
  path: with VIN unused, the on-board USB power MOSFET stays on and
  conducts in both directions. A diode on the external rail only blocks
  USB→buck current; it does **not** prevent buck→host backfeed. Before
  the external 5 V rail and host USB are connected simultaneously, the
  schematic review must confirm isolation on the **USB branch** — e.g. a
  USB cable/adapter with VBUS disconnected, or reverse-current protection
  (ideal diode / load switch) between host VBUS and the board 5 V net.
  No backfeed into the host or the buck is allowed.
- All grounds meet at a single star point beside the protection stage;
  no ground-loop paths through signal wiring.

## Protection requirements (every vehicle-facing signal)

1. Series resistance (≈100 Ω–1 kΩ, value set at schematic review per
   transceiver datasheet) on every signal entering the MCU domain.
2. TVS clamp to the local ground on every vehicle line, VRWM ≥16 V for
   12 V-referenced signals.
3. PTC fuse on the K/L signal lines and the +12 V feed.
4. Reverse-polarity protection on the power input.
5. Bulk + decoupling capacitance on each rail.
6. No pin may be driven by the MCU toward the vehicle; transceivers are
   open-drain/high-impedance when unpowered (MC33290 standby).
7. Revision-B option: full galvanic isolation (digital isolator +
   isolated DC-DC) if field experience demands it — documented as an
   expansion, not assumed for v1.

## Connectors

- Vehicle side: commercial OBD-II breakout/tap wired **only** to verified
  positions (16, 4/5, 7, 15; 12/13 only after factory pin verification;
  pin 3 never).
  No probing of unknown terminals.
- MCU side: pin headers per the connection table; JST-XH for the SSM1
  experiment header; USB-A to the host.

## Board, logic, and firmware

- Logic level: 5 V throughout the MCU domain (Mega native); the K-line
  transceiver presents 5 V CMOS to the MCU and 12 V to the bus.
- Serial: Mega UART1 (pins 18/19) for K-line; UART2 (16/17) reserved for
  SSM1; UART3 (14/15) expansion.
- USB: native ATmega16U2 CDC → `/dev/ttyACM*`; the Go service owns the
  connection exclusively via `internal/adapter`.
- Firmware update/debug: USB bootloader (avrdude) or ICSP header;
  firmware is the read-only acquisition firmware of issue #64.
- Board revision: **v0.1 prototype** — soldered perfboard/prototype
  layout first; custom PCB only after bench validation (#66).

## Schematic-ready connection table

| Net | From | To | Notes |
|-----|------|----|-------|
| VBAT_FUSED | DLC16 → fuse → rev-polarity → TVS → bulk C | buck IN | 12 V vehicle rail |
| +5V | buck OUT | Mega 5V pin, MC33290 VDD, ISP pull-ups | regulated rail |
| GND_STAR | DLC 4/5, all stage grounds | single star point | no loops; DLC3 not connected |
| KLINE_BUS | DLC7 → PTC → TVS → series R | MC33290 K pin | 12 V bus side |
| K_TXD | MC33290 TXD → series R | Mega pin 19 (RX1) | 5 V logic |
| K_RXD | Mega pin 18 (TX1) → series R | MC33290 RXD | 5 V logic |
| K_INH | Mega GPIO (TBD at schematic) | MC33290 INH | standby control, optional |
| LLINE_* | DLC15 → same protection | second transceiver footprint | unpopulated v1 |
| SSM1_RX/TX | DLC12/13? → PTC+TVS+series R | JST header → Mega UART2 | pins unverified |
| CAN_* | DLC6/14 → footprint only | unpopulated | expansion |
| USB | Mega USB | host Go service | `/dev/ttyACM*` |

The machine-readable net list lives in
`docs/hardware/arduino-interface-netlist.csv` — one row per connection,
ready to drive a schematic capture. Pin numbers for the MC33290 follow
its SO-8 datasheet Rev 8.0 (2008) and must be re-verified against the
actual package marking at schematic review.

Exact resistor/capacitor values, transceiver app-circuit components, and
connector part numbers are finalized in the schematic review — values
above are starting points, not verified designs.

## Expansion plan

1. v0.1: K-line only, non-isolated, perfboard.
2. v0.2: populated L-line channel, SSM1 header after pin verification.
3. v0.3: CAN transceiver footprint populated only if a supported vehicle
   variant requires it.
4. Rev B: custom PCB + optional galvanic isolation.

## Safety review gate

This design does not authorize any vehicle connection. Required before
hardware meets the car: schematic review against MC33290 datasheet Rev
8.0 (2008) and TJA1021 Rev 9 (2023), protection calculations, bench test
with simulated DLC (#66), and explicit operator approval.
