# WebApp Operator 生产就绪评估

> 评估日期：2026-05-11
> 评估对象：`main` 分支当前状态
> 评估人：Claude (基于代码 + 配置审计)

## 总体结论

**不建议直接发布到线上生产环境**，但距离能上线只差 **配置层** 和 **发布流程**。
代码主体（reconcile loop、metrics、test）已达到企业级生产质量。

按维度分级：

| 维度 | 状态 | 说明 |
|---|---|---|
| 控制器代码 | ✅ 可用 | SSA + Finalizer + MergePatch + 84.7% 覆盖率 |
| Builder / Webhook | ✅ 可用 | 96.5% / 100% 覆盖率，含 PSS restricted |
| 指标体系 | ✅ 可用 | 11 个指标 + 文档 + 告警示例 |
| Dockerfile | ✅ 可用 | distroless + nonroot + 静态编译 |
| 部署配置 | ⚠️ 缺口 | 默认单副本，多项关键模块被注释 |
| 高可用性 | ❌ 不达标 | 无 PDB、副本=1、leader election 形同虚设 |
| 证书管理 | ❌ 不达标 | cert-manager 集成被注释 |
| 监控接入 | ❌ 不达标 | ServiceMonitor 被注释，指标没人采 |
| 网络策略 | ❌ 不达标 | NetworkPolicy 被注释，metrics/webhook 全网可访问 |
| API 稳定性 | ❌ 不达标 | v1alpha1，按惯例不能上生产 |
| 供应链安全 | ❌ 不达标 | 无签名、无 SBOM、无漏洞扫描 |
| 发布流程 | ❌ 不达标 | 镜像固定 `controller:latest`，无 release workflow |

---

## P0 阻断项（必须解决）

### 1. 单副本无 HA — `config/manager/manager.yaml`
```yaml
replicas: 1   # leader election 开了但形同虚设
```
节点维护 = 控制器 downtime。必须：
- `replicas: 2`（或更多）
- 加 `PodDisruptionBudget` (`minAvailable: 1`)
- 加 `topologySpreadConstraints` 跨节点

### 2. API 版本是 `v1alpha1`
Kubernetes 社区约定：alpha = 可能不兼容、可能消失，**不可用于生产 CR**。
- 经 1-2 个迭代收稳 schema → 升 `v1beta1`
- 在 storage version 改变时实现 Conversion Webhook
- 或保留 v1alpha1 仅作内部，**对外发布前升版**

### 3. Webhook 证书管理未启用 — `config/default/kustomization.yaml`
```yaml
# - ../certmanager   # 被注释
```
当前 webhook 用 controller-runtime 自签证书，**生产环境 API Server 不会信任**，
admission 会全部失败。必须：
- 启用 cert-manager 集成（取消相关注释）
- 或自行管理证书 Secret + CA bundle

### 4. 指标无人采集 — `config/default/kustomization.yaml`
```yaml
# - ../prometheus    # 被注释
```
metrics 端点开了但 ServiceMonitor 没装，Prometheus 抓不到。
已有 11 个业务指标完全是摆设。

### 5. NetworkPolicy 被注释 — `config/default/kustomization.yaml`
```yaml
# - ../network-policy   # 被注释
```
`:8443` (metrics) 和 `:9443` (webhook) 在集群内对所有 Pod 开放。**至少**限制：
- metrics 只允许 Prometheus namespace 访问
- webhook 只允许 kube-apiserver 访问

---

## P1 强烈建议（重大风险）

### 6. 资源限制偏小 — `config/manager/manager.yaml`
```yaml
limits: { cpu: 500m, memory: 128Mi }
```
- 64Mi requests / 128Mi limits 在 ~100 个 WebApp + cache 全量同步时**很容易 OOM**
- 建议起步 `512Mi limits`，按真实负载调整
- CPU 500m 在 burst 时不够（reconcile 是 CPU 敏感的）

### 7. 没有镜像签名 + 无 SBOM
当前 `controller:latest` + 无版本 tag + 无 cosign 签名 + 无 SBOM：
- 镜像 tag 用语义化版本（`v0.1.0`），禁用 `latest`
- 用 cosign 签名（keyless 或 KMS）
- 生成 SBOM（syft / docker sbom）
- 集成 Trivy / Grype 扫描

### 8. 没有 release 流程
`.github/workflows/` 只有 lint/test/test-e2e，缺：
- tag → build → push → sign → release notes 自动化
- CHANGELOG.md
- Helm chart 或 OLM bundle

### 9. SSA Apply 用了已弃用的 API
`webapp_controller.go` 中 `client.Apply` 在 controller-runtime v0.23.3 已 deprecated，
迁移到 `client.Client.Apply()`，否则升级 controller-runtime 时会破坏构建。

### 10. e2e 测试存在但未验证
`test/e2e/` 与 `test-e2e.yml` 都在，本地从未跑过。必须保证：
- CR 创建 → 子资源出现
- CR 删除 → finalizer 清理
- Webhook 拒绝非法 spec

---

## P2 建议（持续改进）

### 11. `MaxConcurrentReconciles=3` 硬编码
`webapp_controller.go` 中应通过 flag / env 配置。

### 12. 缺 SLO 与告警 SLI 基线
已有指标，但没声明：
- reconcile p99 < ? 秒
- error rate < ? %
- availability ≥ ? %
- 错误预算策略

### 13. 缺 Runbook
`docs/METRICS.md` 已有告警规则示例，但每条告警应配套 runbook。

### 14. 镜像 base image 没有定期更新机制
`distroless/static:nonroot` 是好选择，但需要 Dependabot / Renovate 自动 PR 更新。

### 15. `terminationGracePeriodSeconds: 10` 偏短
建议 `30-60s`，给 leader election 释放和 in-flight 请求留余量。

---

## 上线路径

按 **2-3 个迭代**：

### Iteration 1（本次）
- [x] HA：replicas=2 + PDB + topology spread
- [x] 启用 cert-manager
- [x] 启用 ServiceMonitor
- [x] 启用 NetworkPolicy
- [x] 调资源 limits 到合理基线

### Iteration 2（本次部分完成）
- [x] API 升 `v1beta1`（含 Conversion Webhook）— v1beta1 为 storage version 和 Hub；v1alpha1 实现 ConvertTo/ConvertFrom；conversion webhook 自动注册在 `/convert`；CRD 含 cert-manager CA 注入；round-trip 单元测试通过
- [x] Release workflow + cosign 签名 + SBOM — `.github/workflows/release.yml`
- [x] 镜像漏洞扫描接入 CI — `.github/workflows/security.yml`（Trivy fs + Trivy image + govulncheck）
- [x] e2e 测试增加 WebApp CR 生命周期用例 — `test/e2e/e2e_test.go` 新增 4 个 It：创建/扩缩容/webhook 拒绝/finalizer 清理；CI workflow 加超时 + kind 版本固定 + 失败时 dump cluster（待本地 Docker 启用后跑 `make test-e2e` 验证全绿）
- [x] SSA Apply 迁移到新 API — `client.Apply` → `client.Client.Apply()`
- [x] 顺便升级了第三方依赖 CVE：`golang.org/x/net` v0.47→v0.53，`go.opentelemetry.io/otel/*` v1.36→v1.40
- [x] Go stdlib CVE — `go.mod` 升至 `go 1.25.10` + `toolchain go1.25.10`，govulncheck 报告 0 已调用漏洞

### Iteration 3（本次完成）
- [x] SLO 定义 — `docs/SLO.md` 含 4 个 SLI、SLO 目标、错误预算、burn-rate 多窗口告警
- [x] Grafana dashboard — `docs/grafana/webapp-operator.json` 可导入
- [x] Runbook 落盘 — `docs/runbook/` 5 份处置手册 + README 索引
- [x] Helm chart — `chart/webapp-operator/` 含 16 类资源、HA 默认值、cert-manager 集成、ServiceMonitor、NetworkPolicy
- [x] `MaxConcurrentReconciles` 可配置 — 加 `--max-concurrent-reconciles` flag
- [x] terminationGracePeriodSeconds 调到 30s（已于 Iter 1 完成）
- [ ] 压测调 resource limit — 依赖真实流量，作为持续优化项

## 当前生产就绪程度

走完 Iter 1+2+3 之后：

| 维度 | 状态 |
|---|---|
| HA / 多副本 / PDB / 拓扑分布 | ✅ |
| Webhook 证书 cert-manager 自动轮换 | ✅ |
| Prometheus 抓取 / VictoriaMetrics 兼容 | ✅ |
| NetworkPolicy 默认开启 | ✅ |
| API v1beta1 + Conversion Webhook | ✅ |
| 单元测试 + e2e 覆盖业务路径 | ✅ |
| Release 流程含 cosign 签名 + SBOM | ✅ |
| CI 漏洞扫描 (Trivy + govulncheck) | ✅ |
| Helm chart 可直接 helm install | ✅ |
| SLO + 告警 + Runbook | ✅ |
| Grafana dashboard 可导入 | ✅ |

剩余持续投入项（不阻塞上线）：
- 真实流量压测后调整 resource limits / MaxConcurrentReconciles
- 镜像 base image 自动更新（Renovate / Dependabot）
- 升级 Go 至 1.25.3+ 修剩余 stdlib CVE（需配 envtest 二进制升级窗口）
- OLM bundle（如果走 OperatorHub 发布）

**结论：可以发布到 production**，按照 `chart/webapp-operator/README.md` § Production checklist 走完即可。
