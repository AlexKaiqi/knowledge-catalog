# declarativeindex/projection/

声明式索引插件的查询与物理投影：Repository/Binding 之上的规划、候选定位与可丢投影。它不拥有对象正文，不进入 Writer/Dataset 服务 核心；命中后必须由独立 hydrator 在计划声明的 basis 上读取完整对象与版本。

## Dataset 修订检索空间

`PrepareDataset` 从固定 `DatasetPin` 的选入单位构建一个 corpus，`SearchDataset` 在该 corpus
中执行一次物理查询，跨来源使用同一套打分。编译命名空间通过
[`discovery/space`](../../discovery/space/README.md) 汇总所选 Schema 与索引成员；范围外的方面
不参与构建或候选回读。同一对象的不同方面由既有 `ComposeHoldingValues` 组合，保留真实
`Sources` 版本向量，不把 Dataset 的派生索引摘要当作某个来源仓的提交。

`DatasetMember` 只创建同一固定范围的声明视图，供实例登记委派；它不打开引擎或准备投影。

`DatasetAccessSpec`、`CheckDatasetProjection` 和 `DescribeDataset` 读取同一个准备依据。
Query 只核验就绪与投影保留的精确声明地址；未就绪、失效、不支持与未声明均拒绝，查询
不构建索引或扫描来源。候选的范围和依据先于正文读取验证；同 corpus 中不存在或未选入
的对象坐标失败关闭。查询、语义窗口和构建在预取消时不打开引擎或读取权威，元数据读取
沿上下文端口传播取消。

冻结对象的结构 Schema 可以只是 READ 依赖，没有选入检索声明或 Schema 时不参与
字段构建，也不阻止合法 Dataset 准备。选入声明引用的 Schema 或已选 Schema 的精确
采用释放仍须固定可解；不能通过跳过解释掩盖损坏的声明依据。

`RefreshDatasetState` 为当前修订发布独立观察空间，取值描述在 Dataset 固定依赖中解析，
只允许 `shared` 值进入它。刷新入口及接受前拒绝已成为保留修订的请求；观察期间发布新
修订会拒绝本次刷新，原观察仍可查。消费方经通用候选补判
接缝按本次 READ 的观察重核条件，淘汰候选后继续翻页；命中同时保留索引原观察及 READ
新观察。取消或准备失败不切换已发布的观察代。

OpenSearch 对新观察值的 MATCH 补判使用已发布物理字段的分析器，按 token 和位置核对
全文、任一词及短语；分析请求只读并沿本次搜索上下文取消。缺失、无效或无法证明的
分析响应明确报错，不改成零命中，也不以子串比较代替分析语义。

`SemanticDatasetWindow` 只从显式向量成员形成的同一 corpus 召回，返回近似窗口；全文成员
不自行产生向量。命中依据按声明成员、来源版本和角色归因，调用者不传成员键。

`RelationsDataset` 使用该 corpus 的关系候选与代际续页，在选入的固定来源中回读完整
Relation。正式 Dataset 一跳查询只执行一次物理关系查询，遍历的每一跳复用该有界分页
执行器；命中保留 Relation 自身的真实来源仓与提交，端点不扩大候选范围。底层
`RelationsAtContext` 接到 Dataset scope 时转入此端口，其余提供方合同仍在固定来源上执行。

下述按来源的 `SearchAt`、`EnsureAt` 等仍是提供方和维护合同的底层端口；正式消费命令只
走 Dataset 修订检索空间。成员增量准备、外部失败单元单独说明、丢失 corpus 的自动恢复，
以及最后一次当前修订校验与 Dataset 发布之间的原子条件边界
尚由 `INDEX-SEARCH-SPACE-01` 继续收口，本段不表示这些验收已经通过。

## 目标边界

`RecoverResources(ctx)` 为维护控制器遍历当前已打开、支持资源恢复端口的引擎；不持有运行时
锁跨越提供方 IO。各引擎失败仍继续其他项，未决物理保护报告具体资源；查询与打开句柄不隐式
回收。资源扫描进度与准入由提供方耐久保存，当前接入不代表已实现 Dataset 生命周期的引用释放。

```text
discovery                 SearchRequest / SearchResult / SearchView / ObjectVersion
  ↑                       AccessSpec / residual / merge
discovery/projection      Engine / CandidateRef / Meta / Notice / budget / candidate scope
  ↑                       通用消费与物理维护端口
declarativeindex/projection Index / Planner / Snapshot + State 发布与查询
  ↑                       只消费通用端口，不 import control
declarativeindex/opensearch 公开 adapter → internal/opensearchimpl 实际物理引擎

declarativeindex/control   Controller / TargetStore / published HEAD 对账与 worker
  →                       通过维护端口调用 Index，不进入查询执行路径
```

Schema 只声明逻辑类型与结构。采用 `kc/index-declaration` 的成员是查询能力的唯一来源；
Writer 校验形状与 Schema 字段定位符，实例按其能力清单逐成员判断支持。
声明变化单独判断索引影响，不自动构成对象值结构不兼容；固定旧版本按当时声明解释。
编译分三步：

```text
AccessSpec      固定 repository + commit + FieldRef(schema, aspect, path)
ProjectionSpec 某 provider 对 AccessSpec 的可重建物理实现
RetrievalPlan  某次 SearchRequest 的 provider fragments、residual、combine、hydrate、claims
```

Schema 不写索引能力；索引声明可以从实例清单选择 analyzer 等构建参数，不携带 provider、
index name 或原始 mapping/settings。`SetDeclarationManifest(check, digest)` 同时安装实例检查和
清单摘要；`PhysicalDigest` 合成 driver 与清单摘要，清单变化使旧物理依据失效。
`AccessDigest` 仍是逻辑声明摘要；Snapshot、Serving State、关系与查询就绪沿同一物理判断，
不能用支持结果恰好相同的旧投影冒充当前清单。

Provider 端口必须分开：

```text
Retriever             Probe(requirement) / Retrieve(fragment, continuation)
ProjectionMaintainer  Describe / Rebuild / Apply
```

`Index.SetHydrator(object.Hydrator)` 为同版本 Snapshot 正文回读提供独立接缝；
nil 保留原 `ReadMany`/精确读。SEARCH/RELATIONS 先验证候选身份和 basis，再批量查上层缓存，
仅 miss 回源；动态 State 查询继续读取已发布的同 revision Serving State，不进入 Snapshot 缓存。

后台维护、耐久目标和 notice 处理由 [control](../control/README.md) 拥有；独立 Snapshot
消费者合同由 [object/maintenance](../../object/maintenance/README.md) 拥有。
Index 构造不启动 worker，查询只读取已发布投影；维护与资源关闭由插件 Entry 组合。

`Probe` 逐 clause fragment 返回 `exact / superset / approximate / unsupported` 与 coverage。superset 在 Canonical hydrate 后执行 residual；完成 residual 后仍可返回 complete，approximate 只能支持 partial。仅支持 source pushdown 的 Binding 可以只实现 Retriever，不被迫伪造 rebuild/apply。

CandidateRef 是 provider 与 hydrator 之间的内部值，只保留 repository/object 或 dynamic resource identity、basis 与 LaneEvidence。provider 的 `_source`、stored field、summary/doc value 不得穿透为对象结果。SEARCH 在固定 basis hydrate Canonical；调用方信封是否含全文走交付链首段（`doc/design/permissions.md`）。

多 provider score 不直接归一为概率。合并保留 provider、lane、local rank/score、matched fields；稳定 tie-break 至少使用 `(repository, object_id)`。执行器必须支持 candidate continuation，因为 residual false positive、去重或授权过滤后仍需翻页填满请求 limit；预算提前耗尽时标 partial。候选坐标错误、同 basis Canonical 缺失或 hydrate I/O 失败是执行错误，不能当成普通候选消耗后继续。

Snapshot 物理投影按 `(repository, basisCommit, provider, physicalDigest)` 共享，不按 Workspace 建表。live 工作投影可以跟随 `AfterSnapshot`，但消费检索必须使用本次 ResolvedDataset 的 commit，不回绕 live。State 动态投影在同一固定声明 commit 上按 observation basis 独立发布；runtime 仍在墙外，本包只经通用 `ObservationSource` 注入的 lookup 完成 hydrate、编译和维护。

Workspace 是请求范围，不是投影文档属性。`CompiledDoc` 和 OpenSearch 文档不得出现
`workspace_id/workspace_ids` 或 PinID；一次 Workspace SEARCH 从固定 ResolvedDataset 为每个
已授权成员生成 fragment，再合并 Candidate 并 hydrate。同一 Repository basis 因而可以被多个
Workspace 复用。OpenSearch 多 index、`_msearch` 或按不可变 PinID 建短期 alias 都可以作为部署
优化，但 alias 必须可丢、可回收，且不能承担授权或版本语义。

## 当前 Go 实现

- `reader.DescribeRepoSchema` 在固定仓与版本，通过有界 Schema locator 读取字段声明，
  `retrieval.AccessSpecFromReport` 编译逻辑类型与查询能力，再进入 provider Probe 与就绪/执行检查。
  `AccessDigest` 保留归一化字段合同；查询能力改变触发重建，普通内容版本推进保持增量。
- `retrieval.AccessSpec` 只编译 `text/filter/sort`，字段身份是完整 `(schema, aspect, path)`；裸 path 仅在唯一时可用。
- `CompiledDoc` 是一个 object_id 一篇完整文档；Aspect/Member 只是维护单元。Member path 相对每个 member value 求值并合并成多值 cells。
- 类型化 cells 分离 string/long/double/boolean/date/text；`eligibleFields` 保留“适用但缺值”，使 MISSING 不会命中无关 schema。
- Relation 仍是独立对象，通用 type/direction/endpoints 进入保留投影字段；属性仍经 AccessSpec 编译。
- `Retriever` 与 `ProjectionMaintainer` 是独立端口；OpenSearch managed engine 还实现 streaming rebuild session，500-doc batch 写 candidate generation 后原子 publish。
- Provider 先逐叶子 `Probe`，声明 exact/superset/approximate/unsupported 与 coverage；显式
  `SearchExpr` 还要求 `ExpressionProber` 对整棵 `All/Any` 组合给出独立证明。无法兑现的成员不会
  被伪装成完整结果。
- `CandidateRef` 不携带正文；Executor 校验 repository/basis 后，在同一 Snapshot commit 通过
  `object.BatchReadStore.ReadMany` 按候选页 hydrate Canonical；不支持批量端口的 Repository
  才退回逐对象读。
- 公开 `SearchResult` 固定 SearchView，并返回 Completeness、Claims、完整 ObjectValue、ObjectVersion 与 LaneEvidence。
- stale/removed/wrong-basis candidate 返回 `PRECONDITION_FAILED`，不得静默降级为 partial；公开 opaque continuation 绑定 query、SearchView 与 Projection revision，residual false positive、去重或授权过滤消耗候选时继续翻页。
- AccessDigest 与 PhysicalDigest/ProviderRevision 分开，逻辑声明和物理重建原因可独立解释。
- `CheckSearchProjectionAt(repo, commit)` 只读校验固定 Snapshot 投影是否满足 `SearchAt` 的 basis、READY、AccessDigest 与 PhysicalDigest/ProviderRevision 前置条件，并返回观察到的 `Meta`。失败时仍返回已读到的生命周期，便于区分构建中、失败、停用和缺失；不会执行查询、hydrate、Ensure 或 rebuild。成功只说明 Snapshot 投影可用，不授予 SEARCH 权限，也不保证某个具体查询或动态 State 的能力。
- `RefreshState` 对固定 commit 逐 Binding lookup，用 `UnitObservation` 区分 observed null 与未观察；Serving State 落本地有界批次不可变存储，在发布前写入含来源绑定、声明提交、访问与观察摘要的描述符并同步目录。OpenSearch 以 500-doc streaming warm rebuild 发布独立 generation，不保留全量 map 或在响应中返回全量 observations。
- `StateProjectionReady(ctx, repo, commit)` 为维护读取已发布的 Serving State 介质和实际 State
  提供方 Meta，核固定依据、READY、access/physical digest、revision 与 observation digest。
  未发布或介质丢失返回未就绪；提供方或本地读取错误保留。它不检索、hydrate、外部取值或重建。
  重启后按提供方确认的精确观察身份只读恢复固定值，并校验完整内容摘要；缺文件不创建，不回退最新值或外呼 runtime。
  `StateView` 只返回进程内已加载观察身份，供查询规划，不证明物理就绪；已有续页由提供方原生游标选择原生成代，不能拿当前进程的最新观察替代。
- 配置持久目录时，正常关闭只释放不可变值文件的只读句柄，不删除文件。未配置目录时首次构建才创建 Index 自有临时根；旧观察文件在同一 Index 生命周期内保留，关闭时统一释放，不在刷新时逐代删除。提交结果不确定的候选也保留；候选文件存在不代表已发布。观察代截止时间、独立证据、配额与权威回收仍在 `INDEX-PHYSICAL-01` 交付范围，不以本地残留文件作为长期保留判定。
- State 查询在已有提供方 Meta 读取后、规划和候选读取前，复核同一已发布观察的固定声明依据、
  READY、access/physical digest、revision 与 observation digest。错依据返回 `PRECONDITION_FAILED`，
  不从驻留旧观察补齐另一代候选，也不现场重新取值。
- `RefreshStateObjects` 只刷新指定动态身份；刷新失败保留原已发布 revision，消费 SEARCH 从同 revision
  Serving State hydrate。通知入站与自动对账的调度归 [control](../control/README.md)。

当前仍未实现通用的多 provider cost-based `RetrievalPlan`。MVP planner 只选择 OpenSearch；它逐
clause Probe，并能证明和翻译嵌套 `All/Any`。`RetrievalFragment` 目前仍是能力解释记录，不是独立
调度的物理分支。OpenSearch 使用固定 typed mapping、Bulk、generation rebuild、独立 control
index，以及绑定 generation 与 basis 的 `search_after` continuation；每页只临时持有 PIT，发布或
增量改变依据后旧游标明确失效。它覆盖
MATCH/EQ/IN/NEQ/EXISTS/MISSING/PREFIX/CONTAINS/range/SORT；SORT 的多值规则固定为 asc=min、desc=max、
missing last。未配置 OpenSearch 时 SEARCH 返回 `CAPABILITY_UNSATISFIED`。

## 文件定位

`SearchAtContext`、`SearchStateAtRevisionContext` 与 `RelationsAtContext` 接收调用方 context；旧入口
使用默认执行策略。通用 [discovery/projection](../../discovery/projection/README.md) 的 `WithSearchBudget` 让一次 Workspace 的成员共享 `SearchBudget`，默认上限为
10,000 个候选、100 个后端页、30 秒；零值选择默认值。准备阶段超时返回临时失败，已建立固定计划后
耗尽预算则返回 partial/claims 与可继续位置。`SearchBudgetExhausted` 区分执行时限与调用方取消。
`ContextRetriever`、`ContextMetaLoader` 以及 `retrieval.ContextRelationRetriever` 是兼容的可选端口；
没有 context 的旧 Schema/Canonical 读取只能在边界检查，不能强行中断其 I/O。

涉及 MATCH 的 Superset 补判要求提供方实现 `ResidualMatchVerifier` 并证明与其分析语义一致。
类型化补判与 Workspace 比较复用 `retrieval.CompareScalarValues`，时间使用 `ParseScalarTime`。
Workspace 扇出、合并和成员 continuation 属于通用 [discovery](../../discovery/README.md)；本包只返回
固定 Repository basis 的有界候选页和查询结果。预算与范围 context 状态只在通用 projection 包定义，
本包通过其公开函数预留页、检查取消、过滤候选并绑定 continuation，不复制 context key。

增量按去重后的变更身份，分批回读两个已证明连续的 basis 后比较投影摘要；每批最多 500 个身份，
不扫描全仓。只声明 text 的字段不产生标量槽位；相同投影只推进 basis，真实变化才写入后端。

| 文件 | 负责 |
|---|---|
| `../../discovery/projection/engine.go` | 通用 Retriever / ProjectionMaintainer 端口、Candidate 与物理 Meta |
| `plan.go` | provider capability 探测与单 provider RetrievalPlan |
| `extract.go` | Canonical value + AccessSpec → provider-neutral `CompiledDoc` |
| `../../discovery/residual.go` | provider 只保证 superset 时，对已 hydrate Canonical 做逻辑补判 |
| `index.go` | `Index` 生命周期、Hook 入口与 id 清洗 |
| `runtime.go` | live / frozen pin 物理引擎缓存 |
| `spec.go` | 固定 commit 的 `AccessSpec` 编译入口 |
| `sync.go` | Ensure / Apply / Rebuild 及投影一致性判定 |
| `../control/` | durable 目标、State notice 队列、HEAD 对账、worker 与显式追赶 |
| `../../discovery/projection/notice.go` | 通用 `ChangeNotice` 入站合同：定位 hint，拒绝正文 |
| `search.go` | 候选检索、continuation、Canonical hydrate |
| `state.go` | State Binding 选择、refresh / RefreshStateObjects、Serving State 与动态 projection 发布 |
| `describe.go` | 固定 basis 的 `IndexDescriptor` |

`ProtectDatasetResource` 只固定已经准备的 Dataset 静态语料，不触发构建或读来源 HEAD；
核对该完整范围的就绪 `Meta` 后向实际提供方取得保护。无托管资源端口的提供方返回空票据；
实际托管提供方缺少确权或元数据不符时失败，不能把空票据作为旧资源迁移。

`SetResourceRecovery` 由组合根连接实例资源权威和发布 resolver。后台 `RecoverResources` 先运行
已接受版本的显式修复/迁移，再对账未决发布和候选；各路径的错误独立报告。具备实例权威时不用缓存查询句柄反推库存，空查询缓存
也照常恢复；兼容提供方没有该权威时保留原 scope 维护路径。

`SetDatasetPublicationLookup` 注入只读的已接受准备证据。Dataset 查询先打开该固定 scope，
再绑定已接受生成代，不经移动来源的 live fallback；临时定义不能借用同名 Dataset 的收据。
旧纯文件版本和无托管资源端口保留固定 scope 兼容读取，已知发布的绑定失败不能走兼容分支。
查询句柄只属于本次请求，关闭它不关闭缓存 owner。静态查询、静态语义窗口共用这一绑定；
观察生成代、查询保护和接管后的修复/迁移仍按相应维护合同处理。

`RepairDatasetPublication` 是显式维护入口：按原冻结读取范围强制准备新静态语料，由提供方
`PublicationRepairer` 观察旧映射版本后接管；健康绑定不调用准备。组合根通过
`SetPublishedDatasetRecovery` 连接已接受版本盘点与 Dataset 当前资格复核，来源查找沿用
controller 的实例准入边界。它不新增 Dataset 修订或改写原收据，查询本身仍不执行修复。
目前盘点未退休的已接受版本；检索保留期限、跨进程清除围栏与旧引用退出仍待生命周期合同交付。

`PrepareLegacyDatasetResource` 为没有准备收据的旧发布按其原固定范围强制重建并取得托管票据。
它不把旧索引名当归属证明；不支持托管资源的提供方不分配票据。组装层沿同一来源准入与
DatasetPin 转换调用该入口，再由文件权威原子补收据；旧定义的来源、依赖和版本号保持不变。

静态 Dataset Search 按已经校验外层格式的原提供方续页，调用
`PublicationContinuationBinder` 选择原受保护生成代，再以其 Meta 复核完整查询和读取范围。
原续页不借当前映射或活动指针。关系分页先用实际提供方持久签名材料认证外层位置，
再选择原生成代，执行前继续复核查询、固定范围和生成代；不支持托管签名存储的兼容提供方
保留原进程签名路径，实际托管存储失败不能回退。观察查询按下述独立接受引用选择固定观察及原续页，不借静态发布映射。

仓观察与未注入 Dataset 权威的观察搜索通过可选 `ObservationBinder` 同时固定生成代与匹配的观察值文件，保留原
SearchView 和原生续页。旧代按精确提供方元数据只读打开并验证完整值摘要；提供方失败、原文件缺失或
身份不匹配都失败，不回退当前缓存。文件校验与打开检查请求 context，锁等待受剩余期限约束。
无该端口的提供方保留原能力边界，不声称支持跨代恢复。

公开交付仍执行当前逻辑 READ 补判及授权：索引观察固定不代表外部值随 Dataset 冻结。结果分别保留
原索引观察与实际回读观察；最新回读不再匹配的候选被过滤，读取失败仍按单元报告，不能借旧缓存变成命中。
Dataset 首个观察未准备的缺能力、仓入口既有的未准备前置条件，以及过期游标和权威读取故障分别保留原错误语义。

构建候选使用排他创建的独立文件，不能通过时间戳路径重开另一个构建者的可变 inode；失败清理只删除
自己的候选文件，不修补已有文件，也不清理其他构建者可能正在使用的共享目录。

`SetDatasetObservationPublication` 为具名 Dataset 注入实际观察接受与只读证据查找。先在外部
取值之前固定实际权威基点，再由接受闭包同步调用准备。托管提供方通过 `PreparedRebuildSession.CommitResource` 连续完成生成代提交和准备保护，
返回构建中登记的原 `PreparedResource`，再由装配方准入、原权威基点 CAS 和确认。
仅接受成功后切换本地服务句柄；确认响应失败仍可能已有接受收据，查询据实际引用恢复，不凭错误撤销。
临时 Dataset 不借具名引用；非托管提供方保留原合同。进程内修订锁只负责本地串行，不代替实际 CAS。

具名托管观察查询使用 `PublicationBinder` 选择权威当前证据；原续页使用
`PublicationContinuationBinder` 和仍有效的原接受证据，原生位置保持不变。允许尝试的证据全部来自
该修订的实际引用，缺失或过期不借活动 ROOT；运输、取消等未知错误不能被其他代的可用性掩盖。
固定生成代与匹配观察值文件仍走同一只读校验与请求释放路径。

接受证据以 `ObservationReference` 传递原观察截止时间。旧观察续页须由
`PublicationContinuationDeadlineBinder` 把该时间带入资源 CAS 准入；仅在查找证据时检查时间不足以
排除跨截止的慢准入。已准入查询仍有独立请求期限，不被观察截止提前结束。

`SetPublicationReferenceLookup` 注入已接受引用的实际资格判定。维护先恢复未决准入，再经可选
`PublicationReferenceMaintainer` 对账已接受引用，之后继续独立候选回收。实例协调器即使没有打开
查询引擎也执行对账，存在实例协调器时不重复跑各查询句柄。未知资格保留并报告，不取消其它独立
维护；接受引用释放不等于物理代、观察文件或独立查询保护已经释放。

静态 `ProtectDatasetResource` 与准备后查询复用同一份固定声明见证核验：按原 pin 解释保留的
Schema 采用地址，核对访问摘要、提供方版本及实例物理配置，才取得保护。READY/basis 相同不能代替
完整兼容；不重扫或构建语料，不选择来源 HEAD。托管提供方返回空票据明确拒绝；携带错误返回的
已分配票据保留给调用方取消/关闭，不丢失句柄所有权。


`PrepareDatasetResource` 合并静态准备与票据交接。需要新建的托管语料使用受保护流式提交，不能先
`Rebuild` 再按移动 ROOT 取票据；缺少该提交能力明确失败。已有兼容语料复用 `ProtectDatasetResource`
核验完整固定声明见证及实例配置后取得精确保护，READY 本身不是见证。旧发布资源的强制重建与具名观察
采用同一受保护提交接缝。非托管提供方保留原准备合同，未新增物理引用要求。


实例 `RecoverResources` 同时调用可选 `ComputationMaintainer`，传入本 Index 的明确运行目录；
没有查询句柄也执行，不从打开的语料反推计算库盘点。计算维护错误与其他独立资源的错误合并报告，
上下文取消时不启动新的维护调用；资源只读视图不能取得此维护权限。

显式 `PrepareDataset` 在建立观察发布检查点前，先对这一已接受修订补做历史准备采用或丢失资源修复；
尚未发布的候选及临时范围沿原准备路径。该单修订维护与本实例的后台已接受修订维护协调，
防止后台为同一旧修订写入准备围栏而使前台观察检查点自行过期。维护仍使用实际权威 CAS，
不把跨进程旧检查点重定基，也不在查询中触发此修复。全实例库存维护与显式单修订维护共用
同一实现，单修订入口不枚举或修改其他 Dataset。

维护协调的前台等待尊重请求取消；被取消的请求不必等后台实例修复完成，也不进入其准备回调。

显式静态/观察构建和普通增量编译可注入提供方的 `MemberComputations`：字段提取与成员构造只缓存
不可变值；完整候选范围、当前归因、来源提交和固定读取依据不从缓存借用。未索引字段改动可保持成员
有效输入不变，单输入或同键模板变更只使对应运算失配。新查询语料仍独立物化，不能以减少计算次数
报告避免了文档重写。文档内容摘要覆盖语义输入，纯向量文本变化也进入普通增量 upsert。

已发布资源修复向 `PublicationRepairer` 传入当前固定解释及提供方/物理兼容合同；资源可读但编译器
已不兼容时，同样从原固定来源构建新代并在实际资格复核下 CAS 更新映射。不能把成功读取旧资源当作
完成配置升级。
