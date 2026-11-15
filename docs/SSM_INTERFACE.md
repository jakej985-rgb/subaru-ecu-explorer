# Subaru SSM Hardware Interface (P2.3)

Scope: evaluation of Subaru Select Monitor (SSM) interface candidates, their
host connection types and electrical requirements, and the evidence status
for the 1999 USDM Legacy Outback. Issue #25. Companion documents:
`SSM_PROTOCOL.md` (protocol research), `OBD2_INTERFACE.md` (DLC/pin map and
protection requirements), `ADAPTER_ARCHITECTURE.md` (driver contracts).

## Protocol/electrical background (evidence summary)

| Path | Physical layer | Connector | Evidence |
|------|---------------|-----------|----------|
| SSM2 | Single-wire K-line, addressed, ~4800 baud, 12 V TTL | DLC pin 7 (shared ISO 9141 K-line) + ground | FreeSSM docs; community teardown notes |
| SSM1 / SSM0 | Two-wire RX/TX, 1953 baud 8E1 (or 500 baud), **5 V TTL** | Manufacturer-discretionary pins (12/13 per community sources) — **unverified on this vehicle** | Community sources only |
| SSM2-over-CAN | ISO 15765 CAN | later models only | not applicable to 1999 |

Important consequence: **SSM1 needs 5 V TTL level shifting on
dedicated pins** — a plain USB-serial K-line cable cannot serve it. Whether
the target vehicle's transmission controller uses SSM1 or SSM2 is unknown;
FreeSSM documents transmission support only from about MY2001, so the
1999 manual transmission controller may be unreachable via SSM entirely.

## Candidate interfaces

| Candidate | Host connection | Electrical | Evidence status | Verdict |
|-----------|-----------------|------------|-----------------|---------|
| Tactrix OpenPort 1.3 / 1.3U | USB → FTDI virtual COM | K/L line drivers, OBD-II connector, designed for 12 V K-line | Documented SSM K-line interface (OpenPort schematic public; FreeSSM/RomRaider field use on pre-CAN Subarus) | **Primary candidate** — inferred compatibility, pending bench + vehicle verification |
| VAG-COM KKL cable (FTDI clone) | USB → FTDI virtual COM | Varies by clone; FTDI chip required for reliable K-line | Community-documented for SSM K-line; clone quality inconsistent | Secondary candidate — lower confidence, must pass protection review |
| OpenPort 2.0 | USB | CAN-capable, microcontroller | Targets CAN-era vehicles | Not required for 1999; excluded |
| ELM327-compatible OBD-II adapter | USB/serial/Bluetooth | Certified OBD-II front end | Generic OBD-II protocol only | **Explicitly NOT an SSM adapter** — ELM327 OBD-II mode speaks ISO 15765/9141 OBD modes, not SSM. Acceptance criterion: the two paths stay separate drivers (`elm327-odbii` vs `subaru-ssm`) |
| Arduino/custom K-line | USB serial + custom front end | Requires designed transceiver stage (see #62/#63) | Not yet designed | Deferred — gated by hardware design issues |

Machine-readable candidates live in `data/definitions/interfaces/`:
`subaru-ssm` (path profile), `tactrix-openport-13`,
`vag-com-kkl-ftdi-serial`. All SSM protocol claims are `inferred`, never
`confirmed`, until exercised on the vehicle.

## Vehicle support evidence (FreeSSM)

- Models LEGACY/LIBERTY/OUTBACK/Baja/Impreza/Forester/Tribeca **from MY1999**
  for engine ECUs — covers the target.
- **Transmission control units supported only from ~MY2000–2001** — the
  target's manual transmission controller may not be reachable; treat as
  unresolved until an actual SSM session identifies it.
- FreeSSM is GPL-3.0 reference software; message definitions are XML per
  ECU and must match the identified ROM/system ID.

## Electrical & safety requirements

All requirements in `docs/OBD2_INTERFACE.md` apply unchanged. SSM-specific
additions:

- SSM2 K-line is a **12 V TTL** single-wire bus — the interface must drive
  and receive 12 V logic levels with proper protection; do not feed it
  into 5 V/3.3 V MCU pins directly.
- SSM1's 5 V TTL two-wire scheme needs level translation and is a separate
  hardware design — not covered by K-line cables.
- Read-only posture: identification and measurement requests only. No
  adjustments, coding, or actuator tests regardless of what the interface
  hardware is capable of.

## Driver status

No SSM driver is implemented yet — deliberately. The adapter contract
(`internal/adapter`) already defines `KindSubaruSSM`; per the issue,
drivers are implemented **only against verified interface behavior**, and
no SSM hardware has been bench- or vehicle-verified. Until then:

- `Manager.Connect` for a `subaru-ssm` candidate with no registered SSM
  driver returns `ErrUnsupported` — unknown hardware is rejected safely
  (tested).
- A future SSM driver will plug into `adapter.Driver` behind the same
  exclusive-ownership manager as ELM327 and Arduino.

## Unresolved questions

- Exact SSM connector terminals on this vehicle (DLC pins 12/13 vs
  dedicated connector) — requires factory wiring diagram.
- Whether the target's transmission controller speaks SSM at all.
- SSM1 vs SSM2 determination for each module (system ID at runtime).
- Whether a USB-serial dongle's line driver meets the 12 V K-line
  requirement for any specific clone cable.
- ECU/ROM identification — prerequisite for any message decoding.
