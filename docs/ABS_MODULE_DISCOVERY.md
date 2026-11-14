# ABS Module Discovery Design

## Goal

Identify the installed ABS system without requiring the operator to locate or read the module label before the software can begin useful work.

The detector combines vehicle configuration, a versioned fitment catalog, optional physical part-number entry, and documented read-only diagnostic evidence.

## Identification levels

- `vehicle_candidate`: based only on year/model/engine/transmission/options.
- `part_number_match`: physical ECU or hydraulic-unit part number matches catalog data.
- `protocol_identified`: a documented electronic identity response matches a known profile.
- `procedure_identified`: the module's documented diagnostic procedure and response format are known, but hardware identity may remain ambiguous.
- `unknown`: evidence is insufficient.

A vehicle candidate is not a detected module. A blink-code stream may identify a diagnostic family without uniquely identifying the installed part.

## Safe discovery sequence

1. Load the vehicle profile and candidate ABS definitions.
2. Display which factory connector/procedure is applicable and what evidence supports it.
3. Require operator confirmation of the physical connector before any electrical diagnostic action.
4. Execute only a whitelisted, documented, read-only procedure.
5. Save raw response and metadata.
6. Match using explicit identifiers and validated signatures.
7. Report confidence and unresolved ambiguity.

No blind pin scanning, arbitrary address sweeps, unknown terminal grounding, actuator commands, code clearing, or write operations.

## Candidate profile matching

Matching should be explainable. Each candidate records:
- matching fields
- conflicting fields
- missing fields
- source quality
- profile revision

If two candidates remain indistinguishable, return both and mark the result ambiguous. Never choose the first catalog row as a fallback.

## Future software components

- `AbsVehicleMatcher`: creates candidate profiles from vehicle configuration.
- `AbsPartNumberMatcher`: matches ECU and hydraulic-unit identifiers independently.
- `AbsProcedureRegistry`: lists approved diagnostic procedures by profile.
- `AbsReadOnlyProbe`: executes only approved identification/read procedures.
- `AbsProfileResolver`: returns confidence and evidence.
- `AbsCapabilityRegistry`: lists DTC/live-data capabilities per verified profile.

## Example scan report

```json
{
  "system": "ABS",
  "status": "candidate",
  "vehicle": {
    "year": 1999,
    "market": "USDM",
    "model": "Legacy Outback",
    "engine": "EJ25",
    "transmission": "manual"
  },
  "identified_part_numbers": {
    "abs_ecu": null,
    "hydraulic_unit": null
  },
  "profile_candidates": [],
  "identification_method": "vehicle_fitment_lookup",
  "protocol": null,
  "capabilities": {
    "blink_codes": "not_yet_verified",
    "electronic_dtcs": "unknown",
    "live_data": "unknown",
    "actuator_tests": "disabled"
  },
  "confidence": "candidate",
  "warnings": [
    "Vehicle fitment is not physical module identification."
  ]
}
```
