# Roadmap

## Phase 0 — Establish the vehicle baseline

- [ ] Record VIN-derived model/market information (keep the full VIN private).
- [ ] Photograph the ECU label and record part number/revision.
- [ ] Record ECU ROM ID once safely readable.
- [ ] Identify the exact diagnostic connectors from factory service documentation and physical inspection.
- [ ] Record current ABS warning-lamp behavior and existing symptoms without assuming faults.

**Exit criteria:** A vehicle profile with verified hardware identifiers and clearly marked unknowns.

## Phase 1 — Safe communication

- [ ] Select a known-compatible, electrically protected Subaru SSM interface.
- [ ] Verify host operating-system and driver support.
- [ ] Confirm connector power, grounds, and interface wiring against authoritative documentation.
- [ ] Establish read-only communication.
- [ ] Capture ECU identification and a raw communication trace.
- [ ] Document protocol, baud rate, addressing, timing, and interface settings only when observed.

**Exit criteria:** Repeatable ECU identification with a saved, sanitized trace.

## Phase 2 — Engine diagnostics

- [ ] Read and decode engine DTCs.
- [ ] Read supported live measurements.
- [ ] Map each measurement to its source definition, unit, scaling, and ECU variant.
- [ ] Add timestamped logging and CSV export.
- [ ] Build replayable fixtures from sanitized captures.

**Exit criteria:** Reproducible DTC and live-data sessions with validated decoding.

## Phase 3 — Multi-module ABS discovery and diagnostics

- [ ] Build a 1999 USDM Legacy ABS fitment catalog across supported model/body/engine/transmission variants.
- [ ] Separate ABS ECU repair part numbers from complete hydraulic-unit assembly part numbers.
- [ ] Add candidate lookup from vehicle configuration.
- [ ] Add manual part-number entry and evidence-backed profile matching.
- [ ] Map dedicated ABS diagnostic connectors and procedures by verified profile.
- [ ] Implement only documented, read-only ABS identification and blink-code procedures.
- [ ] Determine which profiles support electronic DTC retrieval and which expose live data.
- [ ] Add per-profile decoders and captured test fixtures.
- [ ] Validate the Outback manual-transmission profile against the actual vehicle.

**Exit criteria:** The detector reports confirmed/probable/candidate/unknown with evidence and never guesses a module identity. Read-only diagnostics work only for verified profiles.

## Phase 4 — Transmission and additional systems

- [ ] Determine whether the manual-transmission vehicle exposes a separately addressable transmission controller.
- [ ] Verify supported systems before implementing them.
- [ ] Add other modules only with vehicle-specific evidence.

## Phase 5 — Research and dashboard

- [ ] Build local session browser and live-data display.
- [ ] Add plotting, comparisons, and log annotations.
- [ ] Document ECU memory-read capabilities and limits.
- [ ] Investigate firmware access only through documented, non-destructive methods.
- [ ] Require explicit risk review before any write-capable feature is considered.

## Non-goals for early milestones

- ECU flashing or tuning.
- Arbitrary memory writes.
- ABS actuator tests.
- Immobilizer/security bypass.
- Assuming generic OBD-II provides Subaru-enhanced measurements.
- Assuming green engine test-mode connectors are ABS diagnostic connectors.
