# Subaru ECU Explorer — Roadmap

## Project scope

Initial target: **1999 USDM Subaru Legacy Outback, 2.5L EJ25, manual transmission**.

The first release targets this vehicle only. Reusable architecture is encouraged, but support for other years, markets, or models must be researched and validated separately.

## Locked architecture

- **Diagnostic backend:** Go.
- **User interface:** browser-based web dashboard.
- **Hardware:** ELM327-compatible OBD-II adapters and Subaru-specific SSM interfaces through separate interchangeable drivers.
- **Connection ownership:** a local Go service manages physical adapter connections.
- **Browser communication:** controlled local REST APIs and WebSocket streaming; no direct browser-to-vehicle connection.
- **Storage:** local-first diagnostic sessions and vehicle data.
- **Initial operation boundary:** read-only identification, supported DTC retrieval, supported live data, and logging.
- **Evidence:** vehicle-specific profiles, connector details, protocols, and capabilities require traceable sources and confidence.
- **Safety:** no arbitrary pin probing, connector bridging, actuator commands, ECU writes, security bypass, or flashing in early milestones.

ELM327 compatibility does not imply Subaru SSM capability. The adapter driver and diagnostic protocol are separate layers.

## GitHub roadmap

The master issue tracks all eight phase parents and the overall project:

- [Master project roadmap #9](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/9)

### Phase 1 — Vehicle Identification & Configuration

- [Phase 1 parent #10](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/10)
- Vehicle profile/configuration schema.
- VIN decoding and manual identification.
- Engine, transmission, drivetrain, and trim modeling.
- Hardware compatibility catalog.
- Profile validation and fixtures.

### Phase 2 — Hardware & Communication Interface

- [Phase 2 parent #11](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/11)
- Adapter abstraction and driver architecture.
- Protected OBD-II interface research/implementation.
- Subaru SSM hardware research/implementation.
- USB, serial, and network adapter discovery.
- Connection lifecycle, health, and safety tests.

### Phase 3 — Protocol Engine

- [Phase 3 parent #12](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/12)
- Protocol/message abstractions.
- ELM327 command handling.
- Generic OBD-II support.
- Subaru SSM protocol research/implementation.
- ECU identification and session management.
- Validation, timeout, recovery, and replay tests.

### Phase 4 — Engine Diagnostics

- [Phase 4 parent #13](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/13)
- Target EJ25 ECU identification/profile.
- Read-only DTC retrieval and decoding.
- Verified live sensor definitions.
- Readiness and supported monitoring.
- Diagnostic reports.

### Phase 5 — Data Acquisition & Storage

- [Phase 5 parent #14](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/14)
- Vehicle and session schemas.
- Local database/persistence.
- Timestamped capture.
- CSV import/export.
- Diagnostic history and analysis.

This phase can progress alongside Phases 3 and 4.

### Phase 6 — ABS & Additional Module Diagnostics

- [Phase 6 parent #15](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/15)
- Existing ABS epic [#1](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/1) and its research/implementation issues [#2–#8](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/2).
- Transmission controller variant research.
- Additional Legacy electronic controller investigation.
- Shared module identity and capability reporting.
- Multi-module support matrix.

The existing ABS issues are preserved; no history is discarded or recreated.

### Phase 7 — ECU Research & Advanced Capabilities

- [Phase 7 parent #16](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/16)
- ECU hardware identification.
- Firmware format/binary research.
- Protocol documentation.
- Firmware read feasibility.
- Separate write/flash feasibility and safety review.

Research does not authorize write or flash implementation.

### Phase 8 — Web Application, Dashboard & Release

- [Phase 8 parent #17](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/17)
- Go API and web architecture.
- Vehicle garage and dashboard.
- Live gauges and WebSocket streaming.
- DTC history and reports.
- End-to-end and hardware validation.
- Packaging, documentation, and initial release.

## Dependency and parallel-work guidance

1. Phase 1 establishes vehicle and hardware profile foundations.
2. Phase 2 establishes safe adapter abstractions and connection management.
3. Phase 3 builds protocol handling over the adapter layer.
4. Phase 4 uses verified engine ECU and protocol support.
5. Phase 5 begins alongside Phases 3 and 4 as shared infrastructure.
6. Phase 6 research can proceed in parallel; module communication requires verified profiles and protocol support.
7. Phase 7 is a parallel research track with separate safety gates.
8. Phase 8 develops incrementally against stable Go API contracts; release follows integration and validation.

## Milestones

1. **M1 — Foundation:** vehicle profiles, hardware abstraction, protocol foundation.
2. **M2 — First Diagnostics:** verified engine ECU identification and read-only diagnostics.
3. **M3 — Data Platform:** local session storage, capture, and export.
4. **M4 — Multi-Module Research:** ABS and additional controller research and supported diagnostics.
5. **M5 — Initial Release:** integrated web dashboard, end-to-end validation, packaging, and documentation.

Milestones are outcome gates, not calendar promises.

## Project-wide acceptance criteria

- The initial target vehicle is explicitly represented; unknown identifiers remain unknown.
- Adapter drivers, transports/protocols, vehicle definitions, and diagnostic modules are separate concerns.
- Every supported operation is tied to a documented capability and applicable hardware/profile.
- Simulated adapters and replayable fixtures cover protocol and decoder tests before vehicle testing.
- Diagnostic captures are timestamped, validated, locally stored, and exportable.
- Factory definitions remain immutable; user annotations and modifications are separate.
- Unknown, ambiguous, unsupported, and failed identification states are explicit.
- Read-only behavior is the default and enforced at the capability/operation boundary.
- Safety-critical behavior and hardware-dependent claims receive review and validation.

## Non-goals for early milestones

- ECU flashing/tuning or arbitrary memory writes.
- ABS actuator, pump, or valve commands.
- Immobilizer/security bypass.
- Assuming generic OBD-II provides Subaru-enhanced measurements.
- Assuming one diagnostic connector or pinout applies across all modules/variants.
- Claiming support for other Subaru years/models without separate evidence and validation.
