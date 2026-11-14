# Vehicle Profile Schema

**Schema version:** `1.0.0`  
**Go package:** `internal/vehicle`

## Purpose

Represent a vehicle configuration without silently converting missing information into facts. Every configurable property is a fact with an explicit status and provenance.

## Fact states

| Status | Meaning | Value required? | Evidence required? |
|---|---|---:|---:|
| `confirmed` | Directly confirmed by an applicable source or user observation | Yes | Yes |
| `inferred` | Best-supported inference, not yet directly confirmed | Yes | Yes |
| `unknown` | Not established | No | No |
| `unsupported` | This profile does not support or claim the property/capability | No | No |

A confirmed or inferred value without evidence is invalid. Unknown and unsupported facts must not carry a value. Do not use placeholder strings such as `"unknown"` as values.

## Profile identity

- `schemaVersion` identifies the profile document format, independently of the vehicle model year.
- `id` is a stable, lowercase, hyphenated profile identifier. Do not derive it from a VIN or other personal identifier.
- `displayName` is a human-readable label and may change without changing the stable ID.
- `identifiers` stores module-specific identifiers (for example, engine ECU and ABS controller part numbers) as independent facts. Do not conflate an ABS controller with its hydraulic assembly.

## Evidence

Each evidence entry includes a non-empty `source` and may include a description and reference (document/page, URL, capture ID, or observation record). Evidence should identify applicability to year, market, model, engine, and transmission where relevant.

## Initial target profile

The initial target is the 1999 USDM Subaru Legacy Outback, 2.5L EJ25, manual transmission. The initial values are limited to what has been user-confirmed or otherwise supported. Exact trim, production range, drivetrain detail, ECU identifiers, and ABS identifiers remain unknown until verified.

## Validation

The Go validator enforces:
- supported schema version;
- non-empty stable profile ID and display name;
- explicit valid state for each fact;
- evidence for confirmed/inferred values;
- no value for unknown/unsupported facts;
- non-empty evidence source labels.

Run tests with:

```sh
go test ./...
```

## Compatibility policy

Schema changes that alter meaning or validation rules require a schema version change and migration notes. Additive optional fields may remain within the same major version when backward compatibility is preserved. Factory facts and user annotations must remain separate; editing a user annotation must never mutate factory profile data.
