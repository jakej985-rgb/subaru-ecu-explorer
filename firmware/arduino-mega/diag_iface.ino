// diag_iface.ino — read-only acquisition firmware for the Arduino Mega
// 2560 R3 diagnostic interface (issue #64).
//
// Safety posture:
//  - Speaks only the versioned framed protocol in protocol.h (byte-
//    compatible with internal/adapter/arduino/frame.go).
//  - Downlink ops are allowlisted: OP_SELF_TEST only. All vehicle-facing
//    transmit is disabled at compile time (VEHICLE_KLINE_ENABLED 0) and
//    no vehicle code path exists in this build.
//  - Bounded RX/TX buffers, parser resync on corrupt frames, RX stall
//    timeout, hardware watchdog, and stateless reconnect (host resyncs
//    by magic scan; sequence numbers detect loss).

#include <Arduino.h>
#include <avr/wdt.h>
#include "protocol.h"

// ---------- configuration ----------
#define SERIAL_BAUD        115200UL
#define HEARTBEAT_MS       1000UL
#define RX_STALL_MS        500UL    // reset parser if a frame stalls
#define MAX_CONSEC_CRC_ERR 8        // resync hard after repeated CRC failures

// Vehicle K-line transmit stays OFF until protocol + wiring are approved.
// This build contains no vehicle-facing transmit path at all.
#define VEHICLE_KLINE_ENABLED 0

#define FW_VERSION   "0.1.0"
#define BOARD_NAME   "mega2560"

// Downlink op allowlist (explicit).
static const uint8_t allowedOps[] = { OP_SELF_TEST };
static bool opAllowed(uint8_t op) {
    for (uint8_t i = 0; i < sizeof(allowedOps); i++)
        if (allowedOps[i] == op) return true;
    return false;
}

// ---------- TX ----------
static uint8_t txBuf[PROTO_MAX_FRAME];
static uint16_t txLen = 0;

static void txReset() { txLen = 0; }

// queueFrame appends an encoded frame to the bounded TX buffer.
// Returns false if it would overflow (caller drops oldest? we drop new).
static bool queueRawFrame(uint8_t type, uint16_t seq, const uint8_t *payload, uint16_t plen) {
    uint16_t need = PROTO_HEADER_SIZE + plen + PROTO_CRC_SIZE;
    if (txLen + need > sizeof(txBuf)) return false;
    uint8_t *f = txBuf + txLen;
    f[0] = PROTO_MAGIC0; f[1] = PROTO_MAGIC1;
    f[2] = PROTO_VERSION; f[3] = type;
    f[4] = seq >> 8; f[5] = seq & 0xFF;
    f[6] = plen >> 8; f[7] = plen & 0xFF;
    if (plen) memcpy(f + 8, payload, plen);
    uint16_t crc = diag_crc16(f, PROTO_HEADER_SIZE + plen);
    f[8 + plen] = crc >> 8; f[8 + plen + 1] = crc & 0xFF;
    txLen += need;
    return true;
}

// Drain as much as the HW serial ring accepts (≤63 B on AVR) and keep the
// unsent suffix; never wait for the whole queue to fit.
static void flushTx() {
    if (!txLen) return;
    int avail = Serial.availableForWrite();
    if (avail <= 0) return;
    uint16_t n = (uint16_t)avail < txLen ? (uint16_t)avail : txLen;
    Serial.write(txBuf, n);
    memmove(txBuf, txBuf + n, txLen - n);
    txLen -= n;
}

// ---------- frame builders ----------
static uint16_t seqCounter = 0;

static void sendIdentity(uint8_t frameType) {
    uint8_t p[2 + 2 + 4 + 1 + HELLO_MAX_STRING + 1 + HELLO_MAX_STRING];
    uint16_t o = 0;
    uint16_t pv = PROTO_VERSION;
    p[o++] = pv >> 8; p[o++] = pv & 0xFF;
    uint32_t caps = CAP_SELF_TEST; // no sample producer yet; no vehicle TX caps
    p[o++] = caps >> 24; p[o++] = caps >> 16; p[o++] = caps >> 8; p[o++] = caps & 0xFF;
    uint8_t fl = strlen(FW_VERSION);
    p[o++] = fl; memcpy(p + o, FW_VERSION, fl); o += fl;
    uint8_t bl = strlen(BOARD_NAME);
    p[o++] = bl; memcpy(p + o, BOARD_NAME, bl); o += bl;
    queueRawFrame(frameType, seqCounter++, p, o);
}

static void sendHello() { sendIdentity(FRM_HELLO); }

static void sendStatus(uint8_t state, const char *detail) {
    uint8_t p[1 + 1 + ST_MAX_DETAIL];
    uint16_t o = 0;
    p[o++] = state;
    uint8_t dl = detail ? strlen(detail) : 0;
    if (dl > ST_MAX_DETAIL) dl = ST_MAX_DETAIL;
    p[o++] = dl;
    if (dl) memcpy(p + o, detail, dl); o += dl;
    queueRawFrame(FRM_STATUS, seqCounter++, p, o);
}

static void sendHeartbeat(uint32_t uptimeMs, uint8_t state, uint16_t crcErrs) {
    uint8_t p[HB_PAYLOAD_SIZE];
    p[0] = uptimeMs >> 24; p[1] = uptimeMs >> 16; p[2] = uptimeMs >> 8; p[3] = uptimeMs & 0xFF;
    p[4] = state;
    p[5] = crcErrs >> 8; p[6] = crcErrs & 0xFF;
    queueRawFrame(FRM_HEARTBEAT, seqCounter++, p, HB_PAYLOAD_SIZE);
}

static void sendError(uint8_t code, const char *detail) {
    uint8_t p[1 + 1 + 48];
    uint16_t o = 0;
    p[o++] = code;
    uint8_t dl = detail ? strlen(detail) : 0;
    if (dl > 48) dl = 48;
    p[o++] = dl;
    if (dl) memcpy(p + o, detail, dl); o += dl;
    queueRawFrame(FRM_ERROR, seqCounter++, p, o);
}

static void sendAck(uint16_t seq) {
    uint8_t p[2] = { (uint8_t)(seq >> 8), (uint8_t)(seq & 0xFF) };
    queueRawFrame(FRM_ACK, seqCounter++, p, 2);
}

// ---------- RX parser ----------
// States resync on any corruption: drop the offending byte and scan for
// magic again.
enum RxState { RX_MAGIC0, RX_MAGIC1, RX_HEADER, RX_PAYLOAD, RX_CRC_HI, RX_CRC_LO };
static enum RxState rxState = RX_MAGIC0;
static uint8_t rxFrame[PROTO_MAX_FRAME];
static uint16_t rxLen = 0;       // bytes accumulated in current frame
static uint16_t rxPayloadLen = 0;
static uint32_t rxLastByteMs = 0;
static uint16_t rxCrcErrors = 0;
static uint8_t fwState = ST_BOOT;

static void rxReset() {
    rxState = RX_MAGIC0;
    rxLen = 0;
    rxPayloadLen = 0;
}

static void onFrame(const uint8_t *frame, uint16_t len) {
    uint8_t type = frame[3];
    uint16_t seq = (frame[4] << 8) | frame[5];
    const uint8_t *payload = frame + PROTO_HEADER_SIZE;
    uint16_t plen = (frame[6] << 8) | frame[7];

    switch (type) {
        case FRM_ACK:
            // Host acknowledged a firmware frame; nothing required.
            break;
        case FRM_DATA:
            // Downlink command channel: op code is payload[0].
            if (plen < 1) {
                sendError(ERR_BAD_PAYLOAD, "empty data frame");
                break;
            }
            uint8_t op = payload[0];
            if (!opAllowed(op)) {
                sendError(ERR_UNSUPPORTED_OP, "op rejected by allowlist");
                sendAck(seq);
                break;
            }
            if (op == OP_SELF_TEST) {
                // Read-only self-test: report TX queue headroom and parser
                // health. No vehicle I/O.
                uint8_t r[4];
                uint16_t room = sizeof(txBuf) - txLen;
                r[0] = room > 255 ? 255 : (uint8_t)room; // saturate
                r[1] = rxCrcErrors >> 8; r[2] = rxCrcErrors & 0xFF;
                r[3] = fwState;
                queueRawFrame(FRM_DATA, seqCounter++, r, sizeof(r));
            }
            sendAck(seq);
            break;
        case FRM_CAPABILITY:
            // Host requests capabilities: reply with a capability frame
            // carrying the same payload layout as hello.
            sendIdentity(FRM_CAPABILITY);
            break;
        default:
            // Firmware-originated types from the host are ignored.
            break;
    }
}

static void rxFeed(uint8_t b) {
    rxLastByteMs = millis();
    switch (rxState) {
        case RX_MAGIC0:
            if (b == PROTO_MAGIC0) { rxFrame[0] = b; rxLen = 1; rxState = RX_MAGIC1; }
            break;
        case RX_MAGIC1:
            if (b == PROTO_MAGIC1) { rxFrame[1] = b; rxLen = 2; rxState = RX_HEADER; }
            else if (b != PROTO_MAGIC0) rxReset();
            break;
        case RX_HEADER:
            rxFrame[rxLen++] = b;
            if (rxLen == PROTO_HEADER_SIZE) {
                rxPayloadLen = (rxFrame[6] << 8) | rxFrame[7];
                if (rxPayloadLen > PROTO_MAX_PAYLOAD) {
                    rxCrcErrors++;
                    rxReset();
                } else {
                    rxState = rxPayloadLen ? RX_PAYLOAD : RX_CRC_HI;
                }
            }
            break;
        case RX_PAYLOAD:
            rxFrame[rxLen++] = b;
            if (rxLen == PROTO_HEADER_SIZE + rxPayloadLen) rxState = RX_CRC_HI;
            break;
        case RX_CRC_HI:
            rxFrame[rxLen++] = b;
            rxState = RX_CRC_LO;
            break;
        case RX_CRC_LO:
            rxFrame[rxLen++] = b;
            {
                uint16_t want = (rxFrame[rxLen - 2] << 8) | rxFrame[rxLen - 1];
                uint16_t got = diag_crc16(rxFrame, PROTO_HEADER_SIZE + rxPayloadLen);
                if (want == got && rxFrame[2] == PROTO_VERSION) {
                    onFrame(rxFrame, rxLen);
                } else {
                    rxCrcErrors++;
                }
                rxReset();
            }
            break;
    }
}

// ---------- Arduino entry points ----------
void setup() {
    Serial.begin(SERIAL_BAUD);
    Serial.setTimeout(0);
    wdt_enable(WDTO_1S);
    txReset();
    rxReset();
    sendHello();
    sendStatus(ST_BOOT, "firmware boot");
    fwState = ST_READY;
    sendStatus(ST_READY, "acquisition ready (vehicle TX disabled)");
}

void loop() {
    wdt_reset();

    // RX with stall timeout -> parser resync.
    while (Serial.available() > 0) {
        rxFeed((uint8_t)Serial.read());
    }
    if (rxState != RX_MAGIC0 && millis() - rxLastByteMs > RX_STALL_MS) {
        rxCrcErrors++;
        rxReset();
    }

    static uint32_t lastHb = 0;
    uint32_t now = millis();
    if (now - lastHb >= HEARTBEAT_MS) {
        lastHb = now;
        sendHeartbeat(now, fwState, rxCrcErrors);
    }

    flushTx();
    delay(1); // yield; watchdog still fed each loop
}
