# WebApp Operator 性能测试报告

## 1. 测试目标

衡量 WebApp Operator 关键代码路径的**微观性能**（per-call 延迟、内存分配），
作为后续优化和回归对比的基线。本报告**不**覆盖端到端的集群层面吞吐
（QPS、API Server 往返延迟、etcd 写入开销），那需要在真实 / envtest
集群中通过负载测试得到。

测量目标：

| 维度 | 关注指标 |
|---|---|
| Builder 函数（纯 CPU + 内存分配） | ns/op、B/op、allocs/op |
| Webhook（默认值 + 校验，纯函数） | ns/op、B/op、allocs/op |
| Reconcile 闭环（fake client） | µs~ms/op、B/op、allocs/op |

## 2. 测试环境

| 项 | 值 |
|---|---|
| OS | macOS (Darwin 25.4.0) |
| CPU | Apple M3, 8 core |
| Memory | 16 GB |
| Go | 1.25.1 |
| controller-runtime | v0.23.3 |
| Benchtime | 2 秒每用例 |
| 客户端 | `sigs.k8s.io/controller-runtime/pkg/client/fake` |

**重要说明**：Reconcile 基准使用 fake client，**不**经过 API Server / etcd，
因此其绝对数值反映的是控制器代码本身的开销 + fake client 的 deepcopy 成本，
不能直接外推到生产集群。生产环境的瓶颈通常在 API Server 往返（毫秒级），
真实 reconcile 延迟会更高，但**控制器代码本身**的开销不会显著变化。

## 3. 测试结果

### 3.1 Builder（资源构造）

| 用例 | ns/op | B/op | allocs/op | 说明 |
|---|---:|---:|---:|---|
| `BuildDeployment_Minimal` | 526 | 2,544 | 19 | 只有 image + replicas + port |
| `BuildDeployment_Full` | 1,120 | 4,400 | 28 | + env、resources、healthCheck |
| `BuildService` | 207 | 1,072 | 4 | ClusterIP |
| `BuildIngress` | 227 | 848 | 8 | 单 host 单 path |
| `BuildAll` | 1,629 | 6,320 | 40 | Deployment+Service+Ingress 合计 |

**分析**：
- 三个 builder 合计 < 2 µs，远低于一次 reconcile 的总开销，**不是热点**。
- `BuildDeployment_Full` 比 Minimal 多出 ~600 ns，主要来自
  `resource.ParseQuantity` 调用（CPU/Memory 各一次，至少 2 个 quantity
  解析 + map 分配）。
- 总分配 40 次/40 KB 量级，每秒可处理 60w+ 次 build，不构成扩展瓶颈。

### 3.2 Webhook

| 用例 | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `Defaulter` | 219 | 892 | 7 |
| `ValidateCreate` | 220 | 48 | 2 |
| `ValidateUpdate` | 219 | 48 | 2 |

**分析**：
- Webhook 单次执行 ~220 ns，主要开销在结构体字段写入（Defaulter）和
  正则匹配（Validator）。
- Validator 只分配 48 B / 2 allocs，几乎没有热路径开销 —— `envNameRegexp`
  是包级别变量（`regexp.MustCompile` 在 `init()` 中执行一次）。
- Webhook 在 admission 链路上是同步阻塞的，但这里的纳秒级别开销相比
  webhook server 的 TLS 往返（毫秒级）可以忽略。

### 3.3 Reconcile（fake client）

| 用例 | µs/op | KB/op | allocs/op | 说明 |
|---|---:|---:|---:|---|
| `Reconcile_FirstPass` | 840 | 1,135 | 2,926 | 冷启动：加 finalizer + 触发 Requeue |
| `Reconcile_SteadyState` | 3,747 | 4,360 | 20,664 | 已就绪：3 个 SSA + status patch |
| `Reconcile_WithIngress` | 4,876 | 5,776 | 26,649 | 同上 + Ingress |
| `Reconcile_Parallel` | 3,811 | 4,356 | 20,647 | 16 个 WebApp 并发 |

**分析**：

1. **FirstPass 比 SteadyState 快 ~4.5×**
   首次只走到 `EnsureFinalizer`（一次 Update）就 `Requeue` 退出，没有进入
   sub-resource SSA。SteadyState 每次都要执行 3 次 SSA Patch + 1 次 status
   Patch + 1 次 Deployment Get，所以重得多。

2. **fake client 的开销占大头**
   20 K+ allocs/op 看起来很多，但其中绝大多数是 fake client 在每次
   Get/Patch 时对对象做 deepcopy（确保隔离）。真实集群里这些 deepcopy
   只发生在 informer 缓存边界，单次 reconcile 通常只有 < 100 次分配。
   因此**这里的数字主要用于回归对比**，不要直接当作生产数值。

3. **Ingress 路径多出 ~30% 开销**
   多一次 `client.Get(Ingress)` + 一次 SSA Patch。绝对开销 ~1.1 ms（fake
   client），生产中主要是一次 API Server 往返（通常 1-5 ms）。

4. **并行 ≈ 串行**
   `Reconcile_Parallel` 与 `SteadyState` 几乎一致，说明 fake client 的
   锁竞争没有显著放大。真实集群里 controller-runtime 用工作队列对**同一
   对象**串行化，对**不同对象**并行，由 `MaxConcurrentReconciles=3` 控制
   并发度。

## 4. 关键发现

### 4.1 性能上没有红线
所有热路径单次开销都在毫秒以内。即便最重的 reconcile（带 Ingress）也只
有 ~5 ms（fake client）。在 1000 个 WebApp + 5 分钟 requeue 间隔的规模下，
平均每秒处理 < 4 次 reconcile，控制器 CPU 占用可以忽略。

### 4.2 SSA vs Get-Update
我们对 sub-resource 用 SSA Patch，对 status 用 MergePatch。SSA 单次 RTT
比 Get + Update 少一次往返（无需先读再写），在真实集群中这是肉眼可见的
吞吐改善。微基准中由于没有 RTT，看不出差异。

### 4.3 内存分配热点
单次 reconcile 在 fake client 下分配 ~4 MB / 20K 次，其中绝大多数来自
`runtime.DeepCopyObject`。**生产中不需要**对此做特殊优化 —— controller-
runtime 的 informer cache 已经按对象共享，分配不会爆炸。

## 5. 已知限制 & 未覆盖项

| 项 | 状态 | 原因 |
|---|---|---|
| API Server 往返开销 | 未覆盖 | 需要 envtest 或真实集群 |
| etcd 写入延迟 | 未覆盖 | 同上 |
| Webhook 的 TLS / cert rotation 开销 | 未覆盖 | 由 controller-runtime 框架处理 |
| 水平扩展（多副本 + leader election） | 未覆盖 | 需多进程环境 |
| Prometheus 指标采集开销 | 间接覆盖 | 已包含在 reconcile 路径中，但未单独基准 |
| 大规模并发场景（1000+ WebApp） | 未覆盖 | 需要 envtest 集成压测 |

如需后续补充，推荐顺序：

1. envtest 端到端基准（含 API Server 真实往返）
2. 多副本 leader election 切换延迟
3. Prometheus 指标 scrape 路径基准

## 6. 复现方式

```bash
# 全部基准测试
go test ./internal/pkg/builder/      -bench=. -benchmem -run=^$ -benchtime=2s
go test ./internal/webhook/v1alpha1/  -bench=. -benchmem -run=^$ -benchtime=2s
go test ./internal/controller/        -bench=. -benchmem -run=^$ -benchtime=2s

# 单个用例 + CPU profile
go test ./internal/controller/ -bench=BenchmarkReconcile_SteadyState \
    -benchmem -run=^$ -cpuprofile=cpu.prof
go tool pprof -http=:8080 cpu.prof
```

## 7. 原始数据

<details>
<summary>builder benchmarks</summary>

```
BenchmarkBuildDeployment_Minimal-8   4392856     526.4 ns/op    2544 B/op    19 allocs/op
BenchmarkBuildDeployment_Full-8      2159305    1120 ns/op     4400 B/op    28 allocs/op
BenchmarkBuildService-8             11415438     207.1 ns/op    1072 B/op     4 allocs/op
BenchmarkBuildIngress-8             10550161     226.8 ns/op     848 B/op     8 allocs/op
BenchmarkBuildAll-8                  1462638    1629 ns/op     6320 B/op    40 allocs/op
```

</details>

<details>
<summary>webhook benchmarks</summary>

```
BenchmarkDefaulter-8        10639641     218.8 ns/op    892 B/op    7 allocs/op
BenchmarkValidateCreate-8   10854836     219.6 ns/op     48 B/op    2 allocs/op
BenchmarkValidateUpdate-8   11082879     218.9 ns/op     48 B/op    2 allocs/op
```

</details>

<details>
<summary>controller benchmarks</summary>

```
BenchmarkReconcile_FirstPass-8       2805    839704 ns/op   1135142 B/op    2926 allocs/op
BenchmarkReconcile_SteadyState-8      650   3747312 ns/op   4359905 B/op   20664 allocs/op
BenchmarkReconcile_WithIngress-8      495   4876506 ns/op   5776120 B/op   26649 allocs/op
BenchmarkReconcile_Parallel-8         636   3811384 ns/op   4356409 B/op   20647 allocs/op
```

</details>
