# Protected OBD-II Interface (P2.2)

Scope: the OBD-II data link connector (DLC) path for the 1999 USDM Legacy
Outback, the protected-interface requirement any hardware must meet, and the
host-side transport support that is verified so far. Issue #24.

## Sources and evidence rules

- **SAE J1962** defines the generic 16-position DLC pinout. Assignments
  below are *standard roles*, recorded in
  `data/definitions/connectors/1999-usdm-legacy-dlc.json` with standard
  evidence. A standard role never implies the pin is populated or wired on
  this vehicle.
- **Factory documentation** reviewed for the 1999 USDM Legacy describes a
  16-pin DLC shared by OBD-II and Subaru SSM
  (`docs/SSM_PROTOCOL.md`), but this repository holds **no verified
  per-pin vehicle mapping**. Every vehicle-specific claim stays `unknown`.
- **No guessed pin mapping.** Pin functions are never inferred from wire
  color, connector position, or generic tables. Manufacturer-discretionary
  positions (8–13) carry no vehicle function.

## DLC pin map (generic SAE J1962 assignments)

| Pin | Function (standard role) | Vehicle status |
|----:|--------------------------|----------------|
| 1   | J1850 PWM+               | unverified — not expected on 1999 EJ25 |
| 2   | J1850 PWM−               | unverified |
| 3–5 | ground (chassis/signal)  | unverified subclass; verify vs factory diagram |
| 6   | CAN-H                    | unverified — CAN not expected on 1999 |
| 7   | ISO 9141-2 K-line        | expected OBD-II data path; vehicle wiring unverified |
| 8–13| manufacturer-discretionary | **unknown** — possible SSM territory; do not assume |
| 14  | CAN-L                    | unverified |
| 15  | ISO 9141-2 L-line        | expected OBD-II data path; vehicle wiring unverified |
| 16  | +12 V battery (unswitched)| standard role; vehicle-side verification pending |

Machine-readable form: `data/definitions/connectors/1999-usdm-legacy-dlc.json`
(schema `schemas/connector-map.schema.json`, Go type `vehicle.Connector`).

### Supported signal paths (documented, not yet exercised)

- **OBD-II generic path:** ISO 9141-2 K/L on pins 7/15 through a certified
  ELM327-compatible adapter. This is the only host-side path with a
  commercially protected interface available today.
- **Subaru SSM path:** separate terminals per factory documentation, exact
  pins **unknown** — tracked under issue #25 / `docs/SSM_PROTOCOL.md`.
- **Pins 8–13** may carry manufacturer-specific signals including SSM;
  they are explicitly *not* claimed by any definition.

## Protected automotive interface requirement

Any hardware that touches the DLC — commercial or custom — must meet all of
the following before vehicle connection:

1. **No bare MCU GPIO** on any vehicle line, ever (`docs/SAFETY.md`).
2. **Certified/protected front end.** Prefer a commercially certified
   ELM327-compatible dongle (its interface protection is the vendor's
   responsibility and is field-proven). Custom circuitry must add, at
   minimum: automotive-rated transceiver or opto-isolation on every signal
   line, TVS/ESD clamping, series current limiting, reverse-polarity
   protection, and a fuse on the +12 V feed.
3. **Power discipline.** Pin 16 is always-live +12 V; the interface must
   withstand load dump and reverse battery conditions. No interface may
   source voltage onto diagnostic lines.
4. **Host-side transport only (verified today).** The Go service talks to
   ELM327 adapters over USB/serial/Bluetooth RFCOMM via the `adapter.Driver`
   contract (`docs/ADAPTER_ARCHITECTURE.md`). No microcontroller sits in
   this path, so the vehicle-facing protection lives entirely inside the
   certified dongle.
5. **Custom paths are gated.** The Arduino acquisition path
   (`arduino-serial`) requires the protected transceiver stage designed
   under issues #62/#63 before any vehicle wiring — the board alone is not
   a vehicle interface.

## Hardware validation gate

Connection to the vehicle is blocked until, in order:

- [ ] Pin mapping confirmed against the correct factory wiring diagram for
      the exact vehicle configuration (photograph the connector first).
- [ ] Interface protection reviewed and documented (isolation, TVS,
      current limiting, fusing).
- [ ] Bench test with a simulated DLC (power supply + K/L loopback or
      function generator) passing framing/protocol tests.
- [ ] Read-only behavior verified: identification requests only; no
      actuator, write, security, or flash commands.
- [ ] Operator confirmation of connector identity immediately before
      connecting.

## Unresolved questions / limitations

- Exact vehicle-side population of every DLC pin (factory wiring diagram
  not yet in-repo).
- Whether Subaru SSM uses DLC pins 8–13 or a separate dedicated connector
  on this model year.
- Ground subclass per pin (chassis vs signal) — verify per factory diagram.
- No ELM327 or SSM driver is implemented yet; the adapter contract exists
  (`internal/adapter`) but no transport has been exercised against hardware.
- Whether the DLC connector location/shape matches generic OBD-II
  expectations on this vehicle — confirm physically before use.
