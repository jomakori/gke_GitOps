# splash

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)
Placeholder page (directResponse VirtualService) for a host with nothing deployed behind it — rendered by the staging gate only while no pull request carries the staging label

## Under the hood

`splash` (`splash.yaml`) answers a host that currently has **no** VirtualService of its own with a
static page: the app logo, plus a line pointing at the open pull requests carrying the `staging`
label. It is a `directResponse`, so it needs no backend — no pod, no image, no maintenance deployment.

### One claimant per host

The placeholder and the staging environment answer the **same** host
(`staging-openkite.maklab.net`), and VirtualServices claiming the same host on the same gateway are
merged — for identical matches the **older** one keeps the route (measured: a probe VirtualService
created two minutes later never took over). Two claimants would therefore leave the placeholder
permanently shadowing a later staging deploy, so they must never coexist.

Which of the two exists follows the same state that admits staging — the `staging` label on an
`openkite` pull request. CI (`.github/workflows/staging-state.yml` in `jomakori/openkite`) keeps a
marker directory on the `staging-state` branch, present exactly while **no** open pull request carries
the label, and the ApplicationSet in `apps/argocd-appset` renders this chart from that marker with a
git directory generator:

| State | Marker | Result |
|-------|--------|--------|
| No open PR carries `staging` | present | this chart renders; the placeholder answers the host |
| An open PR carries `staging` | absent | the placeholder Application — and this VirtualService — is pruned; the staging environment takes the host |

The marker lives on its own branch because `main` is protected, and the workflow's schedule
reconverges it every ten minutes, so a label event that cannot push (a pull request from a fork gets a
read-only token) only delays the flip. A flip is never instant: CI publishes the marker, the
ApplicationSet polls, ArgoCD syncs and prunes.

### Limits

- Scope is one exact host (`staging-openkite.maklab.net`), so it cannot affect another app's routing.
- The route sets `content-type: text/html; charset=utf-8`. Without it Envoy serves the body as
  `text/plain`, and browsers print the markup instead of rendering the page.
- It covers **only** "nothing deployed". A pod that is up but broken (crash loop, failing probe)
  answers its own `503` before this route is consulted; covering that needs a fronting proxy (e.g. a
  Cloudflare Worker that falls back when the origin 5xxs).
- `status` is a monitoring choice — `200` reads as a healthy page, `503` lets uptime checks flag the
  missing preview.

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `istio-system` — the Gateway's namespace |
| **Sync wave** | 2 |
| **Gateway** | `maklab-gateway`, declared by the istio chart |
| **Rendered by** | the staging gate in `apps/argocd-appset` (`openkite-staging-placeholder`) |
| **Image** | none — `directResponse` only |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| gateway | object | `{"name":"maklab-gateway","namespace":"istio-system"}` | Shared ingress Gateway this placeholder attaches to. The Gateway itself is declared by the istio chart; this chart only references it. |
| gateway.name | string | `"maklab-gateway"` | Name of the Gateway. |
| gateway.namespace | string | `"istio-system"` | Namespace of the Gateway, and the namespace the VirtualService is created in. |
| splash.brand | string | `"OpenKite"` | Name under the logo (the logo itself is the kite mark above it). |
| splash.enabled | bool | `true` | Render the placeholder VirtualService. |
| splash.host | string | `"staging-openkite.maklab.net"` | Host the placeholder answers for. Keep it exact — a wildcard here would answer every unrouted host on the domain. |
| splash.linkText | string | `"Confirm if PR has a staging label"` | Second line, as a link to the open pull requests carrying the staging label. |
| splash.linkUrl | string | `"https://github.com/jomakori/openkite/pulls?q=is%3Apr+is%3Aopen+label%3Astaging"` | URL the link text points at: the open pull requests carrying the staging label, which are the ones that can be staged. |
| splash.message | string | `"There's no PR currently staged."` | First line below the logo. |
| splash.status | int | `200` | HTTP status of the placeholder response. 200 reads as a normal page; 503 makes uptime checks flag the missing preview. |
