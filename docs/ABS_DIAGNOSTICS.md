# ABS Diagnostics

## Separate diagnostic path

ABS diagnostics must be treated independently from engine OBD-II and Subaru SSM until the exact vehicle wiring and module protocol demonstrate otherwise.

Factory documentation reviewed for the 1999 Legacy describes a dedicated ABS diagnosis connector and a blink-code procedure using the ABS warning lamp. The connector location and procedure must be confirmed against the correct manual and physical vehicle before use.

## Important distinction

Green engine test-mode connectors are not automatically ABS diagnostic connectors. Do not bridge green connectors or ground any terminal based on assumptions.

## Research plan

- [ ] Confirm ABS module/hydraulic unit part number.
- [ ] Confirm the dedicated ABS diagnosis connector location and terminal numbering.
- [ ] Verify the factory blink-code procedure for this exact model.
- [ ] Document code format, start code, storage limit, and clearing procedure from the manual.
- [ ] Determine whether ABS live data (such as wheel speeds) is available through a diagnostic protocol.
- [ ] If live data is unavailable through the documented interface, record that limitation rather than inventing a capability.

## Safety

ABS is a safety-critical braking system. Initial tooling must be read-only. Do not command valves, pumps, or actuators, and do not clear codes until the underlying fault and factory procedure are understood.
