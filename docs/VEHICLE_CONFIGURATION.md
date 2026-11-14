# Vehicle Configuration Layer

**Schema version:** `1.0.0` (`ConfigSchemaVersion`)
**Go package:** `internal/vehicle`
**Schemas:** `schemas/engine-config.schema.json`, `schemas/transmission-config.schema.json`, `schemas/drivetrain-config.schema.json`, `schemas/trim-config.schema.json`, `schemas/vehicle-configuration.schema.json`

## Purpose

Model engine, transmission, drivetrain, and trim/body-variant configurations as reusable, evidence-backed documents, then bind them to a [vehicle profile](VEHICLE_PROFILE.md) through a `VehicleConfiguration` document. Component details live in exactly one place — the component definition — so profiles and configurations reference them by stable ID instead of duplicating factory data.

## Document types

| Type | File pattern | Key fields |
|---|---|---|
| `EngineConfig` | `data/definitions/engines/*.json` | make, family, displacementLiters, cylinders, aspiration, fuelSystem, valvetrain, production |
| `TransmissionConfig` | `data/definitions/transmissions/*.json` | type (`manual`/`automatic`/`cvt`/`dct`/`other`), family, gearCount |
| `DrivetrainConfig` | `data/definitions/drivetrains/*.json` | layout (`fwd`/`rwd`/`awd`/`4wd`/`other`) |
| `TrimConfig` | `data/definitions/trims/*.json` | market, model, bodyCode, bodyStyle, trimLevel |
| `VehicleConfiguration` | `data/definitions/configurations/*.json` | profile, engine, transmission, drivetrain, body, trim (all ID references) |

Every attribute is a `Fact` with the same status rules as the vehicle profile: `confirmed` and `inferred` require a value plus evidence; `unknown` and `unsupported` carry no value. A document that only knows "manual" must not claim a gear count, family, or final-drive ratio.

## Linking profiles to configurations

`VehicleConfiguration` fields (`profile`, `engine`, `transmission`, `drivetrain`, `body`, `trim`) are `Fact[string]` references to stable definition IDs. A valued reference asserts "this definition applies to the vehicle" and therefore requires evidence when confirmed or inferred. `body` and `trim` both resolve against trim/body-variant (`TrimConfig`) definitions.

Diagnostic code must branch on resolved configuration IDs, never on hard-coded model-year or nameplate assumptions.

## Registry and reference resolution

`vehicle.NewRegistry()` stores validated profiles, component configs, and configurations by ID. `Register*` validates before storing and rejects duplicate IDs. `Registry.Resolve(cfg)` reports an error for every valued reference that has no registered definition of the matching kind; unknown/unsupported references are explicit and skipped. A missing referenced definition is a data problem, not a reason to guess.

## Current definitions

- `engines/subaru-ej25-2-5l.json` — make, family, and 2.5L displacement confirmed by the project target; cylinders, aspiration, fuel system, valvetrain, and production range remain unknown pending citable evidence.
- `transmissions/manual-transmission-unverified.json` — asserts only `type: manual`; family and gear count are unknown.
- `configurations/1999-usdm-legacy-outback-ej25-manual.json` — binds the target profile to the EJ25 engine and the unverified manual transmission config. Drivetrain, body, and trim references are explicitly `unknown`; no drivetrain or trim definitions exist yet because no evidence supports them.

## Validation

```sh
go test ./...
```

`TestTargetConfigurationDefinitions` loads the shipped JSON definitions, registers them, and verifies the target configuration resolves with unverified components still unknown. JSON files are additionally checked against the Draft 2020-12 schemas in `schemas/` (see `.venv` `jsonschema` tooling used during development).

## Unresolved questions

- Exact transmission family/gear count for the target vehicle.
- Drivetrain layout and variant (AWD details must not be inferred from the Outback nameplate).
- Factory body code, body style, and trim level.
- Whether additional EJ25 variants (e.g., different aspiration or market calibrations) need distinct engine configuration documents.

Hardware/ECU compatibility candidates are tracked separately in the [Hardware Compatibility Catalog](HARDWARE_CATALOG.md); no module is confirmed installed for the target vehicle yet.
