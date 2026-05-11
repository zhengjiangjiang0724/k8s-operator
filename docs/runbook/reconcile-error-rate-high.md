# WebAppReconcileErrorRateHigh

## 告警含义

Reconcile 错误率在 5 分钟窗口内超过 10%。表示 controller 在大量失败地处理
WebApp 对象 — 可能是少数对象在循环失败，也可能是底层依赖（API Server / RBAC
/ admission）异常。

## 影响

- 失败 reconcile 不会更新 sub-resource，期望状态偏离实际状态。
- 错误预算被快速消耗（参见 `docs/SLO.md`）。
- 用户体验：WebApp Phase 长时间停留在 Creating / Updating / Failed。

## 检查清单

### 1. 是哪几个 WebApp 在失败？

```bash
# Top 5 错误源
kubectl exec -n monitoring deploy/prometheus -- promtool query instant \
  'topk(5, sum by (namespace, name) (rate(webapp_reconcile_total{result="error"}[5m])))'
```

如果错误集中在 1-2 个对象 → 跳到「单对象失败」。
如果错误分散在所有对象 → 跳到「全局失败」。

### 2. 看 controller 日志

```bash
kubectl logs -n k8s-operator-system -l control-plane=controller-manager \
  --tail=200 --since=10m | grep -i error
```

关注 `failed to apply`, `failed to update status`, `forbidden`, `timeout`。

### 3. 看子资源失败分布

```bash
# 哪种 sub-resource 失败最多？
kubectl exec -n monitoring deploy/prometheus -- promtool query instant \
  'sum by (kind) (rate(webapp_subresource_operations_total{result="error"}[5m]))'
```

## 处置步骤

### 单对象失败

```bash
# 锁定问题对象
NS=<from above>
NAME=<from above>

# 看对象 status
kubectl get webapp -n "$NS" "$NAME" -o yaml | yq '.status'

# 看 events
kubectl describe webapp -n "$NS" "$NAME" | tail -30
```

常见原因 + 处置：

| 症状 | 原因 | 处置 |
|---|---|---|
| Degraded condition: ReconcileDeploymentFailed | spec 中资源量无法解析 | 让 owner 修 spec.resources |
| Forbidden on apply | RBAC 漂移 | 重新 `kubectl apply -f config/rbac/` |
| AdmissionWebhook failure | webhook 不健康 | 跳到 `webhook-latency-high.md` |

### 全局失败

> 全集群同时失败几乎都是基础设施问题，**不要**重启 controller 当万金油。

```bash
# 1. API Server 健康？
kubectl get --raw='/readyz?verbose'

# 2. controller 自身 OOMKilled / CrashLoopBackOff？
kubectl get pods -n k8s-operator-system -l control-plane=controller-manager

# 3. cert-manager 健康？webhook 依赖它颁发的证书
kubectl get pods -n cert-manager
kubectl get certificate -n k8s-operator-system
```

如果是 controller 自身 OOM：

```bash
# 临时把 limit 翻倍救场（PR 后续修 manager.yaml）
kubectl set resources deploy/k8s-operator-controller-manager \
  -n k8s-operator-system \
  -c manager --limits=memory=1Gi
```

## 升级路径

- 30 分钟内未恢复 → page 平台 SRE（怀疑是 K8s 层问题）。
- 触发 `WebAppOperatorErrorBudgetBurnFast`（critical）→ 立刻 page。
