# reviewed 替换计划

这是一次文档迁移的工作清单，不是现行主题所有权或 Relation 的替代表示。
`documents/*.okf` 与 `relations/*.okf` 继续决定现行文档关系；本轮未移动 owner，
未把 reviewed 注册成另一套同时生效的设计。迁移完成后删除本清单。

## 本轮目标

`docs/reviewed/` 按可独立评审的责任重构。上一轮六篇设计过度合并，本轮把索引控制、声明式
索引、权限、Dataset、Hook/Gate、CLI 交互完整展开，并从其它篇移走相应重复正文。
篇数不是完成标准；独立设计被概述或链接代替，仍属于未交接。

公开协议与实现行为不变。当前只删除 reviewed 中已由具体正文接收的重复稿；现行顶层
设计继续保留，不能据此清理旧设计或声称正式替换已完成。

## 本轮补齐的组件评审入口

| 组件 | reviewed 入口 | 必须能评审的决定 |
|---|---|---|
| 索引控制 | `index-control.md` | 服务目标、对账、全路径增量、发布一致性、失败隔离与退出；U1–U6 |
| 声明式索引 | `declarative-index.md` | 逻辑声明与物理实现分离、字段身份、演进影响和能力边界；U1–U5 |
| 权限体系 | `permissions.md` | 身份、资源边界、发现/交付、撤权、源授权、凭证委托与恢复；U1–U6 |
| Dataset | `dataset.md` | 按文件路径选择来源和重组目录、原始字节交付、固定版本、发布与采用、冲突、范围及历史保留；独立于知识解释；U1–U10 |
| Hook/Gate | `hooks-and-gates.md` | 出站与证据分责、精确 Preview、漂移、幂等、失败与恢复；U1–U6 |
| CLI 交互 | `cli.md` | 任务、上下文、逐步披露、产品输出、副作用、失败恢复及脚本使用；U1–U6 |

上表记录本轮接收范围，不是已验证覆盖，也不是新的 ownerTopics。查询正文、知识写入与
应用装配分别收在 `retrieval.md`、`knowledge.md`、`service.md`；正式 owner 仍以 OKF 为准。

## 现行资料的替换去向

| 现行资料 | 新设计中保留什么 | 协议、证据或其它资料的去向 |
|---|---|---|
| `KNOWLEDGE_CATALOG_DESIGN.md`、`LAYERS.md` | `core-architecture.md` 的核心边界；知识身份与读写理由进入 `knowledge.md` | 现有 ADR 引用逐项交接；import 规则继续由架构守卫持有 |
| `TERMINOLOGY.md` | 核心名词在有关组件首次出现时解释，不另起概念层 | 公开名称与别名限制对应声明、注册表、帮助和守卫；未接收的约束不能先删 |
| `STORE_ADAPTERS.md` | 总览中的权威与替换边界；不可再生观察进入 `resource-access.md` | Store 能力、缓存键与介质装配归相应公开接口和包用法 |
| `ASPECT_ACCESS.md` | `knowledge.md` 的维护粒度与读取；`declarative-index.md` 的访问声明、字段语义与编译边界 | Address、选择器、Schema 访问词表及匹配规则归 Knowledge/Retrieval 协议 |
| `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` | 知识发布、Schema 演进进入 `knowledge.md`；采用更新进入 `dataset.md`；使用任务进入 `cli.md`，文件交付装配进入 `service.md` | 文件格式和目录合同归 codec/Schema；U1–U10 的现有场景引用须逐项迁移 |
| `COMPOSITION.md` | `dataset.md`：登记、固定范围、发布与消费生命周期 | Catalog 的发布、版本与清单形状归公开类型和 Conformance |
| `RETRIEVAL.md` | `retrieval.md`：查询计划、覆盖、合并、分页与同依据回读；声明理由进入 `declarative-index.md` | 代数、候选、续页和提供方端口归 retrieval/index 协议 |
| `PROJECTION_CONTROLLER.md` | `index-control.md`：目标、对账、增量、发布、恢复与消费者隔离 | 控制记录、公开状态与通知形状归 index 协议 |
| `LIVE_MATERIALIZATION.md`、`CONNECTORS.md` | `resource-access.md`：观察、采集、恢复、授权与事件边界 | Binding/Observation、通知与 Collector 合同回到公开协议；U1–U9 保留语义和原证据范围 |
| `GATES.md`、`HOOKS.md` | `hooks-and-gates.md`：证据治理与出站扩展分别展开，保留协作与失败边界 | Gate 规则、Hook 动作、执行与重试合同归 controlplane/gate/hook |
| `PERMISSIONS.md` | `permissions.md`：完整权限体系；资源访问和服务篇只解释交接 | 动作闭集和策略形状归授权注册表与公开接口；未决政策仍留 TASK，不按现实现拍板 |
| `SERVICE_ARCHITECTURE.md`、`DEPLOY_AUTH.md` | `service.md`：统一应用入口、装配、文件交付、恢复；身份及强制边界进入 `permissions.md` | Client/HTTP/CLI、认证、部署配置、供给账和文件网关形状归各组件 |
| `OBSERVABILITY.md`、`SYSTEM_OBSERVABILITY.md` | `service.md` 中证据与诊断的区别及失败影响 | 事件、指标、标签、保留配置与告警归协议和部署资料 |
| `PROVIDER_ABSTRACTION_CONTRACT.md` | 总览与检索篇保留能力替换的理由、不得模拟成功的边界 | 可编译接口、能力声明、失败语义与替换 Conformance 是协议；缺失部分接收后才能删除旧规格 |
| `CLI.md` | `cli.md`：完整任务交互、上下文、帮助、结果、恢复和评价；服务篇只保留同一执行入口 | argv、帮助、动作与路由归生产注册表；用法归 CLI/Client 文档 |
| `WALKTHROUGH_v5.1.md` | `walkthrough.md` 只导航真实任务与前态 | 完整步骤和断言继续在场景，必要的用户操作说明属于指南 |
| `ARCHITECTURE_INVARIANTS.md`、`TEST_CATALOG.md` | 不合并成设计章节 | 保留可证伪约束和验证方法，去掉可以从代码生成的重复清单是后续独立整理 |
| `MVP_ACCEPTANCE.md`、`REFACTOR_ACCEPTANCE.md`、`PROVIDER_CONTRACT_VALIDATION.md`、`CLI_EVALUATION.md` | 不计入理解架构的阅读前置 | 验收范围、缺口与资格归验证资料；已有结果与限制不得折算成当前通过 |
| `SCALE_ARCHITECTURE.md`、`SCALE_BENCHMARK.md` | 核心设计保留有界工作量与版本身份要求 | 容量实验、代际开放方案和资格线保留专项资料，不当当前实现保证 |
| `INGESTION_RETRIEVAL_RESEARCH.md`、`KNOWLEDGE_EXPLORATION_RESEARCH.md` | 只将已经作出的选择及理由吸收到有关组件 | 原始外部来源、比较与未选路线保留研究资料，不复制成另一套组件设计 |
| `REFACTOR_TOPOLOGY.md`、`PROVIDER_REFACTOR_GUIDE.md` | 有效目标边界进入对应组件设计 | 执行序、差距与尚未完成事项归 TASK/迁移记录，不成为永久设计篇 |
| 根 `README.md`、`docs/README.md`、`product.html` | 更新为新的阅读入口，不增加设计权威 | 使用手册保留用户任务；派生内容在正式切换时与来源核对 |

## reviewed 稿件的接收关系

- `core-concepts.md`、`snapshot-store.md`：吸收到总览与知识篇，旧图及其生成脚本一起退出。
- `dataset.md`：恢复为独立设计，接收上一轮 `catalog-composition.md` 的内容并展开发布与生命周期；
  `dataset-authorization.md` 的授权理由进入 `permissions.md`。旧稿“协议尚待实现”的叙述按现有公开合同修正，文档图仍未切换。
- `declarative-access.md`：逻辑声明设计进入 `declarative-index.md`；`ingestion-control.md`：后台控制设计进入 `index-control.md`。
- `retrieval-algebra.md`、`retriever.md`：查询理由与执行保证进入 `retrieval.md`，可执行代数仍归协议。
- `binding-observation.md`：并入外部资源访问。
- `merge-gates.md`、`outbound-hooks.md`：完整设计进入 `hooks-and-gates.md`，知识篇仅保留写面交接。
- `authentication.md`、`authorization.md`：进入 `permissions.md`；`access-evidence.md`、
  `host-file-projection.md`、`durable-recovery.md` 的应用装配进入 `service.md`；身份/权限恢复的决定
  进入权限篇，固定文件生命周期进入 Dataset 篇，具体协议保留原公开入口。

这些是迁移记录，不保留空文件做跳转。新的独立篇必须展开问题、理由、取舍和方向用例，
不能恢复成一组“能力介绍”后就宣称设计完整。

## 既有方向性用例的交接

下表只给出待切换的落点，当前 `_views.yaml` 与 `verifies` 不变。合并到同一新用例的旧条目，
仍需逐条保留实际断言与缺口；不能因为新标题更宽就宣称覆盖扩大。

| 现行条目 | reviewed 落点 |
|---|---|
| `knowledge-product-schema#U1`–`U4`：系统、声明、实例与同批发布 | `knowledge.md` U1/U2；System 信任根与不可写的已有反例保留 |
| `knowledge-product-schema#U5`：Schema 演进 | `knowledge.md` U3；原位 breaking 迁移仍未选定 |
| `knowledge-product-schema#U6`：发现与消费 | `cli.md` U1 与 `service.md` U5；库存、自描述、Schema 与权限的局部证据分别保留 |
| `knowledge-product-schema#U7`–`U9`：已有项目、语义文件与界面隐藏 | `dataset.md` U8 与 `service.md` U3 及文件交付章节；挂载、原文件、固定版本和 UI 开关的独立断言不能合并掉 |
| `knowledge-product-schema#U10`：消费性能 | `service.md` U6；原性能目标和未测范围保留在测量/验收资料，不能用新稿的定性要求替代 |
| `materialization#U1`–`U9` | `resource-access.md` 同号用例；U8 的维护成本判据进入 `index-control.md` U2 |

上一轮 reviewed 的检索 U2（单对象增量）与 U3（通知丢失恢复）进入 `index-control.md` 同号用例；
`retrieval.md` U2/U3 现在展开补判续页与多来源查询。这些草稿 ID 尚无现行场景绑定，不能当作
正式产品 ID 已迁移。其它新增方向性用例只描述设计要求，尚未建立新的产品视图或覆盖声明。

Dataset 替换稿进一步收回文件层：原 U6 的切面与 Schema 解释进入 `knowledge.md` U7，
原 U7 的固定声明动态服务由 `resource-access.md` U5 承接；查询投影与历史保留仍在
`index-control.md`，知识应用的发布准备留在 `service.md`。Dataset U6/U7 分别说明
部分文件选择与不安装知识能力的文件交付，不新增现行场景绑定或变更现行 owner。

## 正式替换前的交接

1. 逐条核对旧设计的决策、否决、开放问题与不变量引用。理由进入有关组件，协议进入可执行
   合同；已有协议缺口保留明确目标，不能因没有实现而消失。
2. 迁移原有用例 ID 与场景关联。新文中的方向用例不直接变成测试覆盖；关联细节与缺口一并保留。
3. 在 `docs/graph/` 一次更新有效节点、主题所有权与 Relation，调整检查所用的正式文档入口。
   不通过放宽唯一所有权、链接或断言规则来容纳两套现行设计。
4. 更新根导航、包内旧设计链接与派生手册，再退出被替换顶层旧稿；协议、研究和验证资料按
   其角色保留，不因为不再叫“设计”就删除。

本轮只做替换稿与迁移清单，未执行文档检查或测试，未将上述交接记为完成。
