"""``agent:start`` hook: warn in-channel when the Claude lane is down.

Claude is the fleet's only reasoning route, and when the proxy's OAuth session
dies every claude* model fails silently while LiteLLM walks the chains to the
DeepSeek-direct fallbacks. The proxy's /health carries the auth probe result
(no model call, no subscription quota), so one probe per session start is
enough to catch a dead lane before it costs a whole session.
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

_last_check = 0.0


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

    problem = ""
    try:
        timeout = aiohttp.ClientTimeout(total=5)
        async with aiohttp.ClientSession(timeout=timeout) as sess:
            async with sess.get(_HEALTH_URL) as resp:
                if resp.status != 200:
                    problem = f"claude-proxy /health returned HTTP {resp.status}"
                else:
                    auth = (await resp.json()).get("auth", {})
                    if not auth.get("ok", False):
                        problem = f"auth not ok — {auth.get('message', 'no message')}"
                    elif auth.get("consecutiveFailures", 0) > 0:
                        problem = (
                            f"{auth['consecutiveFailures']} consecutive auth failures"
                        )
    except Exception as exc:  # noqa: BLE001 — a probe failure must never break a turn
        problem = f"claude-proxy unreachable ({type(exc).__name__})"

    if not problem:
        return

    token = os.environ.get("DISCORD_BOT_TOKEN", "")
    if not token:
        print(f"[claude-health-hook] {problem}; DISCORD_BOT_TOKEN unset", flush=True)
        return

    payload = {
        "content": (
            f"⚠️ **Claude lane down** — {problem}.\n"
            "claude* requests fail over to DeepSeek-direct/Kilo meanwhile. "
            "Fix: `k8s-eso-auth-recovery` skill § Claude OAuth session renewal."
        )
    }
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
