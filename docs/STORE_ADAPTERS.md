# Store Adapter 与派生介质

日期：2026-08-27

本文回答 Snapshot 权威、检索索引、缓存和分析投影分别应落到什么介质。实时 State/Stream 的运行与存储已移交上层 Materialization 产品，不属于 Knowledge Catalog Store Adapter。

---

## Goal

回答 Snapshot 权威、检索索引、缓存和分析投影应落到什么介质，使身份、版本、来源与读写结果不随引擎变化。

## Non-Goals

- 不让 Repository 接口同时承担全文、缓存、事件流和分析投影。
- 不把某一种数据库提升为知识协议本体。
- 实时 State/Stream 不属于 Knowledge Catalog Store Adapter（文首）。

## 硬性约束 / Invariants

- `A-01` 只改变 provider 绑定，不改变 Reader/Writer/Catalog 语义。
- `C-01` 公开结果不得返回 OpenSearch `_source` 充当 Canonical。
- `P-01` 投影可删可重建；`R-02` 无 READY provider 时 SEARCH 失败关闭。
- ⓪–③ 与介质梯子不得混名（`LAYERS.md`）。
- `CA-01` ⓪ Snapshot、① Catalog 与② Reader 不持有跨请求知识对象缓存的具体实现。对象缓存属于上层 retriever lane，经 Knowledge hydrate 端口由应用装配；Snapshot Adapter 只能保留不解释 `object_id`/Aspect 的连接、原始 tree/blob 或 transport cache。
- `CA-02` / `CA-03` 正文缓存保持完整读取身份、固定 authority basis 与副本隔离；缓存和预热有界，miss 仍从同版本批量回源。
- 部署服务凭证通过运行环境注入；接入方授权的逐仓连接凭证由 Server 私有凭证存储耐久保存，只在具体连接装配时注入。二者都不进入公开 binding、layout、Catalog、Schema、知识正文或过程证据。

## 选定方案 / 被否决方案

- 选定：[ADR-018](KNOWLEDGE_CATALOG_DESIGN.md#adr-018)（修订）：Gitea/LakeFS 作 Snapshot authority，本机与规模化形态统一为 LakeFS；Dolt adapter 已按 2025 裁定退役删除；OpenSearch 作 Retrieval provider；同一 Conformance。
- 选定：自有仓连接先只读验证既有 authority，再保存连接并登记；凭证轮换独立于知识身份。失败不替换可用连接，也不初始化或改写外部仓。
- 选定：上层正文缓存首版使用可丢的进程内 LRU；以不可变版本隔离内容，由独立后台消费者预热。
- 否决：让新 commit 的正确读取依赖逐条失效通知；把索引文档摘要当作完整正文版本；把动态 observation 缓存为 Snapshot 内容。
- 否决（本文边界）：在 Snapshot 口加索引方法；Writer/Catalog 核心 import `index/`。

## 接口契约 / 状态机

介质角色以本文为准：Snapshot authority、检索投影、缓存、分析投影分开，同一 Conformance。装配根选择 adapter；参考实现文件名（如 `home/authority_drivers.go`）不是协议。合同测试在各 adapter README 与 `internal/testkit/`。


## 1. 为什么介质要正交

协议需要同时适应本机 Git、远程 Git 托管、规模化 Snapshot、全文和列过滤，同时保持身份、版本、来源与读写结果不随引擎变化。

错误做法是让 Repository 接口同时承担全文、缓存、事件流和分析投影，或把某一种数据库提升为知识协议本体。

目标是：

```text
稳定 Snapshot/Knowledge 语义 × 可替换介质
```

---

## 2. 协议层与介质职责正交

```text
⓪ Snapshot 操作语义
① Catalog 组合
② Knowledge 解释
③ Retrieval Projection
```

| 介质角色 | 含义 | 丢失后果 |
|---|---|---|
| Snapshot 权威 | 已接受的 commit/ref 历史 | 知识不可恢复 |
| 索引 | 从固定 Snapshot 或外部 Binding 派生的检索结构 | 可重建，查询降级 |
| 缓存 | 已有结果或同 basis hydrate 的加速副本 | miss 后回同版本 provider，不改变授权或结果语义 |
| 分析投影 | 面向消费计算的派生形态 | 可重算，不反写权威 |

Catalog Registry 即使落 Git 仍是 ①；OpenSearch projection 即使与 lakeFS 同机仍是 ③。外部 Stream 即使被 Retrieval 索引，也不会成为 ⓪。

访问 / retrieval / refine / feedback 证据不属于上表任一角色：丢失的是审计覆盖率，不是知识不可恢复，也不是可丢的检索投影。它不实现 `snapshot.Store`，不进入 Catalog pin，也不走 AccessSpec。写入是 fail-closed 追加，查询是时间窗上的等值过滤；介质由 `observability/` 的 adapter 承担，见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

上层运行时的**观察记录**（"何时、按哪个声明、以什么一致性观察到了什么"）同样不属于上表任一角色，但它与访问证据不同：来源是 `latest-only` 时，历史观察一旦丢失即永久丢失，**不能**按"可删除、可重建的派生"对待，也不适用 `P-01`（投影可删可重建）。它的运行、保留期、备份等级与恢复由上层 Materialization 产品拥有，不进入 Repository 或 Catalog pin，也不因持久化而成为 `snapshot.Store`；取值路径与重读能力等级见 [`LIVE_MATERIALIZATION.md`](LIVE_MATERIALIZATION.md) §6。

活动 Serving State 和保留观察可以共用介质，但必须区分生命周期：切换活动版本不等于可删除
仍在保留承诺内的完整观察。平台承诺重读的记录须纳入持久恢复与容量管理，不能只保存摘要或
依赖可丢临时目录。按合同到期与保留期内意外丢失应可区分，均不得由重新读取最新值掩盖。
这些要求约束上层运行介质，不扩展 Snapshot Store 的权威范围。

---

## 3. 底座目标介质

Store 选择与 Client/Server 边界正交。“本地”只表示 KC Server 与介质在同一台机器或同一个开发拓扑中；Connector、`kc` 和 `kcfs` 仍分别经 Writer、Knowledge/Catalog 和 Workspace File typed API 进入 Server。更换为自建 Git 或轻量检索实现时，它们也只能作为 Server 后面的 adapter，不能产生另一套直连语义。

| 能力 | 目标介质 | 明确不用 |
|---|---|---|
| 本机 Snapshot | LakeFS（本地部署形态） | 内存模拟作正式权威 |
| 远程 Snapshot | Gitea Git 对象 API | 远程共享工作区 |
| 规模化 Snapshot | LakeFS Graveler + S3/COS/Ceph/MinIO 数据平面（采用须过资格门） | 普通对象桶或关系表冒充版本图 |
| 单实例精确读取/VFS | Server 后的 Snapshot adapter；可不配检索 provider | 让 Client 直开 Home，或伪造与正式 AccessSpec 不一致的搜索语义 |
| 服务检索 | OpenSearch | 把 `_source` 当 Canonical |
| 分析消费 | 上层产品选择的可重建 projection | 反向成为 Writer target |

这张表是本项目承载各类职责的选型，不是性能排名或生产规模资格结论。选择理由是让不可变
版本与 Ref CAS、知识解释、候选检索分别由可独立验证的能力承担；LakeFS、Gitea 与 OpenSearch
分别接受对应合同约束。

**权威介质的对象保留是本表的一等约束。** 上表"Snapshot 权威"的丢失后果是"知识不可恢复"，
因此权威介质不得配置会清除历史版本的保留策略：知识每次修改都会替换单元，任何按天数清理
"已替换的已提交对象"的规则都会让旧 `{repository, commit}` 坐标读不到正文，与 `V-01` 的固定
basis 回读和旧 pin 承诺冲突——即使版本图本身还在，也已经不满足权威语义。容量收敛由
`SCALE_ARCHITECTURE.md` 的 Repository generation 承担，不由删历史承担。垃圾回收只针对未提交
残留与已废弃分支。

本文尚未给出不同后端在同一负载下的成本、延迟和恢复比较，因此不能仅凭选型名称推出容量
上限或迁移收益。面向规模的采用还必须满足 `SCALE_ARCHITECTURE.md` 的 bounded 读取、单仓
原子写入、历史可迁移性和恢复要求，并由带环境与输入来源的规模证据验收。更换后端的决定
需要补齐对应约束与测量，不能反向改变知识协议。

State/Stream 的 log、cursor、retention、热尾缓存和回放引擎由 Materialization 产品选择，不再冻结在本底座的 Snapshot/Retrieval adapter 组合里。

---

## 4. 推导

### 4.1 Snapshot 需要版本图

`snapshot.Store` 必须表达不可变版本、Ref、expected-old CAS 和归档；可选 `TreeReader`
只表达固定 commit 的 path/blob 读取，`TreeStore` 在其上增加原始路径提交。
`HistoryStore` / `ChangeStore` 提供纯坐标加速。拆开读写能力后，native Knowledge authority
可以支持 Workspace File Gateway 的固定版本读取而不暴露绕过 Writer 的 raw tree 写口；
确需普通文件写入的 Home 装配通过明确命名的 file-capability adapter 提供。Git 与
Gitea Adapter 使用不同机制，但只通过 Snapshot Conformance；同一套 Knowledge Conformance
在其上层 Reader/Writer 组合上运行。

普通关系表若没有版本图和 CAS 语义，不能只因“能存 JSON”就声明实现 `snapshot.Store`。

LakeFS adapter 把 Graveler/PostgreSQL 作为 ref、commit 与元数据控制面，把 S3 兼容对象存储作为
字节数据面。KC Server 内部使用 lakeFS 签发的地址直传对象，字节不经过 lakeFS 节点；预签名地址
不得越过 Server 边界。普通 lakeFS `merge` 会创建新 commit，`hard_reset` 又没有 expected-old
前置条件；adapter 因此先以原子 hidden branch create 取得每个发布 ref 的互斥权，持锁后重查
expected、reset 到已审阅 candidate、核验并释放。部署必须让 KC 服务身份成为 published/`kc-*`
branch 的唯一写者；绕过 Writer 的带外 reset 会破坏互斥证明。崩溃留下的锁必须失败关闭，由同一
命令恢复或显式运维清理，不能无条件抢锁。

### 4.2 Dynamic runtime 不是 Store Adapter

State/Stream 的核心是 observation basis、cursor、window、retention、late data 和 source capability。这些语义与 git Snapshot 不同，也不需要 Catalog 组合。

因此底座只保存 Binding 声明。上层产品可以选择 Kafka、数据库 CDC、日志系统、对象存储段或源侧查询，但这些都不进入 `snapshot.Store` 或 Snapshot Adapter。

这里不禁止动态运行时制作自己的持久 checkpoint、WAL 或 savepoint。它们绑定输入 offset、
operator state、generation 和恢复生命周期，是 Materialization Runtime 的恢复产物；Knowledge
Snapshot 则绑定 Repository commit、知识内容和治理历史。两者都可能是 durable snapshot，
但不能因此共用 Store 接口、Catalog pin 或 Conformance。

### 4.3 检索按查询形态选引擎

- MATCH 需要倒排、分词和相关性模型。
- filter/sort/range/aggregate 需要可比较列值。
- 外部 Binding 可以 query-time pushdown，也可以维护 managed projection。

Schema 只声明 `text/filter/sort` 访问语义，不绑定 OpenSearch 或上层 Stream 产品。`stored`、`summary`、doc value、`_source` 等若存在，只是 provider 的私有物理优化：它们不进入 Schema、Candidate 或公开 SEARCH 结果。Candidate 只保留 typed identity 与证据，最终结果从 Snapshot 或固定 Binding hydrate 完整知识及版本。

### 4.4 Snapshot 索引按 Repository basis 共享

一把 Snapshot 工作索引对应 Repository、basis commit 与物理 provider revision，不对应 Workspace。Workspace 只给出本次 pin；`AccessPlan` 给出每仓逻辑 AccessSpec，单次请求再按 provider Probe 选择检索路径。它不是物理索引定义，也不是长期保存的 RetrievalPlan。

因此物理文档不保存 `workspace_id/workspace_ids`。同一 Repository basis 可被多个 Workspace
复用；Workspace SEARCH 在请求时按固定 pin 选择投影。OpenSearch 多 index、`_msearch` 或
PinID 级短期 alias 只属于可丢的执行优化，不能成为组合、版本或授权权威。

动态 projection 则对应 Binding generation 与 observation basis，由上层产品共享和治理；不能塞进 Repository commit 索引表后假装两者同一 basis。

### 4.5 权威成功先于派生成功

Snapshot 写入成功后，投影可以异步追赶。投影失败不回滚已接受 commit，也不反写权威。

对于动态 Binding，源侧观察成功与投影成功同样分开；读侧必须报告 basis、lag、coverage 和 degradation。

### 4.6 正文缓存与维护投影

正文缓存复用固定 Repository、commit 和完整对象或 Address 的读取结果。条目来自 authority 的
完整读取，包含知识内容及其声明、版本与来源；完整对象不能由某个 Aspect 的命中冒充。缓存读取
提供独立副本，不保留调用方可变引用。Snapshot 中的 Binding 声明属于该 commit，但 runtime
observation 与绑定后的动态值属于另一依据，不能进入 Snapshot 正文条目。

不可变版本键把正确性与失效时序分开：新 commit 读取新键，旧 pin 保留原语义；LRU 驱逐和重启只
导致同版本 miss。单次批量 hydrate 只回源未命中部分，不能因引入缓存把有界批量端口退化成逐对象
远程调用。Snapshot、Catalog、Reader 不拥有缓存介质；上层 `retrieval/cache` 由装配根通过
Knowledge hydrate 端口接入，具体合同见其包 README，服务交付边界由 `SERVICE_ARCHITECTURE.md` 拥有。

预热与索引构建可以共享源版本推进的控制入口，各自保留进度和恢复策略。正文变化不必改变任何
索引字段，因而不能用“索引文档更新”决定缓存刷新。预热仅维护有界热点及显式启用的有限冷启动页，失败不影响
缓存 miss 回源，也不能使尚未 READY 的搜索投影变为可用。跨实例缓存或通用消息系统不是首版前提。

---

## 5. Local 与 Scale

底座 Local：

```text
LakeFS Snapshot（本地部署形态）
no retrieval projection（精确 READ / VFS）
```

底座 Scale：

```text
LakeFS Snapshot + S3-compatible object data plane
OpenSearch projection
optional lake projections
```

上层 Materialization 产品可以独立提供本机/规模化运行形态，但不应借用底座 Store 配置把 Stream 重新注册为 Repository。

介质维度的可证伪 ID 与证据见 [`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md) 的 A-01、C-01、P-01、R-01、CA-01。

---

## 6. 具体协议位置

- Snapshot capability：`snapshot/`；Knowledge 声明解释与写入：`knowledge/reader`、`knowledge/writer`；消费侧 State exact hydrate：`knowledge/serving` + 墙外 provider。
- Snapshot Adapter Conformance：`internal/testkit/`。
- 本机与远程/规模化 Snapshot：`snapshot/lakefs/`、`snapshot/gitea/`。唯一装配入口为 `home/authority_drivers.go`。
- Knowledge 解释统一由 `knowledge/reader`/`knowledge/writer` 在 Snapshot 能力之上完成；Relation 候选只由③ provider 产生。
- Snapshot Projection：`index/`；物理 provider：`retrieval/`。
- Dynamic Materialization：`LIVE_MATERIALIZATION.md` 所描述的上层产品边界。
- 访问证据：`observability/` 的 Recorder / AccessLog；本机 JSONL 是参考 adapter，装配在应用层。
