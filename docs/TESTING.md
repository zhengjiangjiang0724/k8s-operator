# 测试策略

本项目按**测试金字塔**组织测试，从下到上数量递减、运行速度递减、还原度递增。

## 三层结构

```
                  ┌────────────────────┐
                  │  CI E2E (kind)     │  ← 1 个核心链路用例
                  │  ~5min, 真集群     │
                  └────────────────────┘
                ┌────────────────────────┐
                │  envtest 集成测试      │  ← 业务逻辑全覆盖
                │  ~10s, mock API Server │
                └────────────────────────┘
            ┌────────────────────────────────┐
            │  单元测试 (unit + benchmark)    │  ← builder/util 函数级
            │  ~1s, 纯内存                    │
            └────────────────────────────────┘
```

## 各层用途

### 1. 单元测试 — `internal/pkg/`

| 包 | 文件 | 覆盖率 |
|---|---|---|
| `internal/pkg/builder/` | `*_test.go` | 96.5% |
| `internal/pkg/builder/` | `benchmark_test.go` | benchmark |
| `internal/webhook/v1alpha1/` | `webapp_webhook_test.go` | 100% |
| `internal/webhook/v1alpha1/` | `benchmark_test.go` | benchmark |

**目标**：函数级正确性 + 性能基线。没有 K8s API 依赖。

```bash
go test ./internal/... -race -cover
```

### 2. envtest 集成测试（"本地 mock E2E"）— `internal/controller/`

| 文件 | 用途 |
|---|---|
| `webapp_controller_test.go` | Ginkgo 集成测试，envtest 真实 API Server，验证 reconcile loop 各分支 |
| `webapp_controller_unit_test.go` | 用 `fake.Client` 测错误路径（setDegradedAndReturn 等） |
| `webapp_controller_test.go` 中 conversion 部分 | API 版本转换 round-trip |

**为什么叫"mock E2E"**：envtest 启动一个**真实**的 kube-apiserver 和 etcd 二进制，
但不启动其它控制器、节点、kubelet。覆盖范围：
- ✅ CRD schema + admission webhook
- ✅ controller-runtime client + cache
- ✅ Reconcile 函数所有分支（包括 status patch、SSA、finalizer）
- ✅ Conversion webhook 字段 round-trip
- ❌ 真实 Pod 调度 / 镜像拉取 / 网络
- ❌ cert-manager 颁发证书
- ❌ kustomize → kubectl 部署管线

**优点**：~10 秒跑完全套，确定性强，易调试。

```bash
make test
# 或单独：
go test ./internal/controller/ -v
```

### 3. CI E2E（"核心链路 E2E"）— `test/e2e/`

**只包含 2 个用例**：

1. `should run successfully` — controller pod 起来 + Ready。
2. `WebApp CRD core lifecycle` — apply CR → Deployment+Service 出现 → 删 CR → 子资源被 GC。

**为什么只留 2 个**：
- E2E 在 GitHub Actions 上跑 kind 集群 + cert-manager + 镜像构建，**每次 8-15 分钟**。
- 业务逻辑分支已经在 envtest 全覆盖了，再在 E2E 重复检查只会让 CI 变慢、变脆弱。
- E2E 真正不能被 envtest 替代的价值是**真实部署管线**：kustomize 渲染、镜像运行时、
  cert-manager 颁证、admission webhook 真实 TLS 握手 — 这些只需一个用例验证一次即可。

**跳过的本地 E2E 用例**（标记为 `PIt`）：
- `should provisioned cert-manager` — 已在 BeforeAll 间接验证
- `should have CA injection for mutating webhooks` — 已在 BeforeAll 间接验证
- `should have CA injection for validating webhooks` — 同上
- `should ensure the metrics endpoint is serving metrics` — 需要复杂的 curl pod 编排，不稳定

需要时本地运行：把 `PIt` 改回 `It` 后 `make test-e2e`。

## 运行命令

| 场景 | 命令 |
|---|---|
| 单元 + envtest 全部 | `make test` |
| CI 跑的 E2E | `make test-e2e`（需 Docker + kind） |
| 加上未覆盖的本地 E2E | 改 `PIt` 为 `It`，再 `make test-e2e` |
| benchmark | `go test ./... -bench=. -benchmem -run=^$ -benchtime=2s` |

## 写新测试时的判断标准

提交新功能前，按以下顺序选择测试位置：

1. **能用纯函数测吗？** → unit test (`*_test.go`)
2. **需要 K8s API 但不需要真节点？** → envtest (`internal/controller/`)
3. **必须验证真实部署/网络/cert 流程？** → CI E2E (`test/e2e/`)，且**仅在核心闭环用例中**

写到 CI E2E 的成本（CI 时间 + 维护负担）是 envtest 的 50 倍，不要轻易放进去。

## 当前覆盖快照

| 模块 | 覆盖率 |
|---|---|
| `internal/controller/` | 84.7% |
| `internal/pkg/builder/` | 96.5% |
| `internal/webhook/v1alpha1/` | 100% |
| `api/v1alpha1/` (conversion) | 100%（round-trip 3 cases）|

剩余未覆盖部分主要是 `SetupWithManager`（需要真 manager）和一些极端错误路径。
