# k8s-operator

面向 Go 平台开发岗位面试的 Kubernetes Operator 实战项目

# K8s Operator 项目技术设计文档

> **项目定位**：面向 Go 平台开发岗位面试的 Kubernetes Operator 实战项目
> **技术栈**：Go 1.25+ / Kubernetes 1.35+ / Kubebuilder v4 / controller-runtime v0.23
> **文档版本**：v1.1 | **日期**：2026-05

---

## 1. 项目概述

### 1.1 要解决的问题

在现代微服务架构中，部署一个完整的应用（如 Web 应用 + Redis 缓存 + 定时任务）通常需要编写多个独立的 K8s 资源清单（Deployment、Service、ConfigMap、CronJob 等），存在以下痛点：

- **碎片化管理**：一个应用涉及 5-10 个 K8s 资源，开发者需要逐个维护
- **状态不可见**：应用部署后，没有统一的健康状态视图
- **手动编排**：资源之间的依赖和启动顺序需要手动保证
- **缺乏业务语义**：Deployment 等是基础设施概念，不具备应用级抽象

### 1.2 解决方案

开发一个自定义 Kubernetes Operator，引入 `WebApp` CRD（Custom Resource Definition），实现**声明式应用编排**：

```
用户只需提交一个 WebApp YAML
       ↓
Operator 自动创建 Deployment、Service、ConfigMap、Ingress
       ↓
持续 Reconcile 保证期望状态 = 实际状态
```

### 1.3 业务价值

| 维度       | Before           | After                           |
| ---------- | ---------------- | ------------------------------- |
| 部署清单数 | 5-10 个分散文件  | 1 个 CRD YAML                   |
| 状态查看   | 逐个 kubectl get | `kubectl get webapp` 一行可见   |
| 更新流程   | 手动改多个文件   | 改 spec 一处，Operator 自动编排 |
| 故障恢复   | 手动排查重建     | Operator 自动检测并修复         |

### 1.4 为什么面试官看重

1. **K8s 深度**：理解 controller 模式、Reconcile 循环、Finalizer、Webhook 等核心机制
2. **Go 工程能力**：接口设计、错误处理、并发控制、client-go 使用
3. **系统思维**：CRD 设计、状态机、幂等性、优雅降级
4. **生产意识**：监控告警、Leader Election、Graceful Shutdown

---

## 2. 技术架构

### 2.1 架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Kubernetes API Server                        │
│                                                                     │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│   │   WebApp CR  │  │ Deployment   │  │   Service    │              │
│   │     (CRD)    │  │              │  │              │              │
│   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘              │
│          │                  │                  │                     │
│          └──────────────────┼──────────────────┘                     │
│                             │ (etcd 持久化)                          │
└─────────────────────────────┼───────────────────────────────────────┘
                              │  Informer / SharedIndexInformer
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Operator Manager (Pod)                          │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────┐      │
│  │                    Cache Layer                            │      │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐            │      │
│  │  │WebApp Index│ │Deploy Index│ │Svc Index   │            │      │
│  │  └─────┬──────┘ └─────┬──────┘ └─────┬──────┘            │      │
│  │        │               │              │                   │      │
│  └────────┼───────────────┼──────────────┼───────────────────┘      │
│           │               │              │                          │
│           ▼               ▼              ▼                          │
│  ┌───────────────────────────────────────────────┐                 │
│  │          Event Queue (workqueue)              │                 │
│  │  [WebApp/namespace/name] → Reconcile Loop     │                 │
│  └─────────────────────┬─────────────────────────┘                 │
│                        │                                            │
│                        ▼                                            │
│  ┌───────────────────────────────────────────────┐                 │
│  │              WebAppReconciler                 │                 │
│  │                                               │                 │
│  │  1. Get WebApp CR                             │                 │
│  │  2. Build owned resources (Deploy/Svc/CM)     │                 │
│  │  3. CreateOrUpdate each resource              │                 │
│  │  4. Update WebApp.Status                      │                 │
│  │  5. Handle Finalizer on deletion              │                 │
│  └─────────────────────┬─────────────────────────┘                 │
│                        │                                            │
└────────────────────────┼───────────────────────────────────────────┘
                         ▼
              K8s API Server (Create/Update/Patch)
```

### 2.2 核心组件说明

| 组件                   | 说明                                      | 关键技术                     |
| ---------------------- | ----------------------------------------- | ---------------------------- |
| **CRD (WebApp)**       | 自定义资源定义，定义应用的业务模型        | OpenAPI v3 Schema            |
| **Reconciler**         | 核心控制器，驱动期望状态→实际状态         | controller-runtime Reconcile |
| **Manager**            | 管理 Controller 生命周期、Leader Election | ctrl.Manager                 |
| **Cache/Informer**     | 本地缓存，减少对 API Server 的直接调用    | SharedIndexInformer          |
| **Workqueue**          | 事件队列，解耦事件产生与处理              | RateLimitingInterface        |
| **Webhook Server**     | Mutating + Validating 准入控制            | admission.Webhook            |
| **Status Subresource** | 独立的 status 更新，避免与 spec 冲突      | subresource: status          |

---

## 3. 技术选型对比

### 3.1 方案对比

| 维度               | Kubebuilder ⭐推荐                   | Operator SDK           | client-go 手写       |
| ------------------ | ----------------------------------- | ---------------------- | -------------------- |
| **脚手架生成**     | ✅ 完善 (makefile/CRD/RBAC)          | ✅ 完善                 | ❌ 手动搭建           |
| **CRD 生成**       | ✅ controller-gen 自动从 Go 结构生成 | ✅ 支持                 | ❌ 手动写 YAML        |
| **Reconcile 抽象** | ✅ 高级 (Builder pattern)            | ✅ 类似                 | ❌ 需自实现循环       |
| **学习曲线**       | 中等                                | 中等                   | 陡峭                 |
| **社区活跃**       | ⭐⭐⭐⭐⭐ (k8s-sigs 官方)               | ⭐⭐⭐⭐ (Red Hat)         | ⭐⭐⭐⭐⭐ (底层库)       |
| **Webhook 支持**   | ✅ 一行注册                          | ✅ 支持                 | ❌ 自实现 HTTP Server |
| **灵活性**         | 高                                  | 高                     | 最高                 |
| **适合场景**       | 新项目、标准 Operator               | Ansible/Helm/Go 多语言 | 极简场景、学习底层   |

### 3.2 推荐理由

```
Kubebuilder = controller-runtime + controller-gen + kustomize
```

1. **controller-gen**：Go 结构体 → CRD YAML 自动生成，包含 OpenAPI v3 validation
2. **controller-runtime**：封装 Informer、Workqueue、Reconcile 循环，开发者只需关注业务逻辑
3. **kustomize 集成**：开箱即用的 RBAC、部署清单生成
4. **面试加分**：Kubebuilder 是当前最主流的 Operator 开发框架，阿里、字节等大厂广泛使用

### 3.3 项目最终选型

```
┌──────────────────────────────────────────────┐
│                  技术栈                      │
├──────────────────────────────────────────────┤
│  框架:      Kubebuilder v4 (controller-gen)  │
│  运行时:    controller-runtime v0.23         │
│  客户端:    client-go (通过 controller-runtime│
│             封装，必要时直接使用)             │
│  Go 版本:   1.25+                            │
│  K8s 版本:  1.35+                            │
└──────────────────────────────────────────────┘
```

---

## 4. 核心数据模型

### 4.1 CRD YAML (完整)

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: webapps.myapp.example.com
  annotations:
    controller-gen.kubebuilder.io/version: v0.16.5
spec:
  group: myapp.example.com
  scope: Namespaced
  names:
    kind: WebApp
    listKind: WebAppList
    plural: webapps
    singular: webapp
    shortNames:
      - wa
    categories:
      - myapp
  versions:
    - name: v1alpha1
      served: true
      storage: true
      # Subresource: status 独立更新
      subresources:
        status: {}
        # scale:
        #   specReplicasPath: .spec.replicas
        #   statusReplicasPath: .status.replicas
      # Additional Printer Columns: kubectl get 显示
      additionalPrinterColumns:
        - name: Phase
          type: string
          description: Current phase
          jsonPath: .status.phase
        - name: Replicas
          type: integer
          description: Ready replicas
          jsonPath: .status.readyReplicas
        - name: Service
          type: string
          description: Service name
          jsonPath: .status.serviceName
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
      schema:
        openAPIV3Schema:
          type: object
          required:
            - spec
          properties:
            spec:
              type: object
              required:
                - image
              properties:
                image:
                  type: string
                  description: Container image for the web application
                  minLength: 1
                replicas:
                  type: integer
                  minimum: 0
                  maximum: 100
                  default: 1
                  description: Number of replicas
                port:
                  type: integer
                  minimum: 1
                  maximum: 65535
                  default: 8080
                  description: Container port
                resources:
                  type: object
                  description: Resource requirements
                  properties:
                    requests:
                      type: object
                      properties:
                        cpu:
                          type: string
                          pattern: "^[0-9]+(\\.[0-9]+)?(m|)$"
                        memory:
                          type: string
                          pattern: "^[0-9]+(\\.[0-9]+)?(Ki|Mi|Gi|Ti|Pi|Ei|k|M|G|T|P|E)$"
                    limits:
                      type: object
                      properties:
                        cpu:
                          type: string
                          pattern: "^[0-9]+(\\.[0-9]+)?(m|)$"
                        memory:
                          type: string
                          pattern: "^[0-9]+(\\.[0-9]+)?(Ki|Mi|Gi|Ti|Pi|Ei|k|M|G|T|P|E)$"
                env:
                  type: array
                  description: Environment variables
                  items:
                    type: object
                    required:
                      - name
                    properties:
                      name:
                        type: string
                        pattern: "^[A-Za-z_][A-Za-z0-9_]*$"
                      value:
                        type: string
                serviceType:
                  type: string
                  enum:
                    - ClusterIP
                    - NodePort
                    - LoadBalancer
                  default: ClusterIP
                  description: Kubernetes Service type
                enableIngress:
                  type: boolean
                  default: false
                  description: Whether to create an Ingress resource
                ingressHost:
                  type: string
                  pattern: "^[a-z0-9]([a-z0-9\\-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9\\-]*[a-z0-9])?)*$"
                  description: Ingress hostname
                healthCheck:
                  type: object
                  description: Health check configuration
                  properties:
                    path:
                      type: string
                      default: /healthz
                    initialDelaySeconds:
                      type: integer
                      minimum: 0
                      default: 10
                    periodSeconds:
                      type: integer
                      minimum: 1
                      default: 10
                updateStrategy:
                  type: string
                  enum:
                    - RollingUpdate
                    - Recreate
                  default: RollingUpdate
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum:
                    - Pending
                    - Creating
                    - Running
                    - Updating
                    - Failed
                    - Deleting
                  description: Current lifecycle phase
                readyReplicas:
                  type: integer
                  minimum: 0
                  description: Number of ready replicas
                desiredReplicas:
                  type: integer
                  minimum: 0
                  description: Desired number of replicas
                serviceName:
                  type: string
                  description: Created Service name
                ingressUrl:
                  type: string
                  description: Ingress URL (if enabled)
                observedGeneration:
                  type: integer
                  format: int64
                  description: Last observed generation
                conditions:
                  type: array
                  description: Current conditions
                  items:
                    type: object
                    required:
                      - type
                      - status
                      - lastTransitionTime
                    properties:
                      type:
                        type: string
                        description: Condition type
                      status:
                        type: string
                        enum: ["True", "False", "Unknown"]
                      reason:
                        type: string
                      message:
                        type: string
                      lastTransitionTime:
                        type: string
                        format: date-time
```

### 4.2 用户使用的 CR 示例

```yaml
apiVersion: myapp.example.com/v1alpha1
kind: WebApp
metadata:
  name: my-webapp
  namespace: default
spec:
  image: nginx:1.25
  replicas: 3
  port: 80
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
  env:
    - name: APP_ENV
      value: production
    - name: LOG_LEVEL
      value: info
  serviceType: ClusterIP
  enableIngress: true
  ingressHost: myapp.example.com
  healthCheck:
    path: /healthz
    initialDelaySeconds: 5
    periodSeconds: 10
  updateStrategy: RollingUpdate
```

---

## 5. Go 代码结构

### 5.1 完整项目目录树

```
webapp-operator/
├── Makefile                    # 构建、测试、部署入口
├── Dockerfile                  # 容器镜像构建
├── PROJECT                     # Kubebuilder 项目元信息
├── go.mod
├── go.sum
│
├── cmd/
│   └── main.go                 # 入口：Manager 初始化、Controller/Webhook 注册
│
├── api/
│   └── v1alpha1/
│       ├── webapp_types.go     # CRD Go 结构体定义 (Spec / Status / Phase)
│       ├── groupversion_info.go # SchemeBuilder / GroupVersion
│       └── zz_generated.deepcopy.go  # controller-gen 自动生成
│
├── internal/
│   │   # ---------- Controllers ----------
│   ├── controller/
│   │   ├── webapp_controller.go      # WebApp Reconciler 主逻辑
│   │   ├── webapp_controller_test.go # 控制器单元测试 (envtest)
│   │   └── suite_test.go             # 测试环境 setup
│   │
│   │   # ---------- Webhooks ----------
│   ├── webhook/
│   │   └── v1alpha1/
│   │       ├── webapp_webhook.go      # Mutating + Validating Webhook
│   │       ├── webapp_webhook_test.go # Webhook 测试
│   │       └── webhook_suite_test.go  # Webhook 测试环境 setup
│   │
│   │   # ---------- 内部业务逻辑 ----------
│   └── pkg/
│       ├── builder/
│       │   ├── deployment.go     # Deployment 构建器
│       │   ├── service.go        # Service 构建器
│       │   └── ingress.go        # Ingress 构建器
│       ├── condition/
│       │   └── condition.go      # Status Condition 管理
│       ├── finalizer/
│       │   └── finalizer.go      # Finalizer 工具函数
│       └── k8sutil/
│           └── owner.go          # OwnerReference / CommonLabels 工具
│
├── config/
│   ├── certmanager/             # cert-manager 证书配置
│   ├── crd/
│   │   └── bases/               # controller-gen 生成的 CRD YAML
│   │       └── myapp.example.com_webapps.yaml
│   ├── default/                 # kustomize 默认配置
│   ├── manager/
│   │   └── manager.yaml         # Manager Deployment
│   ├── network-policy/          # 网络策略
│   ├── rbac/                    # RBAC 权限配置
│   ├── samples/
│   │   └── myapp_v1alpha1_webapp.yaml  # 示例 CR
│   ├── webhook/                 # Webhook 服务配置
│   └── prometheus/              # 监控配置
│
├── test/
│   ├── e2e/                     # 端到端测试
│   └── utils/
│
└── hack/
    └── boilerplate.go.txt       # 自动生成文件的头注释
```

---

## 6. 核心接口定义

### 6.1 Reconciler 接口

```go
// controller-runtime 定义的标准 Reconciler 接口
type Reconciler interface {
    // Reconcile 执行一次协调循环
    // req 包含触发此次 reconcile 的 object 的 namespace + name
    // 返回 ctrl.Result 控制下次 reconcile 的时间和是否需要 requeue
    // 返回 error 会在指数退避后自动重新入队
    Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
}
```

### 6.2 WebAppReconciler 结构体

```go
// internal/controller/webapp_controller.go

type WebAppReconciler struct {
    client.Client                        // K8s 客户端（已封装 cache）
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder        // K8s 事件记录器
}
```

### 6.3 关键函数签名

```go
// === Reconcile 主循环 ===
func (r *WebAppReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error)

// === 子资源协调 ===
func (r *WebAppReconciler) reconcileDeployment(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) error

func (r *WebAppReconciler) reconcileService(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) error

func (r *WebAppReconciler) reconcileIngress(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) error

// === 状态更新 ===
func (r *WebAppReconciler) updateStatus(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
    patchFn func(*myappv1alpha1.WebAppStatus),
) error

// === 删除处理 ===
func (r *WebAppReconciler) handleDeletion(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) (ctrl.Result, error)

// === Setup ===
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error
```

### 6.4 Builder 函数

```go
// internal/pkg/builder/ — 纯函数式构建器，无状态

func BuildDeployment(webapp *myappv1alpha1.WebApp) *appsv1.Deployment
func BuildService(webapp *myappv1alpha1.WebApp) *corev1.Service
func BuildIngress(webapp *myappv1alpha1.WebApp) *networkingv1.Ingress  // 未启用时返回 nil
```

### 6.5 Webhook 接口

```go
// Mutating webhook: 实现 webhook.CustomDefaulter (typed)
func (d *WebAppCustomDefaulter) Default(ctx context.Context, obj *myappv1alpha1.WebApp) error

// Validating webhook: 实现 webhook.CustomValidator (typed)
func (v *WebAppCustomValidator) ValidateCreate(ctx context.Context, obj *myappv1alpha1.WebApp) (admission.Warnings, error)
func (v *WebAppCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *myappv1alpha1.WebApp) (admission.Warnings, error)
func (v *WebAppCustomValidator) ValidateDelete(ctx context.Context, obj *myappv1alpha1.WebApp) (admission.Warnings, error)
```

---

## 7. 关键实现细节

### 7.1 Informer / List-Watch 机制

```
┌─────────────────────────────────────────────────────────────────┐
│                     K8s API Server (etcd)                        │
│                                                                  │
│   客户端通过 HTTP 长连接 watch 指定资源的变更                     │
│   ?watch=true&resourceVersion=12345                               │
└──────────────┬──────────────────────────────────────────────────┘
               │ HTTP Chunked Transfer (长连接)
               │
               ▼
┌─────────────────────────────────────────────────────────────────┐
│                     SharedIndexInformer                          │
│                                                                  │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │  Reflector                                                │  │
│   │  1. 初始 List: 全量获取资源                                │  │
│   │  2. 持续 Watch: 增量接收 ADDED/MODIFIED/DELETED 事件      │  │
│   │  3. 断线重连: 使用 resourceVersion 避免数据丢失            │  │
│   └────────────────────────┬─────────────────────────────────┘  │
│                            │                                     │
│                            ▼                                     │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │  DeltaFIFO Queue                                          │  │
│   │  去重合并同一 object 的连续事件                            │  │
│   │  (避免对同一 key 的反复处理)                               │  │
│   └────────────────────────┬─────────────────────────────────┘  │
│                            │                                     │
│                            ▼                                     │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │  Indexer (本地存储)                                        │  │
│   │  - Thread-safe map[string]Object                          │  │
│   │  - 支持按 label/namespace 索引                             │  │
│   │  - 零 GC 压力 (无 reflect)                                 │  │
│   └────────────────────────┬─────────────────────────────────┘  │
│                            │                                     │
│                            ▼                                     │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │  EventHandler → workqueue                                 │  │
│   │  OnAdd/OnUpdate/OnDelete → 入队 key                       │  │
│   └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

**关键面试点**：

1. **为什么用 Informer 而不是直接 list/get？**

   - 减少 API Server 压力（本地缓存）
   - 避免惊群效应（多个 Controller 共享一个 Informer）
   - 自动处理断线重连（resourceVersion 机制）

2. **controller-runtime 的缓存是** `TypedInformer` **，按 GVK 区分**

3. **Watch 断线重连机制**：

   ```
   Watch 连接断开
   → 使用上次收到的 resourceVersion 重新 watch
   → 如果 RV 过期（etcd 压缩），退化为全量 List
   → 重新建立 watch
   ```

### 7.2 Reconcile 循环幂等性设计

**核心原则：无论执行多少次，结果应该一致。**

```go
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. 获取 WebApp CR
    webapp := &myappv1alpha1.WebApp{}
    if err := r.Get(ctx, req.NamespacedName, webapp); err != nil {
        if apierrors.IsNotFound(err) {
            // CR 已被删除 → 无需处理（幂等）
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, fmt.Errorf("failed to get WebApp: %w", err)
    }

    // 2. 检查 DeletionTimestamp → Finalizer 处理
    if !webapp.DeletionTimestamp.IsZero() {
        return r.handleFinalizer(ctx, webapp)
    }

    // 3. 确保 Finalizer 存在（多次执行无副作用）
    if !controllerutil.ContainsFinalizer(webapp, finalizerName) {
        controllerutil.AddFinalizer(webapp, finalizerName)
        if err := r.Update(ctx, webapp); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 4. 协调子资源（幂等：CreateOrUpdate）
    if err := r.reconcileDeployment(ctx, webapp); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to reconcile Deployment: %w", err)
    }
    if err := r.reconcileService(ctx, webapp); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to reconcile Service: %w", err)
    }

    // 5. 更新状态
    if err := r.updateStatus(ctx, webapp, func(status *myappv1alpha1.WebAppStatus) {
        status.Phase = myappv1alpha1.PhaseRunning
        status.ObservedGeneration = webapp.Generation
    }); err != nil {
        return ctrl.Result{}, err
    }

    log.Info("Reconcile completed successfully")
    return ctrl.Result{}, nil
}
```

**幂等性保证策略**：

| 操作           | 幂等实现                           |
| -------------- | ---------------------------------- |
| Create         | `CreateOrUpdate` / `CreateOrPatch` |
| Update         | Server-Side Apply (SSA)            |
| Finalizer 添加 | 先判断 ContainsFinalizer 再 Add    |
| Status 更新    | 使用 Patch 而非 Update             |

### 7.3 Finalizer 优雅删除流程

```
用户执行: kubectl delete webapp my-webapp
                │
                ▼
        API Server 设置 DeletionTimestamp
        (资源进入 Terminating 状态)
                │
                ▼
    ┌───────────────────────────────┐
    │   Reconcile 被触发              │
    │   检测到 DeletionTimestamp ≠ 0  │
    └───────────────┬───────────────┘
                    │
                    ▼
    ┌───────────────────────────────┐
    │   执行清理逻辑                  │
    │   1. 删除关联的外部资源          │
    │      (如云负载均衡、DNS 记录)    │
    │   2. 等待子资源完全删除          │
    │   3. 发送清理完成事件            │
    └───────────────┬───────────────┘
                    │
                    ▼
    ┌───────────────────────────────┐
    │   从 Finalizers 列表中移除      │
    │   k8s.io/myapp-webapp-finalizer│
    └───────────────┬───────────────┘
                    │
                    ▼
        API Server 真正删除资源
        (从 etcd 中移除)
```

```go
// internal/pkg/finalizer/finalizer.go
const WebAppFinalizer = "myapp.example.com/webapp-finalizer"

// internal/controller/webapp_controller.go
func (r *WebAppReconciler) handleDeletion(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) (ctrl.Result, error) {
    if !finalizer.HasFinalizer(webapp) {
        return ctrl.Result{}, nil
    }

    log.Info("Performing cleanup for WebApp deletion")

    // 更新 Phase 为 Deleting
    _ = r.updateStatus(ctx, webapp, func(s *myappv1alpha1.WebAppStatus) {
        s.Phase = myappv1alpha1.PhaseDeleting
    })

    // 执行清理（owned 资源通过 OwnerReference 自动级联删除，
    // 此处可扩展外部资源清理逻辑）
    r.Recorder.Event(webapp, corev1.EventTypeNormal,
        "CleanupComplete", "External resources cleaned up")

    // 移除 finalizer，允许 K8s 完成真正的删除
    if err := finalizer.RemoveFinalizer(ctx, r.Client, webapp); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
    }
    return ctrl.Result{}, nil
}
```

### 7.4 Status 更新策略：Patch vs Update

```go
// ❌ 不推荐：Update 会更新整个对象，容易与 spec 更新冲突
func (r *WebAppReconciler) updateStatusBad(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) error {
    return r.Status().Update(ctx, webapp)
}

// ✅ 推荐：Merge Patch，只更新 status 字段
func (r *WebAppReconciler) updateStatus(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
    patchFn func(status *myappv1alpha1.WebAppStatus),
) error {
    // 保存旧状态用于比较
    oldStatus := webapp.Status.DeepCopy()

    // 应用状态变更
    patchFn(&webapp.Status)

    // 如果状态没有变化，跳过 patch（减少 API 调用）
    if reflect.DeepEqual(oldStatus, &webapp.Status) {
        return nil
    }

    // 使用 Merge Patch
    patch := client.MergeFrom(webapp.DeepCopy())
    return r.Status().Patch(ctx, webapp, patch)
}

// ✅ 进阶：Server-Side Apply (SSA)
func (r *WebAppReconciler) updateStatusSSA(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) error {
    // 构造仅包含 status 的对象
    statusOnly := &myappv1alpha1.WebApp{
        ObjectMeta: metav1.ObjectMeta{
            Name:      webapp.Name,
            Namespace: webapp.Namespace,
        },
        Status: webapp.Status,
    }
    return r.Status().Patch(ctx, statusOnly,
        client.Apply,
        client.FieldOwner("webapp-operator"),
        client.ForceOwnership)
}
```

**Patch vs Update 对比**：

| 维度     | Update       | Merge Patch  | Server-Side Apply |
| -------- | ------------ | ------------ | ----------------- |
| 冲突概率 | 高           | 中           | 低                |
| 网络开销 | 高（全对象） | 低（仅变更） | 低                |
| 并发安全 | ❌ 需重试     | ✅            | ✅ (FieldManager)  |
| 推荐度   | ❌            | ✅            | ⭐⭐ 最佳           |

### 7.5 Webhook 开发

#### 7.5.1 Mutating Webhook (自动注入默认值/标签)

```go
// internal/webhook/v1alpha1/webapp_webhook.go

// WebAppCustomDefaulter — 无状态，使用 Kubebuilder v4 typed webhook 签名
type WebAppCustomDefaulter struct{}

func (d *WebAppCustomDefaulter) Default(_ context.Context, obj *myappv1alpha1.WebApp) error {
    // 注入默认标签
    if obj.Labels == nil {
        obj.Labels = make(map[string]string)
    }
    obj.Labels["app.kubernetes.io/managed-by"] = "webapp-operator"
    obj.Labels["app.kubernetes.io/instance"] = obj.Name

    // 设置默认 replicas
    if obj.Spec.Replicas == nil {
        defaultReplicas := int32(1)
        obj.Spec.Replicas = &defaultReplicas
    }

    // 设置默认 port / serviceType / updateStrategy
    if obj.Spec.Port == 0 { obj.Spec.Port = 8080 }
    if obj.Spec.ServiceType == "" { obj.Spec.ServiceType = corev1.ServiceTypeClusterIP }
    if obj.Spec.UpdateStrategy == "" { obj.Spec.UpdateStrategy = "RollingUpdate" }

    // 如果启用了 Ingress 但未设置 host，自动生成
    if obj.Spec.EnableIngress && obj.Spec.IngressHost == "" {
        obj.Spec.IngressHost = fmt.Sprintf("%s.%s.svc.cluster.local",
            obj.Name, obj.Namespace)
    }

    // 默认 healthCheck
    if obj.Spec.HealthCheck == nil {
        obj.Spec.HealthCheck = &myappv1alpha1.HealthCheck{
            Path: "/healthz", InitialDelaySeconds: 10, PeriodSeconds: 10,
        }
    }
    return nil
}
```

#### 7.5.2 Validating Webhook (参数校验)

```go
// WebAppCustomValidator — 无状态
type WebAppCustomValidator struct{}

func (v *WebAppCustomValidator) ValidateCreate(
    _ context.Context, obj *myappv1alpha1.WebApp,
) (admission.Warnings, error) {
    return nil, validateWebApp(obj)
}

func (v *WebAppCustomValidator) ValidateUpdate(
    _ context.Context, _, newObj *myappv1alpha1.WebApp,
) (admission.Warnings, error) {
    return nil, validateWebApp(newObj)
}

func (v *WebAppCustomValidator) ValidateDelete(
    _ context.Context, _ *myappv1alpha1.WebApp,
) (admission.Warnings, error) {
    return nil, nil
}

func validateWebApp(webapp *myappv1alpha1.WebApp) error {
    var allErrs field.ErrorList

    if webapp.Spec.Image == "" { /* ... Required error ... */ }
    if webapp.Spec.Replicas != nil && (*webapp.Spec.Replicas < 0 || *webapp.Spec.Replicas > 100) { /* ... */ }
    if webapp.Spec.Port < 1 || webapp.Spec.Port > 65535 { /* ... */ }
    for i, env := range webapp.Spec.Env { /* ... 正则校验 env name ... */ }
    if webapp.Spec.EnableIngress && webapp.Spec.IngressHost == "" { /* ... Required error ... */ }

    if len(allErrs) == 0 { return nil }
    return fmt.Errorf("validation failed: %v", allErrs.ToAggregate().Error())
}
```

#### 7.5.3 Webhook 注册

```go
// internal/webhook/v1alpha1/webapp_webhook.go
func SetupWebAppWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &myappv1alpha1.WebApp{}).
        WithValidator(&WebAppCustomValidator{}).
        WithDefaulter(&WebAppCustomDefaulter{}).
        Complete()
}

// cmd/main.go 中通过 ENABLE_WEBHOOKS 环境变量控制
if os.Getenv("ENABLE_WEBHOOKS") != "false" {
    if err := webhookv1alpha1.SetupWebAppWebhookWithManager(mgr); err != nil { ... }
}
```

---

## 8. 实现路线图

### Phase 1: MVP -- 已完成

**目标**：可运行的最小 Operator，实现 CRD 创建 → 子资源编排

| 任务            | 产出                                       | 状态 |
| --------------- | ------------------------------------------ | ---- |
| 项目初始化      | `kubebuilder init` 完成项目搭建            | Done |
| CRD 定义        | `webapp_types.go` + `make manifests`       | Done |
| Reconcile 骨架  | 完整 Reconcile + Get CR + 日志             | Done |
| Deployment 协调 | 从 spec 构建并 Create/Update Deployment    | Done |
| Service 协调    | 创建 ClusterIP/NodePort/LoadBalancer       | Done |
| 基本 Status     | Phase 状态机 + readyReplicas               | Done |
| 编译验证        | `make generate && make manifests && make build` 通过 | Done |

**Phase 1 验收标准**：

```bash
kubectl apply -f config/samples/myapp_v1alpha1_webapp.yaml
kubectl get webapp webapp-sample
# NAME            PHASE    REPLICAS  SERVICE  AGE
# webapp-sample   Running  3         webapp-sample  30s

kubectl get deploy,svc -l app.kubernetes.io/instance=webapp-sample
# 能看到 Deployment 和 Service
```

### Phase 2: 进阶功能 -- 已完成

| 任务               | 产出                                   | 状态 |
| ------------------ | -------------------------------------- | ---- |
| Finalizer          | 优雅删除 + 外部资源清理                | Done |
| Status Conditions  | K8s 标准 conditions (Available, Progressing, Degraded) | Done |
| Ingress 协调       | 可选创建/删除 Ingress 资源             | Done |
| OwnerReference     | 级联删除 (Controller GC)               | Done |
| Mutating Webhook   | 默认值注入 + 标签自动添加              | Done |
| Validating Webhook | spec 校验 (image/replicas/port/env/ingressHost) | Done |
| Events             | 关键操作发 K8s Events                  | Done |
| 单元测试           | envtest 控制器测试 (脚手架已生成)      | TODO |

### Phase 3: 生产级 (待实现)

| 任务               | 产出                                    | 状态        |
| ------------------ | --------------------------------------- | ----------- |
| Leader Election    | 多副本高可用部署 (框架已支持)           | 框架就绪    |
| Prometheus Metrics | 自定义指标 (reconcile_duration, errors) | TODO        |
| 结构化日志         | 日志级别、key-value 结构化 (已使用 zap) | 框架就绪    |
| RBAC 精细化        | 最小权限原则 (controller-gen 已生成)    | Done        |
| Graceful Shutdown  | 优雅退出 + context 传播 (框架内置)      | 框架就绪    |
| SSA 迁移           | Server-Side Apply 替代 Update           | TODO        |
| E2E 测试           | Kind 集群端到端测试 (脚手架已生成)      | TODO        |
| CI/CD              | GitHub Actions + golangci-lint (已生成) | 框架就绪    |

---

## 9. 面试可讲亮点

### 亮点 1: Reconcile 幂等性与声明式 API

**面试官可能问**：*"为什么 Reconcile 必须是幂等的？"*

**回答要点**：

- K8s 是**声明式 API**，用户定义期望状态（spec），Operator 负责让实际状态趋近期望
- Reconcile 可能被**反复触发**（事件、定时、手动），必须保证结果一致
- 实现方式：`CreateOrUpdate` 替代 `Create`，Patch 替代 Update，先判断再操作
- **类比**：就像 `systemd` 的 `ensure` 语义，执行 100 次和 1 次效果相同

### 亮点 2: Informer 的 List-Watch 与断线重连

**面试官可能问**：*"Informer 的 Watch 断开了怎么办？"*

**回答要点**：

- Informer 内部有**自动重连**机制
- 重连时使用上次的 `resourceVersion`，从该位置继续 watch
- 如果 RV 已过期（etcd 默认 5 分钟 compact），则退化为**全量 List** 重新同步
- **DeltaFIFO** 去重：同一 object 的多个连续事件会合并，只处理最终态
- **SharedInformer**：多个 Controller 共享一个 Informer，避免多个 Watch 连接

### 亮点 3: Finalizer 优雅删除

**面试官可能问**：*"Finalizer 是什么？和 OwnerReference 有什么区别？"*

**回答要点**：

- **Finalizer**：在 object 的 `metadata.finalizers` 列表中的字符串，阻止物理删除
- **删除流程**：`kubectl delete` → API Server 设 DeletionTimestamp → Controller 检测到 → 执行清理 → 移除 Finalizer → 物理删除
- **OwnerReference**：声明资源归属关系，Controller GC 自动级联删除
- **区别**：Finalizer 是**主动清理**（如删除云资源），OwnerReference 是**被动级联**（如删除子 Deploy）
- **坑**：Finalizer 清理逻辑也必须幂等，否则删除卡死

### 亮点 4: Status Subresource 隔离

**面试官可能问**：*"为什么 status 要作为 subresource？"*

**回答要点**：

- **防止覆盖**：用户更新 spec 和 Controller 更新 status 不会冲突
- **权限分离**：RBAC 可以只允许 Controller 更新 status
- **API 语义**：`/status` 子路径，`Update()` 和 `Patch()` 仅作用于 status 字段
- **实现**：CRD spec 中设置 `subresources.status: {}`，代码中用 `r.Status().Patch()`

### 亮点 5: Server-Side Apply (SSA)

**面试官可能问**：*"SSA 和传统的 Create/Update 有什么区别？"*

**回答要点**：

- **Create/Update**：全量替换，多个 Controller 操作同一资源时会冲突
- **SSA**：按 field 划分 ownership，多个 manager 可以管理不同字段
- **FieldManager**：每个 Controller 声明自己管理的 field set
- **ForceOwnership**：解决 conflict（需谨慎使用）
- **类比**：Git merge vs Git rebase — SSA 是智能合并

### 亮点 6: Controller 的 Requeue 策略

**面试官可能问**：*"什么时候应该 Requeue？"*

**回答要点**：

| 场景           | 策略                     | 代码                                         |
| -------------- | ------------------------ | -------------------------------------------- |
| 业务成功       | 不 Requeue               | `return ctrl.Result{}, nil`                  |
| 外部依赖未就绪 | 定时 Requeue             | `return ctrl.Result{RequeueAfter: 10s}, nil` |
| 临时错误       | 返回 error（指数退避）   | `return ctrl.Result{}, err`                  |
| 永久错误       | 不 Requeue + 更新 status | `return ctrl.Result{}, nil`                  |

### 亮点 7: 多版本 API 兼容性

**面试官可能问**：*"如果将来要支持 v1alpha2 怎么办？"*

**回答要点**：

- **转换 Webhook**：`conversion webhook` 实现 v1alpha1 ↔ v1alpha2 转换
- **存储版本**：只有一个版本设置 `storage: true`，其他版本自动转换
- **兼容性原则**：新增字段必须有 `default` 值，不能删除字段（只废弃）
- **Kubebuilder 支持**：`kubebuilder create webhook --conversion` 生成脚手架

---

## 10. 常见坑和解决方案

### 坑 1: Reconcile 无限循环

**现象**：Status 更新触发了新的 Reconcile 事件，导致无限循环

**原因**：更新 Status 时触发了 Watch 事件，事件又被 handler 处理

**解决方案**：

```go
// 方案 1: 只在状态变化时更新
func (r *WebAppReconciler) updateStatus(...) error {
    oldStatus := webapp.Status.DeepCopy()
    patchFn(&webapp.Status)
    if reflect.DeepEqual(oldStatus, &webapp.Status) {
        return nil // 跳过不必要的更新
    }
    return r.Status().Patch(ctx, webapp, patch)
}

// 方案 2: 使用 Generation 判断 spec 是否变化
if webapp.Status.ObservedGeneration == webapp.Generation {
    // spec 未变化，可以跳过子资源协调
    return ctrl.Result{}, nil
}
```

### 坑 2: Leader Election 导致单点

**现象**：Operator 部署多副本但只有一个在工作，挂了就没人处理

**解决方案**：

```go
// cmd/main.go
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:                 scheme,
    LeaderElection:         true,
    LeaderElectionID:       "webapp-operator-leader",
    LeaderElectionNamespace: "kube-system",
    // ...
})
```

- Leader 通过 Lease 对象选举，宕机后 15s 内自动切换
- **面试延伸**：Lease 对象是什么？（coordination.k8s.io/v1 资源）

### 坑 3: 缓存不一致

**现象**：刚 Create 的资源，下一次 Get 却 NotFound

**原因**：Informer 缓存更新有延迟（event 传播需要时间）

**解决方案**：

```go
// 方案 1: 创建后直接读取返回值，不从缓存 Get
deploy := &appsv1.Deployment{...}
if err := r.Create(ctx, deploy); err != nil {
    return err
}
// 直接用 deploy 对象，不从缓存重新 Get

// 方案 2: 使用 UncachedReader (绕过缓存直接读 API Server)
if err := r.APIReader.Get(ctx, key, obj); err != nil {
    return err
}

// 方案 3: 等待缓存同步
mgr.GetCache().WaitForCacheSync(ctx)
```

### 坑 4: Finalizer 导致删除卡死

**现象**：`kubectl delete` 一直卡着，资源处于 Terminating 状态

**原因**：Finalizer 清理逻辑有 bug 或外部依赖不可用

**解决方案**：

```go
// 1. 清理逻辑必须幂等 + 有超时
func (r *WebAppReconciler) cleanupExternalResources(
    ctx context.Context,
    webapp *myappv1alpha1.WebApp,
) error {
    // 带超时的 context
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    // 幂等检查：资源不存在视为清理成功
    if externalResourceNotFound(webapp) {
        return nil
    }
    return deleteExternalResource(ctx, webapp)
}

// 2. 提供强制移除 finalizer 的方法（运维应急）
// kubectl patch webapp my-webapp -p '{"metadata":{"finalizers":null}}' --type=merge
```

### 坑 5: Webhook 证书管理

**现象**：Webhook 部署后报 `x509: certificate signed by unknown authority`

**原因**：Webhook 需要 TLS 证书，APIServer 要验证证书

**解决方案**：

```yaml
# Kubebuilder 的 certmanager 方案（推荐生产使用）
# config/certmanager/certificate.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: serving-cert
spec:
  dnsNames:
    - webapp-operator-webhook-service.default.svc
    - webapp-operator-webhook-service.default.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: selfsigned-issuer
  secretName: webhook-server-cert
```

**开发阶段简化方案**：

```bash
# 使用 kubebuilder 自带的 cert 生成脚本
make generate && make manifests

# 或者用 kind 时自动注入 cert（通过 webhook 的 CABundle）
./hack/setup-webhook-certs.sh
```

---

## 附录 A: Manager 完整启动代码

```go
// cmd/main.go
package main

import (
    "crypto/tls"
    "flag"
    "os"

    // +kubebuilder:scaffold:imports
    myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
    "github.com/example/webapp-operator/internal/controller"
    webhookv1alpha1 "github.com/example/webapp-operator/internal/webhook/v1alpha1"
    "k8s.io/apimachinery/pkg/runtime"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    _ "k8s.io/client-go/plugin/pkg/client/auth"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(myappv1alpha1.AddToScheme(scheme))
    // +kubebuilder:scaffold:scheme
}

func main() {
    var metricsAddr string
    var enableLeaderElection bool
    var probeAddr string
    var secureMetrics bool
    var enableHTTP2 bool
    flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080",
        "The address the metric endpoint binds to.")
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
        "The address the probe endpoint binds to.")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false,
        "Enable leader election for controller manager.")
    flag.BoolVar(&secureMetrics, "metrics-secure", false,
        "If set the metrics endpoint is served securely")
    flag.BoolVar(&enableHTTP2, "enable-http2", false,
        "If set, HTTP/2 will be enabled for the metrics and webhook servers")
    opts := zap.Options{Development: true}
    opts.BindFlags(flag.CommandLine)
    flag.Parse()

    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

    // HTTP/2 安全警告处理
    if enableHTTP2 {
        setupLog.Info("WARNING: HTTP/2 is not recommended for production")
    }

    tlsOpts := []func(*tls.Config){}
    if !enableHTTP2 {
        tlsOpts = append(tlsOpts, func(c *tls.Config) {
            c.NextProtos = []string{"http/1.1"}
        })
    }

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme: scheme,
        Metrics: server.Options{
            BindAddress:   metricsAddr,
            SecureServing: secureMetrics,
            TLSOpts:       tlsOpts,
        },
        HealthProbeBindAddress: probeAddr,
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "webapp-operator-leader",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    // 注册 Controller
    if err := (&controller.WebAppReconciler{
        Client:   mgr.GetClient(),
        Scheme:   mgr.GetScheme(),
        Recorder: mgr.GetEventRecorderFor("webapp-operator"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "Failed to create controller", "controller", "webapp")
        os.Exit(1)
    }

    // 注册 Webhook（可通过 ENABLE_WEBHOOKS=false 禁用，便于本地开发）
    if os.Getenv("ENABLE_WEBHOOKS") != "false" {
        if err := webhookv1alpha1.SetupWebAppWebhookWithManager(mgr); err != nil {
            setupLog.Error(err, "Failed to create webhook", "webhook", "WebApp")
            os.Exit(1)
        }
    }

    // 健康检查
    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        setupLog.Error(err, "Failed to set up health check")
        os.Exit(1)
    }
    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        setupLog.Error(err, "Failed to set up ready check")
        os.Exit(1)
    }

    setupLog.Info("Starting manager")
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "Failed to run manager")
        os.Exit(1)
    }
}
```

## 附录 B: 快速上手

### 前置条件

- Go 1.25+
- Kubebuilder v4
- 一个 Kubernetes 集群（可以使用 kind 创建本地集群）

### 开发命令速查

```bash
# 生成 CRD 和代码
make manifests        # 生成 CRD YAML + RBAC
make generate         # 生成 deepcopy 代码
make build            # 编译二进制

# 安装 CRD 到集群
make install

# 本地运行（禁用 Webhook 便于开发）
ENABLE_WEBHOOKS=false make run

# 创建示例 CR
kubectl apply -f config/samples/myapp_v1alpha1_webapp.yaml

# 查看状态
kubectl get webapp
kubectl get deploy,svc -l app.kubernetes.io/instance=webapp-sample

# 部署到集群
make docker-build docker-push IMG=<registry>/webapp-operator:v0.1.0
make deploy IMG=<registry>/webapp-operator:v0.1.0

# 测试
make test             # 单元测试
make test-e2e         # 端到端测试 (需要 kind)

# 清理
make undeploy
make uninstall
```

### 项目初始化回放（已执行）

```bash
kubebuilder init --domain example.com --repo github.com/example/webapp-operator
kubebuilder create api --group myapp --version v1alpha1 --kind WebApp --resource --controller
kubebuilder create webhook --group myapp --version v1alpha1 --kind WebApp \
  --defaulting --programmatic-validation
```

---

*本文档为面试准备专用，实际生产环境还需考虑：多集群支持、多租户隔离、性能调优、升级策略等。*
