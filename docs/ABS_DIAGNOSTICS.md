# ABS Diagnostics and Multi-Module Discovery

## Objective

Support multiple ABS controller/hydraulic-unit variants used across the 1999 USDM Subaru Legacy family. Do not make the Outback's presumed module the only supported implementation.

The initial family scope includes 1999 Legacy configurations listed in parts/service references (including Brighton, GT, L, Outback, and SUS where applicable). Exact trim/body/engine/transmission fitment must be validated against authoritative VIN-specific catalogs and factory documentation before a profile is marked confirmed.

## Evidence-backed initial parts catalog

A 1999 Legacy OEM parts catalog lists two ABS ECU repair assemblies:

| ABS ECU repair part number | Catalog application shown | Status |
|---|---|---|
| `27529AC05A` | Automatic-transmission option codes | Catalog-listed; verify exact vehicle fitment |
| `27529AC06A` | Manual-transmission option codes | Catalog-listed; verify exact vehicle fitment |

The same catalog lists hydraulic unit assemblies:

| Hydraulic unit part number | Catalog application shown | Status |
|---|---|---|
| `27539AC05A` | Automatic-transmission option codes | Catalog-listed; verify exact vehicle fitment |
| `27539AC06A` | Manual-transmission option codes | Catalog-listed; verify exact vehicle fitment |

A separate catalog page gives broad 1999 Legacy submodel fitment for these assemblies, while the detailed diagram distinguishes option codes. These records are discovery clues, not a complete authoritative interchange table. Do not infer that an ECU repair part and hydraulic assembly are independently interchangeable, or that suffixes alone prove protocol compatibility.

References:
- 1999 Legacy ABS parts diagram: https://www.subarupartsdeal.com/parts-list/1999-subaru-legacy/suspension_axle_brake/antilock_brake_system.html
- 1999 Legacy ABS pump/hydraulic assembly listing: https://www.subarupartsdeal.com/oem-1999-subaru-legacy-abs_pump_and_motor_assembly.html
- 1999 USDM factory service manual index: https://www.jdmfsm.info/Auto/Japan/Subaru/Legacy_Outback/1999/Service%20Manual%20USDM/

## Discovery architecture

The ABS discovery subsystem must support several identification paths, each with its own confidence level:

1. **Vehicle-configuration lookup**
   - Input model year, market, model/body, engine, transmission, and factory option information.
   - Return candidate module profiles from a versioned fitment catalog.
   - Never claim this identifies the physically installed module; parts may have been replaced.

2. **Physical label identification**
   - Allow manual entry or image-assisted transcription of ECU and hydraulic-unit markings.
   - Store separate ECU and hydraulic-unit part numbers where available.
   - Normalize formatting but preserve the original raw transcription and photo reference.
   - A part-number match identifies a catalog candidate, not necessarily its diagnostic protocol.

3. **Documented electronic identification**
   - Use only factory-documented ABS diagnostic connector procedures and explicitly supported read-only identification requests.
   - Do not probe arbitrary addresses, send undocumented command sequences, or brute-force pins.
   - Record raw request/response bytes, transport, timing, connector, and source document.
   - If the module exposes no electronic identity through the documented interface, report that result honestly.

4. **Blink-code identification**
   - Treat factory ABS warning-lamp blink-code retrieval as a diagnostic channel, not automatically as a unique hardware identity.
   - Record code format, start/normal code, stored codes, and procedure version.
   - Never confuse the engine green test-mode connectors with the dedicated ABS diagnosis connector.

5. **Manual-assisted resolution**
   - Present candidate profiles with the evidence supporting each.
   - Ask the operator to confirm uncertain physical details.
   - Keep the module marked `unknown` or `ambiguous` if evidence does not distinguish candidates.

## Detection result model

Every scan should return a structured result with:

- vehicle profile and source
- ABS ECU part number (optional)
- hydraulic-unit part number (optional)
- module family/profile ID (optional)
- identification method
- connector/interface and protocol (only if observed)
- raw identification response or blink-code capture
- supported operations and evidence
- confidence: `confirmed`, `probable`, `candidate`, or `unknown`
- unresolved questions and warnings

Do not convert a catalog fitment match into a confirmed electronic module identity.

## Profile catalog

Profiles belong under `data/definitions/abs/`. Each profile should include:
- stable internal profile ID
- applicable model years and market
- supported model/body/engine/transmission options
- ECU and hydraulic-unit part-number aliases
- connector references to factory diagrams
- diagnostic transport/procedure
- known DTC format and decoder version
- live-data capabilities only when demonstrated
- citations/source documents, revision, and confidence
- explicit unsupported/unknown features

Part-number aliases must be evidence-backed. Avoid broad wildcards that can silently select the wrong profile.

## Safe scan state machine

```text
START
  -> Load vehicle profile
  -> Load candidate ABS profiles
  -> Verify operator-selected connector/interface
  -> Validate voltage/power and interface readiness (hardware-specific)
  -> Run only approved read-only identification procedure
  -> Capture response / blink codes
  -> Match against profile evidence
  -> Return confirmed / probable / candidate / unknown
  -> Offer read-only DTC retrieval only if supported
```

A scan must stop if connector identity, wiring, electrical state, or procedure is uncertain. No automatic terminal grounding, relay activation, pump/valve actuation, code clearing, calibration, or write operation.

## Capability matrix

Do not assume all profiles support the same features.

| Capability | Initial policy |
|---|---|
| Vehicle-based candidate lookup | Supported |
| Manual part-number entry | Supported |
| Documented blink-code retrieval | Planned, procedure-specific |
| Electronic module identification | Only where documented and verified |
| ABS DTC decoding | Per-profile evidence required |
| Four-wheel-speed live data | Unknown until demonstrated |
| Pump/valve actuator tests | Out of scope |
| Coding/calibration/writes | Out of scope |

## Research checklist

- [ ] Build a complete 1999 USDM Legacy ABS fitment matrix from factory parts catalog/service documentation.
- [ ] Separate ECU repair part numbers from complete hydraulic-unit assembly numbers.
- [ ] Confirm option-code meaning and body/transmission fitment.
- [ ] Determine whether ABS ECU and hydraulic-unit part numbers have independent revisions.
- [ ] Map connector locations, terminal numbers, and procedures by profile.
- [ ] Verify whether each module supports only blink codes or an electronic diagnostic protocol.
- [ ] Identify supported DTC tables and any live-data definitions.
- [ ] Add test fixtures for every confirmed profile.
- [ ] Validate the Outback manual-transmission candidate against the vehicle before labeling it confirmed.

## Safety

ABS is a safety-critical braking system. Initial tooling is read-only. Do not command valves, pumps, or actuators, and do not clear codes until the underlying fault and exact factory procedure are understood.

Never bridge unknown terminals, infer ABS wiring from engine test-mode connectors, or connect unprotected microcontroller GPIO to vehicle wiring.
