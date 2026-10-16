# REFACTOR-OBJECT-MODEL-01 · 对象模型重构与最终验收（2026-10-14，分支 `refactor/object-model`）

用户决定的顺序 2→1→5→6→3→4→7，最后做验收；开发阶段不做兼容，不推送，按阶段本地提交。用户只评审目标、架构与验收报告；实现裁决由执行方自行决定并记在本条，不再向用户提问（2026-10-15 授权）。

## 目标（做完后系统应当是什么样）

- G1 一个词：对外对内只说 object 与 Dataset；公开形状（命令、动作名、错误码、HTTP 路径、SDK 字段、遥测名）彻底改名，无旧词残留。
- G2 一种存储、一种身份：Snapshot 只有 lakeFS；Gitea 全部移除，包括登录认证与 `/api/v1/` 代理，登录只走 KC 自身身份。
- G3 对象与仓无关：URN 为 `urn:kc:object:<namespace>/<name>`；读取在请求范围（Dataset 或调用者可读的仓）内按 ID 解析，同一 ID 跨仓是同一对象的不同方面，同一方面两处来源报冲突，无权的方面标为无权；仓只出现在写入目标和依据里；旧格式拒绝。
- G4 插件是唯一扩展概念：Schema 只描述结构；索引声明（仓里的 `index` 方面，成员用定位符指向 Schema 字段）与取值来源（方面或成员上，只引用取值描述）都是仓里经 Writer 发布的对象；仓选插件实例后按声明生效；按类型跨仓发现、按 ID 合并；查询归因与采纳反馈形成索引使用链路；Interface 只是 LiveAccess 内部机制。（2026-10-15 用户调整：声明不再写进 Schema，见 discovery／liveaccess／plugins 设计篇。）
- G5 Dataset 只做交付：可见范围、固定版本、消费权限；不拼装对象、不决定插件。
- G6 治理最小化：多环境与评审都用分支解决；KC 只提供分支、条件快进合并、合并前对象校验和 Hook，审批归业务。
- G7 架构清晰：一个能力一个顶层目录，依赖单向；公开 HTTP 接口只有一份封闭清单，由 server 自己持有；`invoke` 并入 `invocation/spec`；会话与登录收进 `client/`；`cli/` 里的合同测试回到所属包；`home/` 拆小；没有生产者的错误码删除。

## 验收标准（最终验收报告逐条给证据）

- A1 门禁：`make check-docs`、`make quality`、`make test`、`make test-contracts`、`make test-all`、`make deploy-local-scenes` 在同一候选上全部退出 0，无 skip、无削弱断言。
- A2 发现通畅：真实部署上，Agent 从 `kc help` 出发，能按关键词、按类型跨仓搜到对象，结果按 ID 合并且只含有权内容；Dataset 范围内搜索只返回范围内版本。
- A3 访问通畅：同一对象 ID 可在部署范围和 Dataset 范围读到；多仓方面组合返回并带版本向量依据；方面冲突、无权、旧格式 URN 分别得到明确错误；LiveAccess 取实时值带依据。
- A4 维护通畅：写入指定仓、分支写入、快进合并、合并前校验与 Hook 否决在真实部署上可复现。
- A5 守卫：旧词、退役路径（gitea/filegit/dolt/controlplane/gate/interfaces/knowledge）、URN 格式、分层依赖、单一路由清单各有自动检查，故意违反时会红。
- A6 架构报告：交付时给出顶层目录与依赖图、每个目录一句职责，以及与 `doc/architecture.md` 的一致性核对。

- [x] 阶段 2（5f9ac0e）：存储只留 lakeFS（保留 Gitea 登录），删除旧存储残留与旧身份迁移；守卫加退役路径与失效守卫名检查。
- [x] 阶段 1（70b9c82）：全仓只说 object / Dataset（`knowledge`、`KnowledgeSet` 退役）；删除 grant `cmds` 与 `dataset.consume` 迁移；术语守卫 `TestObjectIsTheOnlyNameForCatalogContent`。
- [x] 阶段 5：`interfaces/` 收进 `liveaccess/handler`（合同、实现、注册表）与 `liveaccess/dispatch`（调用任务）；删除部署注入额外处理器的入口（`ObjectInterfaces` 选项与可变参数），非 LiveAccess 合同以 `CAPABILITY_UNSATISFIED` 拒绝（`TestPublicInvokeRefusesContractsNoPluginHandles` 取代原 echo 处理器用例）。系统仓删除对外 Interface 说明（概念、分派机制、能力、两篇指南及其关系），`kc invoke` 归入资源访问能力；用户可见文案改为"插件"。顺带清除阶段 2 遗留的 `deployment identity migrate` 命令面登记与文档。
  - 守卫：新增 `liveaccess/dispatch` 依赖规则，`liveaccess/handler` 禁止反向依赖 dispatch；层表与公开深度白名单同步。
  - 耦合基线：显式 `-write-baseline`。传播成本 14.95% 不变；`liveaccess/handler` 不稳定度 0.33→0.43 是注入口删除后入向依赖 6→4 的预期结果，出向依赖不变。
- [x] 阶段 6（fa4f770）：删除 controlplane / gate / 提案命令；Snapshot 公开分支与条件合并；Writer 写指定分支；合并前 Object 校验；保留合并前后 Hook。
  - 形状：`writer branch --repo --name [--from|--commit]`、`writer put/remove/commit --ref`、`writer merge --command-id --repo --from [--commit] [--into] [--expected]`（只快进；`CANDIDATE_MOVED`／`NON_FAST_FORWARD`／`SCHEMA_INSTANCE_INVALID`／`PRECONDITION_FAILED`；只有推进 DefaultRef 才通知目录）；`writer.merge` 前置 Hook 可否决（`HOOK_DENIED`），审批归业务。回执 Surface `MERGE`，遥测 `kc.writer.surface` 同步。
  - 旧测试改写为等价断言（`cli/hook_merge_test.go`、`object/writer/merge_test.go`、kc_test 的 branch/merge 旅程、test-catalog M/I/X 行与错误码表），未删 case、无 skip；场景新节点 `review-branch-written`（两条 probe）取代 proposal 节点。原本命中“命令不存在”的空洞 USAGE_INVALID 断言改成真实 branch/merge 边界。
  - 文档：governance 设计篇重写；object／service／concepts／architecture／affordance／README／roadmap／architecture-invariants（K-07..K-09、K-21 改为分支合并语义）与各包 README、systemrepo 出版物同步；Grafana gate 面板改为 Writer MERGE 面板，telemetry 词表删 `kc.gate.*`。
  - 耦合基线：显式 `-write-baseline`。删除 `controlplane`、`controlplane/service`、`gate` 三个单元后传播成本比例 14.95%→15.18%，但绝对可达对 933→877（分母 79²→76² 的比例效应，不是新增边）；`server/internal` 0.75→0.67、`invoke` 0.57→0.50 是失去对 controlplane 的出向依赖。
  - 验证：make check-docs、make quality（`/tmp/kc-stage6-quality.log`）、make test（`/tmp/kc-stage6-test.log`）、除 cli 外 `go test ./...`（`/tmp/kc-stage6-gotest.log`）、`go test ./cli` 全量、场景树三项与 `_tests`、maintenance_test、perf_tree、check-surface 均通过。
  - 债：`snapshot/access/rule.go` 的 `retiredAction` 有意保留 `governance.`／`operations.gates.` 前缀，用于拒绝旧授权动作；`.data/scenes` 的 role `governance` 仍成立，保留。
- [x] 阶段 3（2dcadfa，设计文档统一另见 62c4abe）（G3）：URN `urn:kc:object:<namespace>/<name>`，范围内解析，方面冲突检测，拒绝旧格式。
  - 形状：对象引用只有 `urn:kc:object:<ns>/<name>[#locator]`，旧 `kc://<repo>/object/...` 与带仓前缀的 objectRef 直接拒绝（不写兼容）；`--repo` 只是范围参数（读取范围或写入目标），不进入身份。持有模型在 `object/holding.go`（`Holding`／`HoldingOf`／`SelectHoldings`／`ComposeHoldingValues`），结果带 `sources`（逐仓版本与方面）与 `withheld`。
  - 裁决：无范围读取在调用者可读的仓内解析，一个都不可读时 `FORBIDDEN`；读权按方面所在仓判定，无权方面列入 `withheld` 不泄露内容；Schema 绑定方面计入所在仓的持有；同一方面两仓来源在三处报 `OBJECT_ID_CONFLICT`：读取、Writer 写入、Dataset 发布。Dataset 接受范围内未绑定引用。
  - 发布冲突检查放在 `discovery/datasets.Preparation`，由组合根 `home` 注入 `Conflicts`（`reader.Serving.CheckHoldingConflicts` + `liveaccess.BoundAspectUnits`），与插件配置无关；两个及以上来源而未注入时 `CAPABILITY_UNSATISFIED`，不跳过。只翻对象 ID（`SnapshotObjectPager`，每页 1000），只读取被多个仓存放的 ID；范围按 Dataset 读取的整对象规则判定。注入而非让 `discovery/datasets` 直接依赖 `object/reader`：后者让耦合棘轮报 `discovery/datasets` 不稳定度 0.50→0.57，注入后传播成本保持 15.18%。
  - `home.DatasetPin` 是 Dataset 解析结果到读取依据的唯一生产转换（`invocation.DatasetPin` 删除，invocation／server 改调它）。
  - 关系：端点只按 ID 引用；`relations` 不要求端点被持有；`traverse` 中被范围持有的端点才算在范围内，否则标为 boundary；遍历事件只记录边。
  - 修复的真实缺陷：CLI `read` 输出整形丢弃 `sources`、`withheld`、`failures`（`object/cli/output.go` 白名单），由新旅程 `TestObjectIdentityComposesWithholdsAndConflictsAcrossRepositories`（`cli/object_identity_scope_test.go`）发现。
  - 用例改写：liveaccess 验收的冲突断言前移到 Dataset 发布，并补无范围读取冲突断言；cache 测试改为单段 ID；client／cli 内部测试的端点改为纯 ID。未删 case、无 skip。test-catalog V-03 改写、新增 V-15；守卫 `TestRetiredObjectIdentityShapesStayAbsent`（`internal/arch`）拒绝旧 URN 与带仓前缀的 objectRef 回到源码、场景与文档。
  - OpenSearch mapping 随身份字段变化，已有索引需重建（开发阶段无兼容，不写迁移）。
  - 债：(1) 位置 `internal/testkit/testkit.go` `DatasetPin` 与 `declarativeindex/plugin` 测试的 `pluginDatasetPin`；信号：与 `home.DatasetPin` 重复；判定：测试代码不能经 home 导入（会成环/越层）；处置：保留；触发：阶段 7 拆小 home 时把转换下沉到不形成环的位置后合并。(2) 位置 OpenSearch 测试夹具；信号：测试遗留索引在共享容器上触发 HTTP 429；判定：夹具未按用例清理索引；处置：本轮手工清理；触发：最终验收前给夹具加清理或独立前缀。(3) 带固定版本的 `kc://` 跨仓 Schema／Interface 引用随阶段 4b 的 `descriptorRef` 一起改。(4) `SceneViews` 登记的三条场景缺口在最终验收真实部署走查中复核。
- [ ] 阶段 4（G4、G5）：插件声明移出 Schema，按仓内声明生效；跨仓按类型发现并按 ID 合并；Dataset 收窄为范围、版本与消费权限。拆成下列子步骤，按顺序做，每步各自跑到绿：
  - [ ] 4a Schema 只描述结构：Schema 解释器删除字段 `access` 与 `origin`／取值声明，Writer 遇到旧写法拒绝；`object.SchemaDefinition` 的 `Bound()`、`BindingSource()`、`BoundDeclarationDigest` 退役。受影响用例（object/writer、object/reader 里带 `access` 的 Schema 夹具、test-catalog B-01..B-04 等行）改写为等价断言，不删 case。K-26 按变更规则同时改不变量文字、反例测试和证据：新表述为“索引声明与取值描述不含插件实例、物理存储载荷、凭证或对象身份的替代定义”。
    - 进展（随 4b 完成）：`origin`／取值声明部分已完成——Schema 写 `origin` 报 `SCHEMA_UNSUPPORTED`，单元头 `origin` 与旧 `value_source` 头报 `USAGE_INVALID`；`Bound()`、`BindingSource()`、`BoundDeclarationDigest` 已删除，B-01..B-06 改写为等价断言。剩余：字段 `access` 是索引声明，随 4c 迁到 `index` 方面时一起删除，S-01／K-26 届时改写（`architecture-invariants.md` §2.3 已注明）。
  - [x] 4b LiveAccess 声明：单元 `valueSource` 只接受 `descriptorRef`，内联 runtime／protocol／operations 拒绝；ResourceDescriptor 增加 `origin` 与 `mode`；`descriptorRef` 按 Schema 引用规则跨仓按 ID 解析并可固定版本；方面级与成员级取值来源互斥由表示一致性检查覆盖；`resolveLegacyDeclaration` 成为唯一路径并改名，Schema 路径删除；声明摘要覆盖单元与取值描述两个版本；`credentialOrigins` 仍是凭证可信范围的唯一来源。新用例 liveaccess#U21。
    - 形状：唯一写法 `valueSource: {kind: binding, binding: {descriptorRef}}`，写在方面或成员单元上，binding 单元不带值（读取 Value=nil，对象值中为 null 键）；ResourceDescriptor 字段集合封闭（kind、name、description、origin、mode、runtime、protocol、access）。唯一解释路径 `liveaccess.ResolveRepoBinding`／`ResolveBinding`／`ResolveBindingAt`／`ResolveDeclaration`，`DescriptorScope` 由 `RepositoryDescriptors(repo, commit)` 或 `ServingDescriptors(scope)` 提供；声明摘要 `BindingDeclarationDigest(unitDigest, descriptorDigest)`。CLI `writer put` 带 `--value-source` 且不给值时写 binding 单元（`object/cli` 与 `invocation` 两条 put 路径）。
    - 裁决：(1) 方面级与成员级声明互斥由结构保证（一个方面只存为一个记录单元或一组成员单元，冲突报 `OBJECT_ID_CONFLICT`），删除单独的 `checkDeclarationExclusive`。(2) `bindings.index` 改为 `[]BindingCoordinate`（binding 单元地址及其采纳的 Schema），由涉及对象的最终单元集精确维护。(3) Member 来源不再按 Schema 推断，只看单元自身 `valueSource`。(4) `ApplyUnits` 中带值的 PUT 一律发布为快照单元。(5) 新增 `collapseRebuildChanges`，显式重建时同一路径只保留最终变更。(6) hydrate 先解析 binding 单元采纳的 Schema 再调用来源；Schema 不可用（未发布、范围外、跨仓不可解析）时本地失败，不访问来源（fail closed）。(7) hydrated 观测中的 `DeclarationDigest` 已是组合摘要，`discovery/fields.go` 直接比较，不再二次组合。(8) 同一请求范围内 descriptor ID 唯一，多仓持有同一 ID 报 `OBJECT_ID_CONFLICT`；多仓夹具按仓命名 descriptor 或只让一个仓持有、其它仓跨仓引用。(9) 由 Schema origin 推断出的“虚拟 Bound 方面”退役：没有维护单元的方面明确失败，不按 Schema 合成；原用例改为显式 binding 单元（未发布的单元、Schema、取值描述三种证据各一个用例，`TestLogicalBindingReferenceRequiresPublishedUnitSchemaAndDescriptor`）。(10) Stream 不再在投影编译期按 mode 跳过（单元声明里没有 mode）；Stream 不产生普通观测，编译器只看到未观测的 binding 内容，不进文档（`TestProjectionCompilerSkipsUnobservedBindingContent`）。(11) ResourceDescriptor 是实体对象，会被索引；相关计数断言按实际对象数加一。(12) Schema 采纳引用包含 binding 单元采纳的 Schema。(13) binding 单元的 Schema 在 Dataset 版本边界外时，该单元本地报 `ValueFailure`／`OBJECT_REF_UNRESOLVED`，整体读取不报错。(14) “删除 Schema 让值失效”的场景改为删除 binding 单元。(15) binding 成员没有固定叶子：叶子读取先报 `OBJECT_REF_UNRESOLVED`，经逻辑读取采用观察后再选取。(16) 场景探针 `invoke-missing-capability-denied` 原靠 descriptor 缺 origin 得到 `CAPABILITY_UNSATISFIED`；origin 现为必填，改为调用未声明的操作，错误码不变。(17) 清河 `table.stats` 改为每张表一个 binding 单元引用 `resource/shop-stats`，接入方采集器为新采集到的表同样产出 binding 单元。
    - 用例改名（按新语义，不删 case）：`TestLogicalVirtualBoundReference…`→`TestLogicalBindingReferenceRequiresPublishedUnitSchemaAndDescriptor`；`TestVirtualAspectUses…`→`TestBindingAspectUsesHistoricalAdoptedSchemaWithoutSiblingReads`；`TestProjectionCompilerSkipsStreamBindingUnits`→`TestProjectionCompilerSkipsUnobservedBindingContent`；`TestIngestRejectsInstanceBinding`→`TestIngestRejectsRetiredValueSourceHeader`；`TestHTTPStateLookupRequiresSchemaOrigin`→`TestHTTPStateLookupRequiresDescriptorOrigin`；场景 `binding-resolves-schema-origin`→`binding-resolves-descriptor-origin`。
    - 守卫与基线：`internal/bindingfixture`（测试夹具，只依赖 object）分到 object 层；LiveAccess 所有权守卫的根入口符号改为 `ResolveDeclaration`。耦合基线显式 `-write-baseline`：新增 1 个单元，传播成本 15.18%→14.83%（收紧）。
    - 文档：test-catalog B-01..B-06、B-13、B-23、U3、U15，architecture-invariants D-01、K-28，object/writer、object/format、liveaccess、liveaccess/service README，系统仓 `dynamic-access` 说明、`system-explanations.json` 源文件指向与 CLI 帮助同步。
    - 验证：`go vet ./...`、`go test ./...`（含 cli 全量，`KC_TEST_OPENSEARCH_URL` 指向本机 OpenSearch）、`make quality`、`make check-docs`、`go run ./scripts/coupling -strict`、`.data/scenes/_tests`（58 项）、`perf_tree.py --check` 全部通过。真实部署验收留给最终验收。
    - 债：(1) 位置 `.data/scenes/**/_meta.yaml`；信号：仍引用已不存在的测试名（`TestCatalogAuditIsGitLog`、`TestRegisterRetireArchive`、`TestRemoteAccessDescribeSendsWorkspaceNotRepository`、`TestHistoricalDatasetPinSearchHasNoIndependentProjection`、`TestDeploymentIdentityMigration…`、两个 `…OnLiveGitea`），本轮已修正自己引入的三处；判定：存量漂移，且同类信号已出现两次以上；处置：升级为守卫（`_meta` 引用的 Go 测试名必须存在）；触发：阶段 2b 删 Gitea 用例后、阶段 7 之前加守卫并清掉存量。(2) `liveaccess/discovery/source.go` 中 4c 的债务注释保留，随 4c 处理。(3) 阶段 3 记录中的 `liveaccess.BoundAspectUnits` 等名字是当时的形状，现已由 binding 单元路径取代。(4) 阶段 3 债 (3) 未在 4b 内完成：位置 `object/schema_ref.go:29`、`object/identity.go:23`、`liveaccess/handler/resolve.go:101`；信号：Schema／Interface 引用仍接受带仓的 `kc://<repo>@<commit>/<id>`；判定：`descriptorRef` 按裁决只用 ID、版本由请求范围或 Writer basis 固定，不需要带仓形式，而 Schema 引用的带仓形式与 G3“仓只出现在写入目标和依据里”不一致，属公开形状；处置：阶段 7 公开形状清理时改为 `<id>[@<commit>]` 并拒绝 `kc://`；触发：阶段 7 开始。
  - [ ] 4c 索引声明：系统仓发布索引声明 Schema；Writer 校验每个成员的目标定位符能在所引用 Schema 上解析；declarative-index 改读来源仓同一版本里的 `index` 方面（同字段多成员用途取并集），Schema 改动导致目标失效时报失效；查询中未声明的仓单独列出；Dataset 准备按冻结版本读取声明；LiveAccess U20 改为由索引声明驱动。新用例 discovery#U7，U1～U4 按新声明位置改写。
  - [ ] 4d 索引使用链路：查询结果证据记录参与的声明成员（声明 URN、成员键、版本）与角色（过滤、排序、打分）及因未声明拒绝的条件；回读访问证据关联查询；反馈支持对回读命中表示采纳／不采纳并关联回读。新用例 discovery#U8 中“记录”部分；按使用记录生成建议属未到，不在本阶段。
  - [ ] 4e 原阶段 4 剩余：按类型跨仓发现、按 ID 合并并列出未声明／未就绪／未选插件的范围；Dataset 收窄为范围、版本与消费权限。
  - 待确认（不阻塞 4a、4b）：索引声明按“发布在哪个仓就对该仓生效”实现；用户若改为按类型全局生效，4c 的声明来源与 Dataset 冻结依据要改。
- [ ] 阶段 2b（G2）：移除 Gitea 登录认证与 `/api/v1/` 代理路由（阶段 2 当时保留了登录，2026-10-15 用户确认不再需要）。
- [ ] 阶段 7（G1、G7）：目录重组与无生产者错误码清理。路由清单裁决：`httpsurface` 与 `server/routes.go` 是同一份接口清单的两份手抄，删除 `httpsurface`，清单只留在 server，`validation-inventory` 改读 server 导出的金样文件，守卫改为"server 实际注册 == 清单 == 金样"。
- [ ] 最终验收（A1–A6）。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。2026-10-15 用户在最终验收前中止：接下来先重构声明式索引与 LiveAccess。阶段 1、2、3、5、6 与 4b 已提交（`5f9ac0e`、`70b9c82`、`e79e3fa`、`fa4f770`、`2dcadfa`、`94383e5`）。去向：4a 剩余的字段 `access` 并入 `INDEX-DECLARATION-01`；4c、4d、4e 按新设计（插件按形状认领、索引声明选进 Dataset 才生效、搜索只在 Dataset 内）由 `PLUGIN-INTERPRETER-01`、`INDEX-DECLARATION-01`、`INDEX-SEARCH-SPACE-01`、`INDEX-USAGE-01`、`DATASET-REVISION-01` 承接；2b → `GITEA-LOGIN-RETIRE-01`；7 → `ARCH-LAYOUT-01`；最终验收 A1–A6 → `FINAL-ACCEPTANCE-01`（A2、A3 按新设计改写）；4b 登记的场景元数据测试名债 → `DEBT-SCENE-META-TESTNAMES-01`；阶段 3 登记的债 (1) → `ARCH-LAYOUT-01`，(2)(4) → `FINAL-ACCEPTANCE-01`。
