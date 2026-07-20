# Rollback: Hermes → Sympozium

## Prerequisites

- Git access to `gke_GitOps` repository
- ArgoCD access (UI or CLI)
- kubectl access to the cluster

## Rollback Steps

### 1. Revert the Merge Commit

```bash
# Find the merge commit SHA
git log --oneline -10

# Revert the Hermes migration commit
git revert <merge-commit-sha>
git push origin main
```

### 2. Verify ArgoCD Sync

```bash
# Check ArgoCD application status
argocd app get openagent

# Or via kubectl
kubectl get application openagent -n argocd -o yaml
```

Wait for ArgoCD to sync (3-5 minute interval).

### 3. Verify Pod Status

```bash
# Check all pods in openagent namespace
kubectl get pods -n openagent

# Expected pods after rollback:
# - openagent-discord-* (custom Go bot)
# - openagent-headroom-* (LLM proxy)
# - openagent-litellm-* (LLM gateway)
# - openagent-pg-* (PostgreSQL)
# - claude-proxy-* (Claude proxy)
```

### 4. Verify Discord Bot

```bash
# Check bot logs
kubectl logs -n openagent -l app.kubernetes.io/name=openagent-discord --tail=50

# Verify bot is online in Discord
# - Bot should show as online
# - Test mention in a channel
```

### 5. Verify VirtualService

```bash
# Check VirtualService points back to sympozium
kubectl get virtualservice -n openagent -o yaml

# Expected destination:
# host: omo-loop-engineering-sisyphus-web-endpoint-server.sympozium-system.svc.cluster.local
# port: 8080
```

## Known Issues After Rollback

### 1. Conversation State Lost
- Hermes stores state in `HERMES_HOME` (file-based)
- Sympozium uses `/state/conversations.json`
- **Impact**: Users will need to start new conversations
- **Mitigation**: None — this is expected behavior

### 2. Template Restoration Required
The following templates were deleted during migration and must be restored:

- `templates/ensemble/omo-loop-engineering.yaml` (Sympozium Ensemble CRD)
- `templates/skillpacks/hashline-editor.yaml`
- `templates/skillpacks/k8s-ops.yaml`
- `templates/skillpacks/omo-core-skills.yaml`

These are restored automatically by git revert.

### 3. Configuration Changes
The following values must be restored:

- `hermes.enabled: false` (remove hermes section)
- `hermes-workspace.enabled: false` (remove hermes-workspace section)
- `openagent-component.bot.enabled: true` (re-enable custom bot)
- `ensemble.enabled: true` (re-enable Sympozium CRD)

These are restored automatically by git revert.

### 4. Gateway Templates
The following gateway templates were deleted:

- `templates/gateway/virtualservice-dashboard.yaml`
- `templates/gateway/authorizationpolicy-dashboard.yaml`

These are restored automatically by git revert.

The `gateways.enable_private: true` in `services/argocd-appset/values.yaml` must be removed.

## Verification Checklist

After rollback, verify:

- [ ] All pods are running and healthy
- [ ] Discord bot is online and responding
- [ ] VirtualService points to Sympozium endpoint
- [ ] LiteLLM gateway is accessible
- [ ] Headroom proxy is functioning
- [ ] PostgreSQL is healthy
- [ ] Claude proxy is working

## Escalation

If rollback fails:

1. Check ArgoCD sync status
2. Check pod events: `kubectl describe pod -n openagent <pod-name>`
3. Check logs: `kubectl logs -n openagent <pod-name>`
4. Contact cluster administrator

## Rollback Time Estimate

- Git revert + push: ~2 minutes
- ArgoCD sync: ~3-5 minutes
- Pod restart: ~2-3 minutes
- **Total**: ~10-15 minutes
