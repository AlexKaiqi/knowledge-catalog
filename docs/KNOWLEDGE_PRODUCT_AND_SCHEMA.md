# 知识接入、消费与 Schema 生命周期

本文从知识接入方、知识消费方和项目使用者的目标出发，定义产品能力、System
Repository、Meta Schema、Domain Schema、Canonical 目录和消费者文件视图。它细化
系统设计、组合、Aspect、Connector 和服务边界，不改变 ⓪–③ 的所有权。

人读派生产品说明（不进文档图、不承载独有决策）：[`product.html`](product.html)。

具体 Schema 字段、错误码和 API 形状由已选定的 Writer/Reader 公开合同拥有；本文拥有用户旅程、维护生命周期和目录约定。实现必须跟上这些旅程，不能把未做完的 U6/U10 改写成产品 Non-Goal。

---

## Goal

从知识接入方、知识消费方和项目使用者三条旅程定义产品能力、System/Meta/Domain Schema、Canonical 目录和消费者文件视图，并约束 Schema 生命周期。

## Non-Goals

- 不改变 ⓪–③ 所有权（文首）。
- 不在本文复制 Schema 字段、错误码和 API 形状。
- Schema 不是项目源码文件（`AGENTS.md` 红线；草稿只放 `.data/`）。
- 不把对象实例分页、空查询或 `*` 当作 BROWSE。
- 不把 git README 或文档图 `catalog-entry` 当作业务源说明。
- 不在源说明中定义领域分类、质量门槛或投影热状态。

## 硬性约束 / Invariants

- `S-01` Schema 只声明逻辑访问语义。
- 同一 Schema object ID 只允许兼容演进，否则 `SCHEMA_INCOMPATIBLE`；带 `schema_ref` 的 PUT 必须在 target 仓解析（系统设计与 Writer README）。
- System Repository 发布 Meta Schema，不是业务 Workspace 的隐式成员（`TERMINOLOGY.md`）。
- BROWSE 是 Catalog/知识集/源说明 + Schema/类型分页，不得变成对象 LIST（`TERMINOLOGY.md`）。
- 每个 Knowledge Repository 最多一个源说明；缺说明时不得由平台或模型补写。

## 选定方案 / 被否决方案

- 选定：三条旅程分开；Domain Schema 随目标 Knowledge Repository 版本化（细化 [ADR-023](KNOWLEDGE_CATALOG_DESIGN.md#adr-023)）。
- 选定：源说明是平台冻信封、接入方填正文的保留知识对象；`catalog show` 的 `repositories` 由应用层读该对象后拼装，不进 Catalog 登记表。
- 否决：把目录树或「知识开关」当成产品；消费方依赖无界公开 Knowledge LIST。
- 否决：对象实例分页 BROWSE；用 git README 或仓根路径当源说明。

## 接口契约 / 状态机

用户旅程和目录约定以本文为准（含 DISCOVER/BROWSE，且不得无界扫描或对象 LIST）。Writer/Reader 公开合同描述字段形状；源说明身份与信封见 `knowledge/README.md`。参考实现缺口见 `MVP_ACCEPTANCE.md`。


## 1. 产品结论

“知识”不是一个开关或文件树，而是三条相互连接但不能混同的用户旅程：

```text
知识接入方
  → 定义 Domain Schema、身份规则与 Connector
  → Preview / validate / publish / maintain

知识消费方
  → 发现 Server、Catalog、知识源与知识类型
  → BROWSE / SEARCH / READ / RELATIONS / PROVENANCE

项目使用者
  → 给已有项目选择一组知识
  → 固定 ResolvedWorkspace
  → 结构化消费 + 只读文件视图
```

平台必须分别提供：

1. **接入与维护面**：Schema、Connector、Preview、Writer、质量、运行状态；
2. **发现与消费面**：Catalog inventory、Schema/类型目录、Browse、Search、Read；
3. **项目附着面**：临时或命名知识集、固定 pin、mount plan、VFS 与 Agent 上下文。

文件浏览只属于第三条旅程。它不能代替 Server 发现、知识目录、结构化查询或接入维护。

---

## 2. 角色与责任

### 2.1 知识接入方

接入方是领域合同的定义者，而不是让平台猜 Schema 的数据提交者。接入方负责：

- 定义 Entity、Aspect、Member/Record、Relation；
- 定义稳定 `object_id` 规则；
- 定义 source key → Knowledge Address 映射；
- 定义字段类型、必填、引用类型与 `text/filter/sort` AccessHints；
- 定义来源、质量门槛、领域语义和兼容性策略；
- 提供 Connector 的源访问、Scope、全量/增量观察、翻译、checkpoint 与测试；
- 处理 Schema 演进、实例迁移、失败恢复和质量告警。
- 接受本 Catalog 的发现合同：仓一经登记，对持有 `catalog.read` 的已认证主体可出现在库存中；进入 discovery Workspace 后进入 Catalog 范围 SEARCH 候选。正文读权仍按仓 grant（`PERMISSIONS.md`）。源说明标题/摘要是库存信封，不要求成员 `knowledge.read`。

接入方拥有 Connector 的领域实现。平台拥有通用运行能力：构建、激活、身份、凭证引用、
调度、重试、checkpoint、运行证据和告警。Connector 只经 Writer typed API 写知识，不打开
Server Home，不直写 Git、Dolt 表或检索投影。

### 2.2 知识消费方

消费方在知道搜索词之前，先要理解“这里有什么、怎么找”。产品必须
让消费方看到：

- 当前 Server 和身份；
- 可见 Catalog；
- Catalog 内可发现的 Knowledge Repository 和命名知识集；
- 每个知识源的源说明（接入方维护的标题与摘要）或明示无说明；
- Entity / Aspect / Relation 的 Schema、可用查询字段；
- 请求时拼装的发布/投影/覆盖 claims（不是仓内对象，也不进源说明）；
- SEARCH / READ / RELATIONS / PROVENANCE / LOG 的固定版本结果（SEARCH 命中后的正文交付见 `PERMISSIONS.md`）；
- 无权、缺投影、能力不足、部分结果和真正零命中的区别。

owner 与授权边界来自 grant / provenance，不是源说明字段。领域分类和质量门槛
是接入方自己的治理叙事，不是平台信封。

公开消费不能依赖无界 authority 扫描，但也不能要求用户先猜关键词。产品需要有界、分页、
由投影或小型 Schema namespace 支持并携带 basis/coverage 的 **DISCOVER/BROWSE**：
Catalog / 知识集 / 源说明，加上该仓 Schema/类型目录。它不是对象实例目录，也不是
“返回内部所有文件”的 Knowledge LIST。空查询或 `*` 不是 BROWSE。未知对象走 SEARCH
（至少一条定位条件）；已知身份走 READ。

### 2.3 项目使用者与 Agent

项目使用者已经有宿主项目。其目标是把选中的知识附着到项目，而不是先学习并维护一个
Catalog Workspace 对象。产品语言使用“知识集”或“添加到项目”；后端仍使用：

```text
WorkspaceDefinition --resolve once--> ResolvedWorkspace
```

用户可以选择：

- 管理员发布的命名知识集；或
- Client 根据本次选择形成的临时 WorkspaceDefinition。

临时配方只用于本次任务，不要求在 Server 新建 Workspace。解析得到的 pin 在任务内固定；
跟随新版本必须显式重新 Resolve。Agent 同时可以使用结构化 Knowledge Client 和只读文件
视图，不能把 VFS 写入变成第二种 Write Surface。

### 2.4 平台运营方

运营方负责部署级而非领域级决定：

- 发布 System Repository；
- 配置 Catalog discovery knowledge set；
- 配置认证、授权、Connector runtime 和 Retrieval provider；
- 在接入方发布之后定义命名知识集（`workspace define`）；live Snapshot 投影由长寿命 `kc serve` 追 published HEAD；瞬时观察走 `operations projection notice`（只带定位，平台按 Binding 拉取）；`operations projection sync` 用于历史 pin、强制重建和排障；
- 管理健康、容量、备份与升级；
- 不替接入方发明对象身份、字段含义或源映射；
- 不把投影维护或 Catalog 组合教给消费方。

---

## 3. System Repository 与 Meta Schema

### 3.1 System Repository

每个 KC 部署必须暴露一个保留的 System Repository。参考实现使用稳定 Repository ID
`kr://kc/system`。它：

- 对所有已认证 principal 可发现、可读；
- 只有平台 release 能发布；运行时 Writer 对它不可写；
- 在初始化时创建并登记到每个 Catalog；
- 默认进入 Catalog discovery 范围，但不隐式加入每个业务 Workspace；
- 发布 Meta Schema、协议核心 Schema、示例和规范；
- 使用固定 release/digest，不跟随任意业务仓内容。

“所有可见”是读取和发现策略，不表示匿名公开，也不表示 System Repository 给用户授予其它
Repository 的权限。

### 3.2 启动信任根

Meta Schema 定义 Schema 文档本身。如果完全依赖 Repository 中的 Meta Schema 校验它自己，
会产生循环启动和可篡改信任根。因此：

1. Server 二进制内置支持的 Meta Schema ID、版本和 canonical digest；
2. System Repository 发布同一份可读知识；
3. 初始化和启动校验仓中对象与内置 digest 一致；
4. Domain Schema 声明或默认使用该 Meta Schema 版本；
5. Writer 使用内置的同版本 validator 校验 Domain Schema。

System Repository 是可发现、可引用和可审计的协议发布面；内置 digest 是启动信任根。

### 3.3 Meta Schema 与 `schema_ref` 不同

`metaSchema` 约束“Schema 文档”。普通知识单元的 `schema_ref` 约束“实例值”。两者不能
共用同一种解析规则：

```text
System Meta Schema --validates--> Domain Schema
Domain Schema      --validates--> Knowledge Address value
```

Domain Schema 和实例必须在同一 Knowledge Repository 的同一固定 basis 上解析。System
Meta Schema 是协议级特殊依赖，不把普通知识开放为跨仓浮动 Schema 引用。

### 3.4 首批系统对象

首批至少发布：

```text
schema/meta/schema-definition/v1
schema/core/resource-descriptor/v1
schema/core/relation/v1
schema/core/source-profile/v1
```

发布树与业务 Knowledge Repository 相同：全部 Schema 平铺在一个 `schemas/`
目录下。跟踪源是 `knowledge/system/schemas/`；见 §6.2。身份仍是
上面的 `schema/*` object_id。

其中 `schema/meta/schema-definition/v1` 定义：

- `entity`、可选 `aspect`；
- `pattern = record | keyed_collection`；
- `fields`；
- 字段 `type`、`required`、`access`、`refTypes`；
- `additionalProperties`；
- 支持的逻辑类型和 AccessHints。

### 3.5 源说明

源说明是 Knowledge Repository 的自描述对象，不是 git README，也不是文档图
`catalog-entry`。平台只冻槽位和信封：System Repository 发布协议 Schema；每个
Knowledge Repository 最多一个实例，保留身份由 `knowledge` 公开常量拥有。路径不是
身份。`schema_ref` 仍须在目标仓解析，因此目标仓要发布与 System 出版物一致的
Schema 副本，不得在该 object ID 上私自演进。

信封只有两段短文本（标题可过滤、摘要可检索）。接入方经 Writer 填正文。平台不审
「写得好不好」，只审形状。下列内容不进该对象：

- owner（grant / provenance）；
- 领域分类或质量门槛；
- Schema 列表（那是 Schema BROWSE）；
- 投影 READY / lag / coverage（请求时拼的热 claims）。

Catalog 核心仍只返回源与知识集身份。消费面 `catalog show` 的 `repositories` 由应用层 READ 保留对象后拼装。普通 Git 成员可以没有源说明。声称可发现的 Knowledge Repository
缺该对象时，失败关闭或列表明示「无说明」，不得由平台或模型补写。

---

## 4. Domain Schema 合同

### 4.1 身份与定位

Schema 是普通的版本化知识对象，`object_id` 必须以 `schema/` 开头。推荐命名：

```text
schema/<entity>/<aspect>/v<major>
schema/<entity>/entity/v<major>
schema/relation/<relation-type>/v<major>
```

同一 major 的兼容变化由 Repository commit 版本化；破坏性变化使用新的 major object ID。
目录不是身份，移动文件不能改变 Schema object ID。

### 4.2 最小形状

```yaml
---
object_id: schema/metric/definition/v1
---
metaSchema: schema/meta/schema-definition/v1
entity: Metric
aspect: definition
pattern: record
additionalProperties: false
fields:
  name:
    type: string
    required: true
    access: [text, filter]
  expression:
    type: string
    required: true
    access: [text]
  unit:
    type: string
    access: [filter]
```

首批逻辑类型：

```text
string / boolean / number / integer
object / record / array
object_ref / object_ref_list / relation_endpoint_list
```

AccessHints 仍只有 `text/filter/sort`。Schema 不声明 provider、analyzer、index、stored、summary
等物理检索词。

### 4.3 Address 与 Schema 匹配

- 有 `aspect` 且 `pattern=record`：约束同名 Aspect/Record；
- 有 `aspect` 且 `pattern=keyed_collection`：约束同名 Member；
- 无 `aspect`：约束 Entity blob 或 Relation；
- `required` 字段必须存在且非 `null`；
- 字段值必须满足声明类型；
- `additionalProperties=false` 时拒绝未声明字段；
- `object_ref*` 的 `refTypes` 是逻辑引用约束，跨仓存在性不在 Writer 内做隐式联邦扫描。

Schema 结构错误返回 `SCHEMA_UNSUPPORTED`；实例不符合已解析 Schema 返回
`SCHEMA_INSTANCE_INVALID`；引用无法在目标仓固定 basis 解析仍返回
`SCHEMA_REVISION_UNRESOLVED`；复用同一 Schema object ID 发布破坏性变化返回
`SCHEMA_INCOMPATIBLE`。

### 4.4 兼容性

兼容修改至少包括：

- 新增非必填字段；
- 放宽 `additionalProperties`；
- 增加不改变已有查询含义的 AccessHint。

破坏性修改至少包括：

- 删除或改名字段；
- 改字段类型；
- 非必填改为必填；
- `record` 与 `keyed_collection` 互换；
- 改 Entity/Aspect 归属；
- 收紧未知字段或引用类型。

兼容变化可以 PUT 同一 v1 Schema，但发布前必须验证该 commit 中所有引用实例。破坏性变化
发布 v2，允许 v1/v2 共存并由 Connector 显式迁移。删除 Schema 前必须证明没有引用者。

---

## 5. 维护生命周期

### 5.1 Schema 生命周期

```text
Provider 草稿
  → Meta Schema validate
  → compatibility diff
  → schema_ref impact analysis
  → affected-instance validate
  → Preview / Proposal
  → Writer COMMIT
  → projection rebuild when AccessSpec changed
  → published
```

首次 bootstrap 可以在同一 ChangeSet PUT Domain Schema 与引用它的实例。Schema 更新时，不能
只校验本次 PUT；必须通过 `schema_ref` 反向依赖索引检查当前 commit 的全部受影响实例。规模化
实现使用 bounded native lookup，不在消费请求中遍历 tree。

### 5.2 知识生命周期

```text
Source observation
  → Connector scope + source-key mapping
  → Address-level desired/observed diff
  → ChangeSet Preview
  → exact Domain Schema validation
  → quality / gate
  → Writer COMMIT or PROPOSAL
  → Snapshot advanced
  → rebuildable projection
```

每次 PUT/REMOVE 仍遵守 CAS、幂等、provenance 和单仓写入。Patch 不推断删除；FULL reconcile
只删除 `Observed ∩ Scope`。Schema、实例和 Connector 版本必须出现在运行证据中。

### 5.3 手工维护

人工修订也不直接编辑 Canonical authority。作者修改 Provider 工程中的草稿或提交 Proposal；
Preview 展示 Address 变化、Schema 影响和质量证据，Merge 后由同一 Writer 语义推进仓 Ref。

---

## 6. 三套目录规范

### 6.1 Provider Integration Repo

```text
provider-integration/
├── connector.yaml
├── schemas/
│   ├── physical/
│   │   ├── table.properties.aspect.yaml
│   │   ├── table.schema.aspect.yaml
│   │   └── column.properties.aspect.yaml
│   └── semantic/
│       ├── metric.definition.aspect.yaml
│       ├── metric.properties.aspect.yaml
│       └── semantic-model.definition.aspect.yaml
├── physical/
│   └── resources/
│       └── mysql-tpch-sql.yaml
├── semantic/
│   ├── metrics/
│   │   └── <encoded-object-id>/
│   │       ├── properties.yaml
│   │       └── definition.yaml
│   ├── semantic-models/
│   │   └── <encoded-object-id>/
│   └── relations/
│       └── <encoded-object-id>.yaml
├── mappings/
│   ├── object-id.md
│   └── source-key-tests.yaml
├── connector/
├── fixtures/
└── tests/
```

这里保存 Connector 代码、Schema 发布输入和测试，不是知识权威。凭证、endpoint、checkpoint
运行值和调度状态不进入 Knowledge Repository。

### 6.2 Canonical Knowledge Repository

推荐路径：

```text
knowledge-repository/
├── schemas/
│   ├── table.properties.aspect.yaml
│   └── metric.definition.v1.aspect.yaml
├── tables/
│   └── <encoded-object-id>/
│       ├── properties.yaml
│       └── schema.yaml
├── metrics/
│   └── <encoded-object-id>/
│       ├── properties.yaml
│       └── definition.yaml
├── relations/
│   └── <encoded-object-id>.yaml
└── resources/
    └── <encoded-object-id>.yaml
```

一个 Canonical 文件只承载一个 Address。Entity blob 不能与同一 `object_id` 的 Aspect/Member
文件混用。文件 frontmatter 保存 Address 与 `schema_ref`；正文可用结构化 YAML 或 JSON。
路径只是 `path_hint`，身份只由 Address 决定。无 `path_hint` 的 `schema/*` PUT
和 ingest 默认写入唯一的 `schemas/` 目录；实例按 Schema 实体类型分目录
（`tables/`、`metrics/`、`relations/`），不再使用含糊的 `objects/` 前缀。
System Repository 使用同一套 Schema 树：跟踪源 `knowledge/system/schemas/`
与发布后的平铺 `schemas/` 一致。

### 6.3 Consumer Semantic File View

Canonical 单元信封（`object_id` frontmatter）不是默认产品展示。对人、IDE 和通用 Agent 文件工具，应用层在固定 pin 上
生成只读、可丢、可重建的语义文件视图：

```text
knowledge/<knowledge-set>/
├── _meta/
│   ├── pin.yaml
│   └── sources.yaml
├── schemas/
│   └── metric.definition.v1.yaml
├── metrics/
│   └── <display-or-encoded-id>.yaml
├── semantic-models/
├── tables/
└── relations/
```

一个实体 YAML 可以组装其多个 Aspect，但必须保留 `_kc.object_id`、Repository、commit 和每个
Aspect 的 Schema。该视图不成为 Canonical，不接受写回；用户修改知识仍走 Proposal/Writer。

需要区分两种 VFS：

1. **Repository 文件 mount**：投影已有 Repository 子路径，只支持可逆路径前缀；
2. **Semantic file view**：从固定 Canonical Address 组装 YAML/Markdown，可按 Schema/类型选择。

前者适合本来就是文档/代码的 Repository；后者才适合 `metrics/*.yaml` 和 Agent `rg/grep`。

---

## 7. Server、Client 与插件能力

### 7.1 Server

Server 产品面需要：

- Server info 与 System Repository 坐标；
- 可见 Catalog、Repository、命名知识集 inventory（身份列表；`catalog show` 的 `repositories` 由应用层拼源说明）；
- discovery knowledge set；
- Schema/类型的分页 BROWSE 与源说明；
- 不是对象实例分页 BROWSE；
- fixed-basis SEARCH/READ/RELATIONS/PROVENANCE/LOG；
- Writer 的 Meta Schema、Domain Schema 和实例校验；
- Connector registry/runtime 的 manifest、run、checkpoint、health；
- Workspace File Gateway 和 Semantic File View Gateway；
- quality、coverage、projection basis/lag 和访问证据。

Catalog Core 仍只组合 Repository/commit；Knowledge Server 解释 Schema/Address；Integration
Runtime 托管 Connector；文件网关只交付已批准的固定视图。

### 7.2 Client

Client 需要提供用户级操作，而不只是 DTO：

- `ConnectServer` / `DiscoverCatalogs`；
- `BrowseKnowledge` / `DescribeSchema`；
- `OpenKnowledgeSet` / `CreateTemporaryDefinition`；
- 任务级固定 `ResolvedWorkspace`；
- `Search` / `Read` / `Relations` / `Provenance`；
- `PreviewMount` / `Mount` / `Unmount` / `ResolveAgain`；
- Provider 侧 Schema validate、ID mapping test、Connector Preview 与 run status。

具体源客户端和 Connector 运行宿主不进入核心 `kc` CLI；它们通过 Integration SDK/服务调用
公开 Writer surface。

### 7.3 插件

插件至少有三个不同入口：

1. **知识目录**：发现 Catalog、知识源说明、Schema/类型、健康和质量 claims；
2. **添加到项目**：选择命名知识集或临时组合，预览并执行只读挂载；
3. **已接入知识**：显示 pin、Repository/commit、mount、更新和故障状态。

文件树开关只能叫“显示已挂载知识文件”。它只改变人用视图，不连接 Server、不选择知识、
不挂载、不发权，也不改变 Agent 能力。实际连接、挂载、卸载和版本更新是独立显式动作。

---

## 8. 用例

### U1：新部署发布 System Repository

```gherkin
Given 一个没有 Repository 的新 KC Home
When 宿主初始化第一间 Catalog
Then System Repository 被创建并登记
And Meta Schema digest 与 Server 内置信任根一致
And 普通已认证用户可以读取 System Schema
And 普通用户不能写 System Repository
```

### U2：接入方定义 Domain Schema

```gherkin
Given 接入方读取了 System Meta Schema
When 它提交一个带未知 type 或 access 的 Domain Schema
Then Writer 返回 SCHEMA_UNSUPPORTED
And Repository HEAD 不移动

When 它提交符合 Meta Schema 的 Domain Schema
Then Schema 作为 schema/* 知识对象被版本化
And DESCRIBE_SCHEMA 返回规范化字段和 AccessHints
```

### U3：实例必须符合 Schema

```gherkin
Given schema/metric/definition/v1 要求 name 和 expression 为 string
When Connector PUT 缺少 expression 或提供错误类型
Then Writer 返回 SCHEMA_INSTANCE_INVALID
And Repository HEAD 不移动

When Connector PUT 合法 Metric definition
Then Writer COMMIT 成功
And READ 返回固定 commit 的值与 schema_ref
```

### U4：首次同批发布

```gherkin
Given 一个空 Domain Knowledge Repository
When 同一 ChangeSet PUT Domain Schema 和引用它的实例
Then Writer 使用批内 Schema 校验实例
And 两者原子进入同一 commit
```

### U5：Schema 演进

```gherkin
Given 多个实例引用 schema/table/properties/v1
When 接入方把已有必填字段改为另一类型
Then 兼容性检查把它标记为 breaking
And 平台要求新建 v2 或提供完整迁移证据

When 只新增非必填字段
Then 可以更新 v1
And AccessSpec 改变时投影进入 rebuild
```

### U6：消费方发现知识

```gherkin
Given 用户只知道 Server 地址
When 打开知识目录
Then 能看到 System Repository、可见 Catalog、知识源说明和知识集
And 源说明来自接入方维护的保留对象，或明示无说明
And 能按 Schema/类型分页浏览而不先猜搜索词、也不枚举对象实例
And 响应声明 basis、coverage 与权限裁剪
```

### U7：给已有项目添加知识

```gherkin
Given 用户已打开一个普通代码项目
When 选择一个命名知识集或临时选择若干知识源
Then Client 只 Resolve 一次并展示固定 pin
And 用户确认后才建立只读 mount
And 用户原有项目文件不被复制或覆盖
```

### U8：Agent 使用语义文件视图

```gherkin
Given 当前项目挂载了 Metric 语义文件视图
When Agent 执行 rg 或 grep
Then 它看到 metrics/*.yaml 的组装业务值
And 每个文件保留 object_id、Repository、commit 与 schema_ref
And 它不会把 Canonical 单元信封误当成业务 Schema
```

### U9：显示开关不改变接入状态

```gherkin
Given 当前项目已经挂载知识
When 用户关闭“显示已挂载知识文件”
Then 插件只隐藏人用文件树
And mount、pin、权限和 Agent 文件访问保持不变
```

### U10：性能与失败解释

```gherkin
Given 本机 Server 和已建立的任务上下文
When 插件展示连接状态
Then 不扫描文件树

When 用户展开一个目录
Then 只读取一页直接子项
And Browser、Plugin、Gateway、authority 分段记录耗时
And 超过本机 p95 100ms 的目录首屏被视为性能回归
```

---

## 9. 不变量

- System Repository 公开可读不代表其它 Repository 公开；
- Meta Schema 是协议启动信任根，Domain Schema 不是；
- 普通知识的 `schema_ref` 必须在目标 Repository 固定 basis 内解析；
- Schema 是知识，正式版本只经 Writer；Provider 工程中的 YAML 是发布输入；
- Canonical 每文件一个 Address；路径不是 object ID；
- Consumer Semantic File View 可丢、只读、固定 pin，不是权威；
- Workspace 是组合配方，不是接入方写入前置条件；
- 插件不拥有 Catalog、Schema、Connector 或权限语义；
- Connector 不拥有 Writer、Snapshot authority 或 Retrieval provider；
- DISCOVER/BROWSE 不得退化为消费请求中的无界 authority 扫描或对象 LIST。
- 源说明每仓至多一个；缺说明不得由平台或模型补写。

---

## 10. 实然不在本文

U1–U10 是应然旅程。完成度、插件 V1 只展示了哪些库存、以及「不得把未提供的能力假装已有」，全部由 `MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` 拥有。BROWSE 属于 U6：源说明 + Schema 分页，不是对象 LIST。
