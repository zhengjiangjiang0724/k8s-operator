# webapp-operator (Helm chart)

Production-grade Helm chart for the WebApp Operator.

## Prerequisites

- Kubernetes ≥ 1.27
- [cert-manager](https://cert-manager.io/) ≥ 1.13 — required when
  `webhook.enabled=true && certManager.enabled=true` (default).
- [Prometheus Operator](https://prometheus-operator.dev/) — required when
  `metrics.serviceMonitor.enabled=true` (default).

## Install

```bash
helm repo add webapp-operator https://example.github.io/webapp-operator
helm install webapp-operator webapp-operator/webapp-operator \
    --namespace webapp-operator-system --create-namespace
```

Label the namespaces that host kube-apiserver and Prometheus so the
NetworkPolicy permits traffic:

```bash
kubectl label ns kube-system webhooks=enabled
kubectl label ns monitoring metrics=enabled
```

## Upgrade

```bash
helm upgrade webapp-operator webapp-operator/webapp-operator \
    --namespace webapp-operator-system \
    --reuse-values --version <new>
```

> CRDs are installed via `templates/` (not `crds/`) so they upgrade with
> the chart. The default `crds.keep=true` keeps them on `helm uninstall`.

## Uninstall

```bash
helm uninstall webapp-operator -n webapp-operator-system
# CRDs are intentionally retained. Delete manually if desired:
kubectl delete crd webapps.myapp.example.com
```

## Production checklist

- [ ] Pin `image.tag` to a digest (`@sha256:...`) — avoid moving tags.
- [ ] Set `image.repository` to your internal registry mirror.
- [ ] Override `certManager.issuerRef.name` to your internal CA Issuer
      (avoid the chart's self-signed fallback).
- [ ] Set `metrics.serviceMonitor.insecureSkipVerify=false` after mounting
      the cert-manager CA bundle into Prometheus.
- [ ] Review `resources.limits` against your expected CR count
      (see `docs/PERFORMANCE.md`).
- [ ] Add `imagePullSecrets` if pulling from a private registry.
- [ ] Confirm `topologySpreadConstraints` matches your cluster topology
      (e.g. use `topology.kubernetes.io/zone` for multi-AZ).

## Values reference

See [`values.yaml`](./values.yaml) — every field has an inline comment.
The defaults are production-grade: HA, hardened Pod Security, NetworkPolicy,
cert-manager TLS, and a ServiceMonitor.

## Differences from the Kustomize install

| Concern | Helm chart | Kustomize (`config/default`) |
|---|---|---|
| Customization | `--set` / values.yaml | `kustomize edit` / patches |
| CRD upgrade | Automatic (in `templates/`) | Manual (`kubectl apply`) |
| Multi-instance per cluster | ✅ via `--name-template` | ✗ (single fixed namespace) |
| Bundled in OperatorHub | ❌ (use OLM bundle) | ❌ |

Both produce equivalent runtime resources. Helm is recommended for
production; Kustomize is convenient for local development.
