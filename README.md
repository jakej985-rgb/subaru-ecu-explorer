# Subaru ECU Explorer

An open-source research and diagnostics platform for Subaru engine, transmission, and ABS systems, beginning with the **1999 USDM Subaru Legacy Outback, EJ25, manual transmission**.

The project uses existing Subaru diagnostic research and the open-source [FreeSSM](https://github.com/Comer352L/FreeSSM) project as a protocol reference and potential integration foundation.

## Project goals

- Identify supported ECUs and diagnostic interfaces for the target vehicle.
- Read ECU identification, diagnostic trouble codes, and supported live measurements.
- Capture timestamped diagnostic sessions and export logs.
- Investigate Subaru Select Monitor (SSM) protocol behavior and ECU definitions.
- Develop a separate, evidence-based ABS diagnostic path.
- Build a local dashboard and analysis tools.
- Research ECU memory and firmware capabilities without assuming undocumented features exist.

## Target vehicle

| Attribute | Initial target |
|---|---|
| Model year | 1999 |
| Model | Legacy Outback |
| Market | United States (USDM) |
| Engine | 2.5L EJ25 |
| Transmission | Manual |
| Engine management | To be identified from the vehicle |
| ABS | In scope; exact module/variant to be confirmed |

Treat this profile as a starting point. Confirm the VIN, ECU label/part number, ROM ID, and installed ABS hardware before relying on variant-specific definitions.

## Development principles

1. **Read-only first.** Begin with identification, DTC retrieval, live data, and logging.
2. **Vehicle-specific evidence.** Record source, model-year applicability, connector, pin, and confidence for every protocol claim.
3. **No guessed pinouts.** Never bridge or energize unknown terminals.
4. **Separate diagnostic systems.** Generic OBD-II, Subaru SSM, engine test mode, transmission diagnostics, and ABS diagnostics are distinct paths unless proven otherwise.
5. **Preserve provenance.** Keep factory definitions separate from user annotations and experimental findings.
6. **Reproducible research.** Store sanitized captures, test fixtures, and documented procedures.
7. **Respect upstream licenses.** FreeSSM is GPL-3.0. Review its license and obligations before copying or distributing code.

## Repository map

- `docs/` — vehicle profile, hardware, protocols, safety, and research notes
- `third_party/` — upstream dependencies and license notices
- `core/` — transport, protocol, ECU, decoding, and logging components
- `tools/` — command-line utilities
- `data/` — definitions, schemas, and sanitized captures
- `tests/` — protocol fixtures and decoder tests
- `dashboard/` — future local dashboard
- `firmware/` — future protected diagnostic-interface firmware

## Initial milestones

See [ROADMAP.md](ROADMAP.md). The first milestone is vehicle and ECU identification, followed by safe communication using known-compatible hardware.

## Safety

This project is for research and diagnostics. Do not perform ECU writes, flashing, actuator tests, or experimental pin bridging during the initial read-only phases. Use a properly protected automotive interface; never connect a bare Arduino or ESP32 GPIO directly to vehicle diagnostic lines.

Read [docs/SAFETY.md](docs/SAFETY.md) before connecting hardware.

## Licensing

The project license is **not yet selected**. No license is granted for this repository's original code until a license file is added. FreeSSM is separately licensed under GPL-3.0; see its upstream repository and license. Do not copy upstream code without preserving applicable notices and meeting its license requirements.
