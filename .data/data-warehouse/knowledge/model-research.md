# 数仓领域实体、Aspect 与关系模型：业界调研与最终建议

> 状态：决策依据；可执行契约已收敛到 `schemas/physical/` 与 `schemas/semantic/` 的 Aspect YAML，并由 `../features/*.feature` 验收
> 调研日期：2026-08-25
> 范围：物理数据资产、数据加工、血缘、语义层、治理元数据；不改变 Knowledge Catalog 底座现有写入与版本语义

## 1. 结论摘要

本轮建议收敛为以下七项决定。

1. **统一使用“实体 + Aspect + 关系”三类概念，不把所有内容都叫 Aspect。**
   - Entity 表示具有稳定身份、需要被独立引用和治理的对象。
   - Aspect 表示 Entity 或 Relation 上可独立维护、独立授权来源、独立演进的描述分区。
   - Relation 表示多个 Entity 之间的结构语义；Relation 自身也可以带 Aspect。

2. **关系只维护一份 Canonical，不在两端各写一份。**
   - 正向、反向访问是查询与索引能力，不是双份事实。
   - 关系可能有方向和端点角色，不能因为支持双向查询就认为两端语义完全对等。
   - 所有读结果必须回读同一个 Workspace pin 上的 Canonical Relation，关系索引只是可重建投影。

3. **血缘不是简单的 `A -> B` 属性，而是带加工上下文的关系事实。**
   - 表级和列级血缘均应能指向产生它的 DataJob、Run、SQL 或表达式。
   - 一个加工通常有多个输入、多个输出，Canonical 形态宜建模成 `LineageAssertion`，再派生供遍历使用的二元边。

4. **`ETLJob` / `ETLTask` 不宜作为统一领域名。**
   - 当前 `ETLJob` 实际是编排容器，建议改为 `DataPipeline`。
   - 当前 `ETLTask` 实际是可独立寻址、消费并产出数据的工作定义，建议改为 `DataJob`。
   - DataJob 的具体类型不拆成一批顶级 Entity，而是正交描述：运行模式 `processingMode`、技术形态 `executionKind`、业务用途 `purposes`，并保留来源原文 `nativeKind` / `nativeType`。
   - `DAG`、`TASK`、`JOB` 等来源名称不能直接决定 Canonical 层级；先判断它是编排容器还是实际加工单元。
   - Pipeline 和 Job 的一次执行分别是动态 `PipelineRun`、`JobRun`；运行历史不作为不断增长的 Snapshot Entity。

5. **`MetricView` 建议统一为 `SemanticModel`。**
   - `METRIC_VIEW` 保留为 Databricks 等来源的 `nativeKind`。
   - Dimension、Measure 只有在可复用、可独立治理或被其它对象引用时才成为独立 Entity；否则是 SemanticModel 的成员。

6. **Aspect 按维护责任与变化节奏切分，不按页面模块或随意打包。**
   - 当前笼统的 `structure` 应拆为 `properties`、`schema` 和显式包含关系。
   - `definition` 只表示人工或计算定义，不再承载所有来源属性。
   - `permissions` 改为 `sourcePermissions` 或 `accessGrantsSnapshot`，明确它不等于 Knowledge Catalog 访问控制。
   - `runtimeSummary` 不作为稳定定义；如确有展示需求，改成有 `observedAt` 的派生 `latestRunSummary`。

7. **先定义领域 Schema，再改实体和关系实现。**
   - 已用可直接 ingest 的 Aspect YAML 定义字段、必填引用、访问提示和关系角色。
   - E2E 先把 Schema 作为知识写入三个目标 Repository，再运行采集翻译、关系回读和兼容验证。

## 2. 判断原则

### 2.1 什么时候建 Entity

满足以下任一条件，通常应建独立 Entity：

- 有跨系统或跨版本稳定身份；
- 会被其它对象独立引用；
- 有独立负责人、生命周期、认证或权限边界；
- 有多个独立生产者维护不同描述；
- 需要被搜索、审计、订阅或作为关系端点；
- 删除、重命名和迁移需要单独追踪。

反之，若一个对象只在父对象内部有意义、没有独立治理需求，也不会被外部引用，优先作为 Aspect 字段或 keyed member，而不是为了“图看起来完整”而实体化。

### 2.2 什么时候拆 Aspect

Aspect 是维护和版本化单元。出现以下差异时应考虑拆分：

- 来源或权威不同，例如采集器属性与人工文档；
- 更新频率不同，例如表结构与每日 profile；
- 可见性或读取成本不同；
- 生命周期不同，例如定义与最新观测；
- 冲突处理策略不同；
- 需要独立溯源或独立替换。

不应仅因为 UI 上分成不同卡片就拆 Aspect，也不应把一个来源的所有字段塞入 `definition`。

### 2.3 什么时候建 Relation

满足以下条件时应使用显式 Relation，而不是普通字符串引用：

- 需要从任一端点查询；
- 关系有类型、方向、角色或基数；
- 关系自身有属性、来源、有效期或证据；
- 一个关系涉及两个以上端点；
- 需要做引用完整性、去重或关系级审计；
- 关系将被通用图访问或关系索引消费。

简单的资源定位或只在父对象内部使用的引用仍可作为普通字段，不必全部关系化。

## 3. 当前实现盘点

当前场景的可执行依据是 `validation/workbench/realistic_warehouse_test.go`。它定义了 13 种领域 Entity：

| 分组 | 当前 Entity |
|---|---|
| 来源与访问 | `DataSource`、`ResourceDescriptor` |
| 物理资产 | `Database`、`Schema`、`Table`、`Column` |
| 加工 | `ETLJob`、`ETLTask` |
| 语义层 | `MetricView`、`Dimension`、`Measure`、`Metric` |
| 质量 | `QualityRule` |

当前共声明 29 个 Entity-Aspect Schema 组合，对应 17 个唯一 Aspect 名称：

| Entity | 当前 Aspect |
|---|---|
| DataSource | `definition` |
| ResourceDescriptor | `definition` |
| Database | `structure` |
| Schema | `structure` |
| Table | `structure`、`profile`、`freshness`、`joinEvidence`、`permissions` |
| Column | `structure`、`classification` |
| ETLJob | `definition`、`schedule`、`tasks`、`ownership` |
| ETLTask | `definition`、`inputs`、`outputs`、`columnMappings`、`runtimeSummary` |
| QualityRule | `definition`、`qualityTargets` |
| MetricView | `definition`、`ownership` |
| Dimension | `definition`、`dependencies` |
| Measure | `definition`、`dependencies` |
| Metric | `definition`、`dependencies`、`ownership`、`certification` |

需要特别说明两点：

- 验证报告中的 Aspect 清单只列了 15 个，漏掉 `tasks` 和 `runtimeSummary`；代码中的实际唯一名称是 17 个。
- 当前 Schema 声明的 `fields` 为空，因此这是一套验证 Catalog 协议能力的骨架，不是已定稿的数仓领域 Schema。

### 3.1 当前关系是怎么表达的

当前不是通用 Relation 模型，而是把引用放在不同 Aspect 中：

- 表血缘由 `ETLTask.inputs` 和 `ETLTask.outputs` 组合推断；
- 列血缘由 `ETLTask.columnMappings` 表达，含输出列、输入列、表达式和可选 SQL digest；
- Pipeline 与 Task 的包含关系由 `ETLJob.tasks` 表达；
- 物理层级由 `sourceId`、`databaseId`、`schemaId`、`tableId` 等字段表达；
- 语义依赖由 `dependencies` 表达；
- `joinEvidence` 是可连接性证据，明确不是生产血缘。

验证集会在固定 Workspace pin 上手工检查这些引用是否可解析，但 Writer 目前没有通用关系端点、关系类型、基数和反向查询契约。底座已有 `Relation` kind，Reader Schema 也认识 `relation_set`，但数仓场景尚未把它们落成正式领域模型。

## 4. 业界术语对照

业界没有一套产品都完全一致的实体树，但有较稳定的语义共识：物理数据集、加工定义、运行实例和语义模型应分开。

| 本项目当前名 | DataHub | OpenLineage | OpenMetadata | Databricks / Airflow | 建议 Canonical 名 |
|---|---|---|---|---|---|
| DataSource | Platform Instance / Dataset Platform | Namespace / Dataset source | Database Service | Metastore / connection | `DataPlatformInstance` |
| Database | Dataset 容器语义 | Dataset namespace 的一部分 | Database | Catalog 或 Database | `Database` |
| Schema | Dataset 容器语义 | 名称空间的一部分 | DatabaseSchema | Schema | `DatabaseSchema` |
| Table | Dataset | Dataset | Table | Table | `Table` |
| Column | SchemaField | Dataset field | Column | Column | `Column` |
| ETLJob | DataFlow | 无直接强制层级 | Pipeline | Lakeflow Job / Airflow DAG | `DataPipeline` |
| ETLTask | DataJob | Job | Pipeline Task | Lakeflow Task / Airflow Task | `DataJob` |
| 一次运行 | DataProcessInstance 等运行语义 | Run | Pipeline Status / Task Status | DAG Run / Job Run / Task Instance | `PipelineRun` / `JobRun`，动态 |
| MetricView | SemanticModel | 非核心对象 | DataModel 等语义对象 | Metric View | `SemanticModel` |
| Dimension | SemanticModel 字段注解 | 非核心对象 | 语义模型成员 | Metric View dimension | `Dimension` 或成员 |
| Measure | SemanticModel 字段注解 | 非核心对象 | 语义模型成员 | Metric View measure | `Measure` 或成员 |
| Metric | Metric | 非核心对象 | Metric | Metric | `Metric` |
| QualityRule | Assertion 等 | quality facets 扩展 | TestDefinition / TestCase | expectation / monitor | `DataQualityRule` |

### 4.1 为什么不是统一叫 Job

`Job` 在不同系统里的粒度并不相同：

- OpenLineage 的核心模型是 `Job` 与 `Run`，Job 是一个消费或产出 Dataset 的过程，可能对应 task、model、query 或 checkpoint；
- DataHub 用 `DataFlow` 组织多个 `DataJob`；
- OpenMetadata 用 `Pipeline` 包含 Task；
- Airflow 用 DAG 包含 Task；
- Databricks Lakeflow Job 是编排容器，内部有多个 Task。

因此，不能把源系统的 `job` 字面量直接当成统一领域层级。建议使用：

```text
定义关系：DataPipeline contains DataJob

动态运行：PipelineRun realizes DataPipeline
          └── JobRun realizes DataJob
```

`PipelineRun` / `JobRun` 是概念上的运行实例和外部动态观察，不代表要把运行历史持续追加进 Snapshot Repository。

同时保留来源事实，并把标准化类型放在独立描述中：

```json
{
  "properties": {
    "nativeId": "源系统稳定标识",
    "platformRef": "DataPlatformInstance:airflow-prod",
    "nativeKind": "TASK",
    "nativeType": "SQLExecuteQueryOperator"
  },
  "executionProfile": {
    "processingMode": "BATCH",
    "executionKind": "QUERY",
    "purposes": ["TRANSFORM", "PUBLISH"]
  }
}
```

这样既统一消费语义，也不丢失源系统概念。

### 4.2 DataJob 的具体类型如何处理

业界已经考虑这一问题，但成熟做法不是不断增加 `SQLJob`、`SparkJob`、`StreamingJob` 等顶级 Entity：

- OpenLineage 仍使用统一 `Job`，在 Job Type Facet 中分别记录 `processingType`、`integration` 和 integration-specific `jobType`。例如 Airflow 可上报 `DAG` / `TASK`，dbt 可上报 `PROJECT` / `MODEL`，同时另行声明 `BATCH` / `STREAMING` / `SERVICE`；
- DataHub 仍使用统一 `DataJob`。`dataJobInfo.type` 已从 Azkaban 专用枚举转向开放字符串，并提供 `subTypes` Aspect 对 SQL、Python、Notebook、Container 或自定义类别进行过滤和展示；
- 两者都把 Job 定义和一次 Run 分开，Job 类型不会替代 Run 生命周期。

本项目不应只设计一个含义模糊的 `jobType`。至少拆成以下维度：

| 维度 | 建议字段 | 词表与示例 | 说明 |
|---|---|---|---|
| Catalog 语义身份 | Entity type | `DataPipeline`、`DataJob` | 决定对象契约和关系能力 |
| 运行模式 | `processingMode` | `BATCH`、`STREAMING`、`SERVICE`、`UNKNOWN` | 描述运行是否有自然结束和事件生命周期 |
| 技术形态 | `executionKind` | `QUERY`、`COMMAND`、`CODE`、`MODEL`、`NOTEBOOK`、`STREAM_PROCESSOR`、`OTHER` | 描述它以什么形态执行 |
| 业务用途 | `purposes[]` | `INGEST`、`TRANSFORM`、`PUBLISH`、`QUALITY`、`TRAIN`、`SERVE`、`ORCHESTRATE`、`OTHER` | 一个 Job 可以承担多个用途 |
| 来源原文 | `nativeKind` / `nativeType` | `TASK`、`PythonOperator`、`DBT_MODEL`、`FLINK_JOB` | 不受 Canonical 枚举限制，保证无损回溯 |

`properties.nativeKind` / `nativeType` 是来源事实；`executionProfile` 是标准化消费语义。二者分开，是因为它们可能有不同的 Schema、生产者和演进节奏。标准化映射若由算法派生，应按底座 DERIVATION 规则记录固定输入版本和 algorithm；若是 connector 对来源协议的确定性翻译，则随 SOURCE 写入并保留 source refs。

建议的来源映射如下：

| 来源对象 | Canonical | processingMode | executionKind | purposes | 保留的 native 信息 |
|---|---|---|---|---|---|
| Airflow DAG | `DataPipeline` | 不在 DataJob 上填写 | 不适用 | 不适用（容器） | `DAG` |
| Airflow SQL task | `DataJob` | `BATCH` | `QUERY` | `TRANSFORM` / `PUBLISH` | `TASK` + operator class |
| dbt project | `DataPipeline` | 不在 DataJob 上填写 | 不适用 | 不适用（容器） | `PROJECT` |
| dbt model | `DataJob` | `BATCH` | `MODEL` | `TRANSFORM` / `PUBLISH` | `MODEL` + materialization |
| Spark SQL job | `DataJob` | `BATCH` 或来源值 | `QUERY` | 按来源判定 | `JOB` + Spark application type |
| Flink streaming job | `DataJob` | `STREAMING` | `STREAM_PROCESSOR` | `INGEST` / `TRANSFORM` | `JOB` + operator graph type |
| 数据质量 task | `DataJob` | 通常 `BATCH` | 按实现填写 | `QUALITY` | 源 task / operator type |

其中 `purposes` 不应仅凭名称猜测；来源没有足够证据时可以不填或写 `OTHER`，不能为获得整齐枚举而制造错误知识。

只有具体类型改变了稳定身份、生命周期、必填 Aspect 或必需关系时，才考虑升格为新的 Entity type。例如未来 `MLTrainingJob` 若必须关联 Model、Experiment、TrainingDataset 和 Hyperparameters，才值得单独评估。以下类型首版都不拆实体：

- `StreamingJob`：`DataJob.executionProfile.processingMode=STREAMING`；
- `SQLJob`：`DataJob.executionProfile.executionKind=QUERY`；
- `DataQualityJob`：`purposes=[QUALITY]`，并关联独立 `DataQualityRule`；
- `dbt Model`：`executionKind=MODEL`，来源原文保存在 `nativeKind` / `nativeType`。

OpenLineage 因核心抽象较少，可以同时把 DAG 和 Task 表示成 Job；本项目已有 `DataPipeline`，因此不应仅因为来源或交换协议称其为 Job，就把 DAG 也映射成 `DataJob`。Pipeline 的聚合血缘可以由子 Job 派生，而不是复制一份可独立修改的血缘事实。

## 5. 推荐实体目录

### 5.1 第一阶段必需实体

| 分组 | Entity | 独立实体的理由 |
|---|---|---|
| 来源 | `DataPlatformInstance` | 平台实例有独立来源身份、环境和访问声明 |
| 访问声明 | `ResourceDescriptor` | 保存稳定访问声明；不保存 live 内容 |
| 物理资产 | `Database` | 独立容器与治理边界 |
| 物理资产 | `DatabaseSchema` | 独立容器；名称避免与 `schema/*` 知识对象混淆 |
| 物理资产 | `Table` | 主要数据资产与血缘端点 |
| 物理资产 | `Column` | 列级血缘、分类、术语和 profile 需要独立引用 |
| 加工 | `DataPipeline` | 编排、调度和所有权容器 |
| 加工 | `DataJob` | 数据加工定义与血缘上下文 |
| 语义层 | `SemanticModel` | 语义数据集、Join、维度和度量的组合边界 |
| 语义层 | `Metric` | 可复用、认证、被业务独立消费 |
| 质量 | `DataQualityRule` | 可复用规则定义和治理对象 |
| 关系 | `Relation` / 具体 relation type | 单份 Canonical 关系事实 |
| 血缘 | `LineageAssertion` | 表达多输入、多输出和列映射的加工事实 |

### 5.2 条件性实体

| Entity | 建独立 Entity 的条件 | 否则如何表达 |
|---|---|---|
| `Dimension` | 跨多个 SemanticModel 复用、独立治理、认证或被 Metric 引用 | SemanticModel 的 keyed member |
| `Measure` | 跨模型复用、独立定义和治理 | SemanticModel 的 keyed member |
| `DataProduct` | 产品有负责人、SLA、输入输出和生命周期 | 暂不引入 |
| `Dashboard` / `Chart` | 要覆盖 BI 消费血缘 | 后续扩展消费资产域 |
| `MLModel` / `Feature` | 要覆盖 AI/ML 血缘 | 后续扩展 ML 域 |
| `GlossaryTerm` / `Domain` / `Tag` | 需要稳定身份、层级、负责人或关系 | 简单标签可先用值 |

### 5.3 不进入 Snapshot Entity 的动态对象

以下内容变化快、量大，应通过 Binding、流或外部运行系统访问；如需知识化，只写显式快照或派生摘要：

- `PipelineRun` / `JobRun` / `TaskRun` 完整历史；
- 查询日志、访问事件；
- 持续更新的 profile 明细；
- 实时质量结果；
- 当前源系统授权状态；
- 运行日志与监控指标。

## 6. 一个实体应该如何描述

### 6.1 通用描述骨架

所有 Entity 不必强制拥有所有 Aspect，但应从同一套语义词表选择：

| Aspect | 作用 | 典型权威 |
|---|---|---|
| `properties` | 名称、限定名、平台、环境、native type、source key、外部 URL 等来源属性 | 来源采集器 |
| `documentation` | 说明、用途、示例、注意事项 | 负责人或文档同步 |
| `ownership` | owner、steward、责任角色 | 治理系统或人工 |
| `domains` | 业务域归属 | 治理系统 |
| `tags` | 轻量标签 | 多来源，经明确合并策略 |
| `glossaryTerms` | 与业务术语的关联 | 术语治理 |
| `classification` | 敏感等级、PII、数据类别 | 分类器或治理系统 |
| `lifecycle` | 状态、弃用、替代对象、有效时间 | 来源或负责人 |
| `certification` | 认证状态、认证人、截止时间 | 治理流程 |
| `aiContext` | 面向 Agent 的业务上下文、使用边界与示例 | 领域负责人 |

来源、Actor、时间、算法和固定输入版本仍由协议的 ProvenanceEnvelope 记录，不在每个 Aspect 内重复复制。

### 6.2 物理资产 Aspect

| Entity | 推荐 Aspect |
|---|---|
| DataPlatformInstance | `properties`、`accessDescriptor`、`ownership`、`lifecycle` |
| Database | `properties`、`documentation`、`ownership`、`lifecycle` |
| DatabaseSchema | `properties`、`documentation`、`ownership`、`lifecycle` |
| Table | `properties`、`schema`、`storage`、`partitioning`、`documentation`、`ownership`、`classification`、`profile`、`freshness`、`qualitySummary`、`usageSummary`、`sourcePermissions`、`lifecycle` |
| Column | `properties`、`documentation`、`classification`、`glossaryTerms`、`profile`、`lifecycle` |

具体调整：

- 当前 `structure` 拆为来源属性、表列 Schema 和显式包含关系；
- `Table.schema` 描述字段结构时可以内嵌字段摘要，但列级治理与血缘仍引用独立 `Column`；
- `permissions` 改名为 `sourcePermissions` 或 `accessGrantsSnapshot`，只描述源系统授权知识，绝不能作为 `kc read` 放行依据；
- `profile`、`freshness`、`qualitySummary` 必须带观测时间、输入版本或来源证据，避免被误认为静态定义。

### 6.3 加工实体 Aspect

| Entity | 推荐 Aspect |
|---|---|
| DataPipeline | `properties`、`documentation`、`schedule`、`ownership`、`runBinding`、`lifecycle` |
| DataJob | `properties`、`executionProfile`、`definition`、`sourceCode`、`schedule`、`ownership`、`runBinding`、`lifecycle` |

说明：

- `definition` 描述加工逻辑或人工维护的语义，不再承担 name、platform 等通用属性；
- `properties` 保存 `platformRef`、`nativeId`、`nativeKind`、`nativeType` 等来源事实；
- `executionProfile` 保存标准化的 `processingMode`、`executionKind` 和 `purposes`；不要用单个 `type` 字段混装这三个维度；
- `processingMode` 使用小而稳定的闭合枚举；`executionKind` 和 `purposes` 使用带版本的受控词表，未知值通过 `OTHER` 加来源原文承接；
- `sourceCode` 保存稳定代码引用、版本或 SQL digest，不默认复制整个外部仓内容；
- `runBinding` 可用于 Pipeline 或 Job，只保存如何访问固定声明的稳定配置；Reader 解析 Binding，但不直接调用外部 runtime；
- 完整运行历史不放 Entity；如为了列表展示保留摘要，应使用派生的 `latestRunSummary`，并含 `observedAt`、状态、run ref 和来源。

### 6.4 语义与质量实体 Aspect

| Entity | 推荐 Aspect |
|---|---|
| SemanticModel | `properties`、`definition`、`documentation`、`joins`、`ownership`、`certification`、`aiContext`、`lifecycle` |
| Dimension | `properties`、`definition`、`documentation`、`dependencies`、`ownership`、`aiContext`、`lifecycle` |
| Measure | `properties`、`definition`、`documentation`、`dependencies`、`ownership`、`aiContext`、`lifecycle` |
| Metric | `properties`、`definition`、`documentation`、`dependencies`、`ownership`、`certification`、`aiContext`、`lifecycle` |
| DataQualityRule | `properties`、`definition`、`targets`、`ownership`、`lifecycle`、`resultBinding` |

`joins` 表示语义模型声明的 Join 逻辑；物理层的 `joinEvidence` 表示数据证据。二者都不是生产血缘。

### 6.5 Aspect Schema 至少需要描述什么

正式的 `schema/*` 知识对象至少应给出：

- 适用的 Entity / Relation type；
- Aspect name 与存储形态：record、keyed collection 或 relation set；
- 字段名、类型、是否必填、是否可空；
- Entity ref / Relation ref 的目标类型约束；
- 枚举、单位和时间语义；
- member key 的唯一性规则；
- 检索声明：`text`、`filter`、`sort` 及字段类型；
- Schema 版本与兼容策略；
- 敏感字段和读取策略；
- 示例与反例。

## 7. 业界如何维护关系

### 7.1 对照结论

主流产品都提供正反向访问，但并不通过“两端各维护一份事实”来实现：

| 产品 | Canonical 关系保存方式 | 正反访问方式 |
|---|---|---|
| DataHub | `upstreamLineage` 保存在下游 Dataset 的 Aspect；包含 upstream 与 fine-grained lineage | API 提供 upstream、downstream 和多跳查询；反向由图索引派生 |
| OpenMetadata | `entity_relationship` 一条 from/to 关系记录；血缘是一条 fromEntity/toEntity edge | API 返回 upstreamEdges 和 downstreamEdges |
| Apache Atlas | 一个 Relationship 实例，RelationshipDef 明确两个端点、角色、基数和属性 | 关系服务与图索引从任一端访问 |
| Google Dataplex | 一个 EntryLink 连接两个 Entry；LineageEvent 中的 Link 是 source/target 对 | UI/API 支持 upstream、downstream 或双向视图 |
| OpenLineage | 一次 Job Run 事件声明 input/output Dataset；列血缘放 output dataset facet | 消费端按事件构建血缘图 |
| Unity Catalog | 系统表记录 source/target 的读写血缘事件 | UI 和查询提供上游、下游表及列关系 |

这些实现的共同点是：**事实单写，访问双向；关系索引可重建。**

### 7.2 “关系两端对等”需要拆成两个问题

用户访问权利应对等：给定任一端点，都应能找到同一条关系。但关系语义不一定对等：

- `upstream -> downstream` 有方向；
- `DatabaseSchema contains Table` 中端点角色不同；
- `Team owns Table` 中 owner 与 owned 角色不同；
- `Table relatedTo Table` 或 `synonymOf` 才可能是无向关系。

因此，推荐表述是：

> 关系没有“必须挂在某个端点上”的读取主从，但可以有权威来源、声明上下文、方向和端点角色。

不要用“主实体”决定事实归属。由谁写入，应由生产者、来源权威和 Repository 治理边界决定。

### 7.3 为什么关系仍可以使用 Aspect

Aspect 回答的是“这部分知识如何维护和版本化”，Relation 回答的是“多个实体之间是什么结构语义”。两者不是互斥概念。

- Relation 可以是独立对象；它的 `properties`、`evidence`、`lifecycle` 仍可分别作为 Aspect；
- 一个 Entity Aspect 也可以声明关系集合，例如 Dataset 的 lineage Aspect；
- 问题不在于“用了 Aspect”，而在于是否只有松散字符串引用、缺少端点类型、方向、角色、基数和反向投影。

对于本项目，长期建议以独立、可寻址的 Relation 作为通用 Canonical；为兼容来源格式，可以允许 Entity Aspect 作为输入声明，但必须明确翻译成哪一种 Canonical 关系，不能形成两份可独立修改的事实。

## 8. 推荐关系契约

### 8.1 通用 Relation

建议的最小逻辑形态如下，字段名最终应由正式 `schema/*` 确定：

```json
{
  "relationId": "稳定且不依赖路径的 opaque id",
  "relationType": "contains | producedBy | dependsOn | owns | relatedTo | ...",
  "direction": "DIRECTED | UNDIRECTED",
  "endpoints": [
    {"role": "container", "objectRef": "DatabaseSchema:..."},
    {"role": "member", "objectRef": "Table:..."}
  ],
  "contextRef": "可选的 DataJob、JobRun 或 SemanticModel",
  "attributes": {
    "cardinality": "ONE_TO_MANY",
    "expression": "可选",
    "confidence": 1.0
  },
  "validity": {
    "observedAt": "2026-08-25T00:00:00Z"
  }
}
```

另外由协议层 ProvenanceEnvelope 记录来源、Actor、算法、固定输入版本和 source refs。

### 8.2 血缘应使用 LineageAssertion

加工不是天然二元关系。例如一个 SQL 同时读三张表并写两张表，列映射也可能是多输入表达式。推荐 Canonical：

```json
{
  "lineageId": "opaque id",
  "processRef": "DataJob:daily-order-model",
  "basis": "DESIGN | RUN",
  "inputs": ["Table:orders", "Table:lineitem"],
  "outputs": ["Table:fact_orders"],
  "columnMappings": [
    {
      "outputColumnRef": "Column:fact_orders.gmv",
      "inputColumnRefs": [
        "Column:lineitem.l_extendedprice",
        "Column:lineitem.l_discount"
      ],
      "transformationType": "EXPRESSION",
      "expression": "l_extendedprice * (1 - l_discount)"
    }
  ],
  "sqlDigest": "sha256:..."
}
```

从该 Assertion 派生：

- 表级 `upstreamOf` / `downstreamOf` 边；
- 列级 `upstreamOf` / `downstreamOf` 边；
- `DataJob consumes Table`；
- `DataJob produces Table`。

派生边用于访问和索引，Assertion 是唯一事实。不能同时把 `inputs`、`outputs`、`columnMappings` 和独立边都当成可写 Canonical。

### 8.3 正反访问实现

建议增加可重建的关系投影：

```text
endpoint object_id -> [relation_id, endpoint_role, direction]
```

访问过程：

1. 命令开始时解析 Workspace，固定各 Repository commit；
2. 用端点索引找 Candidate Relation refs；
3. 在同一 basis commit 回读 Canonical Relation；
4. 按 relation type、方向、role 和目标类型过滤；
5. 返回一跳结果，跨多跳由上层做有界遍历。

索引不进入知识仓，也不能成为权威。投影损坏时可以由 Canonical 关系重建。

### 8.4 跨 Repository 关系

- Relation 写入具有该关系权威的 Repository，而不是机械地写入某个端点所在仓；
- 端点通过当前 Workspace pin 解析；
- 不做跨 Repository 事务和级联修改；
- Entity rename 不应改变 `object_id`；
- 缺失端点在 Preview / gate / validation 中报错，由采集或治理流程修复；
- 两端查询通过 Workspace 范围内的关系投影完成。

## 9. 当前模型到目标模型的映射

| 当前 | 目标 | 迁移说明 |
|---|---|---|
| `DataSource.definition` | `DataPlatformInstance.properties` | `engine`、environment、native id 进入 properties；访问声明继续引用 ResourceDescriptor |
| `Database.structure` | `Database.properties` + `contains` | 容器关系不再只靠 `sourceId` 字段 |
| `Schema.structure` | `DatabaseSchema.properties` + `contains` | 名称改为 DatabaseSchema，避免与知识 Schema 混淆 |
| `Table.structure` | `Table.properties` + `Table.schema` + `contains` | 拆分属性、字段结构与容器关系 |
| `Column.structure` | `Column.properties` + `contains` | 列属于表成为显式关系 |
| `ETLJob.definition` | `DataPipeline.properties` / `documentation` | 当前对象语义是编排容器 |
| `ETLJob.tasks` | `contains(DataPipeline, DataJob)` | 单写 Relation；兼容读取可继续组装 tasks |
| `ETLTask.definition` | `DataJob.properties` + `executionProfile` + `definition` | 当前对象语义是加工工作定义；原生类型与标准化类型分开 |
| `inputs` / `outputs` / `columnMappings` | `LineageAssertion` | 三者翻译为一个加工事实，不再独立双写 |
| `runtimeSummary` | `runBinding` 或派生 `latestRunSummary` | 完整运行历史留在外部动态系统 |
| `MetricView.definition` | `SemanticModel.definition` | `METRIC_VIEW` 保留为 nativeKind |
| `dependencies` | typed Relation / Semantic definition refs | 区分 dependsOn、derivedFrom、usesDimension 等具体语义 |
| `QualityRule.qualityTargets` | typed `appliesTo` Relation | 规则定义和目标关系分开 |
| `Table.permissions` | `sourcePermissions` | 明确只是源系统权限知识 |

迁移期可以提供旧形状的组装读视图，但只能存在一个写入真相，即 **dual-read, single-write**，不能 dual-write。

## 10. 实施顺序与归属

### 阶段 1：冻结词表和兼容规则

- 形成 Entity type、Relation type、Aspect name 的正式词表；
- 明确每个源系统原生对象如何映射到 Canonical；
- 保留 `nativeKind`、`nativeType`、`nativeId` 和 `platformRef`；
- 冻结 `processingMode` 枚举，并为 `executionKind` / `purposes` 建立版本化受控词表和 `OTHER` 兼容规则；
- 将 Airflow DAG、dbt project 等编排容器映射为 DataPipeline，将实际加工单元映射为 DataJob；
- 不因 display name 或类型别名变化静默重铸既有 `object_id`。

若从旧类型迁移到新类型会导致身份变化，应显式生成替代对象和 replacement / mapping 证据，不复用旧 ID 冒充同一类型。

### 阶段 2：补全领域 Schema

- 为推荐 Aspect 定义真实字段、类型和必填规则；
- 定义 ref 目标类型、member key 和检索能力；
- 建立 Schema 版本及兼容测试；
- 先覆盖 TPC-H 验证集所需的最小集合。

### 阶段 3：落关系契约

- 定义通用 Relation 与数仓 relation types；
- 定义 LineageAssertion；
- 在 Preview / validation 中检查端点、方向、角色和引用完整性；
- 明确同一事实的幂等键与去重规则。

### 阶段 4：提供双向访问

- 建一跳 endpoint-to-relation 投影；
- 提供正向、反向和按 role 查询；
- 所有命中回读同一 Workspace pin 的 Canonical；
- 将多跳图分析留给上层有界遍历，不在底座加入通用图语言。

### 阶段 5：迁移场景与采集翻译

- 把现有 `inputs`、`outputs`、`columnMappings` 翻译为 LineageAssertion；
- 更新 Airflow、MySQL、StarRocks 等 source key 映射；
- 兼容旧读取形状，但停止旧形状写入；
- 更新 TPC-H fixture、验收路径和报告清单。

### 代码归属

| 内容 | 归属 |
|---|---|
| 通用 Relation 形状、单跳关系读取契约、关系投影与协议 conformance | 仓库根 `main`，确认协议缺口后实现 |
| Entity 名称、数仓 Aspect Schema、数仓 relation types | gitignored `.data/data-warehouse/knowledge/{physical,semantic}/schemas/`，稳定后迁独立知识提供方仓库 |
| Airflow / MySQL / StarRocks 翻译、source key、fixture | gitignored `.data/data-warehouse/`，稳定后整体迁出 |
| Schema 正式内容 | 通过 Writer 写入知识 Repository；落库前草稿仅放 `.data/` |

## 11. 本轮最终建议目录

后续设计与评审建议按以下目录推进：

1. 领域边界与非目标
2. Entity 判定原则
3. Canonical Entity 词表
4. 来源对象到 Canonical 的映射
5. DataJob 类型维度与受控词表
6. Entity 通用身份和 properties
7. 通用治理 Aspects
8. 物理资产 Aspects
9. 加工定义 Aspects
10. 语义层 Aspects
11. 质量 Aspects
12. Relation 通用契约
13. 数仓 Relation type 词表
14. 表级和列级 LineageAssertion
15. 关系正反访问与单跳投影
16. 跨 Repository 引用规则
17. Provenance、有效时间和动态观察
18. Schema 字段与检索声明
19. 引用完整性与 gate 规则
20. 旧模型兼容和迁移
21. Source connector 翻译规范
22. TPC-H 验证集与 conformance

## 12. 本轮已冻结与后续演进点

本轮 fixture 已冻结：Dimension / Measure 默认作为 SemanticModel keyed member；统一 Relation 外壳；LineageAssertion 由来源稳定 key 幂等；一跳关系读取直接覆盖固定 Workspace pin。以下仍属于未来演进：

1. 跨模型复用的 Dimension / Measure 何时升格为独立 Entity；
2. 设计血缘与运行观察血缘冲突时的展示及优先级；
3. `DataPlatformInstance` 是否在特定来源对外显示别名 `DatabaseService`；
4. 旧 `object_id` 的兼容周期和 replacement 关系；
5. 端点关系投影替代当前 reference scan 的规模化时机；
6. `executionKind` / `purposes` 词表的扩展审批和版本兼容方式。

默认演进原则：Dimension / Measure 只在独立治理时实体化；设计与运行血缘并存并标明 basis；关系查询遵守 Workspace pin；DataJob 类型采用多维字段，受控词表通过 `OTHER + nativeKind/nativeType` 前向兼容，不新增 SQLJob、StreamingJob 等顶级 Entity。

## 13. 参考资料

以下均为官方文档或项目规范。

### DataHub

- [DataFlow 与 DataJob 教程](https://docs.datahub.com/docs/api/tutorials/dataflow-datajob)
- [DataJob 实体与 Aspects](https://docs.datahub.com/docs/generated/metamodel/entities/datajob)
- [Dataset 实体与 Aspects](https://docs.datahub.com/docs/generated/metamodel/entities/dataset)
- [Lineage API 与正反向访问](https://docs.datahub.com/docs/api/tutorials/lineage)
- [Semantic Models API](https://docs.datahub.com/docs/api/tutorials/semantic-models)
- [Metrics and Semantic Models](https://docs.datahub.com/docs/features/feature-guides/metrics-and-semantic-models)

### OpenLineage

- [Object Model：Dataset、Job、Run](https://openlineage.io/docs/spec/object-model/)
- [Naming Convention](https://openlineage.io/docs/1.28.0/spec/naming/)
- [Facets](https://openlineage.io/docs/spec/facets/)
- [Job Type Facet](https://openlineage.io/docs/spec/facets/job-facets/job-type/)
- [Column Lineage Facet](https://openlineage.io/docs/spec/facets/dataset-facets/column_lineage_facet/)
- [Schema Dataset Facet](https://openlineage.io/docs/spec/facets/dataset-facets/schema/)

### OpenMetadata

- [Pipeline 与 Task Schema](https://docs.open-metadata.org/main-concepts/metadata-standard/schemas/entity/data/pipeline)
- [Database 层级](https://docs.open-metadata.org/v1.12.x/api-reference/data-assets/databases-overview)
- [Table Schema](https://docs.open-metadata.org/v1.12.x/api-reference/data-assets/tables)
- [Backend Entity 与 Relationship 存储](https://docs.open-metadata.org/v1.12.x/api-reference/main-concepts/backend-db)
- [Add Lineage API](https://docs.open-metadata.org/v1.12.x/api-reference/lineage/add)

### Apache Atlas

- [AtlasRelationshipDef](https://atlas.apache.org/api/v2/json_AtlasRelationshipDef.html)
- [Relationship REST API](https://atlas.apache.org/api/v2/resource_RelationshipREST.html)

### Google Dataplex / Knowledge Catalog

- [Catalog、Entry、Aspect 与 Entry Link](https://docs.cloud.google.com/dataplex/docs/catalog-overview)
- [Data Lineage LineageEvent](https://docs.cloud.google.com/dataplex/docs/reference/data-lineage/rest/v1/projects.locations.processes.runs.lineageEvents)
- [Lineage Views](https://docs.cloud.google.com/dataplex/docs/lineage-views)

### Databricks 与 Airflow

- [Databricks Lakeflow Jobs](https://docs.databricks.com/aws/en/jobs)
- [Unity Catalog Lineage System Tables](https://docs.databricks.com/aws/en/admin/system-tables/lineage)
- [Unity Catalog Data Lineage](https://docs.databricks.com/aws/en/data-governance/unity-catalog/data-lineage)
- [Metric Views](https://docs.databricks.com/aws/en/uc-semantics/metric-views)
- [Metric View Basic Modeling](https://docs.databricks.com/aws/en/uc-semantics/metric-views/basic-modeling)
- [Airflow Core Concepts](https://airflow.apache.org/docs/apache-airflow/stable/core-concepts/overview.html)
- [Airflow Tasks](https://airflow.apache.org/docs/apache-airflow/stable/core-concepts/tasks.html)

## 14. 决策一句话版本

> 数仓 Catalog 采用稳定 Entity 身份、按责任与变化节奏切分的 Aspect，以及单份 Canonical Relation；加工定义统一为 `DataPipeline contains DataJob`，具体 Job 类型用 `processingMode + executionKind + purposes + native type` 正交描述，PipelineRun / JobRun 留在动态运行侧；`SemanticModel` 统一语义容器，血缘用带加工上下文的 LineageAssertion 表达，正反访问由同一 Workspace pin 上的可重建关系投影提供，绝不两端双写。
