"""trading-guard — Hermes plugin for trading-mcp (plan v4, ticket T6).

Registers one ``pre_tool_call`` hook that sits in front of every tool call an
agent can make and refuses three things:

1. **Not armed.** Any mutating trading tool (:data:`MUTATING_TRADING_TOOLS`)
   called while ``internal/arm``'s arm file is not currently valid. This is
   a SECOND, independent check layered in front of ``cmd/trading-mcp``'s own
   ``requireArmed`` gate (``cmd/trading-mcp/main.go``) — not a replacement
   for it. ``check_armed()`` below re-implements the read side of
   ``internal/arm.Checker.Check`` against the exact same file, in this
   process's own trust domain, so an agent that can somehow reach the MCP
   server directly (bypassing this plugin) still hits the authoritative Go
   gate, and an agent that can somehow influence this plugin's process
   still hits the Go gate on the other side. Neither check alone is
   "the" security boundary; both must independently agree the file is
   valid.

2. **Arm-file tamper.** Any agent tool call — not only trading tools, but
   the general filesystem tools every agent has (``write_file``, ``patch``,
   ``terminal``) — that targets the arm file or its nonce sidecar
   (``internal/arm.seenNoncePath``). Arming is exclusively a property of a
   file written by the operator-only ``cmd/trading-arm`` CLI
   (``internal/arm`` package doc); nothing reachable from an agent turn may
   create, extend, or overwrite it.

3. **Signal+place same turn (D10 seat separation, plan v4 §1.3).** The
   stage that generates a trade signal and the stage that places an order
   may not both act within one agent turn, in either order. Correlated on
   the host-supplied ``turn_id`` (``agent/turn_context.py`` sets a
   non-empty ``_current_turn_id`` at every turn start and
   ``agent_runtime_helpers`` passes it to ``pre_tool_call``), tracked per
   turn in memory (bounded, see :data:`_MAX_TRACKED_TURNS`); the state
   needs no durability because a new turn always starts clean.

   When the host supplies NO turn_id the ordering cannot be verified, so
   the PLACEMENT side fails CLOSED — a place tool is refused with
   ``reason_detail = seat_separation_unverifiable`` rather than allowed.
   The signal side stays allowed on its own (a signal emits no order), and
   an unverifiable place is refused unconditionally, so the pair still
   cannot complete. This is deliberate: for a trading guard, "I cannot
   tell" must never resolve to "go ahead".

Every refusal is a structured dict naming the exact control that fired
(``reason_code``), never a generic error — see :func:`_refusal`. The plugin
holds no broker credential and needs none: it has no state that could arm
trading, lower a cap, or place an order on its own.
"""

from __future__ import annotations

import json
import os
import re
import stat
from datetime import datetime, timezone
from typing import Any, Optional

# ---------------------------------------------------------------------------
# Arm file location. Mirrors internal/runconfig: one fixed name inside the
# data directory (/data), so this plugin and trading-mcp always agree on
# where the operator-written arm file lives without a second place to
# configure it.
# ---------------------------------------------------------------------------

ARM_FILE_ENV = "TRADING_MCP_ARM_FILE"
ARM_FILE_PATH = "/data/trading-mcp.armed.json"


def default_arm_file_path() -> str:
    """Mirrors internal/runconfig.NewConfig(""): the arm file is a fixed
    name inside the data directory, not a configurable path. The env var
    above is honoured only as a test/back-compat override."""
    return ARM_FILE_PATH


def arm_file_path() -> str:
    """Mirrors internal/arm.PathFromEnv(): the configured path, falling
    back to the default. Read fresh every call — never cached — so an
    operator repointing the env var mid-session takes effect immediately."""
    return os.environ.get(ARM_FILE_ENV) or default_arm_file_path()


def nonce_sidecar_path(arm_path: str) -> str:
    """Mirrors internal/arm.seenNoncePath(): the sidecar file next to the
    arm file that persists nonce-reuse detection across a server restart."""
    return arm_path + ".nonces.json"


# ---------------------------------------------------------------------------
# Refusal codes — one per control. Every refusal this plugin returns names
# exactly one of these in ``reason_code``; tests assert on the code, never
# on the human-readable message text.
# ---------------------------------------------------------------------------

CODE_NOT_ARMED = "TRADE_GUARD_NOT_ARMED"
CODE_ARM_FILE_TAMPER = "TRADE_GUARD_ARM_FILE_TAMPER"
CODE_SIGNAL_PLACE_SAME_TURN = "TRADE_GUARD_SIGNAL_PLACE_SAME_TURN"

# Sub-reasons for CODE_NOT_ARMED, spelled identically to internal/arm's
# Reason* constants (arm.go) so an operator reading a refusal here recognises
# the same vocabulary cmd/trading-mcp itself would log for the same file.
REASON_ARMED = "armed"
REASON_MISSING = "not_armed_missing_file"
REASON_MALFORMED = "not_armed_malformed_file"
REASON_EXPIRED = "not_armed_expired"
REASON_FUTURE_DATED = "not_armed_future_dated"
REASON_TTL_TOO_LONG = "not_armed_ttl_exceeds_max"
REASON_INSECURE_PERMISSIONS = "not_armed_insecure_permissions"

MAX_TTL_SECONDS = 24 * 60 * 60

# ``reason_detail`` — the structured sub-reason inside a refusal. ``reason_code``
# names WHICH control fired; ``reason_detail`` says how it fired, so an operator
# (or a test) never has to parse prose.
DETAIL_ARM_FILE = "arm_file"
DETAIL_NONCE_SIDECAR = "nonce_sidecar"
DETAIL_SIGNAL_THEN_PLACE = "signal_then_place_same_turn"
DETAIL_PLACE_THEN_SIGNAL = "place_then_signal_same_turn"
DETAIL_SEAT_SEPARATION_UNVERIFIABLE = "seat_separation_unverifiable"

# ---------------------------------------------------------------------------
# Trading tool taxonomy. trading-mcp registers no mutating tool yet — T1
# (place_order and friends) is a later ticket than this one — but the plan
# (plan v4 §T1) commits to these exact names. Listing them explicitly, rather
# than inferring "mutating" from a naming heuristic, means a tool the server
# adds later is SAFE BY DEFAULT (never matched, so never gated) until a
# reviewer deliberately adds it here.
# ---------------------------------------------------------------------------

MUTATING_TRADING_TOOLS = frozenset({
    "place_order",
    "cancel_order",
    "amend_order",
    "modify_order",
    "close_position",
})

# The "placement" half of D10's seat separation. Only an actual order
# submission counts — cancelling or amending an order already placed is not
# "placing", so it is deliberately excluded here.
PLACE_TOOLS = frozenset({"place_order"})

# The "signal" half: whatever stage scores or generates a trade idea.
SIGNAL_TOOLS = frozenset({"generate_signal", "trade_signal", "signal", "score_signal"})

# Generic filesystem-mutating tools this hook also watches. These are not
# trading tools — they are the mechanism by which an agent could otherwise
# write or extend the arm file directly, bypassing the operator-only
# cmd/trading-arm CLI that is supposed to be the only writer.
FS_WRITE_TOOLS = frozenset({"write_file", "patch", "terminal"})

# ---------------------------------------------------------------------------
# MCP tool-name matching. Hermes registers an MCP server's tools under
# ``mcp__<sanitizedServer>__<sanitizedTool>`` (tools/mcp_tool_schema.py:
# MCP_TOOL_NAME_PREFIX, mcp_prefixed_tool_name()) — NOT under the bare tool
# name. If trading-mcp is ever wired in as an MCP server rather than a
# native tool, an exact-string check against "place_order" would silently
# never match "mcp__trading_mcp__place_order" and all three controls below
# would stop firing without any visible error. ``_matches_tool_set`` closes
# that by also checking the suffix after the last "__" against the bare
# name set when the tool name carries the MCP prefix.
# ---------------------------------------------------------------------------

MCP_TOOL_NAME_PREFIX = "mcp__"


def _matches_tool_set(tool_name: str, names: frozenset) -> bool:
    """True if tool_name is one of names, either directly (native tool) or
    as the tool-name suffix of an mcp__<server>__<tool>-prefixed name."""
    if tool_name in names:
        return True
    if tool_name.startswith(MCP_TOOL_NAME_PREFIX) and "__" in tool_name[len(MCP_TOOL_NAME_PREFIX):]:
        return tool_name.rsplit("__", 1)[-1] in names
    return False


# Verbs/operators that indicate a terminal command WRITES rather than only
# reads. Used only to decide whether a command that mentions the arm file's
# basename is a tamper attempt; a bare `cat <armfile>` is a read, not a
# write, and control 2 is about write/extend, not disclosure.
_SHELL_WRITE_PATTERN = re.compile(
    r"(>>|>|\|\s*tee\b|\bcp\b|\bmv\b|\brm\b|\btruncate\b|\bdd\b|\bsed\b[^\n]*-i\b|"
    r"\binstall\b|\bln\b|\brsync\b|\btouch\b|\bshred\b|\bchmod\b|\bchown\b)"
)


def _command_looks_like_write(command: str) -> bool:
    return bool(_SHELL_WRITE_PATTERN.search(command))


def _parse_iso8601(value: str) -> Optional[datetime]:
    try:
        v = value[:-1] + "+00:00" if value.endswith("Z") else value
        parsed = datetime.fromisoformat(v)
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed
    except (ValueError, TypeError, IndexError):
        return None


def check_armed(now: Optional[datetime] = None) -> tuple[bool, str]:
    """Best-effort, read-only re-check of the arm file, from THIS process's
    trust domain.

    This is defense in depth layered in front of cmd/trading-mcp's own
    internal/arm.Checker, which is the authoritative gate and independently
    re-validates on every mutating call it serves. A bug or bypass here can
    only ever over-refuse (fail closed); it must never be the only thing
    standing between an agent and a live order — that guarantee comes from
    the Go process, which holds the broker credential and which this
    plugin never does.

    Deliberately does NOT replicate internal/arm's nonce-reuse detection:
    that persisted, cross-restart state exists to catch a replayed/edited
    file BETWEEN two reads by the authoritative checker, a narrower case
    this secondary, advisory layer does not need to close on its own.
    """
    now = now or datetime.now(timezone.utc)
    path = arm_file_path()

    try:
        st = os.stat(path)
    except OSError:
        return False, REASON_MISSING

    try:
        mode = stat.S_IMODE(st.st_mode)
        if mode & 0o077:
            return False, REASON_INSECURE_PERMISSIONS
        if hasattr(os, "getuid") and st.st_uid != os.getuid():
            return False, REASON_INSECURE_PERMISSIONS
        parent_st = os.stat(os.path.dirname(path) or ".")
        parent_mode = stat.S_IMODE(parent_st.st_mode)
        if parent_mode & 0o022:
            return False, REASON_INSECURE_PERMISSIONS
    except OSError:
        return False, REASON_INSECURE_PERMISSIONS

    try:
        with open(path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, ValueError):
        return False, REASON_MALFORMED

    if not isinstance(data, dict):
        return False, REASON_MALFORMED

    nonce = data.get("nonce")
    armed_at_raw = data.get("armed_at")
    expires_at_raw = data.get("expires_at")
    per_order = data.get("per_order_cap_cents")
    daily = data.get("daily_cap_cents")

    if not isinstance(nonce, str) or not nonce:
        return False, REASON_MALFORMED
    if not isinstance(armed_at_raw, str) or not isinstance(expires_at_raw, str):
        return False, REASON_MALFORMED
    if not isinstance(per_order, (int, float)) or isinstance(per_order, bool):
        return False, REASON_MALFORMED
    if not isinstance(daily, (int, float)) or isinstance(daily, bool):
        return False, REASON_MALFORMED
    if per_order <= 0 or daily <= 0:
        return False, REASON_MALFORMED

    armed_at = _parse_iso8601(armed_at_raw)
    expires_at = _parse_iso8601(expires_at_raw)
    if armed_at is None or expires_at is None:
        return False, REASON_MALFORMED
    if expires_at <= armed_at:
        return False, REASON_MALFORMED

    if armed_at > now:
        return False, REASON_FUTURE_DATED
    if (expires_at - armed_at).total_seconds() > MAX_TTL_SECONDS:
        return False, REASON_TTL_TOO_LONG
    if now >= expires_at:
        return False, REASON_EXPIRED

    return True, REASON_ARMED


_BASENAME_WORD_FMT = r"(?<![\w.-])%s(?![\w.-])"


def _terminal_command_targets_arm_file(command: str, targets: tuple) -> bool:
    """True if a ``terminal`` command reaches the arm file or its nonce
    sidecar by any route, not only the literal resolved absolute path.

    A substring check against the full absolute path (the original
    implementation) misses: a relative path used after ``cd``, a symlink
    with an unrelated name, or a path assembled at runtime from the same
    ``TRADING_MCP_ARM_FILE`` env var this plugin reads. All of those still
    mention the file's BASENAME somewhere in the command text (or, for the
    env-var case, the var name itself), so we match on that instead of the
    full path — deliberately broader, which only risks over-refusing a
    terminal command (fail closed), never under-refusing a tamper attempt.
    """
    for target in targets:
        basename = os.path.basename(target)
        if re.search(_BASENAME_WORD_FMT % re.escape(basename), command):
            return True
        if os.path.realpath(target) in command:
            return True

    # $VAR expansion: the command never mentions the resolved path or
    # basename literally, but builds it from the same env var name
    # trading-mcp and this plugin both read. Only a tamper attempt when
    # paired with a write verb/redirect — printing the var name alone
    # (e.g. to log it) is not a write.
    if ARM_FILE_ENV in command and _command_looks_like_write(command):
        return True

    return False


def _tool_targets_arm_file(tool_name: str, args: dict) -> Optional[str]:
    """Returns which protected target this filesystem-mutating tool call
    reaches — :data:`DETAIL_ARM_FILE` / :data:`DETAIL_NONCE_SIDECAR` — or
    ``None`` when the call is unrelated to arming."""
    if not _matches_tool_set(tool_name, FS_WRITE_TOOLS):
        return None

    armed_path = arm_file_path()
    sidecar_path = nonce_sidecar_path(armed_path)
    # LONGEST realpath first: the sidecar path is the arm path plus a suffix,
    # so a plain substring test would attribute a sidecar write to the arm
    # file. Longest-first makes the more specific target win.
    targets = tuple(
        sorted(
            ((DETAIL_ARM_FILE, armed_path), (DETAIL_NONCE_SIDECAR, sidecar_path)),
            key=lambda kv: len(os.path.realpath(kv[1])),
            reverse=True,
        )
    )

    # Use the BARE suffix (after the mcp__<server>__ prefix, if any) to pick
    # the write_file/patch vs terminal argument shape — an MCP-exposed
    # "terminal" tool still takes a "command" arg, not a "path" one.
    bare_name = tool_name
    if tool_name.startswith(MCP_TOOL_NAME_PREFIX) and "__" in tool_name[len(MCP_TOOL_NAME_PREFIX):]:
        bare_name = tool_name.rsplit("__", 1)[-1]

    if bare_name in ("write_file", "patch"):
        path = args.get("path")
        if not isinstance(path, str) or not path:
            return None
        # realpath (not abspath) resolves symlinks: a symlink whose TARGET
        # is the arm file is caught even though the symlink's own name is
        # unrelated.
        resolved = os.path.realpath(path)
        for kind, target in targets:
            if resolved == os.path.realpath(target):
                return kind
        return None

    if bare_name == "terminal":
        command = args.get("command")
        if not isinstance(command, str) or not command:
            return None
        # $VAR case first, attributed to the ARM file explicitly: the var
        # names the arm file and the sidecar is derived from it, so letting
        # target iteration order decide the attribution would label the same
        # command differently depending on path lengths.
        if ARM_FILE_ENV in command and _command_looks_like_write(command):
            return DETAIL_ARM_FILE
        for kind, target in targets:
            if _terminal_command_targets_arm_file(command, (target,)):
                return kind
        return None

    return None


# ---------------------------------------------------------------------------
# Signal+place seat separation state (control 3). Per-turn, in-memory,
# bounded. Needs no durability: a new agent turn always starts clean, so
# there is nothing meaningful to persist across a process restart.
# ---------------------------------------------------------------------------

_MAX_TRACKED_TURNS = 2048
_turn_tool_seen: dict[str, set] = {}


def reset_state() -> None:
    """Test-only: clear seat-separation tracking between test cases."""
    _turn_tool_seen.clear()


def _seat_separation_verdict(turn_id: str, tool_name: str) -> tuple[bool, str]:
    """D10 seat separation. Returns ``(refuse, reason_detail)``.

    ``reason_detail`` is one of :data:`DETAIL_SIGNAL_THEN_PLACE`,
    :data:`DETAIL_PLACE_THEN_SIGNAL`, :data:`DETAIL_SEAT_SEPARATION_UNVERIFIABLE`,
    or ``""`` when the call is allowed.

    Side effect: a signal/place call is recorded against ``turn_id``
    regardless of outcome, so a LATER call in the same turn is caught too —
    including one made after an earlier call in the turn was refused.

    No ``turn_id`` means the ordering cannot be established. The placement
    side then fails CLOSED (refuse, detail ``seat_separation_unverifiable``);
    the signal side is allowed alone, because a signal places nothing and any
    following place is itself refused while the key is missing. A guard that
    answers "I cannot tell" with "go ahead" is not a guard.
    """
    is_place = _matches_tool_set(tool_name, PLACE_TOOLS)
    is_signal = _matches_tool_set(tool_name, SIGNAL_TOOLS)
    if not is_place and not is_signal:
        return False, ""
    if not turn_id:
        # Fail CLOSED on the placement half only — see docstring.
        return is_place, (DETAIL_SEAT_SEPARATION_UNVERIFIABLE if is_place else "")

    if turn_id not in _turn_tool_seen and len(_turn_tool_seen) >= _MAX_TRACKED_TURNS:
        oldest = next(iter(_turn_tool_seen), None)
        if oldest is not None:
            _turn_tool_seen.pop(oldest, None)

    seen = _turn_tool_seen.setdefault(turn_id, set())
    if is_place and "signal" in seen:
        seen.add("place")
        return True, DETAIL_SIGNAL_THEN_PLACE
    if is_signal and "place" in seen:
        seen.add("signal")
        return True, DETAIL_PLACE_THEN_SIGNAL
    seen.add("place" if is_place else "signal")
    return False, ""


def _refusal(code: str, detail: str, message: str, **extra: Any) -> dict[str, Any]:
    """Build a structured refusal. ``reason_code`` names the exact control
    that fired and ``reason_detail`` how it fired; ``message`` (what a host
    that only reads the block message will show the model) is prefixed with
    both, so the control stays identifiable even where only the string
    survives. The host requires a non-empty ``message`` for a block to take
    effect (``hermes_cli/plugins.py``), so it is never empty here."""
    payload: dict[str, Any] = {
        "action": "block",
        "reason_code": code,
        "reason_detail": detail,
        "message": f"{code} [{detail}]: {message}",
    }
    payload.update(extra)
    return payload


def _on_pre_tool_call(
    tool_name: str = "",
    args: Any = None,
    turn_id: str = "",
    session_id: str = "",
    tool_call_id: str = "",
    **_: Any,
) -> Optional[dict[str, Any]]:
    """``pre_tool_call`` hook. ``None`` = allow; a dict with
    ``{"action": "block", "reason_code", "reason_detail", "message"}`` = refuse.

    Signature and return shape follow the host's own dispatch
    (``hermes_cli/plugins.py::_get_pre_tool_call_directive_details``), which
    calls ``pre_tool_call`` with ``tool_name``/``args``/``turn_id``/
    ``session_id``/``tool_call_id`` and blocks on a directive carrying
    ``action`` + a non-empty ``message``.
    """
    args = args if isinstance(args, dict) else {}
    # Same ids on every refusal, so an operator can join a refusal to the log
    # line of the call that produced it.
    ids = {"tool_name": tool_name, "turn_id": turn_id, "session_id": session_id,
           "tool_call_id": tool_call_id}

    # Control 2: arm-file tamper. Checked for every tool call regardless of
    # tool_name — write_file/patch/terminal reach the arm file through the
    # agent's general filesystem tools, never through a trading tool name.
    tamper_target = _tool_targets_arm_file(tool_name, args)
    if tamper_target is not None:
        return _refusal(
            CODE_ARM_FILE_TAMPER,
            tamper_target,
            "agent tool calls may never write, extend, or overwrite the arm file or its "
            "nonce sidecar; arming is exclusively a property of a file written by the "
            "operator-only cmd/trading-arm CLI, never something an agent turn can reach.",
            **ids,
        )

    # Control 3: signal+place seat separation (D10).
    refuse, detail = _seat_separation_verdict(turn_id, tool_name)
    if refuse:
        message = (
            "the stage that generates a trade signal and the stage that places an order "
            "may not both act within one agent turn."
            if detail != DETAIL_SEAT_SEPARATION_UNVERIFIABLE
            else "this order cannot be cleared: the turn it belongs to is unidentified, so "
                 "seat separation cannot be verified. Re-issue it from a normal agent turn."
        )
        return _refusal(CODE_SIGNAL_PLACE_SAME_TURN, detail, message, **ids)

    # Control 1: not armed, for mutating trading tools only.
    if _matches_tool_set(tool_name, MUTATING_TRADING_TOOLS):
        armed, reason = check_armed()
        if not armed:
            return _refusal(
                CODE_NOT_ARMED,
                reason,
                f"trading is not armed ({reason}); an operator must run cmd/trading-arm "
                "before this tool may be called.",
                arm_reason=reason,
                **ids,
            )

    return None


def register(ctx) -> None:
    ctx.register_hook("pre_tool_call", _on_pre_tool_call)
