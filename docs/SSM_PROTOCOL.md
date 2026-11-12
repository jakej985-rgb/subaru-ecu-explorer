# Subaru Select Monitor (SSM) Research

## Purpose

Document Subaru-specific diagnostic communication independently from generic OBD-II. FreeSSM is the initial open-source reference implementation.

## FreeSSM

Upstream: https://github.com/Comer352L/FreeSSM  
License: GPL-3.0

FreeSSM documents support for Subaru engine and transmission control units on supported vehicles and includes SSM protocol implementations, ECU identification, diagnostic codes, measuring blocks, and XML definitions.

Do not assume every feature applies to this exact vehicle or ECU. Confirm ECU identity and the relevant definition before decoding values.

## 1999 USDM Legacy connector notes

The factory documentation reviewed for the 1999 USDM Legacy describes a 16-pin data link connector used by both an OBD-II general scan tool and Subaru Select Monitor. The manual identifies SSM communication terminals separately from the ISO 9141 K-line and warns against connecting unsupported equipment to the SSM circuit.

**Important:** Verify every terminal against the correct factory wiring diagram and exact vehicle configuration before connecting hardware. This document is a research summary, not a substitute for the factory pinout.

## Protocol research checklist

- [ ] Identify ECU system ID and ROM ID.
- [ ] Identify whether SSM1 or SSM2 applies.
- [ ] Record transport and physical-layer settings from observed communication.
- [ ] Record request/response examples with sensitive data removed.
- [ ] Map measuring-block IDs to the correct ECU definition.
- [ ] Validate scaling, units, ranges, and update rates.
- [ ] Create replay tests for captured frames.

## Firmware limits

Support for ECU identification, diagnostic data, or memory-related commands does not prove that arbitrary firmware dumping, modification, or execution is possible. Treat those as separate research questions requiring independent evidence.
