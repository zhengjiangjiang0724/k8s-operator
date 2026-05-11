# WebAppStuckInFailed

## 告警含义

某个 WebApp 在 Failed phase 停留超过 30 分钟。这个状态由 `setDegradedAndReturn`
路径设置，意味着至少有一个 sub-resource reconcile 持续失败。

## 影响

- 该 WebApp 完全不可用（Deployment 没在跑或副本不健康）。
- 单点问题，不影响其它 WebApp。

## 检查清单

### 1. 看 Degraded condition 说了什么

```bash
NS=<from alert>
NAME=<from alert>

kubectl get webapp -n "$NS" "$NAME" -o jsonpath='{.status.conditions[?(@.type=="Degraded")]}{"\n"}'
```

`reason` 给出失败种类（ReconcileDeploymentFailed / ReconcileServiceFailed /
ReconcileIngressFailed），`message` 是底层错误。

### 2. 对应 sub-resource 状态

```bash
# 看实际 Deployment 状态
kubectl describe deploy -n "$NS" "$NAME" | tail -30

# 看 events
kubectl get events -n "$NS" \
  --field-selector involvedObject.name="$NAME" \
  --sort-by=.lastTimestamp | tail -20
```

### 3. controller 日志中针对该对象的错误

```bash
kubectl logs -n k8s-operator-system -l control-plane=controller-manager \
  --tail=500 | grep -E "$NAME|$NS" | tail -30
```

## 处置步骤

### Case A: spec 不合法（被 webhook 拦截）

例如 `env.name = "123-bad"` 这种应该被 validating webhook 拒绝，但偶发场景下
旧对象通过 schema 变化进入了非法状态。

```bash
# 直接修 spec
kubectl edit webapp -n "$NS" "$NAME"
```

让 owner 修正后保存即可。

### Case B: 镜像拉取失败

```bash
# 看 Pod 真实失败原因
kubectl describe pod -n "$NS" -l app.kubernetes.io/instance="$NAME"
```

如果是 ImagePullBackOff / ErrImagePull：

- 检查 image 拼写
- 检查 image registry 凭据（imagePullSecrets）
- 检查 image 是否真的存在 / 网络是否通

### Case C: 资源量不可解析

```bash
# 查 spec.resources
kubectl get webapp -n "$NS" "$NAME" -o yaml | yq '.spec.resources'
```

修正 CPU/Memory quantity 格式（必须匹配 CRD 正则）。

### Case D: 控制器侧 bug

如果上面都不是，且日志中堆栈指向控制器代码：

1. 把 controller pod 日志、`kubectl describe webapp` 全量截图存档。
2. 提 issue / 拉 controller owner。
3. **不要**手动删 finalizer 让对象消失 — 这会把 sub-resource 留成孤儿。

### 紧急绕过：让对象彻底消失

如果 owner 要求"先让我重建一份新的"：

```bash
# 1. 主动清理 sub-resource（避免删除 webapp 后 GC 失败）
kubectl delete deploy,svc,ingress -n "$NS" "$NAME" --ignore-not-found

# 2. 删 WebApp（finalizer 会被 controller 移除）
kubectl delete webapp -n "$NS" "$NAME"

# 3. 如果 30 秒后还卡在 Terminating，手动移 finalizer（极少需要）
kubectl patch webapp -n "$NS" "$NAME" --type=merge \
  -p '{"metadata":{"finalizers":null}}'
```

## 升级路径

- 重复出现（同一 namespace 一周内 ≥ 3 次） → 拉 owner 复盘其 WebApp spec。
- 多个 namespace 同时出现 → 怀疑是 controller bug，page controller owner。
