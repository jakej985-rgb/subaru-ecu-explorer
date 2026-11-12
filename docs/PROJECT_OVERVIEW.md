# Project Overview

## Mission

Build a modular, evidence-driven Subaru diagnostic and ECU research toolkit. The initial reference vehicle is a 1999 USDM Legacy Outback with EJ25 engine and manual transmission.

## System boundaries

The project distinguishes several systems that may use different connectors, protocols, and procedures:

- Generic OBD-II engine diagnostics.
- Subaru Select Monitor (SSM) engine diagnostics.
- Engine test-mode connectors and procedures.
- Transmission diagnostics, if a separately addressable controller exists.
- ABS module diagnostics.
- Experimental hardware/interface development.

Do not merge these paths until vehicle-specific testing proves they share a transport or procedure.

## Proposed architecture

```text
Vehicle modules
  ├── Engine ECU
  ├── ABS control module
  └── Other supported modules
          |
Protected diagnostic interface(s)
          |
Transport layer
          |
Protocol layer (OBD-II / Subaru SSM / verified ABS procedure)
          |
ECU identification + definitions + decoding
          |
Session logging (SQLite / CSV)
          |
CLI tools and local dashboard
```

## Technology direction

- Reuse or interoperate with FreeSSM where its license and architecture permit.
- Keep protocol and decoding logic testable independently from hardware.
- Use a small CLI first; add a dashboard after acquisition and logging are stable.
- Use SQLite for local sessions and CSV for portable exports.
- Keep factory definitions immutable; store corrections and user annotations separately.

## Evidence requirements

Every vehicle-specific claim should record:
- Source document or capture.
- Applicable model year, market, model, engine, and transmission.
- Connector and terminal identifiers where relevant.
- Whether the information is factory-documented, observed, inferred, or unverified.
- Confidence and outstanding questions.

## Current unknowns

- Exact engine ECU part number and ROM ID.
- Exact ABS module part number and diagnostic capabilities.
- Which compatible SSM interface is available.
- Whether any additional module is separately addressable on this configuration.
