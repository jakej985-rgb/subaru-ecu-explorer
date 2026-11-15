# Arduino Interface — Bill of Materials (P2.7)

Design target: v0.1 prototype (perfboard). Quantities are per interface
unit. **Nothing here is fitted or vehicle-tested.** Sources are
engineering distributors; alternates are documented so a shortage does
not silently change the design.

## Controller

| # | Item | Spec | Source / example | Notes |
|---|------|------|------------------|-------|
| 1 | Arduino Mega 2560 R3 | ATmega2560, 16 MHz, 5 V logic, ATmega16U2 USB | Arduino SA / Adafruit / reputable clone | Clones vary; verify 16U2 (not CH340) for `/dev/ttyACM*` enumeration and bootloader quality |

## Vehicle-side transceivers

| # | Item | Spec | Source / example | Notes |
|---|------|------|------------------|-------|
| 2 | K-line transceiver | ISO 9141 physical layer, 8–18 V supply, 5 V CMOS logic I/O, integrated protection | NXP **MC33290** (SO-8) — datasheet Rev 8.0, 8/2008 | Purpose-built for ISO 9141 K-line; **NRND/discontinued** — last-time-buy or approved alternate required |
| 2a | Alternate K-line transceiver | K-line compatible, 3.3/5 V logic | NXP **TJA1021** (SO-8/HVSON8) — datasheet Rev 9, 10 Feb 2023 | LIN-family; "K-line compatible" per datasheet but open-drain LIN output needs validation against ISO 9141 dominant/recessive levels — bench-test before adopting |
| 2b | Alternate | ISO 9141 K-line | Freescale/NXP **MC33662** | Pin-compatible family with TJA1021; same validation caveat |
| 3 | L-line transceiver | same as #2 | second MC33290 footprint | Unpopulated in v0.1 |

## Protection & power

| # | Item | Spec | Source / example | Notes |
|---|------|------|------------------|-------|
| 4 | Input fuse | 2 A blade/PPTC | any automotive | On DLC pin 16 feed only |
| 5 | Reverse-polarity protection | P-MOSFET ideal diode (e.g. DMP2035U-class) or 3 A Schottky (SS34/1N5822) | TI / Vishay / generic | FET preferred (no forward drop); diode acceptable v0.1 |
| 6 | Supply TVS | standoff ≥16 V, clamp ≤40 V, load-dump rated | e.g. SMBJ18A / SMBJ24A class | Across 12 V rail after fuse |
| 7 | Signal TVS | bidirectional, VRWM ≥16 V, ESD-rated | e.g. PESD1CAN-class or equivalent single-line TVS | On K/L lines to ground; exact part at schematic review |
| 8 | PTC fuses | 500 mA hold (signals), 2 A (power) | Bourns MF-R/MF-MSMF | Resettable preferred for bench work |
| 9 | Series resistors | 100 Ω–1 kΩ, 1 %, 1/4 W | generic | Values finalized vs transceiver datasheet app circuit |
| 10 | Buck regulator | 12 V→5 V, ≥1 A, automotive temp | e.g. Recom R-78E5.0 or equivalent module | Powers Mega 5V pin + transceiver VDD; linear regulators discouraged (heat) |
| 11 | Bulk/decoupling caps | 100 µF electrolytic + 100 nF ceramic per rail | generic | At buck output and transceiver VDD |
| 12 | Pull-ups/bias | per MC33290 typical application | per datasheet Rev 8.0 | Values TBD at schematic review |

## Connectors & mechanics

| # | Item | Spec | Source / example | Notes |
|---|------|------|------------------|-------|
| 13 | OBD-II breakout/tap | passive breakout exposing pins 16/3/4/5/7/15 (+12/13 later) | commercial OBD-II Y-cable or breakout board | Wire only to verified positions; never probe unknown pins |
| 14 | SSM1 experiment header | JST-XH or 0.1" header, 3–4 pos | generic | Pins 12/13 assignment UNVERIFIED |
| 15 | Expansion footprints | 0.1" headers: UART2/UART3, CAN (MCP2551/TJA1050 footprint, unpopulated) | generic | Documented expansion only |
| 16 | ICSP header | 2×3 0.1" | generic | Firmware debug/recovery |
| 17 | Enclosure + strain relief | non-conductive project box | generic | Keeps board off bench/car surfaces |
| 18 | Wiring | 20–22 AWG stranded, automotive TXL preferred | generic | Short runs; twisted pair for K/L if both populated |

## Explicitly NOT in v0.1

- CAN transceiver (MCP2551/TJA1050) — footprint only.
- Galvanic isolation (digital isolator + isolated DC-DC) — Rev B option.
- Any SRS/airbag circuit hardware — prohibited entirely.
- Second K-line channel populated — footprint only.

## Open BOM questions

- MC33290 is NRND: confirm remaining stock or qualify TJA1021/MC33662
  with a bench ISO 9141 test before committing.
- Exact signal TVS part and series-R values require the MC33290
  datasheet typical-application review (Rev 8.0, Aug 2008).
- Whether the Mega clone's USB bridge enumerates as `/dev/ttyACM*`
  reliably — verify on the actual board before relying on discovery.
- Isolation decision deferred to field experience; documented as Rev B.
