# 文档地图

下一版设计书正在 [`reviewed/`](reviewed/README.md) 中按组件重构；验证、研究、规模与过程资料
已迁入 `reviewed/` 并保持原有角色。顶层旧设计稿在交接完成后退出，本页与现行文档图在统一
替换前继续有效。新增设计整理优先进入替换稿，协议形状仍直接维护在公开协议中。

这里不是一组平级文章。文档按“入口 → 基础决策 → 专题决策 → 运行设计 →
验证/演进”组成有向图。

**节点和边的权威**是 [`graph/`](graph/) 里的 OKF 知识单元：每个 Markdown
一篇 `documents/*.okf`（`catalog-entry` Aspect），每条边一篇
`relations/*.okf`（`kind: Relation`，信封为 `schema/core/relation/v1`）。
`make check-docs` 用 `repofile.Parse` 与 `knowledge.DecodeRelation` 校验，
不维护第二份 JSON 图。

设计 Markdown 解释**应然**：要解决什么问题、约束从何而来、调查支持了什么、为什么选择这个方案。
公开代码与 Conformance 定义并验证可执行形状，邻近包 README 解释怎样调用；实现不能反向收窄
设计。产品可用性、用例覆盖和实际执行结果分别由验收、验证目录和带来源的运行产物回答。

## 0. 文件类型分工

| 类型 | 放什么 | 不放什么 |
|---|---|---|
| 根 `README.md` | 一句话意图、setup/run、宏观分层简图、conformance 入口 | Agent 提示词、字段全集 |
| 设计 Markdown | 问题、约束、调研、推导、选择/否决、设计要求及其可证伪观察 | DTO/字段全集、数据库 DDL、配置副本、错误码表、实现阶段流水账 |
| 操作指南 Markdown | 用户目标、前提、操作顺序、可观察结果，引用当前入口与可执行用例 | 第二套协议定义、部署内部细节、未经执行的“成功” |
| 派生 HTML（如 [`product.html`](product.html)） | 面向接入方/消费方的完整阅读路径、最小使用示例和必要的结果解释，保持离线独立 | 独有决策、字段/错误码全集、内部部署手册 |
| `docs/graph/*.okf` | 文档身份、`ownerTopics`、typed Relation | 设计散文 |
| 包 `README.md` + 公开 Go / CLI / HTTP | Address、字段、错误码、状态机、调用形状 | 产品原则复述 |
| `*_test.go` / conformance | 可证伪观察 | 设计理由 |
| `docs/reviewed/architecture-invariants.md` | 不变量 ID → 禁止观察 → 测试名 | 实现状态台账 |
| [`mvp-acceptance.md`](reviewed/mvp-acceptance.md) | 产品验收条件、可用范围与未闭环能力 | 复制测试目录、以历史口头结论宣称本次通过 |
| [`test-catalog.md`](reviewed/test-catalog.md) | 验证体系设计、新增用例规范、生成库存及运行结果入口 | 手工维护可从代码得出的数量、跨运行拼接的“全绿” |
| `.data/scenes/README.md` | 协议旅程用例的组织、维护、执行、断言 | 覆盖格子、架构不变量表 |
| `docs/observability/*.yaml`、`agent-signals.json` | 派生告警/recording 规则与 Agent 查询包 | 独有产品决策 |

一篇 Canonical 知识文件只承载一个 Address。文档图的 Relation 不得改写成 Markdown 列表充当权威。

本仓按职责区分文档与源码；不为套用目录模板迁移已有源码树。文件拆分服务于独立的问题、决定和维护责任，而不以篇幅或标题数量为目标。

| 维护职责 | 本仓位置 |
|---|---|
| 人类名片 | 根 `README.md` |
| Agent 闸门 | `AGENTS.md` |
| 执行接力棒 | 根 `TASK.md`（不是文档图节点） |
| specs（Goal / Non-Goals / 边界） | `class: foundation` 的设计 Markdown |
| decisions（选定 / 否决） | `class: decision` / `evolution` 的设计 Markdown |
| Oracle | `docs/reviewed/architecture-invariants.md`、`internal/arch`、conformance、`.data/scenes` |
| 实现可写区 | 仓库根 Go 包，不是 `src/` |

`class` 为 foundation / decision / runtime / evolution 的 Markdown 必须出现下列二级标题（名称不可改，`make check-docs` 强制）：`## Goal`、`## Non-Goals`、`## 硬性约束 / Invariants`、`## 选定方案 / 被否决方案`、`## 接口契约 / 状态机`。entrypoint / validation / guide 不套这五段。

五段写应然。「尚未实现」「首版没做」「当前包叫这个」不是 Non-Goal，也不是否决。接口段指向**设计要求的缝**（公开类型名、包 README、Conformance）；参考实现路径可以注明，但不能把今天的文件名或缺口写成协议。五段之后展开调研证据和推导；避免仅换标题重复结论，保留有助于理解取舍的反例与可验证要求。

### 设计正文怎样写

从一个具体问题或失败反例开始，说明需要保持的事实和受约束的使用者。调研要区分“来源说明的
机制”与“我们据此作出的推论”；给出可定位来源、适用条件和核对时间，不能用产品名堆砌替代
比较。决策写清选择理由、代价、否决理由和仍开放的问题；不要把暂未实现的能力写成永久边界。

五段合同提供入口，不代替正文推导。概念图、数学关系、反例和解释语义所需的最小示意可以保留；
可编译接口、完整消息、配置、脚本、DDL 或测试数据应放到关联源码、包文档、配置或夹具，正文
链接过去。迁移时必须保留设计要求，不能仅删除代码块。

使用手册可以保留让用户完成任务所需的少量命令和结果字段解释，它们是公开合同的派生用法，
不是另一份字段规范。完整可执行旅程以测试/场景为准；复制示例须核对身份、前置条件、返回结果
与当前入口。离线 HTML 的必要内容可内嵌，来源在文件内说明，不靠外链补全核心阅读步骤。

## 1. 先解决权威冲突

同一个事实只允许一个权威位置：

| 信息 | 唯一权威 | 其它文档怎么写 |
|---|---|---|
| 文档节点、主题所有权、文档间关系 | [`graph/`](graph/) OKF | 本文只解释怎么读图 |
| 公开名词 | [`terminology.md`](reviewed/terminology.md) | 直接使用或链接，不另造同义词 |
| 产品原则、身份、版本、来源、读写语义、ADR 与明确拒绝 | [`KNOWLEDGE_CATALOG_DESIGN.md`](KNOWLEDGE_CATALOG_DESIGN.md) §9.2 / §9.4 | 专题只 `refines`，引用 `ADR-*` / `R-*`，不另写系统级否决表 |
| ⓪–③ 所有权和依赖方向 | [`LAYERS.md`](LAYERS.md) | `internal/arch` 只验证，不得把当前 import DAG 写成新分层 |
| 当前命令/HTTP 形状 | 公开注册表、typed Client、help 与 Conformance | 根 README、包 README、操作指南只展示必要示例；变更先改权威，再更新派生入口 |
| 产品可用范围 | [`mvp-acceptance.md`](reviewed/mvp-acceptance.md) | README 与产品手册摘要必须保留前提；未交付能力不写成可用步骤 |
| 产品缺口 / 实现落后于设计 | [`mvp-acceptance.md`](reviewed/mvp-acceptance.md) | 设计文档不维护阶段台账，也不把缺口改成「永不做」 |
| 验证方法与新增用例规范 | [`test-catalog.md`](reviewed/test-catalog.md) | 场景作者规范继续由 .data/scenes/README.md 细化，不复制另一套状态树规则 |
| 用例库存与覆盖分母 | 公开注册表、测试代码、场景/Agent 清单生成的库存 | 文档解释分母含义和盲区，不手工重复行数 |
| 实际验证结果 | 带运行身份、代码状态、环境、范围与原始输出的运行产物 | TEST_CATALOG/MVP 只引用证据及限制；测试存在、生成库存和历史通过均不等于本次通过 |
| 已选定协议的字段形状 | 公开 Go API、CLI/HTTP、包 README、Conformance | 设计文档不复制字段全集；实现偏离设计时改代码或登记缺口，不改设计迁就 |
| 演变历史 | git history | 被替代结论从 active 文档删除，不保留“新旧两套” |

这也解决几组容易误读的重叠：

- `OBSERVABILITY.md` 只拥有不可采样的知识访问证据；
  `SYSTEM_OBSERVABILITY.md` 只拥有可采样的 metric/log/trace、健康和 SLO。
- `LIVE_MATERIALIZATION.md` 拥有 Binding/Observation 语义；
  `RETRIEVAL.md` 拥有 SEARCH 代数与 RetrievalPlan；
  `PROJECTION_CONTROLLER.md` 只拥有如何据此维护派生投影。
- `STORE_ADAPTERS.md` 拥有权威与派生介质的角色；具体 Gitea/LakeFS/OpenSearch
  机制由各 adapter README 和代码拥有。
- `SCALE_ARCHITECTURE.md` / `SCALE_BENCHMARK.md` 是演进与资格测试，不反向定义
  当前通用协议。
- [`CLI.md`](CLI.md) 拥有产品 argv、help 披露与操作数；[`cli/SURFACE.md`](../cli/SURFACE.md)
  拥有每条命令的操作语义；路径闭集是 `cli/surface.go`。[`cli-evaluation.md`](reviewed/cli-evaluation.md)
  用六维按场景判定全部公开命令是否成立，不改 argv 闭集。[`WALKTHROUGH_v5.1.md`](WALKTHROUGH_v5.1.md)
  只走旅程。[`cli/REFACTOR.md`](../cli/REFACTOR.md) 是迁移记录，不进图。

## 2. 应该读哪几份

先读 [`graph/documents/`](graph/documents/) 找到 `ownerTopics`，再打开拥有该主题的 Markdown。
涉及协议形状时继续读对应包 README、公开代码和 Conformance。不要用本文的表代替原文。

### 理解整个系统

1. [`terminology.md`](reviewed/terminology.md)
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
| 外部资源、采集与变化通知 | [`CONNECTORS.md`](CONNECTORS.md) |
| 权威与派生介质 | [`STORE_ADAPTERS.md`](STORE_ADAPTERS.md) |
| Provider 能力合同与底座替换边界 | [`PROVIDER_ABSTRACTION_CONTRACT.md`](PROVIDER_ABSTRACTION_CONTRACT.md) |
| Binding 与动态观察 | [`LIVE_MATERIALIZATION.md`](LIVE_MATERIALIZATION.md) |
| SEARCH 代数与 RetrievalPlan | [`RETRIEVAL.md`](RETRIEVAL.md) |
| State 投影控制 | [`PROJECTION_CONTROLLER.md`](PROJECTION_CONTROLLER.md) |
| 权限 | [`PERMISSIONS.md`](PERMISSIONS.md) |
| 产品 CLI argv、help 与操作数 | [`CLI.md`](CLI.md) |
| 产品 CLI 六维判定 | [`cli-evaluation.md`](reviewed/cli-evaluation.md) |
| Taihu 部署认证 | [`deploy-auth.md`](reviewed/deploy-auth.md) |
| 出站扩展 | [`HOOKS.md`](HOOKS.md) |
| Merge 证据 | [`GATES.md`](GATES.md) |
| 访问证据 | [`OBSERVABILITY.md`](OBSERVABILITY.md) |
| 诊断遥测与 SLO | [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md) |

### 操作、验证和演进

| 目的 | 文档 |
|---|---|
| 当前能力与启动 | 根 [`README.md`](../README.md) |
| 接入方与消费方使用手册 | 派生 [`product.html`](product.html)，单文件离线阅读、分享与打印（不进图；旅程仍以 [`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`](KNOWLEDGE_PRODUCT_AND_SCHEMA.md) 为准） |
| 用 CLI 走完整闭环 | [`WALKTHROUGH_v5.1.md`](WALKTHROUGH_v5.1.md) |
| 判断 MVP 是否可用 | [`mvp-acceptance.md`](reviewed/mvp-acceptance.md) |
| 验收基础重构闭环 | [`refactor-acceptance.md`](reviewed/refactor-acceptance.md)：`DOLT-01`、`DOC-14/16/17/18/19` 与 `APP-CORE-01` |
| 找自动化证据与缺口 | [`architecture-invariants.md`](reviewed/architecture-invariants.md)、[`test-catalog.md`](reviewed/test-catalog.md) |
| 判断一条产品 CLI 是否成立 | [`CLI.md`](CLI.md) §6、[`cli-evaluation.md`](reviewed/cli-evaluation.md) |
| 比较知识探索的潜在路线 | [`knowledge-exploration-research.md`](reviewed/knowledge-exploration-research.md)：开源机制、词表辅助、模型直接阅读与渐进披露、可选向量及评测条件；研究不等于实现承诺 |
| 对照派生投影控制与 Retriever | [`ingestion-retrieval-research.md`](reviewed/ingestion-retrieval-research.md)：业界 ingestion/retriever 与本仓面 3、候选定位口的映射，以及仍需完善的问题；研究不等于实现承诺 |
| 写/跑协议旅程场景 | [`.data/scenes/README.md`](../.data/scenes/README.md) |
| 讨论规模演进 | [`scale-architecture.md`](reviewed/scale-architecture.md)、[`scale-benchmark.md`](reviewed/scale-benchmark.md) |
| 看重构的目标形态、差距与执行序（入口） | [`REFACTOR_TOPOLOGY.md`](REFACTOR_TOPOLOGY.md) |
| 执行 provider 抽象重构（入口） | [`PROVIDER_REFACTOR_GUIDE.md`](PROVIDER_REFACTOR_GUIDE.md) |
| 判断 provider 合同与跨 provider 等价性 | [`PROVIDER_ABSTRACTION_CONTRACT.md`](PROVIDER_ABSTRACTION_CONTRACT.md)、[`provider-contract-validation.md`](reviewed/provider-contract-validation.md) |
| 控制代码质量的工程过程 | [`quality-loop.md`](reviewed/quality-loop.md)：六层分工、三出口闭环与 agent 腐化对策；过程约定，不改变协议 Gate/Hook 语义 |

走查叶清河茶铺实体、Aspect、关系与接入方 runtime 只在
`.data/scenes/.../named-repositories-created/` 中维护，不回写成通用系统设计。规模生成器在 `.data/scale/`。

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
                              ├─ System Observability
                              └─ Product CLI
```

## 4. 维护规则

1. 新增顶层 Markdown 前，先确认没有现有 `ownerTopics`；确需新增时同时添加
   `docs/graph/documents/<id>.okf` 和必要的 `relations/*.okf`。
2. 改变公开名称，先改 Terminology；改变跨层边界，先改 Layers 和架构守卫。
3. 专题文档只能拥有其 OKF 中声明的主题。独立主题才拆成新文；长篇的实现清单先迁到源码附近，短篇但责任独立的设计不机械合并。涉及别的主题时链接权威文档，不复制结论。
4. 设计文档不记录 P0/P1、已完成/未完成流水账；状态只进 MVP Acceptance/Test Catalog。
5. 包 README 维护已选定协议的用法；设计文档维护理由、不变量和取舍。实现必须跟设计，设计不跟今天的代码收缩。
6. 「尚未实现 / 首版 / 待建」只属于缺口页。只有设计明确**永远不做**的才进 Non-Goals 或否决。
7. 运行 `make check-docs`。漏登记、重复主题、悬空 Relation、环、坏链、不合协议信封的 Relation、设计类文档缺少五段合同标题，都会失败。

生成的 HTML、PNG 和 JSON 架构视图是派生展示，不进入文档权威图；它们必须能依据
Markdown 与 `docs/graph/` 重建，不能承载独有决策；手工编写的产品手册须同步核对来源，不能宣称已有自动生成器。当前的人读产品说明是
[`product.html`](product.html)，对照 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`、
`reviewed/terminology.md`、根 `README.md` 与 `reviewed/mvp-acceptance.md`。
