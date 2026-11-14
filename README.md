# Subaru ECU Explorer

A modular, evidence-driven Subaru diagnostics and ECU research platform, beginning with the **1999 USDM Subaru Legacy Outback, 2.5L EJ25, manual transmission**.

The initial release targets this vehicle only. Reusable architecture is encouraged, but support for other years, markets, or models must be researched and validated separately.

## Architecture

- **Diagnostic backend:** Go.
- **User interface:** browser-based web dashboard.
- **Hardware:** ELM327-compatible OBD-II adapters and Subaru-specific SSM interfaces through separate interchangeable adapter drivers.
- **Connection model:** a local Go service owns physical adapter connections. The browser communicates with the local service through controlled REST APIs and WebSocket streaming.
- **Storage:** local-first vehicle data and diagnostic sessions.
- **Initial behavior:** read-only identification, supported DTC retrieval, supported live data, and logging.

ELM327 compatibility does not imply Subaru SSM capability. Adapter drivers and diagnostic protocols are separate layers.

## Target vehicle

| Attribute | Initial target |
|---|---|
| Model year | 1999 |
| Model | Legacy Outback |
| Market | United States (USDM) |
| Engine | 2.5L EJ25 |
| Transmission | Manual |
| Engine ECU | To be identified from the vehicle |
| ABS | In scope; exact module/variant to be confirmed |

Keep the full VIN private by default. Confirm ECU labels/part numbers, ROM identity, and installed ABS hardware before relying on variant-specific definitions.

## Project roadmap

The project is managed through the [Master GitHub Roadmap](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/9) and eight major phase parents:

1. [Vehicle Identification & Configuration](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/10)
2. [Hardware & Communication Interface](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/11)
3. [Protocol Engine](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/12)
4. [Engine Diagnostics](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/13)
5. [Data Acquisition & Storage](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/14)
6. [ABS & Additional Module Diagnostics](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/15)
7. [ECU Research & Advanced Capabilities](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/16)
8. [Web Application, Dashboard & Release](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/17)

The existing [ABS discovery epic #1](https://github.com/jakej985-rgb/subaru-ecu-explorer/issues/1) and its phase issues #2–#8 are preserved under Phase 6.

## Development principles

1. **Read-only first.** Begin with identification, DTC retrieval, supported live data, and logging.
2. **Vehicle-specific evidence.** Record source, model-year applicability, connector, protocol, and confidence for every vehicle-specific claim.
3. **No guessed pinouts.** Never bridge or energize unknown terminals.
4. **Separate diagnostic systems.** Generic OBD-II, Subaru SSM, engine diagnostics, transmission research, and ABS diagnostics are distinct paths unless proven otherwise.
5. **Preserve provenance.** Factory definitions remain separate from user annotations and experimental findings.
6. **Local-first data ownership.** Diagnostic sessions and captures should remain usable offline and be exportable.
7. **Reproducible research.** Store sanitized captures, test fixtures, and documented procedures.
8. **Respect upstream licenses.** FreeSSM is GPL-3.0. Review its license and obligations before copying or distributing code.

## Repository map

- `docs/` — vehicle profile, hardware, protocols, safety, and research notes
- `third_party/` — upstream dependencies and license notices
- `core/` — planned Go transport, protocol, ECU, decoding, and logging components
- `tools/` — command-line utilities
- `data/` — definitions, schemas, and sanitized captures
- `tests/` — protocol fixtures and decoder tests
- `dashboard/` — planned browser-based dashboard
- `firmware/` — future protected diagnostic-interface firmware, if justified

## Safety

This project is for research and diagnostics. Do not perform ECU writes, flashing, actuator tests, or experimental pin bridging during initial read-only phases. Use a properly protected automotive interface; never connect a bare Arduino or ESP32 GPIO directly to vehicle diagnostic lines.

Read [docs/SAFETY.md](docs/SAFETY.md) before connecting hardware.

## Licensing

The project license is **not yet selected**. No license is granted for this repository's original code until a license file is added. FreeSSM is separately licensed under GPL-3.0; see its upstream repository and license. Do not copy upstream code without preserving applicable notices and meeting its license requirements.
