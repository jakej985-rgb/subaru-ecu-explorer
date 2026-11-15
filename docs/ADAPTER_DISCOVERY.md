# Adapter Discovery & Host Transports (P2.4)

Issue #26 — host-side enumeration and explicit selection for USB/serial and
network adapter transports, with stable identity where available. Companion:
`ADAPTER_ARCHITECTURE.md` (contracts), `OBD2_INTERFACE.md` (vehicle side).

## Enumerators

`adapter.Enumerator` (`internal/adapter/enumerator.go`) lists candidates
without opening them. Two implementations ship:

| Enumerator | Package | Candidates | Identity |
|------------|---------|------------|----------|
| `serialenum.Enumerator` | `internal/adapter/serialenum` | `/dev/serial/by-id/*` symlinks + `ttyUSB*`/`ttyACM*` nodes under a configurable root (default `/dev`) | by-id names are **stable** (`stable=true`); bare tty nodes are flagged `stable=false` because `ttyUSB0` can change across replug |
| `adapter.ListEnumerator` | `internal/adapter` | Explicitly configured endpoints — network `host:port`, manual device paths | Whatever the caller supplies; this is the **explicit user selection** path |

A tty node that a by-id link resolves to is collapsed into the by-id
candidate, so one physical adapter yields one candidate ID. As a backstop,
`serialport` sets `TIOCEXCL` on open, so a second open through any alias
fails with `ErrBusy` (root can bypass `TIOCEXCL`).

`serialenum` never opens devices; a missing `serial/by-id` directory is
normal (returns no by-id candidates), while unreadable roots surface a
joined error alongside whatever was enumerated.

## Drivers

| Driver | Package | Transport |
|--------|---------|-----------|
| `serialport.Driver` | `internal/adapter/serialport` | POSIX serial via `x/sys/unix` termios (8N1 raw, configurable baud from candidate `Metadata["baud"]` or driver default) with `poll`-based `SetDeadline` |
| `tcpdev.Driver` | `internal/adapter/tcpdev` | TCP via `net.Dialer`; native deadline support. **No network scanning** — endpoints come only from `ListEnumerator` (explicit config), consistent with the no-arbitrary-probing rule |

Both produce `adapter.Session`s with `ReadOnly` capability declared —
transport support does not imply protocol or vehicle compatibility.

## Errors

- `ErrNotFound` — device node/path vanished between discovery and open.
- `ErrPermission` — host denied access (e.g. `EACCES` on the device node).
- `ErrBusy` — device already owned (Manager, unchanged).
- `ErrTimeout` / `ErrClosed` — deadline expiry / I/O after close.
- Non-TTY paths fail termios configuration rather than producing a broken
  session.

## Behavior coverage (acceptance criteria)

| Scenario | Test |
|----------|------|
| Hot-plug | `serialenum.TestEnumerateHotPlug` — re-enumeration sees new/removed nodes; `serialport.TestManagerHotPlugReconnect` |
| Missing device | `serialport.TestOpenMissingDevice`, `tcpdev.TestTCPDriverMissingEndpoint` |
| Permission failure | `serialport.TestOpenPermissionDenied` (skipped when root) |
| Ambiguous devices | `serialenum.TestEnumerateAmbiguousDevices` — both stay listed with distinct IDs; user selects explicitly |
| Reconnect | `tcpdev.TestManagerReconnectCycle` — connect → `ErrBusy` → disconnect → new session |

## Usage sketch

```go
// Manager keeps one driver per Kind (re-registering replaces), so compose
// serial + TCP for the same kind with MultiDriver.
m := adapter.NewManager()
m.RegisterDriver(adapter.NewMultiDriver(adapter.KindELM327,
    serialport.NewDriver(adapter.KindELM327, serialenum.New(""), 38400),
    tcpdev.NewDriver(adapter.KindELM327,
        adapter.NewListEnumerator(adapter.Candidate{
            Info: adapter.Info{ID: "wifi-elm327", Path: "192.168.0.10:35000"},
        })),
))

cands, _ := m.Discover(ctx)          // USB serial + configured endpoints
s, err := m.Connect(ctx, chosen)     // explicit user selection by candidate
```

## Limitations

- POSIX/Linux only (`/dev`, termios); no Windows/macOS serial backends yet.
- `stable=false` tty candidates can alias across replug; prefer by-id.
- No BLE/USB-CDC hotplug events (inotify) — discovery is poll-based.
- Serial drivers configure baud but do not implement ELM327/SSM line
  discipline; that is protocol-layer work.
- USB VID/PID identification is not yet decoded from sysfs — candidates
  are distinguished by by-id name/path only.
