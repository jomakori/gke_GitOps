"""Behavioral tests for the claude-health hook classifier.

Run (from this directory)::

    python3 -m unittest -v test_handler

or from the repo root::

    python3 -m unittest discover -s services/helm/openagent/hooks/claude-health -p 'test_*.py'

The hook imports ``aiohttp`` at module load; these tests install a stub so the
pure classifier can be exercised without the runtime dependency present, and
swap in a fake aiohttp for the end-to-end ``handle()`` tests.
"""

from __future__ import annotations

import asyncio
import os
import sys
import types
import unittest
from types import SimpleNamespace

if "aiohttp" not in sys.modules:
    sys.modules["aiohttp"] = types.ModuleType("aiohttp")

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import handler  # noqa: E402

_NOW = "2026-10-06T06:00:00.000Z"


def _payload(outcome, observed_at: str | None = _NOW, stale=False, auth=None, **extra):
    """A /health body shaped like OCP after DEV-51.

    ``outcome`` is one of the proxy's own ``UPSTREAM_OUTCOMES``: ``success`` |
    ``auth_rejected`` | ``usage_limited`` | ``other`` (or ``unobserved``).

    ``auth`` defaults to the proxy's normal token-env state: ``ok is null`` with
    ``lastOutcome == "token-present"`` — inconclusive, NOT a rejection.
    """
    if auth is None:
        auth = {"ok": None, "message": "token present", "lastOutcome": "token-present"}
    body = {"auth": auth, "upstream": {"outcome": outcome, "observedAt": observed_at, "stale": stale}}
    body.update(extra)
    return body


class ContractVocabularyTests(unittest.TestCase):
    """The classifier's outcome vocabulary is OCP's, verbatim.

    OCP publishes ``UPSTREAM_OUTCOMES`` (lib/upstream-status.mjs, DEV-51):
    ``success`` | ``auth_rejected`` | ``usage_limited`` | ``other`` — plus the
    out-of-band ``unobserved``. A drift in these strings silently degrades every
    real outcome to ``stale`` (an unrecognised outcome falls through), which is
    the exact failure this class guards.
    """

    _PROXY_OUTCOMES = {"success", "auth_rejected", "usage_limited", "other"}

    def test_classifier_keys_match_the_proxy_vocabulary(self):
        self.assertEqual(set(handler._OUTCOME_KIND), self._PROXY_OUTCOMES)

    def test_every_proxy_outcome_maps_to_a_distinct_kind(self):
        kinds = {handler.classify(_payload(o))["kind"] for o in self._PROXY_OUTCOMES}
        self.assertEqual(
            kinds, {"healthy", "auth_failure", "usage_limit", "provider_error"}
        )

    def test_tokens_the_proxy_does_not_publish_never_go_fresh(self):
        # The pre-DEV-51 names this hook once assumed: unknown to OCP, so a
        # fresh one must degrade to stale, never be guessed into a category.
        for wrong in ("auth_rejection", "usage_limit", "error"):
            f = handler.classify(_payload(wrong))
            self.assertEqual(f["kind"], "stale", wrong)


class ClassifyFreshOutcomeTests(unittest.TestCase):
    """A fresh authoritative outcome maps straight through."""

    def test_fresh_success_is_healthy_and_not_alerted(self):
        f = handler.classify(_payload("success"))
        self.assertEqual(f["kind"], "healthy")
        self.assertFalse(handler.should_alert(f))

    def test_fresh_auth_rejected_is_auth_failure(self):
        f = handler.classify(_payload("auth_rejected"))
        self.assertEqual(f["kind"], "auth_failure")
        self.assertTrue(handler.should_alert(f))
        self.assertIn("OAuth session renewal", f["remedy"])

    def test_fresh_usage_limited_is_usage_limit_not_auth(self):
        f = handler.classify(_payload("usage_limited"))
        self.assertEqual(f["kind"], "usage_limit")
        self.assertTrue(handler.should_alert(f))
        self.assertIn("do NOT renew OAuth", f["remedy"])
        # A usage limit must never prescribe the auth-renewal remedy.
        self.assertNotIn("OAuth session renewal", f["remedy"])

    def test_fresh_other_is_provider_error_not_auth(self):
        f = handler.classify(_payload("other"))
        self.assertEqual(f["kind"], "provider_error")
        self.assertTrue(handler.should_alert(f))
        self.assertNotIn("OAuth session renewal", f["remedy"])

    def test_fresh_success_beats_inconclusive_auth_probe(self):
        # A completed request is conclusively authoritative; ok=null must not
        # downgrade it.
        f = handler.classify(_payload("success", auth={"ok": None, "lastOutcome": None}))
        self.assertEqual(f["kind"], "healthy")

    def test_usage_limit_wins_over_auth_probe_rejection(self):
        # An Anthropic subscription wall can present as a conclusive auth
        # rejection whose real cause is the usage limit — usage must win.
        f = handler.classify(_payload("usage_limited", auth={"ok": False, "message": "401"}))
        self.assertEqual(f["kind"], "usage_limit")

    def test_other_with_rejected_auth_probe_stays_a_fresh_provider_error(self):
        # Fresh authoritative outcome wins: a generic error is not relabelled.
        f = handler.classify(_payload("other", auth={"ok": False, "message": "boom"}))
        self.assertEqual(f["kind"], "provider_error")


class ClassifyNoFreshResultTests(unittest.TestCase):
    """No fresh authoritative result — report staleness, never health."""

    def test_unobserved_is_stale_not_healthy(self):
        f = handler.classify(_payload("unobserved", observed_at=None))
        self.assertEqual(f["kind"], "stale")
        self.assertFalse(handler.should_alert(f))
        self.assertNotEqual(f["kind"], "healthy")

    def test_aged_out_result_is_stale_not_healthy(self):
        f = handler.classify(_payload("success", stale=True))
        self.assertEqual(f["kind"], "stale")
        self.assertFalse(handler.should_alert(f))

    def test_missing_upstream_field_is_stale_not_healthy(self):
        # An older proxy build without the DEV-51 contract: status unknown, and
        # crucially NOT reported as healthy.
        f = handler.classify({"auth": {"ok": None, "lastOutcome": "token-present"}})
        self.assertEqual(f["kind"], "stale")
        self.assertFalse(handler.should_alert(f))

    def test_token_present_inconclusive_is_stale_and_silent(self):
        # The exact false-alarm the original hook fired on: token present, probe
        # cannot tell. Must be stale, not an alarm.
        f = handler.classify(
            _payload("unobserved", observed_at=None,
                     auth={"ok": None, "message": "token present", "lastOutcome": "token-present"})
        )
        self.assertEqual(f["kind"], "stale")
        self.assertFalse(handler.should_alert(f))

    def test_conclusive_auth_ok_false_is_auth_failure(self):
        f = handler.classify(
            _payload("unobserved", observed_at=None, auth={"ok": False, "message": "401 unauthorized"})
        )
        self.assertEqual(f["kind"], "auth_failure")
        self.assertTrue(handler.should_alert(f))
        self.assertIn("OAuth session renewal", f["remedy"])

    def test_consecutive_failures_are_conclusive(self):
        f = handler.classify(
            _payload("unobserved", observed_at=None,
                     auth={"ok": None, "lastOutcome": "rejected", "consecutiveFailures": 2})
        )
        self.assertEqual(f["kind"], "auth_failure")
        self.assertIn("2 consecutive failures", f["detail"])

    def test_binary_not_executable_is_provider_error(self):
        f = handler.classify(_payload("unobserved", observed_at=None, claudeBinaryOk=False))
        self.assertEqual(f["kind"], "provider_error")
        self.assertTrue(handler.should_alert(f))


class ClassifyTransportTests(unittest.TestCase):
    """Transport / shape failures are proxy errors, never auth claims."""

    def test_unreachable_is_provider_error(self):
        f = handler.classify(error="ClientConnectorError")
        self.assertEqual(f["kind"], "provider_error")
        self.assertTrue(handler.should_alert(f))
        self.assertIn("not an auth rejection", f["remedy"])

    def test_http_non_200_is_provider_error(self):
        f = handler.classify(http_status=503)
        self.assertEqual(f["kind"], "provider_error")
        self.assertIn("503", f["detail"])

    def test_unreadable_body_is_provider_error(self):
        f = handler.classify("not-a-dict")
        self.assertEqual(f["kind"], "provider_error")


class FormatTests(unittest.TestCase):
    def test_every_reading_yields_a_kind_and_no_confidence_defaults(self):
        samples = [
            _payload("success"),
            _payload("auth_rejected"),
            _payload("usage_limited"),
            _payload("other"),
            _payload("unobserved", observed_at=None),
            _payload("success", stale=True),
            {"auth": {}},
            "garbage",
        ]
        for body in samples:
            f = handler.classify(body)
            self.assertIn(
                f["kind"],
                {"healthy", "auth_failure", "usage_limit", "provider_error", "stale"},
            )
            self.assertTrue(handler.format_alert(f))


# ── handle() end-to-end with a fake aiohttp ────────────────────────────────

class _Resp:
    def __init__(self, status=200, body=None):
        self.status = status
        self._body = body

    async def __aenter__(self):
        return self

    async def __aexit__(self, *a):
        return False

    async def json(self):
        if isinstance(self._body, BaseException):
            raise self._body
        return self._body


class _Session:
    def __init__(self, resp, posts):
        self._resp = resp
        self._posts = posts

    async def __aenter__(self):
        return self

    async def __aexit__(self, *a):
        return False

    def get(self, url):
        return self._resp

    async def post(self, url, headers=None, json=None):
        self._posts.append(json)


def _fake_aiohttp(resp):
    posts: list = []
    return (
        SimpleNamespace(
            ClientTimeout=lambda total=None: None,
            ClientSession=lambda timeout=None: _Session(resp, posts),
        ),
        posts,
    )


class HandleTests(unittest.TestCase):
    def setUp(self):
        self._orig_aiohttp = handler.aiohttp
        handler._last_check = 0.0
        handler._logged_kinds.clear()
        os.environ["DISCORD_BOT_TOKEN"] = "test-token"

    def tearDown(self):
        handler.aiohttp = self._orig_aiohttp

    def _run(self, body, status=200):
        handler.aiohttp, posts = _fake_aiohttp(_Resp(status, body))
        asyncio.new_event_loop().run_until_complete(
            handler.handle("agent:start", {"platform": "discord", "chat_id": "42"})
        )
        return posts

    def test_posts_on_auth_failure(self):
        posts = self._run(_payload("auth_rejected"))
        self.assertEqual(len(posts), 1)
        self.assertIn("auth rejected", posts[0]["content"])

    def test_posts_nothing_on_healthy(self):
        self.assertEqual(self._run(_payload("success")), [])

    def test_posts_nothing_on_stale(self):
        self.assertEqual(self._run(_payload("unobserved", observed_at=None)), [])

    def test_ignores_non_start_events(self):
        handler.aiohttp, posts = _fake_aiohttp(_Resp(200, _payload("auth_rejected")))
        asyncio.new_event_loop().run_until_complete(
            handler.handle("agent:stop", {"platform": "discord", "chat_id": "42"})
        )
        self.assertEqual(posts, [])


if __name__ == "__main__":
    unittest.main()
