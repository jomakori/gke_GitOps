#!/usr/bin/env python3
"""Cloudflare Access invariants: every gated host has an Access app behind it,
and the gateway's audience list has exactly one home (the XRs), never git.

Three failures this catches that lint/render/kubeconform cannot see:

1. A service gated in istio (`gateways.enable_private`) with no
   `gateways.accessAppId`. No `CloudflareAccessApp` XR is created, so the host
   contributes no audience to the gateway's `RequestAuthentication` — the browser
   never gets a JWT and the per-host DENY policy rejects every request.
2. A host gated by istio (`privateHosts`) with no Access app in its family. Same
   outcome, discovered only by a 401 in the browser after deploy.
3. A second, git-side source of the audience list creeping back. The list is
   aggregated at runtime from every XR's `.status.aud`
   (`services/helm/crossplane/templates/audience-sync.yaml`); an `audienceTag`
   value or a chart-rendered `RequestAuthentication` reintroduces the copy that
   went stale each time a host was added.

Hosts are matched by name family (alphanumeric tokens of the first label, or of
the domain when the label is a bare wildcard), not by exact string: the preview
app is declared `pr*-openkite.maklab.net` while istio gates
`*.openkite.maklab.net`, and both are the openkite preview family.

Usage: scripts/check_private_hosts_have_access_app.py [-v]
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
APPSET_VALUES = ROOT / "services/argocd-appset/values.yaml"
APPSET_TEMPLATE = ROOT / "services/argocd-appset/templates/applications.yaml"
ISTIO_CHART = ROOT / "services/helm/istio"
ISTIO_VALUES = ISTIO_CHART / "values.yaml"


def family_tokens(host: str) -> set[str]:
    """Name family of a host pattern, as alphanumeric tokens.

    'staging-trade.maklab.net' -> {'staging', 'trade'}
    'pr*-openkite.maklab.net'  -> {'pr', 'openkite'}
    '*.openkite.maklab.net'    -> {'openkite'}   (bare wildcard: use the domain)
    """
    labels = host.split(".")
    tokens = {t for t in re.split(r"[^a-z0-9]+", labels[0].lower()) if t}
    if not tokens and len(labels) > 1:
        tokens = {t for t in re.split(r"[^a-z0-9]+", labels[1].lower()) if t}
    return tokens


def related(a: str, b: str) -> bool:
    """True when two host patterns belong to the same Access app family."""
    if a == b:
        return True
    return bool(family_tokens(a) & family_tokens(b))


def service_name(key: str, cfg: dict) -> str:
    """Mirror of the appset's `$svName` default (camelCase key -> kebab-case)."""
    return cfg.get("name") or re.sub(r"([a-z])([A-Z])", r"\1-\2", key).lower()


def load_yaml(path: Path) -> dict:
    with path.open() as fh:
        return yaml.safe_load(fh) or {}


def access_apps(values: dict) -> tuple[list[tuple[str, str]], list[str]]:
    """(source, host pattern) for every XR the appset creates, plus defects."""
    apps: list[tuple[str, str]] = []
    problems: list[str] = []
    for key, cfg in (values.get("services") or {}).items():
        gateways = cfg.get("gateways") or {}
        if not (cfg.get("enable") and gateways.get("enable_private")):
            continue
        subdomain = gateways.get("subdomain") or service_name(key, cfg)
        if not gateways.get("accessAppId"):
            problems.append(
                f"service '{key}' is gated (gateways.enable_private) but has no "
                f"gateways.accessAppId: no CloudflareAccessApp XR is created, so "
                f"'{subdomain}' gets no audience at the gateway"
            )
            continue
        apps.append((f"services.{key}", subdomain))
    for key, entry in (values.get("accessHosts") or {}).items():
        if not (entry.get("host") and entry.get("appId")):
            problems.append(
                f"accessHosts.{key} needs both host and appId: without an appId no XR "
                f"is created, without a host the XR is unusable"
            )
            continue
        apps.append((f"accessHosts.{key}", entry["host"]))
    return apps, problems


def rendered_request_authentications() -> tuple[list[str], list[str]]:
    """Names of RequestAuthentication objects the istio chart renders, plus errors."""
    try:
        rendered = subprocess.run(
            ["helm", "template", "audit", str(ISTIO_CHART)],
            capture_output=True, text=True, check=True,
        ).stdout
    except FileNotFoundError:
        return [], ["helm not found: the RequestAuthentication absence check needs helm"]
    except subprocess.CalledProcessError as exc:
        return [], [f"helm template services/helm/istio failed: {exc.stderr.strip()[:400]}"]
    found = [
        str(doc.get("metadata", {}).get("name"))
        for doc in yaml.safe_load_all(rendered)
        if isinstance(doc, dict) and doc.get("kind") == "RequestAuthentication"
    ]
    return found, []


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()

    failures: list[str] = []

    appset = load_yaml(APPSET_VALUES)
    apps, defects = access_apps(appset)
    failures.extend(defects)

    gated: list[str] = list(load_yaml(ISTIO_VALUES).get("privateHosts") or [])
    if not gated:
        failures.append("services/helm/istio/values.yaml declares no privateHosts: nothing is gated")

    # 1. Every host istio gates has an Access app in its family.
    for host in gated:
        covering = [src for src, pat in apps if related(host, pat)]
        if not covering:
            failures.append(
                f"istio gates '{host}' but no CloudflareAccessApp covers it: no service "
                f"with gateways.enable_private/accessAppId and no accessHosts entry"
            )
        elif args.verbose:
            print(f"  gated {host:<32} <- {', '.join(covering)}")

    # 2. Every app declared outside the appset (accessHosts: its VirtualService
    #    lives in another chart) is actually gated here. An app for an ungated
    #    host issues tokens nothing checks.
    for src, pat in apps:
        if not src.startswith("accessHosts."):
            continue
        if not any(related(pat, host) for host in gated):
            failures.append(
                f"{src} declares an Access app for '{pat}' but istio does not gate that "
                f"host family (privateHosts in services/helm/istio/values.yaml)"
            )

    # 3. The audience list has one source: the XRs. No git-side value, no object.
    if "audienceTag" in APPSET_TEMPLATE.read_text():
        failures.append(
            "services/argocd-appset/templates/applications.yaml still derives an "
            "audienceTag: the gateway audience list is aggregated from XR status by "
            "services/helm/crossplane"
        )
    if "access" in (load_yaml(ISTIO_VALUES).get("cloudflare") or {}):
        failures.append(
            "services/helm/istio/values.yaml still carries cloudflare.access: the istio "
            "chart no longer renders the CF Access RequestAuthentication"
        )
    rendered_ras, render_errors = rendered_request_authentications()
    failures.extend(render_errors)
    for name in rendered_ras:
        failures.append(
            f"helm template services/helm/istio renders RequestAuthentication/{name}: the "
            f"audience list would have two owners"
        )

    if failures:
        print(f"FAIL — {len(failures)} Cloudflare Access invariant(s) broken:", file=sys.stderr)
        for item in failures:
            print(f"  - {item}", file=sys.stderr)
        return 1

    print(
        f"OK — {len(gated)} gated host(s), {len(apps)} Access app(s); "
        f"the gateway audience list is aggregated from XR status only"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
