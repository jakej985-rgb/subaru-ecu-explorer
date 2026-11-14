# ABS Definition Catalog

Store one versioned profile per verified ABS diagnostic family or hardware variant.

## Required profile metadata

- Stable profile ID and schema version.
- Applicable model years, market, model/body, engine, transmission, and options.
- ECU part-number aliases.
- Hydraulic-unit part-number aliases.
- Source references and evidence status for every fitment claim.
- Connector/procedure references to factory documentation.
- Protocol/transport only when verified.
- DTC format and decoder reference.
- Live-data definitions only when captured and validated.
- Unsupported capabilities and unresolved questions.

Keep ECU and hydraulic-unit identifiers separate. A catalog may list them as an assembly but the physical vehicle can have replacement components; do not silently infer one identifier from the other.

## Confidence labels

- `confirmed`: validated by factory documentation and/or physical/communication evidence for the stated fitment.
- `probable`: multiple reliable clues, but one confirmation step is missing.
- `candidate`: catalog lookup only.
- `unknown`: insufficient evidence.

Do not add wildcard part-number matches without documented evidence and tests.
