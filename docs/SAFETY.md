# Safety and Bench/Vehicle Procedures

## Read-only development policy

Initial releases are limited to identification, reading DTCs, reading supported measurements, and recording diagnostic sessions.

No ECU writes, flashing, actuator tests, calibration changes, or experimental memory writes are in scope for the initial phases.

## Electrical safety

- Use an automotive-rated, protected interface.
- Confirm connector pinout from factory documentation.
- Do not connect bare microcontroller pins to vehicle lines.
- Do not probe SRS/airbag circuits.
- Avoid shorts between adjacent terminals.
- Secure cables away from pedals, steering components, belts, fans, and hot surfaces.
- Do not connect or disconnect diagnostic hardware with ignition state contrary to the interface or factory procedure.

## Vehicle safety

- Do not operate a laptop or diagnostic interface while driving.
- Perform stationary tests with the vehicle secured, parking brake applied, and transmission in a safe state.
- Use a qualified technician for work affecting braking or other safety-critical systems.
- Do not rely on a diagnostic code alone as proof of a component failure.

## Data safety

Redact VINs, license plates, owner information, and credentials from committed logs. Review captures before sharing.
