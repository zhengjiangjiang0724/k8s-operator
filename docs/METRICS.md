# WebApp Operator 指标参考

operator 在 `/metrics` 端点（默认 `:8443`，HTTPS）暴露 Prometheus 格式的指标。
controller-runtime 自动注册若干基础指标（workqueue、reconcile 总数等），本文
仅记录**业务自定义**指标。

> 该端点同时被 **Prometheus** 和 **VictoriaMetrics** 原生支持，无需额外
> 适配。下文 PromQL 在 MetricsQL（VictoriaMetrics 的方言）下也兼容。

## 命名规范

| 后缀 | 类型 | 说明 |
|---|---|---|
| `_total` | Counter | 单调递增计数 |
| `_seconds` | Histogram | 时间（秒），含 `_bucket` / `_sum` / `_count` |
| (无后缀) | Gauge | 瞬时值 |

所有指标共享 `webapp_` 前缀。

## 标签基数

| 标签 | 取值数量 | 说明 |
|---|---|---|
| `name` | WebApp 数量 | 单个 WebApp 实例名 |
| `namespace` | 命名空间数量 | |
| `phase` | 6 | Pending / Creating / Running / Updating / Failed / Deleting |
| `kind` | 3 | Deployment / Service / Ingress |
| `operation` | 2-3 | apply / delete / create_update / create / update / delete |
| `result` | 2-3 | success / error 或 allow / deny / error |
| `image` | 实际镜像数 | **可能产生较高基数**，详见下方注意事项 |

⚠️ **关于 `webapp_info{image=...}` 的基数注意事项**：每次镜像更新都会
产生新的标签组合。本 operator 在 reconcile 完成时通过 `DeletePartialMatch`
清掉旧序列，避免 series 无限增长。如果客户场景下镜像 tag 变更非常频繁
（例如 commit-sha tag），建议在 Prometheus 抓取层用
`metric_relabel_configs` 把 `image` label 丢掉。

## 指标清单

### Reconcile 循环

| 指标 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `webapp_reconcile_total` | Counter | `name`, `namespace`, `result` | reconcile 调用总数 |
| `webapp_reconcile_duration_seconds` | Histogram | `name`, `namespace`, `result` | reconcile 单次延迟 |
| `webapp_reconcile_errors_total` | Counter | `name`, `namespace` | (DEPRECATED) reconcile 错误数，等价于 `reconcile_total{result="error"}` |

### 子资源操作（SSA Apply / Delete）

| 指标 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `webapp_subresource_operations_total` | Counter | `kind`, `operation`, `result` | Deployment/Service/Ingress 上的 SSA / 删除次数 |
| `webapp_subresource_operation_duration_seconds` | Histogram | `kind`, `operation` | 单次 API 调用延迟（不含构造时间） |

### Admission Webhook

| 指标 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `webapp_webhook_requests_total` | Counter | `webhook`, `operation`, `result` | mutating/validating webhook 调用次数 |
| `webapp_webhook_duration_seconds` | Histogram | `webhook`, `operation` | webhook 处理延迟 |

`webhook` ∈ {`mutating`, `validating`}；
mutating 的 `operation` 固定为 `create_update`（CustomDefaulter 不区分），
validating 的 `operation` ∈ {`create`, `update`, `delete`}；
`result` ∈ {`allow`, `deny`, `error`}。

### WebApp 状态

| 指标 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `webapp_info` | Gauge | `name`, `namespace`, `phase`, `image` | 恒为 1，标签携带当前状态 |
| `webapp_phase` | Gauge | `name`, `namespace`, `phase` | 当前 phase=1，其余 phase=0 |
| `webapp_replicas_desired` | Gauge | `name`, `namespace` | spec 中的期望副本数 |
| `webapp_replicas_ready` | Gauge | `name`, `namespace` | Deployment status 中的就绪副本数 |

## 常用 PromQL 查询

### 健康度 SLI

```promql
# Reconcile 成功率（5 分钟滑动窗口）
sum(rate(webapp_reconcile_total{result="success"}[5m]))
/
sum(rate(webapp_reconcile_total[5m]))

# 各 namespace 的就绪率
sum by (namespace) (webapp_replicas_ready)
/
sum by (namespace) (webapp_replicas_desired)
```

### 延迟分布

```promql
# Reconcile p99 延迟
histogram_quantile(
  0.99,
  sum by (le) (rate(webapp_reconcile_duration_seconds_bucket[5m]))
)

# Webhook p99 延迟（按类型拆分）
histogram_quantile(
  0.99,
  sum by (le, webhook) (rate(webapp_webhook_duration_seconds_bucket[5m]))
)

# 子资源 API 调用 p99
histogram_quantile(
  0.99,
  sum by (le, kind, operation) (rate(webapp_subresource_operation_duration_seconds_bucket[5m]))
)
```

### 错误监控

```promql
# 子资源 SSA 错误率
sum by (kind) (rate(webapp_subresource_operations_total{result="error"}[5m]))

# 被 validating webhook 拒绝的请求速率
sum by (operation) (rate(webapp_webhook_requests_total{webhook="validating", result="deny"}[5m]))
```

### 状态分布

```promql
# 各 phase 的 WebApp 数量
sum by (phase) (webapp_phase) > 0

# 长期处于 Failed 状态的 WebApp
webapp_phase{phase="Failed"} == 1
  and on(name, namespace) (time() - webapp_phase{phase="Failed"} offset 30m) > 0
```

### 镜像分布（基数注意）

```promql
# 按镜像分组的实例数（如果担心基数可改用 label_replace 截断 tag）
count by (image) (webapp_info)
```

## 抓取配置示例

### Prometheus（直接 scrape）

```yaml
scrape_configs:
  - job_name: webapp-operator
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/secrets/webapp-operator-ca/ca.crt
      insecure_skip_verify: false
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_name]
        action: keep
        regex: webapp-operator
      - source_labels: [__meta_kubernetes_pod_container_port_name]
        action: keep
        regex: https
```

### Prometheus Operator（ServiceMonitor）

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: webapp-operator
  namespace: webapp-operator-system
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: webapp-operator
  endpoints:
    - port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: true
      interval: 30s
```

### VictoriaMetrics Operator（VMServiceScrape）

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMServiceScrape
metadata:
  name: webapp-operator
  namespace: webapp-operator-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: webapp-operator
  endpoints:
    - port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: true
      interval: 30s
```

## 推荐告警规则（示意）

```yaml
groups:
  - name: webapp-operator
    rules:
      - alert: WebAppReconcileErrorRateHigh
        expr: |
          sum(rate(webapp_reconcile_total{result="error"}[5m]))
          /
          sum(rate(webapp_reconcile_total[5m])) > 0.1
        for: 10m
        labels: {severity: warning}
        annotations:
          summary: "WebApp reconcile error rate > 10%"

      - alert: WebAppStuckInFailed
        expr: |
          max by (name, namespace) (webapp_phase{phase="Failed"}) == 1
        for: 30m
        labels: {severity: warning}
        annotations:
          summary: "WebApp {{ $labels.namespace }}/{{ $labels.name }} stuck in Failed >30m"

      - alert: WebAppWebhookLatencyHigh
        expr: |
          histogram_quantile(
            0.99,
            sum by (le, webhook) (rate(webapp_webhook_duration_seconds_bucket[5m]))
          ) > 0.5
        for: 10m
        labels: {severity: warning}
        annotations:
          summary: "Webhook p99 > 500ms — kubectl apply will be slow"

      - alert: WebAppReplicasBelowDesired
        expr: |
          webapp_replicas_ready < webapp_replicas_desired
        for: 15m
        labels: {severity: warning}
        annotations:
          summary: "WebApp {{ $labels.namespace }}/{{ $labels.name }} has fewer ready replicas than desired"
```

## 兼容性说明

| 后端 | 兼容性 | 备注 |
|---|---|---|
| Prometheus | ✅ 原生 | 直接 scrape |
| VictoriaMetrics | ✅ 原生 | 通过 vmagent 或 VMServiceScrape |
| Thanos | ✅ 原生 | 通过 Sidecar 或 Receive |
| Cortex / Mimir | ✅ 原生 | 通过 remote_write |
| OpenTelemetry | ⚠️ 间接 | 需通过 otelcol prometheus receiver 转换 |

历史指标 `webapp_reconcile_errors_total` 标记为 DEPRECATED，但仍然保留以
兼容已有 dashboard / alert。新查询请使用 `webapp_reconcile_total{result="error"}`。
