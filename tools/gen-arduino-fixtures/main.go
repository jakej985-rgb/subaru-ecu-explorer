// Command gen-arduino-fixtures writes hex-encoded protocol frames to
// tests/fixtures/arduino-serial for codec conformance tests.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/arduino"
)

func mustFrame(f arduino.Frame) []byte {
	b, err := arduino.Encode(f)
	if err != nil {
		panic(err)
	}
	return b
}

func write(name string, data []byte) {
	path := filepath.Join("tests/fixtures/arduino-serial", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	hexs := hex.EncodeToString(data)
	if err := os.WriteFile(path, []byte(hexs+"\n"), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", path, len(data), "bytes")
}

func main() {
	dir := "tests/fixtures/arduino-serial"
	_ = dir

	helloPayload, err := (arduino.Hello{
		FirmwareVersion: "0.1.0",
		Board:           "mega2560",
		ProtocolVersion: 1,
		Capabilities:    arduino.CapSelfTest,
	}).EncodeHello()
	if err != nil {
		panic(err)
	}
	write("hello.hex", mustFrame(arduino.Frame{Type: arduino.TypeHello, Seq: 0, Payload: helloPayload}))

	write("heartbeat.hex", mustFrame(arduino.Frame{
		Type: arduino.TypeHeartbeat, Seq: 1,
		Payload: []byte{0x00, 0x00, 0x03, 0xE8, 0x01, 0x00, 0x00},
	}))

	write("status.hex", mustFrame(arduino.Frame{
		Type: arduino.TypeStatus, Seq: 2,
		Payload: append([]byte{0x01, 0x11}, []byte("acquisition ready")...),
	}))

	write("data-batch.hex", mustFrame(arduino.Frame{
		Type: arduino.TypeData, Seq: 3,
		Payload: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
	}))

	write("error.hex", mustFrame(arduino.Frame{
		Type: arduino.TypeError, Seq: 4,
		Payload: append([]byte{0x10, 0x18}, []byte("op rejected by allowlist")...),
	}))

	write("capability.hex", mustFrame(arduino.Frame{
		Type: arduino.TypeCapability, Seq: 5, Payload: helloPayload,
	}))

	// Corrupt stream: valid data frame with flipped CRC byte.
	bad := mustFrame(arduino.Frame{Type: arduino.TypeData, Seq: 6, Payload: []byte{0xAA}})
	bad[len(bad)-1] ^= 0xFF
	write("corrupt-crc.hex", bad)

	// Resync stream: garbage bytes followed by a valid heartbeat.
	garbage := []byte{0x00, 0x11, 0xDE, 0xAD, 0xBE, 0xEF}
	good := mustFrame(arduino.Frame{
		Type: arduino.TypeHeartbeat, Seq: 7,
		Payload: []byte{0x00, 0x00, 0x07, 0xD0, 0x01, 0x00, 0x00},
	})
	write("resync.hex", append(garbage, good...))
}
