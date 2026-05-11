# WebAppWebhookLatencyHigh

## 告警含义

Mutating 或 Validating webhook 的 P99 延迟超过 500ms（持续 10 分钟）。
Webhook 在 admission 链路同步阻塞，过慢会拖累每次 `kubectl apply webapp`。

## 影响

- `kubectl apply` 慢甚至超时（API Server 默认 webhook timeout 是 10s）。
- 失败 + `failurePolicy=fail` 会**直接拒绝**对象创建/更新。
- 间接影响 controller 自身（controller 也通过 API 创建对象 → 触发自家
  webhook → 一并慢）。

## 检查清单

### 1. 哪个 webhook 慢？

```bash
kubectl exec -n monitoring deploy/prometheus -- promtool query instant '
  histogram_quantile(
    0.99,
    sum by (le, webhook, operation) (rate(webapp_webhook_duration_seconds_bucket[5m]))
  )
'
```

### 2. controller pod 健康？

```bash
kubectl get pods -n k8s-operator-system -l control-plane=controller-manager
kubectl top pods -n k8s-operator-system -l control-plane=controller-manager
```

CPU 打满 / OOM Kill 会让 webhook 一起慢。

### 3. webhook 服务端点是否完整？

```bash
kubectl get endpoints -n k8s-operator-system k8s-operator-webhook-service
```

如果只有一个 endpoint 且 controller HA 配的 2 副本 → 有一个 pod 不在 Ready
状态，承担双倍负载。

### 4. cert-manager 证书是否在轮换？

```bash
kubectl get certificate -n k8s-operator-system
kubectl describe certificate -n k8s-operator-system serving-cert | tail -20
```

证书过期会让 webhook TLS 握手失败 → 表现为延迟尖刺 + 错误率上升。

## 处置步骤

### 短期：恢复 admission 可用

如果延迟离谱（> 5s）且影响业务：

```bash
# 临时把 failurePolicy 改成 Ignore（让请求绕过 webhook 仍能通过）
kubectl patch mutatingwebhookconfiguration k8s-operator-mutating-webhook-configuration \
  --type='json' \
  -p='[{"op":"replace","path":"/webhooks/0/failurePolicy","value":"Ignore"}]'

kubectl patch validatingwebhookconfiguration k8s-operator-validating-webhook-configuration \
  --type='json' \
  -p='[{"op":"replace","path":"/webhooks/0/failurePolicy","value":"Ignore"}]'
```

⚠️ **副作用**：webhook 跳过期间，非法 spec 也能进入 etcd，controller 会进 Failed
phase。事后需要全量扫描受影响对象。

恢复：

```bash
kubectl patch ... --type='json' \
  -p='[{"op":"replace","path":"/webhooks/0/failurePolicy","value":"Fail"}]'
```

### 中期：扩 controller 副本

```bash
# 临时扩到 4 副本（PR 后续修 chart values）
kubectl scale deploy/k8s-operator-controller-manager \
  -n k8s-operator-system --replicas=4
```

### 长期：定位代码热点

如果延迟来自 webhook 处理本身（不是 TLS / 网络），用 pprof：

```bash
kubectl port-forward -n k8s-operator-system deploy/k8s-operator-controller-manager 6060:6060 &
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
```

> 注：默认未开启 pprof，需要在 manager 启动参数加 `--enable-pprof=true`。

## 升级路径

- 影响范围超过 1 个 namespace 持续 15 分钟 → page 平台 SRE。
- 出现 TLS 错误（cert-manager 异常）→ page cert-manager owner。
