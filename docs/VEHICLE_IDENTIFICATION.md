# Vehicle Identification (VIN Decoding & Manual Entry)

**Go package:** `internal/vehicle` (`vin.go`, `identification.go`)
**Fixtures:** `tests/fixtures/vin/`

## Purpose

Decode a VIN where possible and always keep a manual-entry fallback, while recording exactly which fields came from the VIN decode versus the user. The full VIN is never persisted — only a masked identifier and the decoded attributes.

## VIN decoding rules

`DecodeVIN` is tolerant: it strips whitespace/dashes, uppercases, and records problems in `Errors` instead of failing outright. `VINDecode.Complete` means "17 characters from the valid alphabet"; the check digit is reported separately via `CheckDigit` (`valid` / `invalid` / `not-applicable`).

Decoded positions (ISO 3779):

| Positions | Field | Confidence |
|---|---|---|
| 1–3 | WMI (world manufacturer identifier) | Raw segment only; no manufacturer claim without a lookup table |
| 4–8 | VDS (vehicle descriptor section) | Raw segment only |
| 9 | Check digit | Deterministically verified |
| 10 | Model year code | `YearCandidates` — codes repeat every 30 years, so ambiguity is explicit |
| 11 | Plant code | Raw character only |
| 12–17 | VIS (vehicle identifier section / serial) | Raw segment only |

`ResolveYear(contextYear)` selects the correct candidate only when the vehicle context (e.g., known model year) matches a candidate; otherwise the year stays unknown. Characters `I`, `O`, and `Q` are invalid in VINs.

## Privacy

`VINDecode.Masked()` keeps the WMI (manufacturer region, not vehicle-unique) and the last 6 characters, masking the middle 8: e.g. `JF1********000001`. Partial VINs are fully masked. `VehicleIdentification` stores only this masked form — never the full VIN.

## Identification record

`VehicleIdentification` holds `Fact`-typed fields (`VIN`, `ModelYear`, `Make`, `Model`, `Trim`). Every populated field carries an evidence `source`:

- `vin-decode` — set by `ApplyVIN`
- `manual-entry` — set via `ManualStringFact` / `ManualIntFact` or directly with `Evidence{Source: "manual-entry"}`

`FieldSource(fact)` reports which path established a field. `NewVehicleIdentification()` starts with every field explicitly `unknown`.

## Status rules in `ApplyVIN`

| Input | VIN fact status | ModelYear |
|---|---|---|
| Complete VIN, valid check digit | `confirmed` | `inferred` only if context resolves year candidates; else `unknown` |
| Complete VIN, bad check digit | `inferred` (possible misread) | same rule |
| Partial VIN | `inferred`, fully masked | `unknown` unless resolvable |
| Empty VIN | error, nothing recorded | — |

Manual entry always overrides or fills remaining unknowns; it never rewrites decode provenance — each field keeps its own source.

## Fixtures

`tests/fixtures/vin/` contains reproducible cases:

- `valid.txt` — `JF1CE4G22XH000001` (valid check digit, year code X → candidates 1999/2029)
- `formatted.txt` — same VIN with separators/lowercase (normalization)
- `invalid-checkdigit.txt` — one-digit serial change breaks the check digit
- `invalid-chars.txt` — contains `I`, which is not in the VIN alphabet
- `partial.txt` — 10 characters; year code still decodes, check digit N/A

## Validation

```sh
go test ./...
```

## Limitations

- WMI/VDS/VIS are stored as raw segments; mapping them to manufacturer, model, or trim requires a separate lookup catalog (not yet built — see Phase 1 hardware/vehicle catalog work).
- Model-year codes are ambiguous across the 30-year cycle; resolution requires vehicle context and remains `inferred`, never `confirmed`, from VIN alone.
- Plant codes are recorded raw; Subaru plant-code meanings are not asserted without a citable source.
