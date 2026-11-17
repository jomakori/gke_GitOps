# trading-guard — Hermes plugin (ticket T6)

A Hermes `pre_tool_call` hook that sits in front of the trading tools and
refuses three classes of move. It is a **second, independent trust boundary**
in front of trading-mcp's own `internal/arm` gate — not a replacement for it.
The plugin holds no broker credential and needs none.

## Controls

| # | Control | Refusal `reason_code` | `reason_detail` |
|---|---------|----------------------|-----------------|
| 1 | Mutating trading tool while trading is **not armed** | `TRADE_GUARD_NOT_ARMED` | the arm gate's own reason (`missing_file`, `expired`, `unreadable`, `insecure_permissions`, …) |
| 2 | Any agent tool call that **writes, extends or targets the arm file** | `TRADE_GUARD_ARM_FILE_TAMPER` | `arm_file` / `nonce_sidecar` |
| 3 | **Signal and place inside one turn** (D10 seat separation) | `TRADE_GUARD_SIGNAL_PLACE_SAME_TURN` | `signal_then_place_same_turn` / `place_then_signal_same_turn` / `seat_separation_unverifiable` |

Every refusal is returned as a host directive, never a bare error string:

```json
{
  "action": "block",
  "message": "TRADE_GUARD_ARM_FILE_TAMPER: refusing write_file ...",
  "reason_code": "TRADE_GUARD_ARM_FILE_TAMPER",
  "reason_detail": "nonce_sidecar",
  "control": "arm_file_integrity",
  "tool_name": "write_file",
  "turn_id": "...",
  "session_id": "...",
  "tool_call_id": "..."
}
```

`action` + a non-empty `message` are what the host requires to honour a block
(`hermes_cli/plugins.py:1776-1819`); the remaining keys are the structured
reason an operator or the model can act on. `reason_code` names **which**
control fired, `reason_detail` says **how** — a caller can always tell the
three apart without parsing prose.

## Semantics worth knowing

- **Fail closed on unknown turn (control 3).** Seat separation is correlated on
  the host-supplied `turn_id`. When it is absent, a *place* tool is refused as
  `seat_separation_unverifiable` instead of being allowed — an unidentified turn
  cannot be proven seat-separated. A signal alone is still allowed; the place
  half will refuse itself.
- **Refusals still record.** A refused signal is recorded as the turn's signal,
  so a later place in the same turn is refused too — refusing cannot launder a
  tainted turn.
- **The arm file is writable by the operator, not by agents.** Control 2 matches
  `write_file` / `append_file` / `patch` / `edit_file` paths and `terminal`
  commands (including `/data/arm/trading-mcp.armed.json` and its
  `.nonces.json` sidecar), because the nonce sidecar is part of the same
  integrity boundary.
- **Path parity is the whole control.** `internal/runconfig` builds the arm
  path as `<data-dir>/arm/trading-mcp.armed.json` — `/data/arm/trading-mcp.armed.json`
  in the deployment. The `arm` subdirectory is not cosmetic: it is `0700`
  because `internal/arm` refuses to read an arm file out of a group-writable
  directory, and `/data` is group-writable on a shared claim. **A host-side
  adapter guard MUST read exactly that path — `/data/arm/trading-mcp.armed.json`
  — because that is the path the engine writes.** A guard reading any other
  path refuses every mutating call for the wrong reason and protects nothing.
  `TRADING_MCP_ARM_FILE` is a plugin-side override for tests and adapters only;
  the engine reads no path from its environment, so setting it in a deployment
  is a guaranteed mismatch.
- **In-process state.** The turn→tool map lives in the hook process; a restart
  clears it. Control 1 and control 2 hold no state at all.

## Install (build wiring)

Hermes needs both halves, and the second is easy to miss: the package must be in
`$HERMES_HOME/plugins/<name>/` **and** the plugin must be in `plugins.enabled` —
user plugins are opt-in (`hermes_cli/plugins_discovery.py::gate_manifest` →
`"not enabled in config"`).

```bash
./install.sh                       # $HERMES_HOME (default ~/.hermes)
HERMES_HOME=/tmp/hh ./install.sh   # elsewhere
./install.sh --dry-run             # show the steps, change nothing
```

`install.sh` validates, copies the runtime surface (`plugin.yaml`,
`__init__.py`, this file — not the tests), enables the plugin with
`--no-allow-tool-override`, then verifies with `hermes plugins doctor` and with
`host_integration_check.py`.

## Verify

```bash
# hermetic: guard logic + refusal codes + allow path (no Hermes needed)
uv run --python 3.13 --with pytest -- python -m pytest plugin -q -o addopts=

# real host: discovery -> manifest -> register() -> directive resolution
python plugin/host_integration_check.py --hermes-home "$HERMES_HOME"

# which test asserts which exact code (and that none asserts "some error")
python plugin/audit_refusal_codes.py

# non-vacuity: neuter each control -> the suite must go RED
uv run --python 3.13 --with pytest -- python plugin/nonvacuity_check.py
```

`test_guard.py` asserts the exact `reason_code` **and** `reason_detail` for every
refusal, and the allow path for each control. The suite was mutation-checked:
with any single control neutered (or with the empty-`turn_id` case reverted to
fail-open, or refusals degraded to a generic `ERROR`), the suite goes red.

## Scope

This ticket adds only `plugin/`. The rest of the tree (arm gate, supervisor,
heartbeat, broker, runconfig, `cmd/trading-*`) is earlier tickets' work, already
present in the shared worktree; it is isolated in its own commit so
`git diff --stat <baseline>` shows `plugin/` only.
