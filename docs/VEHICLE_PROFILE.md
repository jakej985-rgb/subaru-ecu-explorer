# Vehicle Profile

The machine-readable profile format is defined by [Vehicle Profile Schema](../schemas/vehicle-profile.schema.json), schema version `1.0.0`. The Go model and validator live in `internal/vehicle`.

## Target configuration

| Field | Value | Status |
|---|---|---|
| Model year | 1999 | Confirmed project target |
| Model | Legacy Outback | Confirmed project target |
| Market | United States (USDM) | Confirmed project target |
| Engine | 2.5L EJ25 | Confirmed project target |
| Transmission | Manual | Confirmed project target |
| Body code | Unknown | To verify |
| Exact trim | Unknown | To verify |
| Drivetrain detail | Unknown | To verify |
| Engine ECU part number | Unknown | To inspect |
| Engine ROM ID | Unknown | To read through a supported procedure |
| ABS controller part number | Unknown | To inspect |
| ABS hydraulic assembly part number | Unknown | To inspect separately |
| Production range | Unknown | To verify |

## Fact states and provenance

Every profile fact must use one explicit status:

- `confirmed`: a value backed by evidence.
- `inferred`: a value supported by evidence but not directly confirmed.
- `unknown`: no value has been established.
- `unsupported`: the profile does not claim support for the field/capability.

Confirmed and inferred values require at least one evidence item with a source. Unknown and unsupported facts must not contain a value. Do not use strings such as `"unknown"` as values.

## Identification checklist

- [ ] Record VIN privately and derive exact model/trim; do not publish the full VIN.
- [ ] Photograph vehicle identification/build plate.
- [ ] Photograph ECU label and transcribe part number.
- [ ] Photograph ABS controller label if accessible.
- [ ] Record ABS hydraulic assembly identifier separately.
- [ ] Photograph diagnostic connectors before connecting anything.
- [ ] Record connector locations and wire colors from factory diagrams.
- [ ] Record warning-lamp behavior at key-on and while driving, if safe.

## Data handling

Do not commit full VINs, owner details, license plates, or other identifying information. Use a locally stored redacted vehicle identifier in captures and issue reports. Factory facts must remain immutable; user annotations belong in a separate user-owned layer.
