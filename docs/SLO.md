# WebApp Operator SLO

## 适用范围

本文件定义 WebApp Operator **控制平面**（即 operator 自身）的 SLI/SLO，
不涵盖被它管理的 WebApp 工作负载（那部分是 application owner 的责任）。

时间窗口：30 天滚动窗口。

## SLI 定义

### SLI-1: Reconcile 成功率

```
sum(rate(webapp_reconcile_total{result="success"}[5m]))
/
sum(rate(webapp_reconcile_total[5m]))
```

衡量 reconcile loop 成功完成（无错误返回）的比例。失败包含 SSA 失败、status
patch 失败、builder 错误、API Server 不可达等。

### SLI-2: Reconcile 延迟

```
histogram_quantile(0.99,
  sum by (le) (rate(webapp_reconcile_duration_seconds_bucket[5m]))
)
```

衡量端到端 reconcile 的 P99 延迟。注意这是 controller 内部延迟，**不**包含
informer 事件投递的延迟。

### SLI-3: WebApp 副本可用度

```
sum(webapp_replicas_ready) / sum(webapp_replicas_desired)
```

衡量当前所有 WebApp 的总 ready 副本占总 desired 副本的比例。

### SLI-4: Admission Webhook P99 延迟

```
histogram_quantile(0.99,
  sum by (le, webhook) (rate(webapp_webhook_duration_seconds_bucket[5m]))
)
```

衡量 mutating + validating webhook 的处理延迟。Webhook 在 admission 链路上是
同步阻塞的，过慢会拖累 kubectl apply。

## SLO 目标

| SLI | SLO 目标 | 错误预算 (30天) |
|---|---|---|
| Reconcile 成功率 | ≥ 99.5% | 0.5% = 3.6 小时 |
| Reconcile P99 延迟 | < 1s | — |
| WebApp 副本可用度 | ≥ 99% | 1% = 7.2 小时 |
| Webhook P99 延迟 | < 100ms | — |

## 错误预算策略

错误预算燃烧速度决定告警严重度，采用 Google SRE Book 的多窗口多燃烧速度模型：

| 燃烧速度 | 检测窗口 | 短窗口 | 触发动作 |
|---|---|---|---|
| 14.4× | 1h | 5m | Page (P1) |
| 6× | 6h | 30m | Page (P2) |
| 3× | 24h | 2h | Ticket (P3) |
| 1× | 3d | 6h | Ticket (P4) |

### Burn-rate 告警 PromQL

```promql
# P1: 14.4x burn over 1h AND 5m (会在 ~2h 内耗光 30 天预算)
- alert: WebAppOperatorErrorBudgetBurnFast
  expr: |
    (
      1 - (
        sum(rate(webapp_reconcile_total{result="success"}[1h]))
        / sum(rate(webapp_reconcile_total[1h]))
      )
    ) > 14.4 * (1 - 0.995)
    and
    (
      1 - (
        sum(rate(webapp_reconcile_total{result="success"}[5m]))
        / sum(rate(webapp_reconcile_total[5m]))
      )
    ) > 14.4 * (1 - 0.995)
  for: 2m
  labels:
    severity: critical
    sli: reconcile_success_rate
  annotations:
    summary: "Reconcile error budget burning at 14.4x rate"
    runbook_url: "https://example.com/runbook/error-budget-burn"

# P2: 6x over 6h AND 30m (~5 days to deplete budget)
- alert: WebAppOperatorErrorBudgetBurnMedium
  expr: |
    (
      1 - (
        sum(rate(webapp_reconcile_total{result="success"}[6h]))
        / sum(rate(webapp_reconcile_total[6h]))
      )
    ) > 6 * (1 - 0.995)
    and
    (
      1 - (
        sum(rate(webapp_reconcile_total{result="success"}[30m]))
        / sum(rate(webapp_reconcile_total[30m]))
      )
    ) > 6 * (1 - 0.995)
  for: 15m
  labels:
    severity: warning
    sli: reconcile_success_rate
```

## 释放阀策略

当 30 天滚动窗口内错误预算 **耗尽**（剩余 ≤ 0%）时：

1. **冻结新功能发布** — 只允许 bug fix / 可靠性改进合入。
2. **postmortem 必须包含 reliability 提升项** — 每个 incident 至少一条系统级修复。
3. **暂停 chaos / load test** — 直到预算回正。

当错误预算 ≤ 25%（接近耗尽）时：

1. **review 进行中变更的风险** — 高风险变更推迟到下一个窗口。
2. **加强观测** — 临时降低告警阈值，提早发现退化。

## SLO 评审节奏

- **每周**：值班同学 review SLO 仪表盘，发现异常模式提早响应。
- **每月**：服务 owner 评审 30 天 SLO 达成情况，调整 SLO 目标或资源投入。
- **每季度**：根据用户实际体感和业务诉求，重新校准 SLO 目标。

## 与可观测性的关系

- 指标定义见 [`METRICS.md`](./METRICS.md)。
- 告警规则示例见 [`METRICS.md`](./METRICS.md) § 推荐告警规则。
- 处置手册见 [`runbook/`](./runbook/) 目录。
- Grafana dashboard 见 [`grafana/webapp-operator.json`](./grafana/webapp-operator.json)。
