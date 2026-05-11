# WebAppReplicasBelowDesired

## 告警含义

`webapp_replicas_ready < webapp_replicas_desired` 持续 15 分钟。即某个 WebApp
的就绪副本数长期低于期望值。

## 影响

- 用户流量被分摊到更少的 Pod，可能延迟或饱和。
- 单点故障：如果 ready 副本只剩 1，下一次 Pod 失败就完全不可用。

## 检查清单

### 1. 哪个 WebApp？

```bash
kubectl exec -n monitoring deploy/prometheus -- promtool query instant '
  (webapp_replicas_desired - webapp_replicas_ready) > 0
'
```

### 2. 看底层 Deployment

```bash
NS=<from alert>
NAME=<from alert>

kubectl get deploy -n "$NS" "$NAME"
# READY 列直接反映 ready/desired
```

### 3. 看 Pod 状态分布

```bash
kubectl get pods -n "$NS" -l app.kubernetes.io/instance="$NAME"
```

关注 STATUS 列：是 Pending / CrashLoopBackOff / ImagePullBackOff / 还是
Running 但 ReadinessProbe 失败。

## 处置步骤

### Case A: Pending（节点资源不足）

```bash
kubectl describe pod -n "$NS" -l app.kubernetes.io/instance="$NAME" | grep -A 5 Events
```

通常会看到 `FailedScheduling: insufficient cpu/memory`。处置：

- 扩节点（如果 cluster autoscaler 启用且未触发 → 看 autoscaler 健康）
- 调小 spec.resources.requests
- 临时减 replicas

### Case B: CrashLoopBackOff

```bash
# 看上次容器的日志
POD=$(kubectl get pods -n "$NS" -l app.kubernetes.io/instance="$NAME" -o name | head -1)
kubectl logs -n "$NS" "$POD" --previous --tail=100
```

应用日志会告诉你是配置错误、依赖未就绪还是 panic。改 spec.env / spec.image 后
WebApp 会自动滚动更新。

### Case C: ImagePullBackOff

```bash
kubectl describe pod -n "$NS" -l app.kubernetes.io/instance="$NAME" | grep -A 3 'Pulling image'
```

- 拼写错误：让 owner 改 spec.image
- 私有 registry 没有凭据：在 WebApp 所在 namespace 配 imagePullSecret
  - 注意：当前 WebApp CRD 不支持 imagePullSecrets 字段，这是个**已知限制**，
    需要在该 namespace 的 default ServiceAccount 上配。

### Case D: ReadinessProbe 失败

```bash
# 看 readiness 探测的实际响应
POD=$(kubectl get pods -n "$NS" -l app.kubernetes.io/instance="$NAME" -o name | head -1)
kubectl exec -n "$NS" "$POD" -- wget -qO- http://localhost:<port>/<probe-path>
```

应用层未就绪 → 通常是依赖（DB / 下游服务）不通。

如果探测路径错了：让 owner 改 spec.healthCheck.path。

## 升级路径

- 单 WebApp ready=0 持续超过 10 分钟 → page 该 namespace owner。
- 同一 namespace 多个 WebApp 同时降级 → 怀疑 namespace 级配额 / 网络问题。
- 全集群普遍降级 → page 平台 SRE。
