# 文档地图

这里不是一组平级文章。文档按“入口 → 基础决策 → 专题决策 → 运行设计 →
验证/演进”组成有向图。

**节点和边的权威**是 [`graph/`](graph/) 里的 OKF 知识单元：每个 Markdown
一篇 `documents/*.okf`（`catalog-entry` Aspect），每条边一篇
`relations/*.okf`（`kind: Relation`，信封为 `schema/core/relation/v1`）。
`make check-docs` 用 `repofile.Parse` 与 `knowledge.DecodeRelation` 校验，
不维护第二份 JSON 图。

Markdown 解释**应然**（为什么、永不做、必须成立）。公开 Go API、包 README 和 Conformance 是**已选定合同的形状与证据**，必须符合设计；它们不是收窄设计的依据。实然与缺口只进 `MVP_ACCEPTANCE.md` / `TEST_CATALOG.md`。

## 0. 文件类型分工

| 类型 | 放什么 | 不放什么 |
|---|---|---|
| 根 `README.md` | 一句话意图、setup/run、宏观分层简图、conformance 入口 | Agent 提示词、字段全集 |
| `docs/*.md` | Goal / Non-Goals / 否决方案 / 不变量陈述 / 旅程 | 可复制的协议 Schema、错误码表、命令穷尽清单 |
| `docs/graph/*.okf` | 文档身份、`ownerTopics`、typed Relation | 设计散文 |
| 包 `README.md` + 公开 Go / CLI / HTTP | Address、字段、错误码、状态机、调用形状 | 产品原则复述 |
| `*_test.go` / conformance | 可证伪观察 | 设计理由 |
| `docs/ARCHITECTURE_INVARIANTS.md` | 不变量 ID → 禁止观察 → 测试名 | 实现状态台账 |
| `docs/MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` | 缺口与证据索引 | ADR |
| `docs/observability/*.yaml` | 派生告警/recording 规则 | 独有产品决策 |

一篇 Canonical 知识文件只承载一个 Address。文档图的 Relation 不得改写成 Markdown 列表充当权威。

本仓按 ai-native-project-maintenance **映射角色，不改树**：不把实现搬进 `src/`，也不把设计文搬进 `docs/specs/` / `docs/decisions/`。

| 技能角色 | 本仓位置 |
|---|---|
| 人类名片 | 根 `README.md` |
| Agent 闸门 | `AGENTS.md` |
| 执行接力棒 | 根 `TASK.md`（不是文档图节点） |
| specs（Goal / Non-Goals / 边界） | `class: foundation` 的设计 Markdown |
| decisions（选定 / 否决） | `class: decision` / `evolution` 的设计 Markdown |
| Oracle | `ARCHITECTURE_INVARIANTS.md`、`internal/arch`、conformance、`.data/data-warehouse/features` |
| 实现可写区 | 仓库根 Go 包，不是 `src/` |

`class` 为 foundation / decision / runtime / evolution 的 Markdown 必须出现下列二级标题（名称不可改，`make check-docs` 强制）：`## Goal`、`## Non-Goals`、`## 硬性约束 / Invariants`、`## 选定方案 / 被否决方案`、`## 接口契约 / 状态机`。entrypoint / validation / guide 不套这五段。

五段写应然。「尚未实现」「首版没做」「当前包叫这个」不是 Non-Goal，也不是否决。接口段指向**设计要求的缝**（公开类型名、包 README、Conformance）；参考实现路径可以注明，但不能把今天的文件名或缺口写成协议。

## 1. 先解决权威冲突

同一个事实只允许一个权威位置：

| 信息 | 唯一权威 | 其它文档怎么写 |
|---|---|---|
| 文档节点、主题所有权、文档间关系 | [`graph/`](graph/) OKF | 本文只解释怎么读图 |
| 公开名词 | [`TERMINOLOGY.md`](TERMINOLOGY.md) | 直接使用或链接，不另造同义词 |
| 产品原则、身份、版本、来源、读写语义 | [`KNOWLEDGE_CATALOG_DESIGN.md`](KNOWLEDGE_CATALOG_DESIGN.md) | 专题文档只细化自己的边界 |
| ⓪–③ 所有权和依赖方向 | [`LAYERS.md`](LAYERS.md) | `internal/arch` 只验证，不得把当前 import DAG 写成新分层 |
| 当前命令和已交付能力 | 根 [`README.md`](../README.md) | Walkthrough 只演示，不维护能力清单；不回写设计 Non-Goals |
| 产品缺口 / 实现落后于设计 | [`MVP_ACCEPTANCE.md`](MVP_ACCEPTANCE.md) | 设计文档不维护阶段台账，也不把缺口改成「永不做」 |
| 测试与实现证据 | [`TEST_CATALOG.md`](TEST_CATALOG.md) | 专题文档只声明不变量和证据入口 |
| 已选定协议的字段形状 | 公开 Go API、CLI/HTTP、包 README、Conformance | 设计文档不复制字段全集；实现偏离设计时改代码或登记缺口，不改设计迁就 |
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

先读 [`graph/documents/`](graph/documents/) 找到 `ownerTopics`，再打开拥有该主题的 Markdown。
涉及协议形状时继续读对应包 README、公开代码和 Conformance。不要用本文的表代替原文。

### 理解整个系统

1. [`TERMINOLOGY.md`](TERMINOLOGY.md)
2. [`KNOWLEDGE_CATALOG_DESIGN.md`](KNOWLEDGE_CATALOG_DESIGN.md)
3. [`LAYERS.md`](LAYERS.md)
4. [`COMPOSITION.md`](COMPOSITION.md)
5. [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md)

其余文档不能重定义它们的结论。边的类型见 `graph/relations/*.okf` 的 `relationType`。

### 修改某个专题

以 `docs/graph/documents/<id>.okf` 的 `ownerTopics` 为准。下列是阅读提示，不是第二份所有权表：

| 专题 | 文档 |
|---|---|
| Aspect 写/读/检索形态 | [`ASPECT_ACCESS.md`](ASPECT_ACCESS.md) |
| 接入/消费产品、System Schema 与目录 | [`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`](KNOWLEDGE_PRODUCT_AND_SCHEMA.md) |
| 外部资源与采集 | [`CONNECTORS.md`](CONNECTORS.md) |
| 权威与派生介质 | [`STORE_ADAPTERS.md`](STORE_ADAPTERS.md) |
| Binding 与动态观察 | [`LIVE_MATERIALIZATION.md`](LIVE_MATERIALIZATION.md) |
| State 投影控制 | [`PROJECTION_CONTROLLER.md`](PROJECTION_CONTROLLER.md) |
| 权限 | [`PERMISSIONS.md`](PERMISSIONS.md) |
| Taihu 部署认证 | [`DEPLOY_AUTH.md`](DEPLOY_AUTH.md) |
| 出站扩展 | [`HOOKS.md`](HOOKS.md) |
| Merge 证据 | [`GATES.md`](GATES.md) |
| 访问证据 | [`OBSERVABILITY.md`](OBSERVABILITY.md) |
| 诊断遥测与 SLO | [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md) |

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

## 3. 关系类型

`docs/graph/relations/*.okf` 使用协议 Relation 信封（`from` / `to` 两个 endpoint，
`kr://kc/documentation` 只是文档图的逻辑仓 id，不是可 attach 的业务仓）：

| relationType | 含义 |
|---|---|
| `depends_on` | 改 from 前，to 的结论必须仍然成立 |
| `refines` | from 缩小或解释 to，不得重定义 |
| `verifies` | from 拥有 to 的可证伪证据 |
| `operationalizes` | from 把 to 变成操作旅程 |
| `measures` | from 给演进方案设资格门槛 |
| `catalogs` | from 是 to 的导航入口 |

`depends_on` 必须无环。主干可简化为：

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
```

## 4. 维护规则

1. 新增顶层 Markdown 前，先确认没有现有 `ownerTopics`；确需新增时同时添加
   `docs/graph/documents/<id>.okf` 和必要的 `relations/*.okf`。
2. 改变公开名称，先改 Terminology；改变跨层边界，先改 Layers 和架构守卫。
3. 专题文档只能拥有其 OKF 中声明的主题。涉及别的主题时链接权威文档，不复制结论。
4. 设计文档不记录 P0/P1、已完成/未完成流水账；状态只进 MVP Acceptance/Test Catalog。
5. 包 README 维护已选定协议的用法；设计文档维护理由、不变量和取舍。实现必须跟设计，设计不跟今天的代码收缩。
6. 「尚未实现 / 首版 / 待建」只属于缺口页。只有设计明确**永远不做**的才进 Non-Goals 或否决。
7. 运行 `make check-docs`。漏登记、重复主题、悬空 Relation、环、坏链、不合协议信封的 Relation、设计类文档缺少五段合同标题，都会失败。

生成的 HTML、PNG 和 JSON 架构视图是派生展示，不进入文档权威图；它们必须能从
Markdown 与 `docs/graph/` 重新生成，不能承载独有决策。
