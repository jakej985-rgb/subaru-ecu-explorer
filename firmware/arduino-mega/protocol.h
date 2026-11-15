// protocol.h — wire format for the Arduino Mega diagnostic interface.
// MUST stay byte-compatible with internal/adapter/arduino/frame.go.
// Any change here requires the matching Go change and a protocol-version
// review; fixtures in tests/fixtures/arduino-serial/ pin the format.
#ifndef DIAG_PROTOCOL_H
#define DIAG_PROTOCOL_H

#include <stdint.h>

// Frame layout (all multi-byte fields big-endian):
//   0  2  magic 0xA5 0x5A
//   2  1  protocol version (=1)
//   3  1  frame type
//   4  2  sequence number
//   6  2  payload length
//   8  N  payload
//   8+N 2  CRC-16/CCITT-FALSE over bytes 0..8+N-1
#define PROTO_MAGIC0 0xA5
#define PROTO_MAGIC1 0x5A
#define PROTO_VERSION 1
#define PROTO_HEADER_SIZE 8
#define PROTO_CRC_SIZE 2
#define PROTO_MAX_PAYLOAD 512
#define PROTO_MAX_FRAME (PROTO_HEADER_SIZE + PROTO_MAX_PAYLOAD + PROTO_CRC_SIZE)

// Frame types.
#define FRM_HELLO       0x01  // firmware hello / capability negotiation
#define FRM_DATA        0x02  // acquisition sample batch (either direction)
#define FRM_ACK         0x03  // acknowledgement
#define FRM_ERROR       0x04  // firmware-reported error
#define FRM_CAPABILITY  0x05  // capability report
#define FRM_HEARTBEAT   0x06  // periodic liveness tick
#define FRM_STATUS      0x07  // state transition report

// Capability bits (hello/capability payload).
#define CAP_PERIODIC_SAMPLES (1UL << 0)
#define CAP_EVENT_SAMPLES    (1UL << 1)
#define CAP_SELF_TEST        (1UL << 2)

// Downlink operation codes (inside FRM_DATA payloads, host -> firmware).
// The firmware allowlist is explicit: anything else is rejected.
#define OP_SELF_TEST   0x01  // read-only self-test, no vehicle I/O
// Vehicle-facing operations are intentionally absent: vehicle transmit is
// disabled until protocol and wiring are explicitly approved.

// Firmware status states (FRM_STATUS payload byte 0).
#define ST_BOOT        0x00
#define ST_READY       0x01
#define ST_ERROR       0x02
#define ST_VEH_DISABLED 0x03

// Error codes (FRM_ERROR payload byte 0).
#define ERR_UNSUPPORTED_OP 0x10
#define ERR_BAD_PAYLOAD    0x11
#define ERR_INTERNAL       0x12
#define ERR_VEHICLE_IO     0x20

// Heartbeat payload: uptime_ms u32 BE | status u8 | rx_crc_errors u16 BE.
#define HB_PAYLOAD_SIZE 7

// Status payload: state u8 | detail_len u8 | detail[<=32] ASCII.
#define ST_MAX_DETAIL 32

// Hello payload layout (matches Go Hello.EncodeHello):
//   protoVer u16 | caps u32 | fwLen u8 | fw[fwLen] | boardLen u8 | board[]
#define HELLO_MAX_STRING 32

uint16_t diag_crc16(const uint8_t *data, uint16_t len);

#endif // DIAG_PROTOCOL_H
