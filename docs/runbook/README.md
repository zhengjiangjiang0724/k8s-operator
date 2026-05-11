# Runbook 索引

每个告警都对应一份处置手册。Runbook 的目标是让**值班同学不需要找作者**就能
完成 90% 的处置 — 想清楚后能上手的步骤、kubectl 命令、判定条件，全部写在文档里。

| Alert | 严重度 | Runbook |
|---|---|---|
| `WebAppReconcileErrorRateHigh` | warning | [reconcile-error-rate-high.md](./reconcile-error-rate-high.md) |
| `WebAppStuckInFailed` | warning | [stuck-in-failed.md](./stuck-in-failed.md) |
| `WebAppWebhookLatencyHigh` | warning | [webhook-latency-high.md](./webhook-latency-high.md) |
| `WebAppReplicasBelowDesired` | warning | [replicas-below-desired.md](./replicas-below-desired.md) |
| `WebAppOperatorErrorBudgetBurnFast` | critical | [error-budget-burn.md](./error-budget-burn.md) |

## 通用处置流程

任何告警先做的三件事：

1. **确认告警真实性** — Grafana 看图，确认不是采集抖动。
2. **判定影响范围** — 是单个 WebApp / 单个 namespace / 全集群。
3. **拉群 owner** — 如果影响范围 > 单个 WebApp，立即拉对应 namespace 的 owner。

## Runbook 模板

写新 runbook 时遵循以下结构：

```markdown
# <Alert Name>

## 告警含义
一两句话说清这个告警在告什么。

## 影响
谁会受影响、影响多大。

## 检查清单
1. <kubectl 命令> — 看什么结果
2. <Grafana 链接> — 关注哪几个指标

## 处置步骤
按"先回滚后修复"原则给出可复制粘贴的命令。

## 升级路径
什么情况下升级到下一个 oncall 层级。
```
