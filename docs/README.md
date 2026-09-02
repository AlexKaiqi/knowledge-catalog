# 文档地图

这里不是一组平级文章。文档按“入口 → 基础决策 → 专题决策 → 运行设计 →
验证/演进”组成有向图；机器可读的元信息、主题所有权和关系以
[`DOCUMENT_GRAPH.yaml`](DOCUMENT_GRAPH.yaml) 为准。

具体 Go 类型、字段、错误码和调用行为仍以公开代码、包 README 与 Conformance
测试为准。设计文档解释为什么这样设计，不复制一份容易漂移的协议定义。

## 1. 先解决权威冲突

同一个事实只允许一个权威位置：

| 信息 | 唯一权威 | 其它文档怎么写 |
|---|---|---|
| 公开名词 | [`TERMINOLOGY.md`](TERMINOLOGY.md) | 直接使用或链接，不另造同义词 |
| 产品原则、身份、版本、来源、读写语义 | [`KNOWLEDGE_CATALOG_DESIGN.md`](KNOWLEDGE_CATALOG_DESIGN.md) | 专题文档只细化自己的边界 |
| ⓪–③ 所有权和依赖方向 | [`LAYERS.md`](LAYERS.md) + `internal/arch` | 不在服务或 Store 文档另画一套层级 |
| 当前命令和能力 | 根 [`README.md`](../README.md) | Walkthrough 只演示，不维护能力清单 |
| 产品缺口 | [`MVP_ACCEPTANCE.md`](MVP_ACCEPTANCE.md) | 设计文档不维护阶段台账 |
| 测试与实现证据 | [`TEST_CATALOG.md`](TEST_CATALOG.md) | 专题文档只声明不变量和证据入口 |
| 具体协议形状 | Go API、公开 CLI/HTTP surface、包 README、Conformance | 设计文档不复制字段全集 |
| 演变历史 | git history | 被替代结论从 active 文档删除，不保留“新旧两套” |

这也解决几组容易误读的重叠：

- `OBSERVABILITY.md` 只拥有不可采样的知识访问证据；
  `SYSTEM_OBSERVABILITY.md` 只拥有可采样的 metric/log/trace、健康和 SLO。
- `LIVE_MATERIALIZATION.md` 拥有 Binding/Observation 语义；
  `PROJECTION_CONTROLLER.md` 只拥有如何据此维护派生投影。
- `STORE_ADAPTERS.md` 拥有权威与派生介质的角色；具体 Dolt/Gitea/OpenSearch
  机制由各 adapter README 和代码拥有。
- `SCALE_ARCHITECTURE.md` / `SCALE_BENCHMARK.md` 是演进与资格测试，不反向定义
  当前通用协议。

## 2. 应该读哪几份

### 理解整个系统

按这个顺序读五份主文档：

1. [`TERMINOLOGY.md`](TERMINOLOGY.md)：先固定公开语言。
2. [`KNOWLEDGE_CATALOG_DESIGN.md`](KNOWLEDGE_CATALOG_DESIGN.md)：理解问题、原则和核心语义。
3. [`LAYERS.md`](LAYERS.md)：理解代码所有权和禁止依赖。
4. [`COMPOSITION.md`](COMPOSITION.md)：理解 Catalog、Repository、Workspace 与 pin。
5. [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md)：理解 Server、Client、Writer 与文件网关如何装配。

这五份构成系统设计主干，其余文档不能重定义它们的结论。

### 修改某个专题

| 专题 | 拥有结论的文档 | 直接依赖 |
|---|---|---|
| Aspect 写/读/检索形态 | [`ASPECT_ACCESS.md`](ASPECT_ACCESS.md) | Layers |
| 接入/消费产品、System Schema 与目录 | [`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`](KNOWLEDGE_PRODUCT_AND_SCHEMA.md) | System Design、Composition、Aspect Access、Connectors、Service |
| 外部资源与采集 | [`CONNECTORS.md`](CONNECTORS.md) | Aspect Access、Layers |
| 权威与派生介质 | [`STORE_ADAPTERS.md`](STORE_ADAPTERS.md) | Layers |
| Binding 与动态观察 | [`LIVE_MATERIALIZATION.md`](LIVE_MATERIALIZATION.md) | Aspect Access、Connectors、Store Adapters |
| State 投影控制 | [`PROJECTION_CONTROLLER.md`](PROJECTION_CONTROLLER.md) | Materialization、Service、Store Adapters |
| 权限 | [`PERMISSIONS.md`](PERMISSIONS.md) | Composition、Connectors |
| Taihu 部署认证 | [`DEPLOY_AUTH.md`](DEPLOY_AUTH.md) | Permissions、Service |
| 出站扩展 | [`HOOKS.md`](HOOKS.md) | Layers |
| Merge 证据 | [`GATES.md`](GATES.md) | Hooks、Permissions |
| 访问证据 | [`OBSERVABILITY.md`](OBSERVABILITY.md) | Composition、Permissions |
| 诊断遥测与 SLO | [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md) | Service、Access Observability |

### 操作、验证和演进

| 目的 | 文档 |
|---|---|
| 当前能力与启动 | 根 [`README.md`](../README.md) |
| 用 CLI 走完整闭环 | [`WALKTHROUGH_v5.1.md`](WALKTHROUGH_v5.1.md) |
| 判断 MVP 是否可用 | [`MVP_ACCEPTANCE.md`](MVP_ACCEPTANCE.md) |
| 找自动化证据与缺口 | [`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md)、[`TEST_CATALOG.md`](TEST_CATALOG.md) |
| 讨论规模演进 | [`SCALE_ARCHITECTURE.md`](SCALE_ARCHITECTURE.md)、[`SCALE_BENCHMARK.md`](SCALE_BENCHMARK.md) |

数仓实体、Aspect、关系、源字段、Connector 与业务验收只在
`.data/data-warehouse/` integration suite 中维护，不回写成通用系统设计。

## 3. 依赖关系

`depends_on` 表示理解或修改左侧文档前，右侧结论必须已经成立；`refines` 只缩小
或解释既有结论；`verifies` 提供机器证据；`operationalizes` 把设计变成操作旅程；
`measures` 给演进方案设置资格门槛。完整边和元信息保存在
[`DOCUMENT_GRAPH.yaml`](DOCUMENT_GRAPH.yaml)。主干可以简化为：

```text
Terminology
  └─ System Design
      └─ Layers
          ├─ Composition ── Permissions ── Gates
          ├─ Aspect Access ── Connectors ── Materialization
          │        └─ Knowledge Product & Schema Lifecycle
          └─ Store Adapters ───────────────┘
                         └─ Service Architecture
                              ├─ Projection Controller
                              └─ System Observability

Architecture Invariants / MVP Acceptance / Test Catalog  ── verifies ──▶ 上述设计
Walkthrough                                               ── operationalizes ──▶ 主干
Scale Architecture / Benchmark                           ── evolves/measures ──▶ 运行设计
```

## 4. 维护规则

1. 新增顶层 Markdown 前，先确认没有现有 `ownerTopics`；确需新增时同时登记文档图。
2. 改变公开名称，先改 Terminology；改变跨层边界，先改 Layers 和架构守卫。
3. 专题文档只能拥有文档图中声明的主题。涉及别的主题时链接权威文档，不复制结论。
4. 设计文档不记录 P0/P1、已完成/未完成流水账；状态只进 MVP Acceptance/Test Catalog。
5. 包 README 维护具体协议和用法；设计文档维护理由、不变量和取舍。
6. 运行 `make check-docs`。检查会拒绝漏登记文档、重复主题所有权、悬空关系和循环依赖。

生成的 HTML、PNG 和 JSON 架构视图是派生展示，不进入文档权威图；它们必须能从
上述 Markdown/文档图重新生成，不能承载独有决策。
