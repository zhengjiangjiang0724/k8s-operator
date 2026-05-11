# WebAppOperatorErrorBudgetBurnFast

## 告警含义

错误预算**燃烧速度** ≥ 14.4× ，且在 1 小时和 5 分钟两个窗口同时满足。
按这个速度，**~2 小时**就能耗光整个 30 天的预算。这是 critical 级，必须 page。

参见 `docs/SLO.md` § 错误预算策略。

## 影响

- 30 天 SLO 即将失守。
- 一旦 30 天预算耗尽，触发"冻结新功能发布"政策。
- 通常意味着控制面/数据面同时存在系统性问题。

## 快速分流

```bash
# 错误总量分布（不同 result 哪种最多）
kubectl exec -n monitoring deploy/prometheus -- promtool query instant '
  topk(10, sum by (namespace, name) (rate(webapp_reconcile_total{result="error"}[5m])))
'

# Sub-resource 错误分布
kubectl exec -n monitoring deploy/prometheus -- promtool query instant '
  sum by (kind, operation) (rate(webapp_subresource_operations_total{result="error"}[5m]))
'
```

如果错误集中：跳到 [reconcile-error-rate-high.md](./reconcile-error-rate-high.md)。

## 处置步骤

### 第一步：止血（5 分钟内）

如果原因不明且影响在扩大，**优先回滚最近的变更**：

```bash
# 1. 看 controller 是否最近被重新部署
kubectl rollout history deploy/k8s-operator-controller-manager -n k8s-operator-system

# 2. 如果是几分钟内的部署 → 回滚
kubectl rollout undo deploy/k8s-operator-controller-manager -n k8s-operator-system

# 3. 等 1 分钟看错误率是否下降
sleep 60
kubectl exec -n monitoring deploy/prometheus -- promtool query instant '
  1 - sum(rate(webapp_reconcile_total{result="success"}[1m])) / sum(rate(webapp_reconcile_total[1m]))
'
```

### 第二步：诊断（10 分钟内）

如果回滚后未恢复，问题在更下层：

```bash
# 1. API Server / etcd
kubectl get --raw='/readyz?verbose'
kubectl get componentstatuses

# 2. cert-manager（webhook 依赖）
kubectl get pods -n cert-manager
kubectl get certificate -A | grep -v True

# 3. controller 自身
kubectl logs -n k8s-operator-system -l control-plane=controller-manager --tail=500 --since=15m
```

### 第三步：恢复 + 沟通

- **同步给 stakeholder** — 通知 namespace owners 与上层服务负责人。
- **打开事故频道** — 记录处置时间线。
- **postmortem 必填项**：
  - 触发原因
  - 检测延迟（事故发生到告警触发的时间）
  - 缓解延迟（告警到止血的时间）
  - **系统级修复项** — 防止下次复发，不能只写"加强 review"。

## 升级路径

**立即 page** 以下角色（不要等）：

- Controller owner (operator code)
- 平台 SRE (集群基础设施)
- Service owner (上游业务方，让他们准备降级措施)

## 事后

预算耗尽（残留 ≤ 0%）后，进入 **release freeze**：

- PR 必须打 `reliability-fix` 标签才能合
- 待错误预算回到 ≥ 25% 才能解冻
