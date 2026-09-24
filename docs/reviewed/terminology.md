# Knowledge Catalog 术语表

日期：2026-09-18

状态：规范。公开文档、CLI 帮助、JSON 合同和 Go 导出注释使用这里的名称。

## Goal

固定 Knowledge Catalog 的公开名词，使文档、CLI 帮助、JSON 合同和 Go 导出注释使用同一套名称，避免 Catalog / Repository / 知识集 / Server 被当成同义词，避免知识集和用户工作目录同名，也避免 Collector / Observer / Resource Access 被当成同一个接入进程。

## Non-Goals

- 不定义协议字段、错误码或命令表（见各包 README 与公开 API）。
- 不按部署进程名反向改写 ⓪–③ 分层（见[架构总览](core-architecture.md)）。
- 不把 `repo`、SDK 模块名提升为第二种领域对象。

## 硬性约束 / Invariants

- 公开文字使用全称；短名只用于 flag、变量和路径（本文表格）。
- 知识集公开坐标是 `KnowledgeSet` / `ResolvedKnowledgeSet` / `SearchView`。公开工作名是 Dataset：发布时冻成文件清单，不引入 Session（`KS-01`，`architecture-invariants.md`）。
- 禁止的别名见本文 §4；发现同义复述时改用这里的规范名称，不另造词。

## 选定方案 / 被否决方案

- 选定：一张规范名称表，Go 类型名作为实现锚点写在定义里。产品工作名是 Dataset；资源旗标 `--dataset`。
- 否决：用 Catalog 指整套知识系统；用 CatalogClient/KnowledgeClient 当两个产品；按 Server 部署名改协议包名；用 Loom 当公开产品名。
- 否决：用 Workspace 指知识集；用 selector 指整条配方。

## 接口契约 / 状态机

权威就是本文表格。类型锚点：`catalog.KnowledgeSet`、`catalog.KnowledgeSetRecipe`、`snapshot` Ref/commit。CLI/HTTP 用词必须与本表一致。

## 1. 产品与服务

| 规范名称 | 含义 | 不要混用 |
|---|---|---|
| Knowledge Catalog | 整套产品与协议 | 不用 Catalog 单独指整套知识系统 |
| Catalog | 一间共享组合空间及其登记状态 | 不是 Repository、文件仓或知识索引 |
| Catalog Plane | Repository 发现、知识集组合、selector 解析和文件投影所在逻辑平面 | 不理解 Aspect、Schema、Binding |
| Knowledge Plane | Schema、Entity、Aspect、Relation、READ、SEARCH 和 Writer 所在逻辑平面 | 不拥有 Catalog 生命周期；不存在无界公开 Knowledge LIST |
| Catalog Server | Catalog Plane 的共享控制服务 | `catalog/` 是协议包，不等于部署进程 |
| Knowledge Server | 固定知识集 basis 上的结构化知识消费服务 | OpenSearch 只是其内部 provider |
| Knowledge Set File Gateway | 按固定 ResolvedKnowledgeSet 提供 path/tree/blob 的远程数据端口 | 不叫 Files API；不解释知识 |
| Writer API | Knowledge Repository 的 PUT/REMOVE、COMMIT/PROPOSAL 写入口 | Connector 不直接写 Git 或 OpenSearch |
| Governance API | Proposal、Preview、Validation、Gate 与 Merge 的治理入口 | 不是 Writer，也不是 Hook 执行器 |
| Admin API | 服务端授权策略的管理入口 | 不包含本机 init、Store 配置或 authority attach |
| Operations API | Projection、Hook、Gate 配置和审计/观测入口 | 不属于普通知识消费面 |
| KC Client | 对外客户端产品 | CatalogClient、KnowledgeClient 是其 SDK 模块，不是两个产品 |

`Server` 表示逻辑服务边界；这些边界可以先同进程部署。Go 包名仍使用
`catalog`、`knowledge/reader`、`retrieval` 等协议名称，不能按部署名反向改写分层。
Knowledge Reader Service 是 Knowledge Server 内部的②装配组件：它把 Catalog 交付的
Snapshot 成员包装为知识读能力，并提供 exact-basis ReadMany；不是第三个对外 Server，也不持有 Knowledge object cache 的具体实现。
Reader 可以持有应用注入的同版本 hydrate 端口（公开类型 `knowledge.Hydrator`），而完整正文缓存
由上层 `retrieval/cache` 实现。hydrate 指按完整读取身份取得固定 authority basis 的知识与版本；
缓存命中是该读取的内部优化，不是另一个公开动作、知识版本或授权决定。

## 2. Repository 与知识集

| 规范名称 | 精确定义 |
|---|---|
| Repository | Snapshot authority 和治理边界。正式文字使用全称；`repo` 只用于 CLI flag、短变量和路径名。 |
| Knowledge Repository | 内容通过 Writer 发布并遵守 Address/Schema/Aspect 合同；消费时由 Knowledge Reader 在固定 commit 上解释的 Repository。它不是 Adapter 实现的接口标记。 |
| System Repository | 部署内置的保留 Knowledge Repository（参考 ID `kr://kc/system`）；发布 Meta Schema 和核心协议 Schema，不是业务知识集的隐式成员。读权走仓的已认证默认或 grant，不是按保留 ID 的授权特例。 |
| README | 知识仓自描述 Markdown 知识对象（Address + `schema_ref`，字段 `body` 可 SEARCH）。仓根 `README.md` 是人写与 git 直推解释用的有界 `path_hint`，不是身份，也不是第二条检索代数。`catalog show` 的 `repositories` 不派生 title/summary。不是无 frontmatter 的 git 文件、不是对象 LIST、不是 Catalog 登记字段。 |
| Meta Schema | 约束 Domain Schema 文档自身的协议合同；由二进制内置信任根校验，并在 System Repository 中发布同一内容。不是实例字段基类，Domain Schema 不从它继承可访问字段。 |
| Domain Schema | 接入方定义、随目标 Knowledge Repository 版本化的 `schema/*` 对象；通过 `schema_ref` 约束实例 Address/value。 |
| Address | 一次写入或精确读取的知识单元身份（`object_id` 加可选 aspect / member）。种类是 Entity、Aspect、Relation、Member。这些种类不是四种 Domain Schema；关系记录的值形状另由 Domain Schema（如 `schema/core/relation/v1`）约束。 |
| Plain Repository | 未按知识发布合同维护内容的普通 Repository；只解释组合阶梯（可 pin 无 Schema 的树），仍可被 Catalog 组合。VFS 不是它的理由，知识仓固定 commit 同样可挂。 |
| Knowledge Set（知识集 / Dataset） | 命名、版本化的跨仓文件清单：成员是 `{repository, commit, path\|prefix}`，发布时冻 commit。短名 `dataset`。Go 协议类型是 `catalog.KnowledgeSet`。 |
| KnowledgeSetRecipe | `.kc-dataset.yaml` 的便携序列化形态。它是文件 DTO，不是第二种知识集领域对象。Go 类型是 `catalog.KnowledgeSetRecipe`。 |
| ResolvedKnowledgeSet | 一次 Resolve 的不可变结果：某版本文件清单、`{Repository → commit}`、setId、revision、PinID。Go 协议类型是 `catalog.ResolvedKnowledgeSet`。 |
| pin | `ResolvedKnowledgeSet` 的用户侧简称。文件名使用 `pin.json`，标识使用 `PinID`；不要再造 View。 |
| SearchView | 一次 SEARCH 实际观察到的 Snapshot/Binding basis。它属于检索结果，不等于知识集；知识集范围由请求时的 ResolvedKnowledgeSet 编译，不写进索引文档。 |
| 固定元信息 | 知识对象的协议坐标，不是业务正文：`repository`、`object_id`、`basis`、`schema_ref`。检索索引携带它们供 typed filter。知识集、Pin、allow 规则、当前 principal 以及未选定的仓级可见性分类都不是固定元信息。 |
| 交付链 | hydrate Canonical 之后、编码返回之前按固定顺序挂接的平台规则。输入是知识 ID，输出是调用方可见内容。公开类型 `delivery.Chain`。当前选定仅仓读权屏蔽；不是 Hook，不是检索代数，也不是新的协议层。细节由[权限体系](permissions.md)拥有。 |
| Preview | ControlPlane 中 Proposal + 知识集 overlay 的治理 basis。它只用于 validate/gate/merge。 |
| TaskContext | 客户端宿主私有的任务上下文：身份、KnowledgeSet、ResolvedKnowledgeSet 与 mount 生命周期。它不是服务端 Session，也不写入用户工作目录。 |
| Semantic File View | 在固定 Repository commit 上由 Canonical Address 组装的只读 YAML/Markdown 消费投影；保留 `_kc` 坐标，可丢弃重建，不是 Canonical 或写入口。 |

`knowledge/reader.KnowledgeSetPin` 是 Knowledge Plane 内部从
`catalog.ResolvedKnowledgeSet` 投影出的读取 basis，不是另一个可持久化协议对象，也不
替代 `ResolvedKnowledgeSet` 这个公开名称。

## 3. 动作

| 动作 | 规范含义 |
|---|---|
| attach Repository | 把已经由 KC 连接的 Snapshot authority 登记进当前 Catalog。当前 CLI 是 `kc attach --repo`。不创建 Snapshot、不要求已发布，也不发权。 |
| open authority | 按部署 binding 只读打开既有 Snapshot；属于接入/恢复的内部步骤，不是独立用户接入命令。 |
| register Repository | Catalog 内部承认成员的协议动作，由 attach 应用操作提交；没有独立公开 register 命令。 |
| resolve 知识集 | 把已发布 Dataset 的文件清单解成 ResolvedKnowledgeSet。产品 CLI 在 `search` / `read --dataset` 与 `kcfs` 挂载时内部完成。已发布版本不跟随 live 分支；要新数据就再发一版。 |
| replay pin | HTTP 可用 ResolvedKnowledgeSet JSON 重放同一组坐标，同时按当前权限重新求值。产品 argv 不接受 `--pin`；精确历史重放抄回执里的 `--repo --commit`。 |
| mount 知识集 | 把 Dataset 在挂载时冻结的清单投影为宿主只读文件系统。只使用 `kcfs mount --dataset`；`mount` 不再表示接入 Repository。 |
| browse knowledge | 有界发现：可见 Catalog、知识集，以及单仓已发布实体名单（`schema/*`）。不是对象 LIST；空查询或 `*` 也不是 BROWSE。README 走 READ/SEARCH，不是库存列。 |
| search knowledge | 按 Schema AccessHints 检索并在同一 basis 回读 Canonical。调用方信封是否含全文走权限交付链首段（[权限体系](permissions.md)），检索本身不裁剪。它不是文件 contains；普通文件使用 Knowledge Set File Gateway / `kcfs` + `rg`。 |
| traverse knowledge | 在固定范围（Dataset 清单或单仓 pin）内沿类型化关系步骤做有界邻域遍历，返回按对象去重的到达集合与全部入选边。不是图查询语言，不枚举全部路径，也不承诺最短路径；范围外 frontier 止步并显式标记。 |
| semantic recall | SEARCH 的语义召回策略（`semantic`/`hybrid`）：在候选资格内以派生向量投影召回近似窗口。不是第四个字段访问声明，也不扩权；融合策略另行裁决（[检索](retrieval.md)）。 |
| scan Snapshot | Provider/维护方在固定 commit 上为重建、迁移、导出或验收顺序读取全部知识。公开消费面不提供该动作。 |

## 4. 禁止的别名

以下名称不进入新的公开合同：

- `kset`、`--kset`、`KC_KSET`、`kset define` / `kset.consume` / `kset.manage` / `kset.resolve`、HTTP `/ksets` 与 `/kset-files`、JSON `ksets`、仓根 `.kc-kset.yaml`、登记表 `kset-*.yaml`：统一为 Dataset、`dataset define`、`--dataset`、`KC_DATASET`、`file.read` / `dataset.manage` / `dataset.resolve`、HTTP `/datasets` 与 `/dataset-files/v1`、JSON `datasets`、仓根 `.kc-dataset.yaml`、登记表 `dataset-*.yaml`。
- `Workspace`、`WorkspaceView`：统一为知识集 / Dataset / `KnowledgeSet` / `ResolvedKnowledgeSet` / pin；
- `selector` 作为整条配方的名称：selector 只是成员上的发布线；
- Session、`sessionId`：不进入知识集或认证公开合同。传输连接、SDK
  任务对象、FUSE 进程可以在实现内称 session，但不能成为身份、Pin 或续租资源；
- 裸 `View`：必须写明 `SearchView`、Preview 或 pin 中的哪一种；
- `Workspace Files API`：统一为 Knowledge Set File Gateway；
- `kc mount` 表示 Repository 接入：Repository 使用 `kc attach --repo`，宿主挂载使用
  `kcfs mount`；
- `Repo` 作为正式领域名称：公开说明使用 `Repository`，仅保留 `--repo` 和实现内短变量；
  不提供 `catalog repo` 或 `repo` 命令分组。
- `Loom` 作为公开产品名：产品是 Knowledge Catalog。不要在协议、CLI 帮助、设计标题或新的公开 HTTP/API 路径使用 Loom。
- `watcher`：源变化通知角色统一为 Observer。
- 用 Collector 指 change notice 或访问地址；用 Observer 指 Writer 对账或访问地址；用 Resource Access 指采集或 notice。
- 用「接入方」指上述任一进程：接入方是产品旅程角色，不是容器名。
- 知识 Collector 与 OTel Collector 不得混称；遥测管线写全称 OTel Collector。
- `Source Profile` / 源说明 / Catalog `title`+`summary` 信封：统一为 README 知识对象；库存只列身份。
- 无界 `LIST` 作为知识发现或 SEARCH 降级：自然语言发现使用 SEARCH；面向首次使用的
  DISCOVER/BROWSE 必须有界、分页、声明 basis 并以 continuation 推进，内容是 Catalog/知识集
  与该仓已发布实体名单，不是对象实例目录、也不是 Schema 正文或 DESCRIBE_SCHEMA；维护扫描只使用 `ScanSnapshotPage`，文件遍历使用按目录分页的 Gateway。

## 5. 一条完整链路

```text
KnowledgeSet
  -- ResolveKnowledgeSet --> ResolvedKnowledgeSet (pin.json / PinID)
  -- Knowledge SEARCH --> SearchResult.SearchView
  -- host projection --> datasetfs.Plan --> kcfs mount
```

Provider 侧是三条正交链路，规范名称见 §6：

```text
Source → Collector → 对账 → Writer API → Knowledge Repository commit
Source → Observer → change notice → 投影控制器     （不写仓）
Source → Resource Access ← kc access / StateLookup （origin 访问地址）
       → ProjectionMaintainer → OpenSearch projection
```

## 6. 源侧角色

接入方是产品旅程角色（谁发布知识），不是容器名。源侧三个名称如下；同一进程可以兼任，合同不能混。

| 规范名称 | 含义 | 不要混用 |
|---|---|---|
| Collector | 对账外部当前态，把要进仓的差量发给 Writer（COMMIT/PROPOSAL）。 | 不发 change notice；不提供 origin 访问地址 |
| Observer | 盯源变化，只发 change notice（定位 Binding/Address/source revision hint，不带正文）。公开入口是 `kc operations projection notice` / `POST /operations/v1/projections:notice`。 | 不对账进仓；不提供 origin 访问地址 |
| Resource Access | 提供 Schema Canonical `origin` 上的 `resource-access/v1` 访问地址；`kc access` / StateLookup 按 origin + 实体 ID 取值。 | 不写仓；不发 notice |
| Connector | Collector 对账用的 Address helper（`connector.Preview`）。 | 不连源、不持 Writer、不是 Observer、不是 Resource Access |
| invalidate-and-pull | Observer 通知后，平台按固定 Binding 向 Resource Access 拉取。投影控制文档也称 notify-and-pull，同一机制。 | 通知成功不等于投影完整，更不等于知识已更新 |
