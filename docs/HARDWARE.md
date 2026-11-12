# Hardware and Interfaces

## First principle

Use a known-compatible, electrically protected automotive diagnostic interface before attempting custom hardware. Vehicle electrical systems can produce transients that can damage host computers and microcontrollers.

## Candidate approach

1. Evaluate a supported Subaru SSM interface (for example, a compatible Tactrix/OpenPort device or a documented K-line interface).
2. Confirm exact protocol and operating-system compatibility against the chosen tool and vehicle.
3. Verify cable pinout against the factory wiring manual before connecting.
4. Capture communication passively where practical.
5. Consider custom Arduino/ESP32 hardware only after protocol and electrical requirements are understood.

## Do not

- Connect bare MCU GPIO directly to an ECU diagnostic line.
- Assume all 16-pin DLC terminals are generic OBD-II signals.
- Short unknown terminals or bridge test connectors based on wire color alone.
- Apply external voltage to an unknown pin.
- Probe airbag/SRS circuits.
- Attempt write, flash, or actuator functions during initial development.

## Hardware record

For every interface, record manufacturer, model, firmware, driver, connector pinout source, supported physical layers, isolation/protection, and test results.
