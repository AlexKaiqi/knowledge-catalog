# object/writer/

Writer 是② Object 写入面：一次一个 target、一种 Surface、一个 `command_id`。字面路径写入由 [Snapshot RawWriter](../../snapshot/write_contract.go) 承担。

```text
COMMIT [--ref] / MERGE → Object tree codec → snapshot.TreeStore authority
ChangeSet          → PUT / REMOVE Address
```

文件型 authority 的 codec 只读取本次 Operations 涉及的对象 locator 与 unit，并在同一
TreeChangeSet 更新每对象一个的 `.kc/object-locators/objects/*.entry`；不存在单对象提交时
重读/重写全仓 manifest 的稳态路径。旧 whole-repository locator 会失败关闭并要求先做显式
维护入口 `RebuildTreeLocators` 迁移，普通 Writer 请求不会暗中全仓扫描。文件 codec 整批消费根包的
`object.ApplyUnits`；provider-neutral 单元变更和最终组装只有一份规则，native provider 的 typed
ChangeSet 合同须保持同一语义，实际文件位置和序列化由格式层处理。

新提交和显式 locator 重建在派生条目中固定每个 Address 的实际 Path，位置迁移仍保留原身份。
读取无需从文件名猜测 Aspect/Member；旧 paths-only 条目保持原完整读取与维护入口兼容，精确
选取通过格式层明确报告缺少坐标，不隐式补读其它单元正文。Canonical 正文格式不变。
Schema 变更同步更新 `.kc/object-locators/schemas.index`；`bindings.index` 列出全部 binding 单元及其采纳的
Schema（`BindingCoordinate`），由涉及对象的最终单元集精确维护。旧布局缺 binding 目录时，
写入要求显式 `RebuildTreeLocators`，不能隐式扫描全部单元或遗漏未改的 binding 单元。
同一提交按涉及对象的变更前与最终单位集维护每 Schema 的引用者条目及完整性标记；省略
SchemaRef 的继承、改用另一 Schema、路径迁移和整体删除都沿唯一单位代数计算。Schema
演进只读取该固定版反向条目与引用者维护单元，旧版本 Schema 也用精确 locator；没有完整
反向元数据时明确要求重建，不隐式遍历全仓或把未知引用者当成零引用。

PUT 替换一个完整 Address 单元，可携带 `schema_ref`、provenance 与 `valueSource`。外部取值只写在方面或成员单元上：`valueSource: {kind: binding, binding: {descriptorRef}}` 指向仓内的 ResourceDescriptor（来源 origin 与取值方式 mode 写在取值描述上），binding 单元不带 Snapshot 值；Schema 写 `origin` 报 `SCHEMA_UNSUPPORTED`，旧 `value_source` 头与单元头 `origin` 报 `USAGE_INVALID`。descriptorRef 依次在本 ChangeSet、目标 basis、其他仓 DefaultRef 中按 ID 解析，多仓同 ID 为歧义。动态值要沉淀为对象时，墙外 Collector 显式翻译为 Snapshot ChangeSet 再 COMMIT。

更新未指定 `path_hint` 时保留该 Address 的原有路径；显式指定才迁移。原生 provider 接收原始 ChangeSet，合并现有单元后仅为尚无路径的单元分配默认路径，Writer 不提前把路径省略改写成迁移。

`Ingest` / `Reconcile` 只产生 ChangeSet 预览，不是采集框架，也不是 ingestion control（面 3 只追 published HEAD 的派生投影）。产品 CLI 的目录写入是 `writer commit --dir`；一次性接入大量文件时可加 `--bulk` 显式请求介质的批量写通路——只换对象传输原语，CAS 与 commit 语义不变，介质未提供该能力时整条确定性拒绝；可选 `kc diff --dir` 看同一对照，不把 ChangeSet 交给调用方。HTTP Writer 仍收 ChangeSet（含 `bulkIngest`）。PROPOSAL 只推进 candidate Ref；ControlPlane merge 才推进发布 Ref。人在自有 git 上改 frontmatter 不经过本 CLI 时，进 published 的对象发布仍须过与 COMMIT 相同的校验（CI 代发或只验不写，见 `tasks/done/WRITE-VALIDATE.md`，方向在 `doc/roadmap.md` 的“未到”）；本包本轮不新增 validate-only 命令。部署发布 `systemrepo/content` 完整出版物由 `systemrepo.PublishSystem` 聚合并调用专用 Writer：只写入空的实际 `kr://kc/system` Snapshot，已有内容、Address、Schema 绑定和路径必须与当前版本一致，额外或缺失内容文件拒绝。运行时 Writer 保持拒绝系统仓写入。

目录输入（`Ingest`）可以是目录，也可以是 `.tar` / `.tar.gz` / `.tgz` 归档，用 `archive//子目录`
指定归档内充当仓根的目录。归档按顺序流式读取，文件由有界并发读取并解析，第一个错误按文件顺序报告，
结果与读取同一棵目录树逐字节一致。归档中的越界路径、符号链接等非常规文件、重复条目、空子目录拒绝；
隐藏文件/目录与目录输入一样不参与。大仓目录中逐个打开小文件是本机文件系统往返的主要成本，归档输入把它降为一次顺序读取。

首次上传的空树快路自动生效：Writer 通过可选 `DirectoryReader` 独立确认固定 `BaseCommit`
的根目录为空，对这一不可变基点的规划读取直接返回缺失，省去逐对象旧值读取。任何已有条目
（包括 `.kc`）都使基点按非空处理；不具备目录读取能力时保留原流程，读取失败则拒绝。
Schema、同批路径冲突、操作前置条件、CAS 和命令摘要仍完整校验；实际写入和通知沿原 authority。
应用层 writer head 响应的 `empty` 是同一 `commit` 的可选布尔信息：已确认为空才为 true，
非空为 false，能力未知省略。CLI 仅在未显式指定 `--base` 且该信息为 true 时跳过逐对象对照；
ChangeSet 无需声明“首次上传”，服务端不信任客户端的空树提示。

非空基点同样不重复付费：一次命令内 Schema 规划、tree codec 与路径准入共用同一不可变
基点读取视图，每个 `(commit, path)` 至多读 authority 一次（缺失结果同样复用，传输失败不缓存）；
相互独立的 locator、unit 与目标路径点读以有界并发预取，消费仍按原顺序进行，错误语义与串行一致。
写入集合中与基点字节完全相同的文件（典型为更新时不变的 locator 与完成标记）不再发送给 authority；
可验证无变化路径另按下文处理；未提供该能力的介质保留原写入行为。

幂等规则：同 command_id 同 digest 返回原 Receipt（REPLAYED）；同 id 异 digest 是 `IDEMPOTENCY_CONFLICT`。Snapshot CAS 过期是 `NON_FAST_FORWARD`。带 `schema_ref` 的 PUT 必须在 target commit 可解析。DERIVATION 必须携带固定 inputWorkspaceVersionRef 和 algorithm。

目录对照为零变化时，有已应用命令的重试仍从命令账记录的原始基点重新求差，再交 Writer 核验完整
请求摘要，不能只凭 command ID 或当前正文相同返回回执。目标、bulk 方式或原请求内容不同仍冲突；
无法读取原始基点或缺少核验元数据时明确拒绝，不能把未知状态当作已重放。

Relation PUT 可以引用其他仓的对象；仅校验 `CanonicalRelation` 的结构化信封，不读取或写入端点仓，不为端点发权。单仓 target、CAS、幂等与失败原子性不变。端点是否存在和是否允许读取留给消费上下文处理。

已证明未发布的失败，经错误信封保留 `writeOutcome: "NOT_APPLIED"`；
客户端立即返回原错误，不查不存在的回执，也不自动重发。命令占用释放失败时证明仍保留，
但账本保持 PENDING 和摘要保护，须显式处置，不能把未发布当作已释放占用。
无法确定的发布、原结果无法读取或接受后回执保存失败不附加该证明，保留原恢复边界。

账本先持久化 `PENDING` 再触碰 authority。提交后回执丢失时不得重放；运维以原 digest
`ResolvePending` 写回已核对回执，或以 `AbandonPending` 明确放弃。`Prune` 只删除超过部署
保留窗口的 `APPLIED`/`ABANDONED`，永不推断 `PENDING`；Bolt 实现游标删除，不在启动时加载
全部历史。Canonical 与账本已接受后，次级 evidence sink 失败不反转写入结果。

每个 `schema/*` PUT 先按 System Meta Schema 归一化；未知逻辑 type、字段 `access` 或错误
形状返回 `SCHEMA_UNSUPPORTED`。复用同一 Schema object ID 时先做兼容性 diff；字段删除、
改类型、新增必填、归属/模式变化或约束收紧返回 `SCHEMA_INCOMPATIBLE`，接入方必须发布新
major。`TrustedSchemaOperations` 定义的 Meta Schema（`schema/meta/schema-definition/v1`）与
索引声明 Schema（`schema/core/index-declaration/v1`）是不可演进的内置协议数据，副本必须与其
System 出版物 digest 一致。其余系统出版的 Schema 按普通兼容规则演进，包括 `schema/core/readme/v1`、
`schema/core/relation/v1`；方面名 `readme` 也是普通内容，不保留、不改址。
副本一致性与实例解析是两项规则：只有索引声明 Schema 可经 `ProtocolTrustRootSchema` 免仓内副本解析。
兼容的文档变化还必须对固定 basis 上全部引用实例成立：Writer 经有界
`SchemaReferrerLocator` 取回引用者并逐个校验，失配返回 `SCHEMA_INSTANCE_INVALID`；同批
PUT/REMOVE 的 Address 由本批结果承担。REMOVE 一个仍被引用的 `schema/*` 返回
`SCHEMA_INCOMPATIBLE`。带 `schema_ref` 的实例 PUT 使用同批 Schema 草稿或目标仓固定 basis
校验；**省略 `schema_ref` 时继承该 Address 已存储的声明并同样校验**，不符合合同返回
`SCHEMA_INSTANCE_INVALID`，且不推进 Ref。binding 单元只声明 Schema 与取值描述，不写入动态值。

同一方面的方面级声明与成员级声明互斥（方面只存为一个记录单元或一组成员单元）。消费时由
LiveAccess 采用实际观察并按 binding 单元采纳的 Schema 校验，Writer 不将来源值写回
Snapshot。同提交的固定 locator 记录 binding 单元所采用完整 SchemaRef，保证声明及旧版本可复核。

主要文件：`writer.go` 共用校验与 applySnapshot；`commit.go`、`propose.go` 两个 Surface；`schema.go` 校验 Meta Schema、schema_ref 和实例；`idempotency.go` 解释共享 command ledger 中的 Object 请求；`receipt.go` 定义 durable receipt；`preview.go` 只做预览。

```bash
go run ./cmd/kc -- writer put 'urn:kc:object:schema/service.health' \
  --command-id schema-1 --repo kr://acme/public/core \
  --value '{"entity":"Service","aspect":"health","pattern":"record","fields":{"status":{"type":"string"}}}'
go run ./cmd/kc -- writer put 'urn:kc:object:index/service-search#/index/status' \
  --command-id index-1 --repo kr://acme/public/core --kind Member \
  --schema-ref schema/core/index-declaration/v1 \
  --value '{"catalog":"declarative-index/opensearch@2026-10","inputs":[{"from":"schema/service.health#/fields/status"}],"build":{"kind":"keyword"},"uses":["filter"]}'
```


索引声明是采用 `schema/core/index-declaration/v1` 的成员单元，Schema 字段不再接受 `access`。
Writer 检查声明形状及输入定位符在所引用 Schema 上能否解析；构建方式是否被具体实例支持，
由实例能力清单核对。删除或收窄索引成员不改变值结构，旧 pin 保留旧声明。
`CheckIndexDeclaration` 复用 COMMIT 的结构和 Schema 校验并返回按输入顺序解析的字段，
不写 Snapshot、回执账本或 evidence；它不预判存储路径准入或未来 CAS，也不判断实例能力。
字段删除、改类型、新增必填、归属/模式和引用约束等结构守卫保持；上述两份内置协议 Schema
副本仍须精确匹配系统出版物。仅名为 `index` 而未采用声明 Schema 的业务内容不产生查询能力。

具备 `TreeWriteVerifier`、稳定 authority 绑定与条件阶段账本时，COMMIT 在完整 Schema、
单元代数、路径、provenance 及前置校验后，逐项核对计划树效果与原固定基点的字节。
全部相同才先耐久限定为无变化处理，再请求 authority 完成写入准入与最后 ref 条件检查。
验证成功且最终回执已可靠保存才返回 `result.changed:false`；oldCommit/newCommit 均为原基点，
不创建 commit、不发 `Advanced`。字段投影、本地状态 digest 或当前正文相同都不是替代判据。

lakeFS 仍通过所选的普通或 bulk 数据通路在隔离 wip 检查实际写入权限，再持有原发布锁复核
原 ref、存在性、归档与 expected 条件；不会因 candidate 等于 expected 而借用别人的锁。
这个验证会产生暂存 I/O，不能把“不提交”描述成“不访问写入介质”。原生或其它介质未提供
等价验证时不虚构无变化成功；其既有写入路径和已知／未知结果保持原合同。

正常提交在调用 authority 前记录可能开始提交的阶段。只有无变化阶段允许重进原请求，恢复
再次校验原基准，不能换成新 head；变化路径或来源绑定不符拒绝。最终回执未保存前的内存
判定不算成功，保存后响应丢失才重放原结果。无变化命令各有独立 ReceiptRef，旧回执缺少
changed 保持未知；Go 使用 `object.ChangeState` 保留可比较的三态，JSON 仍为可选 Boolean。
MERGE 按实际目标 commit 是否推进报告 changed，内容树相同也不能抹去评审版本的推进。
目录发布提交完整原始操作，不在客户端按正文摘要过滤；服务端 `CommitIntent` 首次固定基点，
重试沿原坐标核对全部意图。显式不同基点、缺失原请求材料与未知提交结果均拒绝；
真正空目录仍不构成合法操作集。旧客户端已过滤的请求仍只能按其原完整摘要重放，
不从旧增量请求推断未曾保存的完整目录意图。

`Lookup` 对已保存回执返回原结果；对有无变化阶段证明但尚无回执的记录，只返回原请求基准和
空回执，供 `CommitIntent` 与维护地址解析恢复固定范围。它不把 PENDING 转成成功，普通未知
结果仍不提供自动重试许可。请求 Hook 区分这次重新判定与已完成回执重放，前者重新执行正常
前置和成功后置，后者保留既有不重复投递约定。
