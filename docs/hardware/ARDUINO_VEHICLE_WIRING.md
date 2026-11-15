# Arduino Vehicle-Side Wiring (P2.8)

Status: **desk design only — no vehicle connection authorized.** This
document defines the electrical interface between the 1999 USDM Legacy
Outback diagnostic connector and the Arduino acquisition hardware
(`ARDUINO_INTERFACE_ARCHITECTURE.md`, `ARDUINO_BOM.md`). Every connection
below is labeled **CONFIRMED**, **INFERRED**, or **UNRESOLVED** so a
reviewer can distinguish evidence from research gaps.

## Evidence base and citations

| Source | What it supports | Citation |
|--------|------------------|----------|
| SAE J1962 (generic OBD-II DLC pinout) | Standard pin roles: pin 16 +12 V, pins 4/5 ground, pin 7 K-line, pin 15 L-line, pins 6/14 CAN, pins 2/10 J1850, pins 1, 3, 8, 9, 11–13 manufacturer-discretionary | `data/definitions/connectors/1999-usdm-legacy-dlc.json`; `docs/OBD2_INTERFACE.md` |
| Factory service documentation (1999 USDM Legacy, reviewed summary) | A 16-pin DLC is shared by OBD-II and Subaru SSM; SSM terminals are identified separately from ISO 9141 K-line; unsupported equipment must not be connected to the SSM circuit | `docs/SSM_PROTOCOL.md` §"1999 USDM Legacy connector notes" |
| FreeSSM project documentation | SSM2 = K-line physical layer (~4800 baud, 12 V TTL, DLC pin 7 per community cross-reference); SSM1 = two-wire 5 V TTL on manufacturer-discretionary pins | https://github.com/Comer352L/FreeSSM ; `docs/SSM_INTERFACE.md` |
| Community sources (SubaruFanClub.cz thread) | SSM1 on pins 12/13 at 1953 baud 8E1 5 V TTL; SSM2 on pin 7 | **UNVERIFIED for this vehicle — community only** |
| MC33290 datasheet Rev 8.0 (8/2008) | K-line transceiver electrical limits and typical application | NXP MC33290 |
| TJA1021 datasheet Rev 9 (10 Feb 2023) | Alternate transceiver limits | NXP TJA1021 |

**Missing evidence:** the actual factory wiring diagram / connector
views for this exact vehicle is not yet in the repository. Nothing below
claims vehicle-specific pin population beyond what the sources support.

## Connector reference

- **Type:** 16-position DLC, OBD-II physical style (per factory doc
  summary + SAE J1962). Orientation/pin numbering: SAE J1962 defines pin
  1 at the keyed corner; the vehicle-side mating face must be
  photographed and compared to the factory diagram before first use.
- **Vehicle connector location:** under-dash driver-side per generic
  Subaru layout — **UNRESOLVED**, confirm physically.
- **Tap method:** passive OBD-II breakout/Y-cable exposing only the
  positions in the wiring table. No breakout may expose or probe
  unlisted pins.

## Wiring table — vehicle side to interface board

Signal direction is from the **vehicle/ECU perspective** (K-line is
half-duplex; direction alternates).

| DLC pin | Vehicle signal | Evidence | Interface connection | Protection chain | Status |
|--------:|----------------|----------|----------------------|------------------|--------|
| 16 | +12 V battery (unswitched) | CONFIRMED (SAE J1962 role; vehicle population unverified) | J1 → fuse F1 → reverse-polarity D1 → TVS1 → bulk C1 → buck D2 | 2 A fuse, reverse-polarity stage, TVS ≥16 V standoff, bulk cap | CONFIRMED design path; vehicle verification pending |
| 4 | Chassis ground (subclass TBD) | CONFIRMED role / subclass UNRESOLVED | J1 → GND_STAR | star ground | CONFIRMED design path |
| 5 | Signal ground (subclass TBD) | CONFIRMED role / subclass UNRESOLVED | J1 → GND_STAR | star ground | CONFIRMED design path |
| 7 | ISO 9141 K-line (OBD-II + SSM2 candidate) | CONFIRMED standard role; SSM2 use INFERRED | J2 → PTC F2 → TVS2 → R1 → U1 (MC33290 K) → UART1 (Mega 18/19) | 500 mA PTC, TVS VRWM≥16 V, series R per datasheet | CONFIRMED design path; SSM2 function INFERRED |
| 15 | ISO 9141 L-line | CONFIRMED standard role | J3 → same protection → second transceiver footprint | same as pin 7 | FOOTPRINT ONLY (unpopulated v0.1) |
| 12 | SSM1 RX? (5 V TTL) | **UNRESOLVED** — community sources only | J4 → PTC + TVS + series R → UART2 header | 500 mA PTC, 5 V-rated TVS, series R | UNRESOLVED — do not connect until factory-verified |
| 13 | SSM1 TX? (5 V TTL) | **UNRESOLVED** — community sources only | J4 → same | same | UNRESOLVED |
| 6/14 | CAN-H/L | CONFIRMED standard roles; not expected 1999 | J5 footprint only | — | UNPOPULATED |
| 2/10 | J1850 bus+ / J1850 PWM bus− | CONFIRMED standard roles; not expected | not connected | — | NOT CONNECTED |
| 1, 3, 8, 9, 11 | manufacturer-discretionary (pin 3 is **not** a standard ground) | role CONFIRMED / function UNRESOLVED | **not connected** | — | PROHIBITED until identified |

## K-line signal path detail (SSM2 / ISO 9141)

```
ECU K-line (DLC7) ──► PTC F2 ──► TVS2 ──► R1 ──► MC33290 K (pin 5)
                                                        │
   Mega UART1 (5V) ◄── R3 ◄── MC33290 TXD (pin 3)      bus side 12V
   Mega UART1 (5V) ──► R2 ──► MC33290 RXD (pin 4)
   Mega GPIO ──► INH (pin 2, standby control, 10k pull-up TBD)
   VDD (pin 7) ◄── +5V rail     VBAT (pin 8) ◄── VBAT_CLAMPED
```

Half-duplex, single-wire + ground. The MC33290 handles the 12 V bus
levels and protection; the Mega sees only 5 V CMOS. Exact series-R and
bias values come from the MC33290 Rev 8.0 typical application at
schematic review — the values in the netlist are placeholders.

## SSM1 path (documented separately — UNRESOLVED)

Community sources place SSM1 on DLC pins 12/13 at 1953 baud 8E1, 5 V
TTL, two-wire RX/TX. This is **not verified for the 1999 USDM Legacy**:

- Pin assignment may differ; a dedicated connector may exist instead.
- 5 V TTL still requires PTC + TVS + series resistance before the Mega.
- The header (J4) exists in the design but stays **unpopulated/unwired**
  until the factory wiring diagram identifies the terminals.

## Electrical limits (from datasheets)

| Parameter | Limit | Source |
|-----------|-------|--------|
| K-line bus voltage | 0–12 V nominal; transceiver protects pin vs. shorts/overtemp | MC33290 Rev 8.0 |
| Transceiver supply | 8–18 V (VBAT pin); VDD 5 V logic | MC33290 Rev 8.0 |
| ESD | 8 kV attainable with external components | MC33290 Rev 8.0 |
| Data rate | design target ≤50 kBd capability; SSM2 ~4800 baud | MC33290 / FreeSSM |
| Mega logic | 5 V CMOS; never expose to >5.5 V | ATmega2560 datasheet |
| +12 V rail | fused 2 A; reverse-polarity + TVS before buck | this design |
| Power draw | budget ≤500 mA at 5 V rail | this design |

## Bring-up checklist (before any vehicle connection)

- [ ] Factory wiring diagram for the exact vehicle obtained; DLC pin
      functions and SSM terminals confirmed against it (photograph
      connector first).
- [ ] Schematic reviewed against MC33290 Rev 8.0 typical application;
      series-R/TVS values finalized.
- [ ] USB-branch isolation confirmed (host backfeed prevention — see
      ARDUINO_INTERFACE_ARCHITECTURE.md power section).
- [ ] Bench test: simulated DLC (12 V supply + K-line loopback or
      function generator) — framing, baud, protection verified (#66).
- [ ] Multimeter verification of every rail and pin voltage with the
      interface powered but **unconnected** to the vehicle.
- [ ] Operator sign-off recorded; read-only operations only.

## Explicitly unresolved

- Vehicle-side pin population for every DLC position.
- Whether SSM uses DLC pins 12/13 or a dedicated connector on this car.
- SSM1 vs SSM2 per-module determination (runtime system ID).
- Whether the transmission controller is reachable at all (FreeSSM:
  TCM support ~MY2001+).
- Exact L-line necessity for this vehicle's OBD-II implementation.
