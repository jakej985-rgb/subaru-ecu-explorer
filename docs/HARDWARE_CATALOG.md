# Hardware Compatibility Catalog

**Schema version:** `1.0.0` (`CatalogSchemaVersion`)
**Go package:** `internal/vehicle` (`catalog.go`)
**Schemas:** `schemas/hardware-profile.schema.json`, `schemas/hardware-catalog.schema.json`
**Data:** `data/definitions/catalogs/`, `data/definitions/hardware/` (when profiles exist)

## Purpose

Link vehicle configurations to candidate ECU/module hardware with explicit evidence strength. The catalog records *compatibility evidence* — it never asserts what hardware is installed in a specific vehicle. Installed hardware is only established by physical inspection (label photos, part numbers) recorded as confirmed facts elsewhere.

## Document types

- `HardwareProfile` — factory reference data for a known hardware variant (kind, manufacturer, part number). Describes the part, not an installation.
- `CatalogEntry` — one configuration × one module kind (`engine-ecu`, `abs-controller`, `transmission-controller`, `other-module`) with a hardware reference, evidence tier, optional production-range applicability, evidence list, conflicts, and notes.
- `CompatibilityCatalog` — versioned set of entries.

## Evidence tiers

| Tier | Meaning | Hardware ref | Evidence |
|---|---|---|---|
| `confirmed` | Directly confirmed for this configuration | Required | Required |
| `corroborated` | Multiple independent sources agree | Required | Required |
| `candidate` | Plausible but unverified — never presented as installed | Required | Required |
| `unresolved` | Applicability not established | Forbidden | Optional |

`Registry.ResolveCatalog` verifies that every valued `configuration` and `hardware` reference resolves to a registered definition. `CatalogCandidates(configurationID, module)` returns matching entries ordered confirmed → corroborated → candidate → unresolved.

## Current catalog

`data/definitions/catalogs/1999-usdm-legacy-outback-ej25-manual.json` contains three `unresolved` entries — engine ECU, ABS controller, and transmission controller — for the target configuration. No hardware profiles exist yet because no ECU/module has been physically identified. The ABS entry notes that controller and hydraulic assembly part numbers are distinct identifiers.

## Rules

- Model year or engine family alone never promotes a candidate to confirmed.
- Conflicting evidence is recorded in `conflicts`, not silently resolved.
- A `candidate` entry names a *possible* hardware profile; diagnostics must treat it as a hypothesis requiring verification, not a fact.

## Validation

```sh
go test ./...
```

`TestTargetCatalog` loads the shipped catalog and configuration, verifies registration and reference resolution, and asserts every entry is `unresolved` with no hardware claim.
