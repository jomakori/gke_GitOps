{{/*
Audience-sync pipeline, shared by the WatchOperation and the CronOperation in
templates/audience-sync.yaml so the script is written once.

Reads every CloudflareAccessApp XR's `.status.aud` (label-selected by Crossplane
through `requirements.requiredResources`) and force-applies the union onto istio's
RequestAuthentication/cf-access-jwt. Only `audiences` is dynamic: issuer, jwks,
selector and headers are literals below, identical to the object the istio chart
rendered before this change.
*/}}
{{- define "crossplane.audienceSyncScript" -}}
from crossplane.function import request, response

TEAM_DOMAIN = "{{ .Values.audienceSync.teamDomain }}"
NAME = "{{ .Values.audienceSync.name }}"
NAMESPACE = "{{ .Values.audienceSync.namespace }}"
ISSUER = "https://" + TEAM_DOMAIN + ".cloudflareaccess.com"


def _items(req):
    """Required resources for `access-apps`, as a list of dicts.

    The API returns a list for a label selector; tolerate a single dict or an
    `items` wrapper too so a future SDK shape cannot silently empty the list.
    """
    try:
        found = request.get_required_resources(req, "access-apps")
    except AttributeError:  # older function SDK: single-resource accessor only
        found = request.get_required_resource(req, "access-apps")
    if isinstance(found, dict):
        items = found.get("items")
        found = items if isinstance(items, list) else [found]
    return [r for r in found or [] if isinstance(r, dict)]


def operate(req, rsp):
    """Force-apply the istio RequestAuthentication's audience list.

    Collects `.status.aud` from every CloudflareAccessApp XR, drops empties,
    de-duplicates and sorts for a stable diff. Never applies an empty audience
    list: that would 401 every Cloudflare-Access-gated host.
    """
    if not TEAM_DOMAIN:
        response.warning(rsp, "audienceSync.teamDomain is empty; not touching " + NAMESPACE + "/" + NAME)
        return

    apps = _items(req)

    def aud_of(app):
        return (app.get("status") or {}).get("aud") or ""

    auds = sorted({aud_of(a) for a in apps} - {""})
    hosts = sorted({(a.get("spec") or {}).get("host") or "" for a in apps if aud_of(a)} - {""})
    if not auds:
        response.warning(rsp, "no CloudflareAccessApp has .status.aud yet; leaving " + NAMESPACE + "/" + NAME + " unchanged")
        return

    rsp.desired.resources["cf-access-jwt"].resource.update({
        "apiVersion": "security.istio.io/v1",
        "kind": "RequestAuthentication",
        "metadata": {"name": NAME, "namespace": NAMESPACE},
        "spec": {
            "selector": {"matchLabels": {"istio": "ingressgateway"}},
            "jwtRules": [{
                "issuer": ISSUER,
                "jwksUri": ISSUER + "/cdn-cgi/access/certs",
                "audiences": auds,
                "fromHeaders": [{"name": "Cf-Access-Jwt-Assertion"}],
                "forwardOriginalToken": True,
                "outputPayloadToHeader": "x-cf-access-jwt-payload",
            }],
        },
    })
    response.set_output(rsp, {"audiences": len(auds), "hosts": hosts})
{{- end -}}

{{- define "crossplane.audienceSyncPipeline" -}}
mode: Pipeline
pipeline:
  - step: sync-audiences
    functionRef:
      name: function-python
    requirements:
      requiredResources:
        - requirementName: access-apps
          apiVersion: platform.maklab.net/v1alpha1
          kind: CloudflareAccessApp
          matchLabels:
            platform.maklab.net/access-managed-by: crossplane
    input:
      apiVersion: python.fn.crossplane.io/v1beta1
      kind: Script
      script: |
{{ include "crossplane.audienceSyncScript" . | indent 8 }}
{{- end -}}
