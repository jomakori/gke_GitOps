#!/usr/bin/env python3
"""Contract tests for rendered OpenAgent model context-window declarations."""

from pathlib import Path
import subprocess
import unittest

import yaml


CHART = Path(__file__).resolve().parents[1]


# (provider model id, max input/context, max output)
EXPECTED_LIMITS = {
    "copilot-luna": ("github_copilot/gpt-6-luna", 1_048_576, 128_000),
    "copilot-grok": ("github_copilot/grok-4.7", 500_000, None),
    "copilot-sonnet-5.5": ("github_copilot/claude-sonnet-5.5", 1_000_000, 128_000),
    "copilot-gemini-3.8-flash": ("github_copilot/gemini-3.8-flash", 1_048_576, 65_536),
    "copilot-mai-code": ("github_copilot/mai-code-1.1-flash", 256_000, None),
    "copilot-codex": ("github_copilot/gpt-5.3-codex", 400_000, 128_000),
    "deepseek-v4-flash-direct": ("deepseek/deepseek-flash", 1_000_000, 384_000),
    "claude-opus-5": ("openai/claude-opus-5", 1_000_000, 128_000),
    "claude-sonnet-5": ("openai/claude-sonnet-5", 1_000_000, 128_000),
    "claude-haiku-4-5": ("openai/claude-haiku-4-5", 200_000, 64_000),
}


class ModelContextLimitsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        rendered = subprocess.run(
            ["helm", "template", "openagent", str(CHART), "--skip-schema-validation", "--validate=false"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout
        documents = [doc for doc in yaml.safe_load_all(rendered) if isinstance(doc, dict)]
        config_maps = {
            doc.get("metadata", {}).get("name"): doc
            for doc in documents
            if doc.get("kind") == "ConfigMap"
        }
        cls.litellm = yaml.safe_load(config_maps["openagent-litellm-config"]["data"]["config.yaml"])
        cls.hermes = yaml.safe_load(config_maps["openagent-hermes-agent-config"]["data"]["config.yaml"])
        cls.models = cls.litellm["model_list"]
        cls.models_by_name = {model["model_name"]: model for model in cls.models}
        cls.hermes_pins = cls.hermes["providers"]["litellm"]["models"]

    def test_exact_provider_ids_and_declared_limits_render(self):
        self.assertTrue(set(EXPECTED_LIMITS).issubset(self.models_by_name))
        self.assertTrue(set(self.models_by_name).issubset(self.hermes_pins))
        for alias, (provider_id, max_input, max_output) in EXPECTED_LIMITS.items():
            with self.subTest(alias=alias):
                model = self.models_by_name[alias]
                self.assertEqual(model["litellm_params"]["model"], provider_id)
                info = model.get("model_info", {})
                self.assertEqual(info.get("max_input_tokens"), max_input)
                self.assertEqual(info.get("max_output_tokens"), max_output)
                # LiteLLM 1.92.0's schema does not define model_info.max_tokens.
                self.assertNotIn("max_tokens", info)

    def test_unknown_output_caps_remain_undeclared(self):
        for alias in ("copilot-grok", "copilot-mai-code"):
            with self.subTest(alias=alias):
                info = self.models_by_name[alias].get("model_info", {})
                self.assertNotIn("max_output_tokens", info)

    def test_litellm_pre_call_token_guard_uses_each_alias_max_input(self):
        # LiteLLM 1.92.0's `router._pre_call_checks` enforces max_input_tokens
        # before dispatch (router.py): keep that contract explicit for all knowns.
        self.assertEqual(
            {alias: model["model_info"]["max_input_tokens"] for alias, model in self.models_by_name.items() if alias in EXPECTED_LIMITS},
            {alias: expected[1] for alias, expected in EXPECTED_LIMITS.items()},
        )

    def test_all_active_aliases_have_matching_hermes_context_overrides(self):
        self.assertEqual(set(self.hermes_pins), set(EXPECTED_LIMITS))
        for alias, (_, max_input, _) in EXPECTED_LIMITS.items():
            with self.subTest(alias=alias):
                self.assertEqual(self.hermes_pins[alias]["context_length"], max_input)

    def test_limit_values_are_positive_integers(self):
        for alias, model in self.models_by_name.items():
            info = model.get("model_info", {})
            with self.subTest(alias=alias):
                for key in ("max_input_tokens", "max_output_tokens"):
                    if key in info:
                        self.assertIsInstance(info[key], int)
                        self.assertGreater(info[key], 0)


if __name__ == "__main__":
    unittest.main()
