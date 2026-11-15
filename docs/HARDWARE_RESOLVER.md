# Evidence-Aware Hardware Compatibility Resolver

**Go package:** `internal/vehicle` (`resolver.go`, `applicability.go`, `interface.go`)
**Schemas:** `schemas/hardware-catalog.schema.json`, `schemas/interface-profile.schema.json`

## Purpose

Evaluate whether a hardware profile *may apply* to a vehicle configuration, explain exactly why, and preserve ambiguity instead of selecting a speculative winner. The resolver extends the [hardware catalog](HARDWARE_CATALOG.md); it never asserts installed hardware.

## Applicability model

`CatalogEntry.Applicability` holds optional structured constraints, each a fact with the usual status/evidence rules:

| Dimension | Constraint type | Vehicle side |
|---|---|---|
| `modelYear` | `IntRange` (min/max, 0 = unbounded) | profile model year, or VIN-derived year |
| `market`, `model`, `body` | string | profile facts |
| `engine`, `transmission`, `drivetrain` | configuration ID reference | `VehicleConfiguration` refs |
| `production` | `ProductionRange` | profile production range |
| `options.<name>` | string | always unknown — factory options are not yet modeled |

Each supplied constraint evaluates to one of four states:

- `match` — constraint known and vehicle fact agrees
- `conflict` — constraint known and vehicle fact disagrees
- `unknown` — constraint supplied but the vehicle fact is missing or ambiguous
- `not-supplied` — the entry places no constraint on that dimension

**Missing vehicle facts never count as a match.**

## Vehicle facts assembly

`Registry.VehicleFactsFor(configurationID, identification)` resolves the vehicle-side view: profile facts, configuration references, and — when an `VehicleIdentification` is supplied — VIN/manual-entry data. Model year prefers the profile fact, then the identification fact, then a *single* unambiguous VIN year candidate. A partial VIN or ambiguous 30-year code leaves the year unknown with `YearCandidates` preserved, so year-constrained entries evaluate as `unknown` rather than guessed.

## Outcomes

`ResolveFitment` returns a `FitmentReport` per configuration:

| Outcome | Meaning |
|---|---|
| `compatible-candidate` | All supplied constraints match; plausible fit, still not proof of installation |
| `incompatible` | At least one supplied constraint conflicts with a known vehicle fact |
| `insufficient-information` | No conflicts, but ≥1 constraint could not be evaluated (unknown vehicle fact) |
| `conflicting-evidence` | Entry carries open `ConflictRecord`s — its own claims are disputed |
| `unresolved` | Entry tier is `unresolved`; it asserts no hardware, so there is nothing to fit |

**Evidence tier and fitment outcome are independent.** A `confirmed`-tier entry with a conflicting constraint is `incompatible`; a `candidate`-tier entry with clean constraints is `compatible-candidate`.

Every `EntryResolution` lists the per-dimension `ConstraintResult`s (expected vs actual), the entry's evidence, open conflicts, and human-readable reasons.

## Determinism and ambiguity

- `Results` is sorted by entry ID — fully deterministic.
- `Compatible` contains **all** passing entries ranked by tier (confirmed → corroborated → candidate) then entry ID. Multiple plausible matches stay visibly ambiguous; the resolver never picks a winner.
- Ranking applies only among entries that pass applicability — it cannot rescue a conflicting entry.

## Module identity coverage

- `abs-controller` (electronic controller) and `abs-hydraulic-assembly` (modulator/hydraulic unit) are distinct `ModuleKind`s with separate catalog entries — the shipped catalog keeps both `unresolved`.
- `transmission-controller` remains `unresolved` pending evidence that the target vehicle even has a separate controller.
- Hardware kind must match entry module kind (`ResolveCatalog` enforces this).

## Diagnostic interfaces are not modules

`InterfaceProfile` documents the three adapter paths — `elm327-odbii`, `subaru-ssm`, `arduino-serial` — under `data/definitions/interfaces/` with transport, protocol, and read-only posture as facts. They load into a separate registry map and **never appear in fitment results**: interface compatibility is a communication path, not installed hardware, and says nothing about module fitment.

## Evidence rules

- `corroborated` tier requires evidence from **at least two distinct sources** — duplicate citations to one source fail validation.
- `ConflictRecord`s have structured `attribute`, competing `claims` (value + source + optional entry), `status` (`open`/`resolved`), and notes. Open conflicts force the `conflicting-evidence` outcome.
- `Catalog.Conflicts()` still detects cross-entry hardware disagreements at the catalog level.

## Current state

The shipped catalog's four entries (engine ECU, ABS controller, ABS hydraulic assembly, transmission controller) are all `unresolved` — `TestResolveFitmentShippedCatalog` asserts the resolver returns `unresolved` for every one and an empty compatible list. No target hardware is promoted without traceable evidence.

## Validation

```sh
go test ./...
go run ./tools/validate-definitions
```

`resolver_test.go` is table-driven across exact match, known conflict beating tier, missing facts, partial-VIN year ambiguity, multiple preserved candidates, conflicting evidence, ABS identity separation, determinism, and the interface boundary.

## Remaining gaps

- Factory option data is unmodeled; option constraints always evaluate `unknown`.
- VIN make/model decoding needs a lookup catalog (Phase 1+ research).
- Interface profiles describe paths, not per-vehicle reachability — that requires protocol/ECU identification work in later phases.
