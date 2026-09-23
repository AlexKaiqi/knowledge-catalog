# 知识接入、消费与 Schema 生命周期

本文从知识接入方、知识消费方和项目使用者的目标出发，定义产品能力、System
Repository、Meta Schema、Domain Schema、Canonical 目录和消费者文件视图。它细化
系统设计、组合、Aspect、Connector 和服务边界，不改变 ⓪–③ 的所有权。

人读派生产品说明（不进文档图、不承载独有决策）：[`product.html`](product.html)。

具体 Schema 字段、错误码和 API 形状由已选定的 Writer/Reader 公开合同拥有；本文拥有用户旅程、维护生命周期和目录约定。实现必须跟上这些旅程，不能把未做完的 U6/U10 改写成产品 Non-Goal。

---

## Goal

从知识接入方、知识消费方和项目使用者三条旅程定义产品能力：交付客户端封装唯一服务入口，用户在既定策略内自助完成 Snapshot 申请或连接、接入登记、知识发布维护与消费；定义 System/Meta/Domain Schema、Canonical 目录和消费者文件视图，并约束 Schema 生命周期。

## Non-Goals

- 不改变 ⓪–③ 所有权（文首）。
- 不在本文复制 Schema 字段、错误码和 API 形状。
- Schema 不是项目源码文件（`AGENTS.md` 红线；草稿只放 `.data/`）。
- 不把对象实例分页、空查询或 `*` 当作 BROWSE。
- 不把 Meta Schema 做成实例字段基类，也不用类型继承代替 `schema_ref`。
- 不把无 frontmatter 的 git 文件或文档图 `catalog-entry` 当作业务 README。
- 不在 README 中定义领域分类、质量门槛或投影热状态。
- 不把 Snapshot 连接、凭证或部署配置变成 Catalog 协议，也不以自助接入替代认证与授权（`SERVICE_ARCHITECTURE.md`、`PERMISSIONS.md`）。

## 硬性约束 / Invariants

- `S-01` Schema 只声明逻辑访问语义。
- 同一 Schema object ID 的演进必须满足兼容性约束；带 Schema 引用的 PUT 必须在 target 仓解析（系统设计与 Writer README）。
- System Repository 发布 Meta Schema，不是业务 Workspace 的隐式成员（`TERMINOLOGY.md`）。
- BROWSE 是 Catalog/知识集 + 单仓已发布实体名单，不得变成对象 LIST（`TERMINOLOGY.md`）。名单不是 Schema 正文，也不是 Canonical 信封。README 走 READ/SEARCH，不是库存列。
- 每个 Knowledge Repository 最多一份约定路径 README；缺则不得由平台或模型补写，也不得展平成 Catalog `title`/`summary`。

## 选定方案 / 被否决方案

- 选定：三条旅程分开；Domain Schema 随目标 Knowledge Repository 版本化（细化 [ADR-023](KNOWLEDGE_CATALOG_DESIGN.md#adr-023)）。
- 选定：部署方一次性建立运行能力与显式准入策略，交付客户端封装服务地址；后续申请、连接、登记、维护和选源由用户经 Server 自助完成。自动授权只能来自显式策略，登记本身不发权。
- 选定：README 是 Markdown 知识对象（`schema/core/readme/v1` Aspect `readme`）；身份是 Address + `schema_ref`。仓根 `README.md` 只是人写与 git 直推解释用的有界 `path_hint`，不是 SEARCH 合同，也不是 Catalog 库存列。默认 SEARCH 编该对象 `body` 的 `text` 访问（同一 hydrate、无 `knowledge.read` 剥正文），不走文件 contains，也不另造检索文档。
- 选定：VFS / File Gateway 投影已固定 pin 上的树；Catalog attach 才是成员登记。Plain 仓只解释组合阶梯，不是 VFS 的前提。
- 选定：单仓 Schema BROWSE（`schema list`）只说明该仓已发布 basis 上有哪些实体。合同走 READ `schema/*`；检索形状走 DESCRIBE_SCHEMA。这不是对象 LIST。
- 选定：Schema 文档可选 `description` 说明该实体；不是固定元信息、不是仓 README、不是 Catalog title/summary。list 可带这一句。
- 选定：跨类型最少可访问的是固定元信息；实例字段只来自该类型 Domain Schema 与 `schema_ref`；仓说明是 README。Entity / Aspect / Relation / Member 是 Address 种类，不是四种 Domain Schema。Meta Schema 只约束 Schema 文档怎么写。
- 选定：Canonical 仓内 `schema/*` 默认平铺在唯一的 `_schemas/`，与实例类型目录分开排序。身份仍是 Address；`schema list` 仍走有界 Schema 定位，不扫目录。
- 选定：Bound State 访问原点写在 Domain Schema Canonical frontmatter 的 `origin`；`kc access` 用 origin + 实体 ID 取回该 Aspect，不另存空实例。接入方随 Schema 自助发布，不要求部署方按源改 Server。`origin` 不是字段 AccessHint，也不是源库账号。
- 选定：LakeFS 托管仓的协议仓 ID 就是 Graveler 名（`--name` 的合法 slug）。`kc --repo table-meta` 与 lakeFS `table-meta` 相同。commit 只出现在 pin / ref。Gitea 中文名仍可用逻辑坐标。
- 否决：为 LakeFS 业务仓再生成 `kr://<用户>/repo-<hash>` 当作另一套仓名。
- 否决：为瞬时值再 PUT 一份空 Aspect；把访问 URL 写进实例正文或 Catalog 登记表。
- 否决：默认把 Schema 和实例类型目录混排在 `schemas/`；把实例收回含糊的 `objects/`；用目录扫描做 `schema list`。
- 否决：Domain Schema 继承 Meta Schema 的实例字段；发布 `schema/core/entity/v1` 当作所有实体超类；把 Address 种类当成 `schema list` 里的实体。
- 否决：对象实例分页 BROWSE；list 返回 Schema 正文或 DESCRIBE_SCHEMA；无 frontmatter 的仓根路径当 README 或可检索知识；`core/source-profile` 的 title/summary 信封；把 README 展平成 `kc show` 的 title/summary；把根 README 做成第二条检索代数。
- 否决：每接入一个源都要求部署方修改配置或交付服务端秘密；消费方必须等待运营方新建命名知识集。

## 接口契约 / 状态机

用户旅程和目录约定以本文为准（含 DISCOVER/BROWSE，且不得无界扫描或对象 LIST）。Writer/Reader 公开合同描述字段形状；README 身份与字段见 `knowledge/README.md`。参考实现缺口见 `MVP_ACCEPTANCE.md`。


## 1. 产品结论

### 1.1 产品定位、架构与能力边界

Knowledge Catalog 是面向大规模数据的通用知识底座。它集成 lakeFS 等已有版本化存储，在其上
提供 Dataset 组合与交付、自定义知识模型，以及声明式索引与统一访问。

Snapshot Store 是系统的存储适配接口，不是另一个自研版本化存储引擎。普通文件版本管理可由
Git 承担；大文件、大量文件由 lakeFS 与对象存储等方案承载。文件版本与底层存储规模能力属于
所集成的组件；本系统在其上承担数据集范围、独立消费授权、知识解释、索引维护与统一访问。
整体容量和运行资格仍需系统级验证，不能由底层组件的能力直接推出。

系统按 `LAYERS.md` 分为⓪–③四个逻辑层。文件存储与数据集组合不依赖知识声明或检索服务；
知识层解释文件中的业务结构，检索层依据知识声明维护索引。逻辑分层不限定服务的部署数量。

| 核心层 | 负责的能力 | 使用者获得什么 |
|---|---|---|
| ⓪ Snapshot Store | 适配已有存储的文件、目录、快照、分支、提交与历史接口 | 复用 lakeFS、Gitea 等组件的存储和版本能力，接入上层交付与知识读写流程 |
| ① Dataset / Catalog | 来源登记、文件范围切分与跨仓组合、冻结版本的 Dataset 发布；消费授权由应用授权入口执行 | 按用途组织和交付数据集，独立授予消费权，不必开放整个源仓 |
| ② Knowledge | 自定义 Schema、业务对象、字段、Aspect、关系、解释与读写 | 使文件内容具有可声明、可校验和可引用的业务含义 |
| ③ Retrieval | 由知识声明驱动的索引与检索；上层访问缓存与预热由应用装配 | 高效查询大量数据，接入方无需逐次维护索引和缓存 |

统一知识访问是消费入口，使用②的知识读取语义与③的检索、加速能力，不另立编号层。
源侧动态运行时连接②/③，实际观察值不落进 Snapshot；VFS 基于①固定的文件范围，
供本地 Agent 只读访问本机放得下的 Dataset。
lakeFS、索引引擎和缓存分别承载相应层的存储与访问能力，不是独立的协议层。
版本与权限贯穿相应入口，维护通道与消费通道的职责仍按各 owner 分责。

单仓知识访问由⓪与②提供，不依赖 Dataset；跨仓组合通过①确定文件范围和来源版本。
②的对象标识用于持续寻址、引用与关联，不随文件路径变化。

Snapshot Store 与 Dataset 构成不依赖知识解释的独立采用范围（`COMPOSITION.md`）；
普通文件不必先声明 Schema。知识声明负责业务含义，Dataset 负责文件范围、组合和发布，
不按知识对象解释文件。Dataset 消费授权独立于源仓维护授权（`PERMISSIONS.md`），
定义或发布 Dataset 本身不隐式发权。

声明式索引的价值是接入方只描述数据与查询需求，不负责操作检索引擎、逐次触发重建或管理缓存。
部署方仍需提供对应运行能力。统一访问覆盖已声明能力上的检索与读取，不承诺任意查询语言、
任意计算或自动理解未声明文件。外部瞬时状态与本地 VFS 是可选接入/消费方案，不是每个用户的前置条件。

### 1.2 发布与消费的目标体验

Dataset 是面向消费者的稳定交付名称。日常消费默认采用最新已发布 Dataset，系统在单次操作内
固定版本；需要复核时保留具体坐标。上游仓提交后需重新发布 Dataset，既有 Dataset 版本不被改写
（`COMPOSITION.md`、`TERMINOLOGY.md`）。普通 CLI 不要求用户管理 pin 文件。

进一步的产品规划目标是：面向知识消费的 Dataset 新版本在正文与所需索引准备好后，统一切换
消费者可见的服务版本，默认消费最新可服务版；查询、分页及正文回读不混版。这里的原子性是
消费者可见的版本切换，不是多个源仓与索引引擎之间的写事务，也不要求文件型 Dataset 安装检索服务。
现有固定版本、异步索引维护与同依据回读是基础，不能据此声称完整切换闭环已经验收；就绪条件、
切换失败与恢复机制仍需在组合、检索与投影控制 owner 中细化并验证。本段不新增协议字段或状态机。

产品文档应分别列出已有能力、采用条件和仍待验证的目标。大规模容量、完整动态部署与版本切换
资格不能由“接口已存在”推导；也不能因当前实现尚未闭环而从产品目标中删除。

### 1.3 用户旅程

“知识”不是一个开关或文件树，而是三条相互连接但不能混同的用户旅程：

```text
知识接入方
  → 在交付客户端登录，自助申请平台 Snapshot 或连接自己维护的 Snapshot
  → 按既定策略完成连接验证、Catalog 登记和维护授权
  → 定义 Domain Schema、身份规则与 Connector
  → Preview / validate / publish / maintain

知识消费方
  → 在同一客户端登录，发现 Catalog、知识源与知识类型
  → 选择 Repository 或已发布 Dataset，系统固定本次操作版本
  → BROWSE / SEARCH / READ / RELATIONS / PROVENANCE

项目使用者
  → 给已有项目选择一组知识
  → 默认消费最新已发布 Dataset，需要复核时保留具体版本坐标
  → 结构化消费 + 只读文件视图
```

平台必须分别提供：

1. **接入与维护面**：Schema、Connector、Preview、Writer、质量、运行状态；
2. **发现与消费面**：Catalog inventory、已发布实体名单、Browse、Search、Read；
3. **项目附着面**：临时或命名知识集、固定 pin、mount plan、VFS 与 Agent 上下文。

文件浏览只属于第三条旅程。它不能代替 Server 发现、知识目录、结构化查询或接入维护。

---

## 2. 角色与责任

### 2.1 知识接入方

接入旅程从客户端登录、选择并注册知识源开始。接入方可以自助申请平台提供的 Snapshot
仓库，也可以连接自己维护、符合 Snapshot 合同的仓库。两条路径共用接入、发布、读取与
治理语义，仓库的托管位置不改变知识身份和授权边界。

部署方在交付服务时建立存储供给、连接验证、凭证管理和显式准入策略。后续由接入方经
Client 提交申请或连接授权，Server 按策略完成仓库准备、连接验证和登记；日常接入不要求
部署方逐仓修改配置、传递秘密或代执行命令。自有仓的内容与版本历史保留在原 Snapshot，
平台仓的创建与准备属于服务管理能力；Catalog 登记仍只验证已有可访问的发布版本并保存
成员关系。连接配置与凭证属于 Server 管理面，不进入 Catalog 协议或知识正文。

接入方在显式策略允许的范围内取得该仓维护权限，并管理允许的消费分享。认证、连接、
登记和授权分别求值；登记本身不隐式发权，自动授权必须有独立、可追溯的策略依据，不能
把拥有源仓账号等同于拥有任意 KC 权限。超出策略范围时，客户端应说明缺少的权限或能力。
普通文件也不会因登记而自动成为结构化可检索知识；带知识 frontmatter 的文件（含 README）
才是知识单元。Connector / typed API 仍走 Writer；已接入 tree 仓也可以对 published
ref 直推，Reader 按文件解释，live 投影对 HEAD 对账。直推不等于 Writer 校验。

平台仓创建是明确的 Client 请求。成功后接入方即取得稳定仓身份，可在创建策略明确授予的范围内预览、发布和回读；不得要求其先获得全局授权管理权，或让部署者事后逐仓补连接。创建结果、服务管理连接和实际授权规则随服务耐久保存。重试创建与替换实例恢复旧结果，不重新授予已撤销的权限。接入自有仓继续保留只读验证与成员登记的独立边界。

用户只提供可读仓名，并在存在多个获准 Store 时选择其一。LakeFS 上该名称就是协议仓 ID，与 Graveler 仓库名相同；服务仍生成恢复命令，但不另造仓名。Gitea 在无法作为远端仓库名的可读名上才生成逻辑坐标。
用户可在自己的仓库存中重新取得管理地址，不需要整个 Catalog 的发现权限。Gitea 的同名
账号供给与知识所有者保持一致，既有同名账号不能仅凭名称认领。托管仓的管理页由 KC
在同一认证边界内提供；不能把目录或未同步镜像伪装成 provider 原生仓页面。
原生 Store 页需要已配置的共同登录能力；该能力尚未准备时应明确报告，并可通过 KC
管理页访问当前仓状态。Snapshot 可写与原生网页可登录是不同的准备状态。
接入原子性、连接与授权边界遵循 `COMPOSITION.md` / `SERVICE_ARCHITECTURE.md` /
`PERMISSIONS.md`；具体适配类型和操作形状由公开实现合同拥有，当前自助能力缺口由
`MVP_ACCEPTANCE.md` 记录。

接入方是领域合同的定义者，而不是让平台猜 Schema 的数据提交者。接入方负责：

- 定义 Entity、Aspect、Member/Record、Relation；
- 定义稳定 `object_id` 规则；
- 定义 source key → Knowledge Address 映射；
- 定义字段类型、必填、引用类型与 `text/filter/sort` AccessHints；
- 定义来源、质量门槛、领域语义和兼容性策略；
- 提供 Connector 的源访问、Scope、全量/增量观察、翻译、checkpoint 与测试；
- 处理 Schema 演进、实例迁移、失败恢复和质量告警。
- 接受本 Catalog 的发现合同：仓一经登记，对已认证主体默认可出现在库存中（Catalog 可声明 private）；进入 discovery Workspace 后进入 Catalog 范围 SEARCH 候选。正文读权仍按仓 grant（`PERMISSIONS.md`）。README 不是库存展示列；读/搜它仍要成员 `knowledge.read` / `knowledge.search`。

接入方拥有 Connector 的领域实现。平台拥有通用运行能力：构建、激活、身份、凭证引用、
调度、重试、checkpoint、运行证据和告警。Connector 只经 Writer typed API 写知识，不打开
Server Home，不直写 Git、对象存储或检索投影。

### 2.2 知识消费方

消费方在知道搜索词之前，先要理解“这里有什么、怎么找”。产品必须
让消费方看到：

- 当前 Server 和身份；
- 可见 Catalog；
- Catalog 内可发现的 Knowledge Repository 和命名知识集；
- 各仓 README 作为知识对象可读可搜（`kc read --aspect readme` / SEARCH `body`），不是 `kc show` 的 title/summary；
- 各仓已发布实体名单（有界 `schema/*`）；合同走 READ，查询字段走 DESCRIBE_SCHEMA；
- 请求时拼装的发布/投影/覆盖 claims（不是仓内对象，也不进 README）；
- SEARCH / READ / RELATIONS / PROVENANCE / LOG 的固定版本结果（SEARCH 命中后的正文交付见 `PERMISSIONS.md`）；
- 无权、缺投影、能力不足、部分结果和真正零命中的区别。

owner 与授权边界来自 grant / provenance，不是 README 字段。领域分类和质量门槛
是接入方自己的治理叙事，不是平台信封。

消费方在当前授权范围内自主选择源仓或已发布 Dataset。仓级入口按仓检查授权；Dataset
消费按其独立消费授权检查发布清单，不要求因此授予整个源仓的读权。定义组合不隐式发权，
也不能把自助消费解释成所有已认证用户自动获得正文读权。应用接口需要临时组合或固定上下文时，
由 Client 形成配方并固定坐标；普通 CLI 消费不要求手工管理 pin。

公开消费不能依赖无界 authority 扫描，但也不能要求用户先猜关键词。产品需要有界、分页、
固定 basis 的 **DISCOVER/BROWSE**：Catalog / 知识集，加上该仓已发布实体名单（`schema/*`
命名空间，只点名实体，不返回合同正文）。它不是对象实例目录，也不是 DESCRIBE_SCHEMA。
空查询或 `*` 不是 BROWSE。未知对象走 SEARCH（至少一条定位条件）；已知身份走 READ，
包括打开一份 Schema。README 用 READ/SEARCH，不进库存列。

### 2.3 项目使用者与 Agent

项目使用者已经有宿主项目。其目标是把选中的知识附着到项目，而不是先学习并维护一个
Catalog Workspace 对象。产品语言使用“知识集”或“添加到项目”；后端仍使用：

```text
KnowledgeSet --resolve once--> ResolvedKnowledgeSet
```

用户可以选择：

- 获授权的维护者发布的命名知识集；或
- Client 根据本次选择形成的临时 KnowledgeSet。

临时配方只用于本次任务，不要求在 Server 新建 Workspace。解析得到的 pin 在任务内固定；
跟随新版本必须显式重新 Resolve。Agent 同时可以使用结构化 Knowledge Client 和只读文件
视图，不能把 VFS 写入变成第二种 Write Surface。

### 2.4 平台运营方

运营方在首次交付时配置服务能力与默认策略，持续负责服务运维；不进入每次接入、发布或
消费任务的操作链。其职责是：

- 发布 System Repository；
- 建立平台 Snapshot 供给、自有 Snapshot 连接与凭证管理能力；
- 配置认证、显式准入与授权策略、Connector runtime 和 Retrieval provider；
- 配置 Catalog discovery 的发布准入与维护策略；登记不等于进入已发布搜索范围，不能自动搜索所有登记仓；
- 让获授权的维护者管理共享命名知识集，消费方仍可自行临时选源；
- 由后台服务追踪 published Snapshot 并维护检索投影，提供发布与检索就绪状态；人工重建只用于历史版本、恢复和排障；
- 管理健康、容量、备份与升级；
- 不替接入方发明对象身份、字段含义或源映射；
- 不把投影维护或 Catalog 组合教给消费方。

---

## 3. System Repository 与 Meta Schema

### 3.1 System Repository

每个 KC 部署必须暴露一个保留的 System Repository。参考实现使用稳定 Repository ID
`kr://kc/system`。它：

- 对所有已认证 principal 可发现；可读来自仓的已认证默认声明，不是保留 ID 特权；
- 只有平台 release 能发布；运行时 Writer 对它不可写（不可变发布面，不是授权特例）；
- 在初始化时创建并登记到每个 Catalog；
- 默认进入 Catalog discovery 范围，但不隐式加入每个业务 Workspace；
- 发布 Meta Schema、协议核心 Schema、示例和规范；
- 使用固定 release/digest，不跟随任意业务仓内容。

发现仍走 Catalog 登记：公开 Catalog 的库存能看见它。读权走 `PERMISSIONS.md` 的已认证默认
或显式 grant，与任何其它仓相同。这不是匿名公开，也不把其它 Repository 放行。部署配置写
`repositoryAccess`；参考实现把 System Repository 列为推荐声明，授权器不得写
`if repo == kr://kc/system`。

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
schema/core/readme/v1
```

发布树与业务 Knowledge Repository 相同：全部 Schema 平铺在一个 `_schemas/`
目录下。跟踪源是 `knowledge/system/schemas/`；见 §6.2。身份仍是
上面的 `schema/*` object_id。

Meta Schema 规定 Domain Schema 如何声明适用对象、Address 单元、字段约束与逻辑访问语义。
系统对象清单与确切字段由 [`knowledge/README.md`](../knowledge/README.md)、
`knowledge/system/schemas/` 和 Schema 公开解释器维护，本文不建立另一份词表。
`schema/core/relation/v1` 出现在这份清单里，是因为它约束关系记录的值；Entity / Aspect
不是清单上的 Schema，它们是 Address 种类。

### 3.5 README

README 是 Knowledge Repository 的自描述 **Markdown 知识单元**，不是 Catalog 库存的
title/summary，不是无 frontmatter 的 git 文件，也不是文档图 `catalog-entry`。
System Repository 发布协议 Schema `schema/core/readme/v1`（entity `Readme`、aspect
`readme`、字段 `body` 声明 `text`）。人写仓根 `README.md`：frontmatter 用 `entity` /
`aspect` 承载 Address，正文是 markdown。身份是 Address；根路径只是人写与 git 直推
解释用的有界 `path_hint`，不是 SEARCH 身份，也不是 `catalog show` 的列。同一仓可以
有其它 markdown 单元，约定路径只这一份 README。`schema_ref` 仍须在目标仓解析，因此
目标仓要发布与 System 出版物一致的 Schema 副本，不得在该 object ID 上私自演进。

下列内容不进该对象：

- owner（grant / provenance）；
- 领域分类或质量门槛；
- Schema 列表（那是 Schema BROWSE）；
- 投影 READY / lag / coverage（请求时拼的热 claims）。

Catalog 核心与消费面 `catalog show` 的 `repositories` 都只返回源身份（及
`schemaCount`）。不要把 README 第一行/其余正文展平成 `title`/`summary`。读 README
走 `kc read --aspect readme`；默认 SEARCH 编该知识对象 `body` 的 `text` 访问，走现有
AccessHints / hydrate / 交付链；无 `knowledge.read` 时仍剥正文。不要把根文件
contains 或一份伪造检索文档当成 SEARCH。LLM concat 是消费形态，不挡这一步。声称可
发现不等于必须有 README；缺 README 不得由平台或模型补写，也不从库存抹仓。

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

### 4.2 最小声明

接入方需要声明 Schema 适用于哪类对象和哪种 Address 单元，以及字段的逻辑类型、必填性、
引用约束和允许的逻辑访问方式。这样 Writer 能验证实例，消费方能理解值，而不依赖某个
存储或检索产品。规范格式、支持类型与示例以 [`knowledge/README.md`](../knowledge/README.md)、
[`knowledge/schema-document.schema.yaml`](../knowledge/schema-document.schema.yaml) 和内置 System Schema 为准。

AccessHints 仍只有 `text/filter/sort`。Schema 不声明 provider、analyzer、index、stored、summary
等物理检索词。需要墙外 hydrate 时，同一份 Domain Schema 在 Canonical frontmatter 写 `origin`（http(s) 原点，不含 `/v1/access`）。访问协议是 origin + 实体 `object_id`，返回该 Schema 的 Aspect。

### 4.3 Address 与 Schema 匹配

声明必须与目标 Address 的实体、Aspect 或成员单元相符。Writer 在目标仓同一固定 basis 上
校验必填值、逻辑类型、额外字段和引用类型；批内 Schema 与批内实例可以一起校验。
引用类型约束不意味着 Writer 可以跨仓扫描来证明外部对象存在。

声明不合法、引用无法解析或实例违反声明时，必须拒绝整批发布并保持目标版本不动。
精确匹配规则与错误码由 [`knowledge/writer/README.md`](../knowledge/writer/README.md)
及 Schema 公开解释器拥有。

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

开放决策（REVIEW-03）：本节要求 breaking 发布新 major，U5 还允许「提供完整迁移证据」。
需确认是否允许在同一 Schema 身份上进行 breaking 迁移；决定会影响 Schema 身份延续、
实例迁移与发布验证的原子边界。未决前不能把当前实现未支持等同于产品禁止，也不能把 U5
的迁移证据分支当作已交付入口。

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

人工修订也不直接编辑 Canonical authority。作者读取当前发布版本和来源，修改 Provider
工程中的草稿，保持要修订单元的 Address；预览展示变化、Schema 影响和质量证据。没有评审
要求时通过 Writer 发布；需要评审时提交 Proposal，经验证和 Merge 后由同一 Writer 语义
推进仓 Ref。重复发布、删除和恢复都遵守同一单仓写边界。

一次新的修订使用新的命令身份。网络中断后先查询原命令结果；重试同一提交时保留原命令
身份和内容，不能因未收到响应就生成另一笔发布。若当前版本已被别人推进，重新读取最新
内容并核对差异，合并自己的修改后重新预览和提交；不得取消并发前置条件来强行覆盖。

发布后先按回执版本回读，确认值、Schema 与来源，再检查检索是否已覆盖该版本。正文发布
与检索就绪分开呈现；后台追踪和重建投影，接入方无需为每次修订联系部署方刷新索引。

### 5.4 消费更新、移除与恢复

源仓发布新 commit 不会改写既有 Dataset。Dataset 维护者重新发布后，日常消费在下一次操作中
默认解析最新已发布 Dataset，并在本次操作内固定坐标；消费者不必手工保存或传递 pin 文件。
已经打开的固定视图、挂载及显式历史坐标保持原版本，需要采用更新时显式打开新的视图。
跨操作任务如需复核，应保留具体版本依据；不能假定两次默认最新请求之间没有发布。
旧 pin 不冻结授权，权限撤销仍在后续请求生效。最新可服务版的统一切换目标见 §1.2。

移除知识时，接入方显式删除选定 Address 并发布新版本；仅从草稿目录拿走文件不表示删除
权威知识。删除对象内容、停用某个知识集和结束某仓在 Catalog 的登记是不同操作；不得用
归档登记替代内容变更。旧 pin 仍有效、历史 commit 仍存在且当前权限允许时，可以读取
内容移除前的版本；pin 不保证已退役配方或已归档登记继续可用。

恢复历史内容时，先在固定历史版本读取要恢复的值及其 Schema，再与当前版本比较，按当前
Schema 和治理约束形成新的 Writer 变更。恢复产生新的可审计发布，不倒退权威历史；恢复的
知识经新 pin 采用。历史 pin 的读取仍受仓可用性、版本保留和当前授权约束。

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
├── _schemas/
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
和文件发布流程默认写入唯一的 `_schemas/` 目录；实例按 Schema 实体类型分目录
（`tables/`、`metrics/`、`relations/`），不再使用含糊的 `objects/` 前缀。
System Repository 使用同一套 Schema 树：跟踪源 `knowledge/system/schemas/`
与发布后的平铺 `_schemas/` 一致。

### 6.3 Consumer Semantic File View

Canonical 单元信封（`object_id` frontmatter）不是默认产品展示。对人、IDE 和通用 Agent 文件工具，应用层在固定 pin 上
生成只读、可丢、可重建的语义文件视图。

VFS 的产品适用范围是**本机放得下的 Dataset**，受本机存储与资源容量约束，不适合特别大的
数据集。它的优势是本地 Agent 可以像访问普通文件一样浏览、读取并复用文件工具；**目前只读**，
不能在挂载目录修改或写回。按需回读不扩大这一适用范围，也不意味着挂载后可以离线使用。
数据超出本机容量时，应使用服务端统一检索与按需访问，或先切分出本机可容纳的 Dataset。

语义文件视图示意：

```text
knowledge/<knowledge-set>/
├── _meta/
│   ├── pin.yaml
│   └── sources.yaml
├── _schemas/
│   └── metric.definition.v1.yaml
├── metrics/
│   └── <display-or-encoded-id>.yaml
├── semantic-models/
├── tables/
└── relations/
```

一个实体 YAML 可以组装其多个 Aspect，但必须保留 `_kc.object_id`、Repository、commit 和每个
Aspect 的 Schema。该视图不成为 Canonical，不接受写回；用户修改知识仍走 Proposal/Writer。

需要区分 VFS 与 Catalog 成员：

- **VFS / File Gateway** 是固定 pin 上的消费能力（把树给人看），不解释知识，也不等于把仓登记进 Catalog。关掉 Plain 仓之后，VFS 仍可挂**知识仓**的固定 commit。
- **Catalog attach** 是成员登记。Plain Git 只解释组合阶梯（可以 pin 还没有 Schema 的树）；带 frontmatter 的文件才进入 ② READ/SEARCH。

文件投影本身有两种形态：

1. **Repository 文件 mount**：投影已有 Repository 子路径，只支持可逆路径前缀；
2. **Semantic file view**：从固定 Canonical Address 组装 YAML/Markdown，可按 Schema/类型选择。

前者适合本来就是文档/代码的 Repository；后者才适合 `metrics/*.yaml` 和 Agent `rg/grep`。

---

## 7. Server、Client 与插件能力

### 7.1 Server

Server 产品面需要：

- Server info 与 System Repository 坐标；
- 平台 Snapshot 申请、自有 Snapshot 连接验证与授权管理，独立于 Catalog 成员登记；
- 基于显式策略的接入准入、维护权限与消费分享；
- 可见 Catalog、Repository、命名知识集 inventory（身份列表；`catalog show` 的 `repositories` 不附 README）；
- discovery knowledge set；
- 已发布实体名单（`schema list`）；README 走 READ/SEARCH；合同走 READ；
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

- 交付时封装唯一服务地址，登录与后续命令复用连接；用户不接触部署配置或服务端应用秘密；
- 当前连接与身份展示、Catalog 发现；
- 平台仓申请、自有仓连接、登记与授权结果检查；
- 浏览知识源与 Schema；
- 打开命名知识集或创建本次任务的临时配方；
- 任务级固定 `ResolvedKnowledgeSet`；
- 搜索、精确读取、关系浏览与来源追溯；
- 预览挂载、挂载、卸载与显式采用更新；
- Provider 侧 Schema validate、ID mapping test、Connector Preview 与 run status。

具体源客户端和 Connector 运行宿主不进入核心 `kc` CLI；它们通过 Integration SDK/服务调用
公开 Writer surface。

### 7.3 插件

插件至少有三个不同入口：

1. **知识目录**：发现 Catalog、知识源身份、已发布实体、健康和质量 claims；README 按知识对象读/搜；
2. **添加到项目**：选择命名知识集或临时组合，预览并执行只读挂载；
3. **已接入知识**：显示 pin、Repository/commit、mount、更新和故障状态。

文件树开关只能叫“显示已挂载知识文件”。它只改变人用视图，不连接 Server、不选择知识、
不挂载、不发权，也不改变 Agent 能力。实际连接、挂载、卸载和版本更新是独立显式动作。

---

## 8. 用例

### U1：新部署发布 System Repository

```gherkin
Given 一个尚未初始化的空部署
When 部署方完成首次初始化
Then System Repository 被创建并登记
And Meta Schema digest 与 Server 内置信任根一致
And 普通已认证用户可以读取 System Schema
And 普通用户不能写 System Repository
```

### U2：接入方定义 Domain Schema

```gherkin
Given 接入方读取了 System Meta Schema
When 它提交一个带未知 type 或 access 的 Domain Schema
Then Writer 拒绝不合法的 Schema 声明
And Repository HEAD 不移动

When 它提交符合 Meta Schema 的 Domain Schema
Then Schema 作为 schema/* 知识对象被版本化
And DESCRIBE_SCHEMA 返回规范化字段和 AccessHints
```

### U3：实例必须符合 Schema

```gherkin
Given schema/metric/definition/v1 要求 name 和 expression 为 string
When Connector PUT 缺少 expression 或提供错误类型
Then Writer 拒绝不符合 Schema 的实例
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

此用例的 breaking 迁移分支与 §4.4 共同等待 REVIEW-03，保留分歧供产品决策。

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
Given 用户已获得封装服务地址的客户端并完成登录
When 打开知识目录
Then 能看到 System Repository、可见 Catalog 和知识集
And 仓根 README 是可 READ / SEARCH 的知识对象，不是库存 title/summary
And 能按该仓已发布实体名单浏览而不先猜搜索词、也不枚举对象实例
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

- System Repository 选用已认证默认可读，不代表其它 Repository 自动可读；
- Meta Schema 是协议启动信任根，Domain Schema 不是；
- 普通知识的 `schema_ref` 必须在目标 Repository 固定 basis 内解析；
- Schema 是知识，正式版本只经 Writer；Provider 工程中的 YAML 是发布输入；
- Canonical 每文件一个 Address；路径不是 object ID；
- Consumer Semantic File View 可丢、只读、固定 pin，不是权威；
- Workspace 是组合配方，不是接入方写入前置条件；
- 插件不拥有 Catalog、Schema、Connector 或权限语义；
- Connector 不拥有 Writer、Snapshot authority 或 Retrieval provider；
- DISCOVER/BROWSE 不得退化为消费请求中的无界 authority 扫描或对象 LIST。
- README 每仓至多一份约定路径；缺则不得由平台或模型补写，也不得展平成 Catalog `title`/`summary`。

---

## 10. 实然不在本文

U1–U10 是应然旅程。完成度、插件 V1 只展示了哪些库存、以及「不得把未提供的能力假装已有」，全部由 `MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` 拥有。BROWSE 属于 U6：Catalog/知识集 + Schema 分页，不是对象 LIST；README 走 READ/SEARCH。
