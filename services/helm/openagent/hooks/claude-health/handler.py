"""``agent:start`` hook: report the Claude lane's authoritative upstream status.

Claude is the fleet's only reasoning route, and when the proxy's OAuth session
dies every claude* model fails silently while LiteLLM walks the chains to the
DeepSeek-direct fallbacks. The proxy records the REAL result of the last
completed upstream request (OCP DEV-51) and exposes it on ``/health`` as::

    "upstream": {"outcome": "success" | "auth_rejected" | "usage_limited"
                            | "other" | "unobserved",
                 "observedAt": <ISO-8601> | null,
                 "stale": <bool>}

That snapshot is authoritative: it is grounded in what a real request actually
did, not in an inference probe. This hook reads it and reports exactly one
category, making no model/usage call of its own and never inferring health from
token presence.

Categories:

* ``healthy``        — a FRESH authoritative ``success``: a real request completed
  recently. Reported, no channel post.
* ``auth_failure``   — CONCLUSIVE auth rejection: a fresh ``auth_rejected``, or,
  when no fresh result exists, a conclusive auth-probe rejection. This is the
  ONLY category that prescribes OAuth renewal.
* ``usage_limit``    — a fresh ``usage_limit`` (quota / overage / rate window).
  Prescribes waiting for the window to reset (or quota handling), and explicitly
  NOT OAuth renewal.
* ``provider_error`` — a fresh ``other``, or a proxy/transport/HTTP failure. A
  generic provider/proxy fault, never an auth claim.
* ``stale``          — no fresh authoritative result: ``unobserved`` (the proxy has
  completed no request yet) or a result that aged past the TTL. Reported as
  exactly that — never as healthy.

A "token present but the probe cannot tell" reading (``auth.ok is null``) is
inconclusive: it lands in ``stale`` and does not post, because that is the
proxy's normal state on a token env host and alerting on it is a false alarm.
"""

from __future__ import annotations

import os
import time

import aiohttp

_HEALTH_URL = os.environ.get(
    "CLAUDE_PROXY_HEALTH_URL", "http://claude-proxy:4523/health"
)
_DISCORD_API = "https://discord.com/api/v10"
# Session starts arrive in bursts (threads, cron); one probe per window is enough.
_THROTTLE_S = 300

# Categories worth a channel post. ``healthy`` and ``stale`` are deliberately
# absent: a fresh success is not news, and "no fresh result" (boot, quiet period)
# is not a fault — neither should page anyone.
_ACTIONABLE_KINDS = frozenset({"auth_failure", "usage_limit", "provider_error"})

# Upstream outcomes the proxy reports, mapped to a category. These are the
# exact strings OCP's `UPSTREAM_OUTCOMES` publishes (lib/upstream-status.mjs,
# DEV-51): a mismatch would silently degrade every outcome to ``stale``.
# ``unobserved`` is handled separately (it never carries a fresh result).
_OUTCOME_KIND = {
    "success": "healthy",
    "auth_rejected": "auth_failure",
    "usage_limited": "usage_limit",
    "other": "provider_error",
}

_ICONS = {
    "healthy": "✅",
    "auth_failure": "⚠️",
    "usage_limit": "⏳",
    "provider_error": "⚠️",
    "stale": "ℹ️",
}

_last_check = 0.0
_logged_kinds: set[str] = set()  # kinds already logged this process


def _short(text: object, limit: int = 160) -> str:
    """Collapse whitespace and bound length for a one-line detail."""
    return " ".join(str(text).split())[:limit]


def _finding(kind: str, headline: str, detail: str, remedy: str = "") -> dict:
    return {"kind": kind, "headline": headline, "detail": detail, "remedy": remedy}


def _conclusive_auth_rejection(auth: dict) -> bool:
    """True only on POSITIVE evidence the credential was rejected.

    ``ok is None`` (token present, probe cannot tell) is NOT conclusive, and is
    deliberately excluded: that is the proxy's normal state on a token env host.
    """
    failures = auth.get("consecutiveFailures")
    return (
        auth.get("ok") is False
        or auth.get("lastOutcome") == "rejected"
        or (isinstance(failures, int) and not isinstance(failures, bool) and failures > 0)
    )


def _stale_finding(upstream: dict | None, auth: dict, payload: dict) -> dict:
    """No fresh authoritative result — say so, without ever claiming health."""
    if not isinstance(upstream, dict):
        return _finding(
            "stale",
            "Claude lane status unavailable",
            "the proxy does not report an authoritative upstream outcome "
            "(no `upstream` field — an older proxy build without the "
            "actual-outcome status contract).",
            "This says nothing about the credential. The lane's real status is "
            "unknown until the proxy exposes the upstream snapshot.",
        )

    outcome = upstream.get("outcome")
    observed_at = upstream.get("observedAt")

    if outcome == "unobserved" or not observed_at:
        return _finding(
            "stale",
            "Claude lane status unobserved",
            "the proxy has completed no upstream request yet, so there is no "
            "authoritative result to report.",
            "Normal after a restart or during a quiet period. Not a fault and "
            "not a health signal — the lane's status is unknown, not healthy.",
        )

    # A result exists but aged past the proxy's TTL.
    return _finding(
        "stale",
        "Claude lane status stale",
        f"the last upstream result was '{_short(outcome, 40)}' at "
        f"{_short(observed_at, 40)}, aged past the freshness TTL.",
        "This is the last known outcome, not a fresh reading — do not treat it "
        "as current. It says nothing about the credential now.",
    )


def classify(
    payload: object = None,
    *,
    http_status: int | None = None,
    error: str | None = None,
) -> dict:
    """Classify one ``/health`` reading into exactly one category.

    Always returns a finding — never ``None`` and never a default of "healthy".
    Each branch fires on POSITIVE evidence (a concrete status, an authoritative
    outcome, or a conclusive verdict); an absent signal is reported as ``stale``,
    never as health.
    """
    # 1. The probe never reached the proxy. Says nothing about the credential.
    if error:
        return _finding(
            "provider_error",
            "Claude proxy unreachable",
            f"/health could not be reached ({_short(error, 80)})",
            "A proxy/service problem, not an auth rejection — check the "
            "claude-proxy pod/service first, not the OAuth session.",
        )

    # 2. HTTP-level failure.
    if http_status is not None and http_status != 200:
        return _finding(
            "provider_error",
            "Claude proxy health error",
            f"/health returned HTTP {http_status}",
            "A proxy/HTTP problem, not an auth rejection — investigate the "
            "proxy rather than renewing OAuth.",
        )

    # 3. Body we cannot read.
    if not isinstance(payload, dict):
        return _finding(
            "provider_error",
            "Claude proxy health error",
            "/health returned HTTP 200 with a body that is not a JSON object",
            "A proxy/response-shape problem, not an auth rejection.",
        )

    auth = payload.get("auth")
    auth = auth if isinstance(auth, dict) else {}
    upstream = payload.get("upstream")

    # 4. Authoritative, FRESH upstream outcome — taken straight from a real
    #    completed request, so it wins over every inferred signal.
    if isinstance(upstream, dict) and not upstream.get("stale"):
        outcome = upstream.get("outcome")
        observed_at = upstream.get("observedAt")
        kind = _OUTCOME_KIND.get(outcome) if isinstance(outcome, str) else None
        if kind and observed_at:
            if kind == "healthy":
                return _finding(
                    "healthy",
                    "Claude lane healthy",
                    f"last upstream request succeeded at {_short(observed_at, 40)}",
                )
            if kind == "auth_failure":
                return _finding(
                    "auth_failure",
                    "Claude lane down — auth rejected",
                    f"conclusive auth rejection (last upstream outcome at "
                    f"{_short(observed_at, 40)})",
                    "Fix: `k8s-eso-auth-recovery` skill § Claude OAuth session "
                    "renewal.",
                )
            if kind == "usage_limit":
                return _finding(
                    "usage_limit",
                    "Claude lane usage/rate limited",
                    f"the upstream reports a usage/rate limit "
                    f"(last outcome at {_short(observed_at, 40)})",
                    "The credential is valid — this is the Anthropic "
                    "usage/rate/quota limit, so do NOT renew OAuth. It clears "
                    "when the window resets (or handle the quota/overage).",
                )
            return _finding(
                "provider_error",
                "Claude lane upstream error",
                f"the last upstream request failed with a generic error at "
                f"{_short(observed_at, 40)}",
                "A provider/proxy error, not an auth rejection. Check the proxy "
                "logs and the upstream status; do not renew OAuth.",
            )

    # 5. No fresh authoritative result. A CONCLUSIVE auth-probe rejection is the
    #    one inferred signal trusted here — it is positive evidence and the only
    #    thing that justifies OAuth renewal without a fresh outcome.
    if _conclusive_auth_rejection(auth):
        detail = _short(
            auth.get("message") or auth.get("lastOutcome") or "auth probe rejected"
        )
        tally = f" ({auth['consecutiveFailures']} consecutive failures)" if auth.get(
            "consecutiveFailures"
        ) else ""
        return _finding(
            "auth_failure",
            "Claude lane down — auth rejected",
            f"conclusive auth rejection: {detail}{tally}",
            "Fix: `k8s-eso-auth-recovery` skill § Claude OAuth session renewal.",
        )

    # 6. The proxy cannot spawn a model at all (precondition, not auth).
    if payload.get("claudeBinaryOk") is False:
        return _finding(
            "provider_error",
            "Claude proxy health error",
            "the claude binary is not executable in the proxy",
            "Not an auth rejection — the proxy cannot serve any model.",
        )

    # 7. Everything else: no fresh authoritative result. Honest staleness.
    return _stale_finding(upstream, auth, payload)


def should_alert(finding: dict) -> bool:
    """Whether this finding warrants a channel post."""
    return finding.get("kind") in _ACTIONABLE_KINDS


def format_alert(finding: dict) -> str:
    """Human-readable one-liner for a finding (channel post or log line)."""
    icon = _ICONS.get(finding["kind"], "⚠️")
    detail = finding["detail"].rstrip()
    if detail and detail[-1] not in ".!?":
        detail += "."
    body = f"{icon} **{finding['headline']}** — {detail}"
    remedy = finding.get("remedy")
    return f"{body}\n{remedy}" if remedy else body


async def handle(event_type: str, context: dict) -> None:
    """Entry point called by HookRegistry.emit()."""
    global _last_check
    if event_type != "agent:start":
        return
    if context.get("platform") != "discord":
        return
    chat_id = context.get("chat_id", "")
    if not chat_id:
        return

    now = time.monotonic()
    if now - _last_check < _THROTTLE_S:
        return
    _last_check = now

    finding: dict
    try:
        timeout = aiohttp.ClientTimeout(total=5)
        async with aiohttp.ClientSession(timeout=timeout) as sess:
            async with sess.get(_HEALTH_URL) as resp:
                if resp.status != 200:
                    finding = classify(http_status=resp.status)
                else:
                    try:
                        body = await resp.json()
                    except Exception:  # noqa: BLE001 — unreadable body is a finding
                        body = None
                    finding = classify(body)
    except Exception as exc:  # noqa: BLE001 — a probe failure must never break a turn
        finding = classify(error=type(exc).__name__)

    if finding["kind"] == "healthy":
        # A fresh success is not news; stay quiet.
        return

    message = format_alert(finding)
    if not should_alert(finding):
        # Not actionable (stale): log once per process, never a channel alarm,
        # so the proxy's normal "no fresh result / token present" state does not
        # page anyone.
        if finding["kind"] in _logged_kinds:
            return
        _logged_kinds.add(finding["kind"])
        print(f"[claude-health-hook] {message}", flush=True)
        return

    token = os.environ.get("DISCORD_BOT_TOKEN", "")
    if not token:
        print(f"[claude-health-hook] {message}; DISCORD_BOT_TOKEN unset", flush=True)
        return

    payload = {"content": message}
    try:
        timeout = aiohttp.ClientTimeout(total=10)
        async with aiohttp.ClientSession(timeout=timeout) as sess:
            await sess.post(
                f"{_DISCORD_API}/channels/{chat_id}/messages",
                headers={
                    "Authorization": f"Bot {token}",
                    "Content-Type": "application/json",
                },
                json=payload,
            )
    except Exception as exc:  # noqa: BLE001
        print(f"[claude-health-hook] alert post failed: {exc}", flush=True)
