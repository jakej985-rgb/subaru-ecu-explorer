# Profile & Definition Validation

**Go package:** `internal/vehicle` (`load.go`, `catalog.go`)
**Tool:** `tools/validate-definitions`

## Loading definitions

`vehicle.LoadDefinitions(root, registry)` walks a `data/definitions`-style directory, strictly decodes every `.json` file in a known kind directory, validates it, and registers it:

| Directory | Document type |
|---|---|
| `vehicles/` | `VehicleProfile` |
| `engines/` | `EngineConfig` |
| `transmissions/` | `TransmissionConfig` |
| `drivetrains/` | `DrivetrainConfig` |
| `trims/` | `TrimConfig` |
| `configurations/` | `VehicleConfiguration` |
| `hardware/` | `HardwareProfile` |
| `catalogs/` | `CompatibilityCatalog` |

Unknown directories are skipped. Each file produces a `LoadResult`; `LoadReport.OK()` is false if any file failed, and loading continues so all problems surface in one pass. Invalid documents are never registered.

## Strictness and unknown fields

The loader uses `json.Decoder.DisallowUnknownFields()`, matching the `additionalProperties: false` rule in the JSON schemas — unknown **top-level** fields are rejected. Nested fact objects rely on the Go struct shape; the JSON schemas remain the authoritative check for nested fields (run them with a Draft 2020-12 validator such as the `jsonschema` CLI in `.venv`).

Explicit `null` for optional object fields (`production`, `applicability`) is rejected at decode time by custom `UnmarshalJSON` methods, matching schema behavior.

## Cross-document checks

`vehicle.ValidateRegistry(registry)` runs after loading:

- every `VehicleConfiguration`'s valued references must resolve to registered profiles/component configs;
- every `CompatibilityCatalog` entry's valued `configuration` and `hardware` references must resolve, and hardware kind must match the entry's module kind.

`CompatibilityCatalog.Conflicts()` reports configuration/module pairs whose entries name different hardware profiles — conflicting attributes are surfaced, never silently resolved.

## Schema versioning

Each document family has a schema version constant (`CurrentSchemaVersion` for profiles, `ConfigSchemaVersion` for component configs and vehicle configurations, `CatalogSchemaVersion` for hardware/catalogs). `Validate()` rejects any other version. `TestSchemaVersionEvolution` pins every `schemas/*.schema.json` `schemaVersion` const to the matching Go constant, so JSON and Go cannot drift. Per the compatibility policy in `VEHICLE_PROFILE_SCHEMA.md`, meaning changes require a version bump plus migration notes; additive optional fields may stay within a major version.

## CLI

```sh
go run ./tools/validate-definitions [path]
```

Defaults to `data/definitions`; prints per-file status and the reference-check summary; exits non-zero on any failure. Equivalent to `go test ./internal/vehicle` fixture coverage, useful in scripts and CI.

## Test coverage

`internal/vehicle/load_test.go` covers the acceptance criteria:

- valid target profile and full shipped definition set load and resolve;
- incomplete profiles fail validation (facts must be explicit, not absent);
- conflicting catalog attributes are detected by `Conflicts()`;
- unknown facts reject placeholder values; confirmed facts require evidence;
- schema-version mismatches (older and future) are rejected;
- unknown top-level JSON fields are rejected by strict decoding.
