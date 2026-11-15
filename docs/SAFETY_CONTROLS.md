# Hardware Safety Controls (P2.6)

Issue #28 — operation allowlists, read-only capability gates, health
preconditions, rate limits, and simulated transport tests. Companion:
`SAFETY.md` (policy), `ADAPTER_ARCHITECTURE.md` (session contracts).

## The gate

`internal/safety` provides `GatedSession` — the **only supported API for
application-level operations**. It wraps an `adapter.Session` and every
request goes through `Transact(ctx, op, request)`:

1. **Operation allowlist** — only `identify`, `read-data`, `read-dtc`,
   `read-measurement` are permitted by default. Everything else —
   including `write`, `clear-dtc`, `actuator-test`, `programming`,
   `flash`, `security-access`, `immobilizer`, `pin-probe`, and any
   unrecognized operation — fails with `ErrProhibitedOperation` **before
   a single byte is written**.
2. **Read-only capability gate** — sessions that do not declare
   `Capabilities.ReadOnly` are rejected with `ErrReadOnlyRequired`
   (policy-enforced, not just conventional).
3. **Health precondition** — only sessions in an allowed health state
   (default: `ready`) may transact; `ErrHealthGate` otherwise.
4. **Custom preconditions** — `Policy.Preconditions` hooks run before
   every transaction (voltage checks, operator confirmations, etc.).
   First failure aborts with `ErrPrecondition` and no I/O.
5. **Rate limit** — `MinInterval` enforces minimum spacing between
   transactions; violations return `ErrRateLimited` without writing.
6. **Bounded I/O** — deadlines come from the context and
   `ResponseTimeout`/`ResponseIdleTimeout`; responses cap at
   `MaxResponseBytes`.

`Authorize(ctx, op)` performs the same checks without I/O (for UI
previews). `Inner()` exposes the raw session for protocol-layer code —
framing, checksums, message decoding — which is the only legitimate
bypass, and it is not part of the normal API surface.

## Why a gate above the transport

`adapter.Session` exposes raw `Read`/`Write` because protocol
implementations need byte streams. The safety boundary therefore cannot
live in the transport — it lives in the operation layer, where "send a
request" is an `Operation`, not bytes. Prohibited operations cannot be
invoked through `Transact` at all, and tests prove zero bytes reach the
transport for every prohibited op.

## Simulated transport tests

`internal/safety/safety_test.go` uses the fake transport/session:

- All prohibited + unknown ops → `ErrProhibitedOperation`, empty write
  buffer, nil response.
- All allowed ops → request bytes written, response accumulated.
- Non-read-only session → `ErrReadOnlyRequired`, no write.
- Degraded health → `ErrHealthGate`.
- Rate limit → second immediate op `ErrRateLimited`, no write; succeeds
  after interval.
- Failed precondition → `ErrPrecondition`, no write.
- Cancelled context → `context.Canceled`, no write.

## Limitations

- The gate is advisory at the library level: nothing physically prevents
  a caller from using `Inner().Write` directly. REST/WebSocket APIs built
  later must expose only `Transact`-level operations.
- Voltage/connection preconditions are hook points; no transport reports
  vehicle voltage yet (no hardware).
- Rate limiting is per `GatedSession`, not global — wrap sessions at
  creation to get consistent policy.
- Operation names are project-defined strings; protocol-specific command
  sets map onto them in later layers.
