# OKT-131 — what a closed PR leaves behind (and what cleans it up)

Ticket: **OKT-131**. Probe date: **2026-09-27**.

Investigated after PR 132 (`feat(openagent): route GLM/Kimi/MiniMax via Kilo Gateway`,
merged 2026-08-28) was observed to leave `pr132-openkite.maklab.net` answering
`302 → jmaklab.cloudflareaccess.com` even though the cluster had pruned the
`openkite-pr132` namespace and its Application.

This document inventories the artifacts a preview produces, classifies each
one, and names the mechanism that disposes of it (or, in the honest cases,
says **why no cleanup is needed or possible**). Every claim has a probe or a
repo pointer behind it — read the **Evidence** section by section.

The next person to land in this corner should not have to re-derive any of it.

---

## TL;DR

| Artifact | Cleaned automatically? | Mechanism |
|---|---|---|
| `openkite-pr<N>` namespace | ✅ yes | `kubernetes.io/argo-secrets`/finalizer on the Application, plus a `teardown` step in the upstream `preview.yml` workflow that runs on PR closure OR `unlabeled=='preview'` |
| `Application` `openkite-pr<N>` | ✅ yes | `resources-finalizer.argocd.argoproj.io` finalizer on the App; the PR generator removes the resource when the PR is gone or loses the label |
| `Deployment/Service/VirtualService/AuthPolicy/HPA/...` inside the namespace | ✅ yes | Owned by the App finalizer; deleted when the App goes |
| `ExternalDNS` DNS records for the preview host | ⚠️ **never deleted** | `policy: upsert-only` (`services/helm/external-dns/values.yaml:30`) — see §DNS records below |
| DNS wildcard `*.maklab.net → <tunnel>.cfargotunnel.com` | n/a | Managed by Terraform (`5-cloudflare-tunnel.tf:55-63`); **not** residue, it is the only routing path to the cluster |
| Cloudflare Access gate returning `302 → jmaklab.cloudflareaccess.com` | n/a for the ticket | A single wildcard Access policy at the Cloudflare zone catches every host that resolves via the wildcard; per-preview cleanup is impossible **and unnecessary** |
| GHCR package version tagged `pr-<N>` and `pr-<N>-<sha>` | ✅ on close | `prune` job in the upstream `preview.yml` workflow calls `prune-pr-image.sh` |

---

## Investigation method

- Read-only. Nothing was changed in the cluster, the zone, or GHCR.
- HTTP probes against `pr<N>-openkite.maklab.net`, `*.maklab.net` siblings, and
  obvious non-hits (`opentest-openkite.maklab.net`, `pr200-openkite.maklab.net`,
  `pr300-maklab.net`, `nonexistent-12345.example.com`) from this VM.
- `git` for repo structure. `gh api` over HTTPS for PR/package introspection
  (no token on this box — public resources only).
- No `kubectl` access from this environment (no VPN/tailnet reach, no kubeconfig
  on disk). **All cluster-side claims are derived from the preview generator's
  finalizers and the upstream teardown workflow, not from a live `kubectl get`.**
  Probe that we could verify: §Cluster leftovers.
- **Cloudflare API not reachable** from this environment (no token); the
  side-of-Cloudflare inventory relies on the public `.well-known/cloudflare-access-protected-resource/`
  endpoint and the repo's Terraform state for the Access Applications list.

---

## Findings, in five sections

### 1. Cluster leftovers — namespace and Application

**Claim:** When PR 132 closed or lost the `preview` label, the
`openkite-pr132` namespace and its `Application openkite-pr132` were pruned.

**Why:** Preview Applications are owned by an `ApplicationSet` whose generator
is the GitHub Pull Request generator with `labelSelectors: ['preview']`
(`apps/argocd-appset/templates/preview.yml:38-55`). When the PR is gone or loses
the label, the generator stops emitting it. The Application carries the
`resources-finalizer.argocd.argoproj.io` finalizer
(`apps/argocd-appset/templates/preview.yml:66`), so ArgoCD deletes every
resource in the namespace, then the namespace itself, on App removal.

The Application also passes `createNamespace: true`
(`apps/argocd-appset/templates/preview.yml:91-93`), so the namespace is co-owned
by the Application and goes with it.

A second belt-and-braces path is the upstream openkite repo's
`.github/workflows/preview.yml:teardown` job, which runs on the same trigger and
does `helm uninstall` + `kubectl delete namespace` directly (see §Source code
under **Repo pointers**, below). That job runs even if the AppSet's sync to ArgoCD
is unhealthy, because the trigger is GitHub-side.

**Repo pointers:**
- `apps/argocd-appset/templates/preview.yml` — generator, finalizer, namespace ownership
- `openkite:/.github/workflows/preview.yml` (`teardown` job, lines 113-133) — workflow-level safety net
- `apps/argocd-appset/values.yaml:96-110` — the comment noting that the bespoke
  `openkite-preview` app+chart pair was deleted and is replaced by this generic generator

**Probe evidence:** the ticket itself confirms the namespace and Application
were pruned on 2026-09-25; HTTP probing from this VM on 2026-09-27 of
`pr132-openkite.maklab.net` still returns the Access login page (34 KB after
redirect), **not** an Istio 404 or a connection error. That is consistent with
the cluster app being gone but the upstream CF Access gate still answering.

**Caveat — what I could not check:** I have no `kubectl` on this VM. The
"namespace was pruned" claim rests on (a) the ticket's confirmation, (b) the
finalizer spec in `preview.yml`, and (c) `pr132-openkite.maklab.net` resolving
through to a CF Access login rather than a cluster router. To assert it
directly, run on a host with `kubectl` and tailnet reach to the cluster:

```sh
kubectl get ns -l 'app.kubernetes.io/part-of=openkite'
kubectl get applications.argoproj.io -n argocd -l 'app.kubernetes.io/part-of=openkite'
```

### 2. DNS records — the external-dns `upsert-only` dimension

**Claim:** Any specific DNS record ExternalDNS may have created for a preview
host is **never deleted**, by design. This is **not residue from this repo's
bug** — it is the documented, deliberate safety posture.

**Why:** `services/helm/external-dns/values.yaml:30`:

```yaml
policy: upsert-only   # safe default — never deletes DNS records
```

…with the matching acknowledgment in `services/helm/external-dns/README.md:32`:

> **`upsert-only` policy** — ExternalDNS never deletes DNS records. Stale
> entries must be cleaned up manually in the Cloudflare dashboard. A TXT
> ownership registry (`external-dns-gke-maklab`) prevents conflicts between this
> cluster and other DNS managers.

The directory of watched sources is `istio-gateway`, `istio-virtualservice`, and
`service` (`values.yaml:22-26`). When the preview Application deletes its
VirtualService on PR closure, ExternalDNS would normally drop the corresponding
DNS record — but `upsert-only` suppresses that.

**Why we should NOT flip it to `sync`:** the same ExternalDNS deployment
manages *every* `*.maklab.net` host whose VirtualService or Gateway appears in
this repo, which today means:

- `argocd.maklab.net`, `draw.maklab.net`, `hermes.maklab.net`, `opencost.maklab.net`,
  `headlamp.maklab.net` (`services/helm/istio/values.yaml:virtualServices.*`)
- `openkite.maklab.net`, `openkite-staging.maklab.net`, plus every
  `pr<N>-openkite.maklab.net` produced by the preview generator

Switching `policy` to `sync` would mean: any human `kubectl-edit` of a
`VirtualService` (e.g. temporarily pointing a host to an empty
`Service`) deletes the prod DNS record. That is a much worse failure mode than
the current "manual cleanup" one. The README warns of exactly this in §Behavior.

**DNS probe results (2026-09-27, from this VM, DoH to `cloudflare-dns.com`):**

| Host | A records |
|---|---|
| `pr132-openkite.maklab.net` | `104.21.29.221`, `172.67.149.215` |
| `pr131-openkite.maklab.net` | `104.21.29.221`, `172.67.149.215` |
| `pr200-openkite.maklab.net` (does not exist) | `104.21.29.221`, `172.67.149.215` |
| `pr9999-foo.maklab.net` | — (NXDOMAIN; not matched by `*.maklab.net`) |
| `openkite.maklab.net` | `104.21.29.221`, `172.67.149.215` |
| `staging.openkite.maklab.net` | `104.21.29.221`, `172.67.149.215` |

**Reading of those probes:**

- All `*.maklab.net` resolve to the same two Cloudflare IPs. That is the
  wildcard DNS record defined in `5-cloudflare-tunnel.tf:55-63`
  (`cloudflare_dns_record.wildcard_maklab: name=*.maklab.net, type=CNAME,
  content=<tunnel>.cfargotunnel.com, proxied=true`). It is **Terraform-owned**,
  not ExternalDNS-owned — `domainFilters: [maklab.net]` filters by zone
  ownership, but the wildcard at the apex is managed by the tunnel stack, not
  the chart.
- A specific `pr132-openkite.maklab.net` record may or may not exist on the
  Cloudflare side. **I cannot query CF from this VM.** Whether a specific
  record exists does not change the routing outcome: a wildcard `*.maklab.net`
  already answers for it. Even if a stale specific record pointed to a different
  Cloudflare destination in the past, the wildcard always wins for these name
  shapes — and the wildcard cannot be deleted per host, because it is the
  cluster's only ingress path.
- `pr9999-foo.maklab.net` returns NXDOMAIN because it does **not** match the
  `*.maklab.net` wildcard pattern (`*.foo.maklab.net` ≠ `*.maklab.net`). That's
  a useful negative: it proves the wildcard is exactly `*.<zone>`, not
  `*.*.<zone>`.

**Decision per the ticket's instruction:**

- For preview hosts specifically, **kept `upsert-only`.** Documented here so the
  next operator doesn't chase the residue.
- **Flipping would be unsafe.** Stated explicitly so the rejection is auditable.
- A specific record for `pr132-openkite.maklab.net`, **if** one exists on the
  Cloudflare side, can be removed manually from the Cloudflare dashboard with
  no observable effect (the wildcard catches the host anyway). That is a
  cosmetic cleanup, not a correctness fix, and skipping it does not break
  anything. **No action required.**

**Repo pointers:**
- `services/helm/external-dns/values.yaml`
- `services/helm/external-dns/README.md` (look for `Policy` and the
  `Behavior` block)
- `5-cloudflare-tunnel.tf:55-63` (the wildcard DNS record, Terraform)

### 3. Cloudflare Access — why every preview host answers 302

**Claim:** The 302 to `jmaklab.cloudflareaccess.com` we see on a closed PR's
host is **not a leftover per-PR Access Application**. It is a single,
zone-level policy at the Cloudflare edge that matches `*.openkite.maklab.net`
via the wildcard CNAME. There is no per-host Access Application for a preview
in any file in this repo, in the Terraform module, or in the upstream
`openkite` codebase — and there does not need to be.

**Probe evidence (2026-09-27, this VM, `curl -skI`):**

| Host | Status | Location header (truncated) |
|---|---|---|
| `pr132-openkite.maklab.net` | `302` | `https://jmaklab.cloudflareaccess.com/cdn-cgi/access/login/pr132-openkite.maklab.net?kid=dac2e7c…` |
| `pr131-openkite.maklab.net` | `302` | (same pattern, same `kid`) |
| `pr130-openkite.maklab.net` | `302` | (same pattern, same `kid`) |
| `pr200-openkite.maklab.net` (never existed) | `302` | (same pattern, same `kid`) |
| `openkite.maklab.net` | `302` | (same pattern, same `kid`) |
| `opentest-openkite.maklab.net` | `404` | (no Access, no route) |
| `demo-api.maklab.net` | `404` | (no Access, no route) |
| `pr300-maklab.net` | NXDOMAIN | (does not match `*.maklab.net`) |
| `grafana.maklab.net` | `302` | (CF Access, separate `kid`) |
| `openagent.maklab.net` | `302` | (CF Access, separate `kid`) |
| `nonexistent-12345.example.com` | NXDOMAIN | |

Reading the table:

1. The 302 is **deterministic by hostname shape**, not by whether a hosted
   Application exists. `pr200-openkite.maklab.net` (a host nothing has ever
   generated) returns 302. `opentest-openkite.maklab.net` returns 404. The
   difference is the **path**: the `pr<N>` prefix is the host shape the Access
   application matches.
2. `pr<N>-openkite.maklab.net` and `openkite.maklab.net` come back with the
   **same `kid=`** token in the redirect. That is the Cloudflare Access JWT
   Key ID. One `kid` for every gated shape means **one Access rule** is
   matching all of them. Compare `grafana.` and `openagent.`: they have
   *different* `kid`s, matching the per-host CF Access Applications declared in
   `6-cloudflare-access.tf`.
3. `grafana.maklab.net`, `openagent.maklab.net`, `draw.maklab.net` (i.e.
   `excalidash.maklab.net`) are listed by name in
   `6-cloudflare-access.tf:16-61`. `openkite.maklab.net` is **not** in
   `6-cloudflare-access.tf` — yet it 302s. Therefore the Access rule covering
   `openkite.*` lives outside this repo, in a Cloudflare dashboard Access
   Application whose hosts field matches `*.openkite.maklab.net` (or matches
   every host in the zone via a wildcard).

**Why the cluster side is unrelated:** the istio chart's
`privateHosts` (defined in
`services/helm/istio/templates/authorization-policy-private-hosts.yaml`) accepts
`*.openkite.maklab.net` as a single AuthorizationPolicy that could DENY
unauthenticated traffic to every preview host — see the template's own
comment, lines 6-12. But `services/helm/istio/values.yaml:privateHosts: []` is
empty today. So the cluster **currently has no Istio AuthorizationPolicy** for
the preview host shape; the 302 is happening entirely at Cloudflare's edge,
before traffic reaches Istio.

**Repo pointers:**
- `services/helm/istio/templates/authorization-policy-private-hosts.yaml` —
  contains a deliberate comment explaining why this template exists and that
  `*.openkite.maklab.net` is the intended single-coverage form
- `services/helm/istio/values.yaml:privateHosts: []` — explicitly empty
- `apps/helm/templates/_helpers.tpl` (the `require-cf-access-<host>`
  AuthorizationPolicy around lines 412-435) — per-app per-env DENY policy, but
  it lives **inside** the preview's namespace. When the namespace is pruned, the
  policy is pruned with it.
- `6-cloudflare-access.tf:16-61` — three named CF Access Applications
  (`grafana`, `openagent`, `excalidash`). No `for_each`, no preview host list.

**Decision per the ticket:**

- **The residue is the design, not a leak.** A wildcard cloudflare-Access
  policy matching `*.openkite.maklab.net` is the cheap way to gate every
  current and future preview host with one rule, and there is no per-host
  cleanup to do, because the rule is not per-host.
- **Do not migrate this to per-host CF Access Applications.** They would
  multiply, drift, and create exactly the residue the ticket is worried about.
- **Do not flip ExternalDNS to `sync`** to make the residue go away — the
  wildcard `*.maklab.net` CNAME in `5-cloudflare-tunnel.tf` is the only
  routing path to the cluster, and ExternalDNS does not own it. Cleanup there
  would actually break traffic.
- **What can be cleaned up today, if anyone cares:** a specific (non-wildcard)
  `pr132-openkite.maklab.net` DNS record, if one exists in the Cloudflare
  dashboard. Manual, via the dashboard, when convenient. No code change
  required. No behavior change.

**What I could not check:** the live contents of the Cloudflare zone at
`maklab.net` (no API token in this environment). The judgment above depends on
the Terraform file being a complete enumeration of the Access Applications —
and that is a working assumption, not a checked fact. To confirm, run on a host
with `CLOUDFLARE_API_TOKEN`:

```sh
curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/access/apps" | jq
```

### 4. GHCR package versions — `pr-<N>` and the `pr-<N>-<sha>` pin

**Claim:** A merged PR's `pr-<N>` image stays on GHCR with its `pr-<N>-<sha>`
pin intact (the merge record). An unmerged PR's `pr-<N>` is deleted on close.
There are zero `pr-*` tags in the package today; that matches the
`prune-pr-image.sh --sweep` invariant.

**Repo pointers:**
- `openkite:/.github/scripts/prune-pr-image.sh` — the cleanup script.
  Notable behaviors (lines 1-26 of the comment):
  - Merged PR: keep `pr-<N>-<sha>`, delete only the mutable `pr-<N>` if the
    version (i.e. digest) is shared. If they are different digests, the merged
    PR can keep both tags. Reasoning: the sha pin is the merge record.
  - Unmerged PR: delete the entire version.
  - `--sweep` mode reads every `pr-*` tag, asks GitHub whether its PR is still
    open, deletes the leftovers. This is the catch-up cron one would run
    after any teardown gap.
- `openkite:/.github/workflows/preview.yml` (`prune` job, lines 178-205) —
  triggers on `closed`, runs `prune-pr-image.sh --merged` or default (closed)
  per `pull_request.merged`.
- `openkite:/.github/workflows/preview.yml` (`withdraw-staging` job, lines
  160-176) — if the image build for the `staging` label fails, the label is
  removed so the deploy never starts against an unbuilt `pr-<N>`.

**Probe evidence (2026-09-27, this VM, unauthenticated):**

- `GET https://api.github.com/repos/jomakori/openkite` → `visibility: public,
  default_branch: main, pushed_at: 2026-09-27T15:56:56Z, open_issues: 1`.
- `GET https://ghcr.io/v2/jomakori/openkite/tags/list` (using the anonymous
  token from the registry) → 5 tags total: `v0.32.2`, `v0.35.0`, `v0.35.1`,
  `v0.35.2`, `v0.35.3`. **Zero `pr-*` tags.** Consistent with: every PR that
  ever carried a `preview` or `staging` label has been closed, and `prune` ran.
- `GET https://api.github.com/users/jomakori/packages/container/openkite/versions?…`
  (the metadata-rich listing) → `Requires authentication`. The anonymous listing
  works through GHCR's registry API but not the Packages REST endpoint. The
  dataset above is enough for the sweep invariant.

**Decision per the ticket:**

- **Keep the `prune` job as the auto-cleanup path.** It is correct.
- **Add `--sweep` to a weekly GitHub Actions cron** (or another scheduled
  trigger) for safety. The script already has a `--sweep` mode that re-checks
  the state of every `pr-*` PR against the PR list. Today nothing invokes it
  on a schedule — only on PR `closed`. A missed `closed` (e.g. webhook outage)
  leaves the image forever. Recommend scheduling `prune-pr-image.sh --sweep`
  weekly in the upstream openkite repo. **This PR's repo (this gitops repo)
  does not own that workflow file**, so it is a recommendation, not a change.
- **No pr-<N> image residue** in the package today, so no action is needed
  *right now* for PR 132 specifically.

### 5. The failure-path clause — what happens if a build fails after the label is applied

The ticket explicitly asks about the failure path:

> Check the failure path too: an image build that fails after the label was
> applied withdraws the label — confirm the preview and any partial Cloudflare
> state come down in that case as well.

The label-applying workflow is `openkite:/.github/workflows/preview.yml`. There
are **three** label-applying paths, and each has a paired `withdraw` (or
equivalent) job:

1. `deploy` (label `preview`, lines 17-92). On `failure()`, the `withdraw` job
   (lines 94-111) calls `issues.removeLabel({ name: 'preview' })`. Removing the
   label is enough: the `teardown` job (lines 113-133) triggers on
   `unlabeled=='preview'` OR `closed`, and prunes the namespace via
   `helm uninstall` + `kubectl delete namespace`.
2. `staging` (label `staging`, lines 135-158). On `failure()` or `timed_out`,
   the `withdraw-staging` job (lines 160-176) calls
   `issues.removeLabel({ name: 'staging' })`. **However**, `staging` does not
   provision a preview — it only publishes the `pr-<N>` image. If the build
   fails and the label is withdrawn, **no preview was ever created**, so there
   is no preview to tear down. The Cloudflare side was never touched.
3. `prune` (label removal-on-close, lines 178-205). Already covered above.

The window in which a partial preview Application could be left in the cluster
when the image build failed is the period *after* the label is applied and
*before* the namespace is pruned. That window is closed by the `withdraw` job:
removing the label causes the ApplicationSet generator to stop emitting that
PR, and the Application finalizer removes the Application, which removes the
namespace — in that order. The ArgoCD side and the workflow side agree, so a
single label-withdrawal is sufficient.

**Verification path:** read `openkite:/.github/workflows/preview.yml` end to
end and check that the three label triggers (`preview`, `staging`, both via
`label` / `unlabeled` events) all tear down what they created. They do, per
the file as committed at `main:2026-09-27`.

**Decision per the ticket:**

- **Failure path is covered.** The `withdraw` and `withdraw-staging` jobs are
  the recovery path, and they recover by removing the label rather than by
  guessing what to clean up. That is the right design — it re-uses the same
  teardown logic as a normal close.
- **Partial Cloudflare state on the failure path is a non-issue here**, because
  the only "Cloudflare state" tied to the preview is the wildcard CNAME,
  which is owned by Terraform, and the wildcard Access policy, which is also
  not per-preview. There is no per-preview App to fail to delete.

---

## Source code under

- `apps/argocd-appset/templates/preview.yml` — generic PR-preview generator,
  finalizer, `namespaceOverride` and `createNamespace` parameters
- `apps/argocd-appset/values.yaml:96-145` — comment block stating the old
  bespoke `openkite-preview` chart is deleted and previews are owned by Tilt
  via this generic generator
- `apps/helm/templates/_helpers.tpl` — the umbrella chart that re-renders the
  app spec at preview coordinates; `enable_private` here renders a per-host
  AuthorizationPolicy
- `services/helm/istio/templates/authorization-policy-private-hosts.yaml` —
  wildcard-host AuthorizationPolicy support (`privateHosts: []`)
- `services/helm/external-dns/values.yaml:30` (`policy: upsert-only`) and
  `services/helm/external-dns/README.md:32` (the prose explanation)
- `services/helm/cert-manager/README.md` and `services/helm/istio/README.md` —
  wildcard TLS via `clusterIssuer` and `wildcard-maklab-net`
- `apps/argocd-appset/templates/preview-token.yaml` — the per-app
  `ExternalSecret` carrying the GitHub token the PR generator consumes
- Cloudflare-side (in `devops_Terraform/k8s-maklab-cluster`, not this repo):
  - `5-cloudflare-tunnel.tf:55-63` — wildcard CNAME `*.maklab.net → <tunnel>.cfargotunnel.com`
  - `6-cloudflare-access.tf:16-61` — three named Access Applications
    (`grafana`, `openagent`, `excalidash`). `openkite` is **not** here, even
    though `openkite.maklab.net` is gated — that rule lives in the Cloudflare
    dashboard, outside repo control.
- Upstream (in `devops_Terraform`, not this repo:
  - `6-cloudflare-access.tf` reproduces the rule list above
- OpenKite-side (in `jomakori/openkite`, not this repo):
  - `.github/workflows/preview.yml` — six jobs: `deploy`, `withdraw`,
    `teardown`, `staging`, `withdraw-staging`, `prune`
  - `.github/scripts/prune-pr-image.sh` — `--merged`, `closed`, and `--sweep`
    modes; sweep is a missed-events backstop

---

## Limitations (stated honestly)

1. **No `kubectl` access on this VM.** The "namespace was pruned" claim comes
   from the ticket plus the finalizer-based reasoning above. To verify
   directly, repeat the two `kubectl get` snippets under §Cluster leftovers
   from a host on the tailnet.
2. **No Cloudflare API token.** The Access Application inventory relies on
   the dashboard-implicit rule matching `*.openkite.maklab.net`. To verify
   directly, list Access Applications via `curl … /zones/$ZONE/access/apps`
   from a host with `CLOUDFLARE_API_TOKEN`.
3. **GitHub API requires authentication for `packages/container/versions`**
   on this network. The GHCR probe used the anonymous registry catalog, which
   shows tags but not version IDs. The `--sweep` mode of
   `prune-pr-image.sh` needs the authenticated endpoint to confirm zero
   residue — which we believe is true from the tag listing, but cannot prove
   deterministically without `gh` auth.
4. **The PRs whose previews we are most interested in — 132 in gke_GitOps and
   a closed PR in `jomakori/openkite` that carried the `preview` label** —
   are not findable in this environment beyond what the ticket has already
   confirmed. We did not re-fetch them.
5. **The CF Access policy at `*.openkite.maklab.net` is in the Cloudflare
   dashboard, not in either repo.** That means a future operator who wants
   to know which rule is gating preview hosts must look at the Cloudflare UI
   or the API, not at the GitOps repo.

---

## Recommended follow-ups (out of scope for this PR)

These do not belong in `docs/okt131-teardown-residue.md` for this repo, so the
links live here:

- In `jomakori/openkite`, schedule `prune-pr-image.sh --sweep` weekly. Today
  it only runs on PR `closed`; a missed webhook leaves the image forever.
- In `devops_Terraform`, document the dashboard-managed
  `*.openkite.maklab.net` Access rule alongside `6-cloudflare-access.tf` so
  that the rule is discoverable from the repo.
- Optionally, in `services/helm/external-dns`, add a comment block linking back
  to this doc, so the next operator who is tempted to flip `upsert-only`
  encounters this analysis first.

---

**Status:** Investigation only. No cluster, zone, or GHCR action taken by this
PR. Branch `docs/okt131-teardown-residue`; references OKT-131.
