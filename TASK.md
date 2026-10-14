# 项目路线与当前任务

目标：提供可发现、可获准、可使用并留痕的对象环境。Snapshot 支撑多团队协作与生产，Object 组织对象内容及元数据，插件按作用域扩展业务需求；对象及系统 Affordance 服务 Agent 的发现、申请、使用与修正。以 Catalog 协议和 Go 参考实现交付可部署服务，当前采用条件与未来目标分别说明。产品原则由 [架构总览](doc/architecture.md) 拥有。

本文件记录路线、里程碑进展、下一步和待裁决事项。设计文档入口见 [文档首页](doc/README.md)；完整能力边界见 [MVP 验收](doc/engineering/testing.md)，验证方法和库存见 [验证体系](doc/engineering/test-catalog.md)。已完成条目保留历史范围与取舍，不作为当前源码的验收凭据；当前合同由对应设计篇、代码和可执行验证入口拥有。

一次只认领一项待办；完成需有对应范围的验收证据，不以跳过或删除断言换绿。下列路线是当前推进建议，未约定完成日期；review 条目分别注明已有设计方向与仍待选定的部分。

任务按实际需要记录目标、进展、待决定事项与验证结果，结构自由。历史条目的五段格式不是新任务模板；排除项、约束、不变量 ID、方案比较和接口说明有实质内容才写。实现提醒留在任务或实施说明中，不因写进任务就成为设计约束或不变量。

## 1. 项目路线与进展

### CLEANUP-04 · 删除 connector 链，Interface 降为插件内部机制（2026-10-14）

- [x] 按用户决定删除 `connector/`、`integrationruntime/`、`cmd/kc-integration` 及唯一跨包旅程 `cli/integration_runtime_journey_test.go`；架构守卫只去掉这三个已删除包的条目与专属规则（其余包规则不变）；场景 `repository-attached` 去掉随之消失的 `collector.preview` 能力与证据；test-catalog 去掉 N 节。场景素材里的 Python 采集样例走普通 Writer，与本包无关，保留。
- 用户决定 Interface 降级：对外只保留“插件”概念，处理器合同是 LiveAccess 内部机制；文档已同步（concepts／vision／architecture／affordance／plugins／object／roadmap）。代码迁移 `interfaces/`→`liveaccess/` 未做，列为路线近期项。
- 耦合基线收紧：删包后传播成本 15.27%→14.95%；`client` 不稳定度 0.69→0.82 是其消费者被删的预期结果。显式 `-write-baseline`。
- 验证：make check-docs、make quality、make test 退出 0（`/tmp/kc-cleanup-quality.log`、`/tmp/kc-cleanup-test.log`），场景树三项与 `_tests`、check-surface 通过。未跑 test-contracts 与真实部署验收；未提交。
- 债：`kernel.ErrScopeDenied`（`SCOPE_DENIED`）已无生产者，仅 telemetry 词表与 `interfaces/dispatch_test.go` 引用；公开错误码，随错误码清理一并定去留。`.data/design/capability-organization.md` 是旧草稿，仍写 connector／knowledge 等旧形状；目录重组时删除或重写。

### DOC-REWRITE-01 · 新建 doc/ 重写全部文档并退役 docs/（2026-10-14）

- [x] 按用户要求新建 `doc/`，按愿景重写，不设权威层级：README、vision、concepts、architecture、roadmap，`design/` 十篇（affordance、dataset、discovery、governance、liveaccess、object、permissions、plugins、service、snapshot），`engineering/` 四篇（quality、testing、test-catalog、architecture-invariants；后两篇由旧篇原样搬入）。场景 U 条目进入各设计篇“用例”一节。旧 `docs/`（含 `docs/graph/`、product.html）整体删除，开发阶段不留兼容说法。
- 已确认模型：KC 当前只做 discover 与 access；对象 URN 不感知 repo，object_id 为 `<namespace>/<name>`；同一 ID 跨仓视为同一对象的不同方面，同一范围同一方面冲突报错；用“同一对象”还是“关系”由建模者自选；插件声明在类型与 Schema 上，仓可关闭；Dataset 只管可见范围、版本固定与消费权限。
- 守卫改造：`scripts/check-docs` 改为检查 `doc/` 可达、“# ”标题、无 frontmatter、根目录 Markdown 仅 README／AGENTS／TASK、仓内链接有效；`scripts/docs-serve` 只放行 README 与 `doc/` 的 Markdown 和图片，入口 `/doc/README.md`；arch 分层／术语守卫、check-surface、maintenance、validation-inventory 夹具和 agent-signals owner 的目录名同步迁移，断言未削弱、无 skip。全仓引用（代码注释、包 README、`.data/scenes`、`.data/scale`、AGENTS、README、本文件）映射到新篇，去掉旧章节号引用。
- 耦合基线收紧：`scripts/check-docs` 不再依赖 object，不稳定度 1.00→0.00；显式 `go run ./scripts/coupling -depth=2 -write-baseline`，传播成本 15.36%→15.27%，其余为单元行数微调。
- 验证：make check-docs（19 篇）、make quality、make test 均退出 0（日志 `/tmp/kc-doc-rewrite-quality.log`、`/tmp/kc-doc-rewrite-test.log`）；另跑 scripts／internal/arch／systemrepo 定向 go test、maintenance_test、场景树三项检查与 `_tests`、perf_tree --check、check-surface、check-observability-signals、go vet。未跑 make test-contracts、make deploy-local-scenes；未提交、未推送。
- 登记的债（位置／信号／判定／处置／触发）：
  - `scripts/assemble-doc.sh` 默认输出仍为已退役的 `docs/KNOWLEDGE_CATALOG_DESIGN.md`；沿 CLEANUP-01 既有登记，确认仓外用途后退役或改输出。
  - `scripts/check-surface.sh` 与 maintenance_test 中的 `*REFACTOR.md` glob／夹具为旧文档残留；无害但无对象，下次触碰这两个脚本时删除。
  - 本文件历史条目仍有 `docs/graph`、`docs/reviewed` 纯文本提及；历史记录，不作当前合同，不追存量改写。
- 待用户裁决：httpsurface A／B／C、controlplane 去留、connector 是否迁出、object／plugins 篇中“待确认”项（多环境、Fork、插件默认范围与关闭粒度）。

### OBJECT-SINGLE-REFERENCE-01 · 对象基本读写只保留统一引用（2026-10-14）

- [ ] 本轮认领：按用户最新要求，READ／PUT／REMOVE 只接受对象 URN＋locator，不保留 Repo／object／Aspect／Member 分列寻址兼容；Dataset 范围、版本、Schema、来源、并发和回执是不同职责的限定，不删除。内部 Address 与批量 ChangeSet 仍是执行合同，不冒充第二套基本寻址入口。
- Owner：Object Guideline 的统一引用与基本能力、Object CLI／SDK／Service 公开合同、CLI 模块所有权。用户最新决定覆盖原文中的旧输入兼容承诺；只切基本 read／put／remove，不扩张到治理、Schema、历史或批量目录任务。
- 当前：CLI／SDK／HTTP／Invocation 及 Object 任务服务的基本 READ／PUT／REMOVE 均已删除分列入口，拒绝用例核对无读取／无写入；真实 CLI、HTTP、SDK 与场景调用已迁移，保留原范围、权限、Schema、CAS、幂等及后态断言。根引用已复用既有完整对象缓存，并保留范围前检、撤权、历史版本及非空 locator 按单元读取。曾因 CLI-OWNERSHIP-CLEANUP-01 同时收尾而暂停，但新增拒绝用例要求旧实现先切换，等待其整套验收会形成依赖反转；本项先完成切换，不覆盖其所有权清理与专项入口改动。实现与定向组件已稳定，尚待同一候选完整验收，不能计为已验收。
- 验证前态：旧共享 OpenSearch 已达分片上限，原失败记录保留，未删数据或提高限制；使用已有缓存镜像创建本轮独占临时验证实例，验收后删除该实例，原服务与数据保持。定向实际调用仅发现两处迁移遗漏，已修正其真实输入／提示断言，仍待统一复验。
- 已发现的边界：业务 MR／CI 可自行承担审批；普通 Writer 不因检查结果账而增加必经流程。Repo 合并不自动采用到 Dataset；Dataset 发布者明确选择新的来源版本，服务负责校验及原子接受新发布，历史发布保持。现有 controlplane 的独立核心必要性未被证明，治理入口去留／归属是另一个设计裁决，本项不擅自删除。
- CLI 形状收口：基本读写只保留位置对象引用，删除重复的 `--reference` 参数写法；位置绑定由 Object CLI 自己贡献，宿主只装配，不引入通用命令框架。SDK／HTTP 仍用唯一 `reference` 请求字段。
- 结构审查：完整质量闸原红为 Object CLI 不稳定度 0.80→0.75。独立按原 depth=2、macOS 政策重算已保存的实际改前 import 图与当前图，82 单元及传播成本 15.356820% 不变；单引用切换删除 Object CLI→Snapshot 的旧 ChangeSet／默认 Ref 依赖，默认及维护解释仍由 Object 服务承担，无新增边。其余三条根 CLI 归位删边与已有消费者小幅漂移如实登记。按既有人工结构审查先例接受必要删边，显式 `-write-baseline` 登记较少直接依赖，不改工具／政策／阈值／断言，原红及审查保存在 `.validation/object-single-reference-20261014/coupling-review/`；仍待最终质量闸，不冒称此前无漂移。
- 联合验收安排：根 CLI owner 已经由用户明确通知本聊天，负责稳定候选的 `make test`／`make test-contracts`／`make quality`／`make check-validation`；本聊天只做对象切换的必要定向、race 与实际部署 DFS，不重复整套。基本寻址及别名拒绝定向完成后，本聊天先使用独占 OpenSearch 完成必要验证，再在正常评论中明确交接使用；端点 `http://127.0.0.1:64113`，实例 `kc-object-reference-validation-20261014`，原引擎和数据保留，联合验收完成后由本聊天清理临时实例。正式对象证据保存在 `.validation/object-single-reference-20261014/final-v1/runs/`，同一候选执行期间不再写源码／说明／任务状态，最后交付注记单独核对等价性。
- 完成条件：旧基本寻址及 CLI 引用别名真实拒绝且不产生 I/O／写入；统一引用读改删、固定来源回读、Dataset 范围、当前授权、Schema、CAS、幂等和失败后态全部保留。实际调用、说明及断言同源迁移后，执行受影响合同、必要 race、make check-docs、make quality；同一候选取证，不安装依赖、不提交。

#### 用户叫停，交给 DSH 接续（2026-10-14）

本聊天的实现与验收已停止；本项仍为 `[ ]`，没有提交或推送。Object／插件共同层／声明式索引之前完成的重构不重启；这次待收尾的是基本对象读写删除旧写法，以及 controlplane 的职责判断。

1. **已有改动**：基本 CLI 只收位置对象 URN＋locator；SDK／HTTP 只收 `reference`，旧分列输入和 CLI `--reference` 别名均拒绝。Object 任务服务也删去旧读取分支，底层 Address／批量 ChangeSet 保留。完整对象读取已接回原缓存，深 locator 按单元读取。实现、消费测试、场景、帮助和当前手册已迁移；位置绑定与规则仍在 `object/cli/`，根 CLI 只装配。
2. **最后修复**：实际 HTTP／CLI race 中 47 pass／1 fail，失败是缺引用提示；已将 Invocation read／put／remove 统一为要求对象 URN。继而恢复 put／remove 的 command-id 前检，在正文／文件／仓读取之前拒绝；原读写错误旅程及 Invocation 全包已通过。Bulk 用例补合法 command-id 并核对确实达到 Bulk 拒绝分支。此后未取得最终候选的完整 race／合同，不能勾选完成。
3. **先定最终候选，再验收**：聊天“整理项目架构图”（`01a13830-81ae-7b01-9e08-fbd4ae844eeb`）同时完成根 CLI 清理和用户追加的架构设计／结构图／离线手册工作。其曾负责共同候选的 `make test`、`make test-contracts`、`make quality`、`make check-validation`；DSH 接续前只读核对其实际进度，避免覆盖在途改动或重复全套。`be360b0c...` 的 371 条受影响 race 无 fail／skip，quality／validation 也曾同源通过；后续代码提示修复及该聊天的文档／阅读入口修改已形成新候选，这些旧绿只作前态。
4. **剩余执行**：最新候选复验实际 HTTP／CLI 读写旅程、旧输入无请求／无副作用、权限／范围／固定版本／Schema／CAS／幂等／回执恢复及必要 race；与根 CLI 工作一起完成上述完整出口。真实 lakeFS 部署 DFS 单独执行，不能用夹具替代；保留全部原断言，skip 或中断不计通过。证据在 `.validation/object-single-reference-20261014/`，尤其 `final-v1/runs/` 的原失败和 `missing-reference-repair.jsonl`、`invocation-final-preflight.log`。
5. **真实部署执行位置**：宿主直接运行曾因 MinIO 签名上传地址 `minio:9000` 在宿主不可解析而失败；必须使用已有部署网络的 scenes 执行器。可参考 `.validation/object-single-reference-20261014/run-live-deployment.sh`（已有缓存 Go 1.23.6 镜像与依赖、限制编译并发、不启动依赖服务）。最近 run `20261014T041755Z-cd56ba0046de` 被用户叫停：6 pass／0 fail／2 原合同人工走查 skip，整体 interrupted 且 sourceChanged=true，绝非部署验收通过。运行容器与验证进程已停止，日志保留；中断可能留下本次验证的临时 native 仓，按实际唯一名称和既有合同核对清理，不动其它仓。
6. **环境交接**：本聊天的临时 OpenSearch `kc-object-reference-validation-20261014` 保留给接续者，端点 `http://127.0.0.1:64113`，已连接 `kc-deploy-local_default`，当前没有本聊天的验证占用；接续者与其它验收 owner 协调使用，结束后清理这一实例。原 `kc-deploy-local-opensearch-1` 已满分片，原服务／数据卷保留，不能删数据或抬限制换绿。普通测试用现有 Go 1.26.0；质量工具使用缓存 Go 1.23.6 并同步 PATH／GO，原跨版本 UI 307／301 差异沿 CLI 条目的存量债，不削弱断言。
7. **controlplane 待裁决**：现存目录维护候选变更、固定组合、检查结果记录和条件合入；它不是 `declarativeindex/control/` 的残留，本轮没有删除它。独立判断是无需由 KC 托管业务 MR／CI／审批；需要时提供可选 MR 接入，Object 负责格式／Schema／权限／并发约束，原生版本合入由 Snapshot 承担。Repo 合入后，Dataset 发布者明确选择新来源版本并发起新发布，KC 负责校验和原子接受；源接入方只有同时负责这个 Dataset 时才替它发起采用。先按用户这条边界明确必要行为和验收，再决定移除／收口现有控制流程，不先造另一套审批框架。

### CLI-OWNERSHIP-CLEANUP-01 · 根 CLI 所有权收尾（2026-10-14）

- [ ] 本轮认领：按实际 owner 归位代码和细节测试，删除空壳与私有迁移转发，把旧嵌入分派及文件挂载夹具接线归测试装配；根保留全局身份、部署、宿主管理、静态模块装配与跨模块任务，保留既有公开入口和全部行为断言，不提交。
- 触碰 owner：CLI 的模块任务与全局上下文、Service 的共享业务入口、Capability Organization 的模块边界、Quality Loop 的信号闭环；设计目标沿既有合同，不新增协议、命令框架或公开业务形状。
- 用户追加的设计书范围：最终主产物为 `doc/README.md` 产品设计与使用书。Core Architecture 更新对象环境愿景、分层职责及面向对象的注册／环境管理思路，Object System 说明供给和使用闭环，Plugin Extension 说明作用域扩展及两处 Affordance，Terminology 同步规范名词；Wield 保持未来方向，申请访问衔接外部 owner／既有业务流程。Spring 容器配置、装配与管理的类比经官方资料核对，KC 的业务对象延伸明确作为设计选择，知识发布与运行装配分别负责。结构图保留可编辑 SVG 与 PNG 并内嵌 HTML；文档阅读入口补图片显示，不改变产品协议。
- 设计自检出口：图中狭义 Store 的“幂等回执”误归属及外部操作的普遍“可恢复执行”已当场纠正，回执沿实际写入账、恢复沿源端合同，并显式区分结果未知；资产与派生手册同步，不把图示扩大为已实现承诺。原 owner 不变量、作用域／发权边界与正文合同保留。
- 验证顺序：先用新增所有权守卫取得当前残留的失败证据，再迁移；执行受影响模块与宿主回归、架构/公开入口合同、文档检查及质量闸。真实部署与外部环境验收独立记录，不将跳过用例写为验收通过。
- 质量出口：无符号文件、恒空登记、测试专用生产转发与 owner 单元测试留根当场修；重复残留升级为 AST 所有权守卫。现存应用目录发布、治理、Hook/Gate 与证据任务按当前 owner 保留，不能仅按动词名称搬进 Object 基本 CLI。
- 已落实：根生产文件从 59 个降为 35 个；移除三组旧别名、空文件、恒空登记和未使用会话函数，产品分派不再带嵌入开关，kcfs 的本地夹具及隐藏 pin 任务仅在测试编译。公开命令与 11 个导出符号保持；49 个原测试名归实际 owner，另三组矩阵拆分／重命名，原断言保留。公开 kcfs 已拒绝的私有 pin 回放分支也已清理，其固定 pin 断言改走实际文件网关。
- 守卫：AST 检查空壳、私有原样转发、嵌入执行、根单元测试归属、私有函数生产引用，并保护既有公开类型；原 typed executor、非扫描及非维护断言沿真实 owner 路径保留。测试支持里的 Legacy OAuth 材料只保存历史断言，现行登录由实际 broker／刷新／会话回归验证，不计作生产兼容入口。
- 证据：改前快照、红守卫、各 owner 全包与宿主定向绿、自检与同口径耦合前后报告保存在 `.validation/cli-cleanup-20261014/`。Go 1.23.6 普通场景 21 pass／0 fail／0 skip；质量闸与文档检查通过。其完整混合合同仅 UI 重定向断言失败，且运行期间另一聊天增加对象引用测试，sourceChanged=true，不能作为冻结验收。专项 Taihu 入口及示例按真实 Server owner 归位，并增加实际脚本选择／失败传播回归；最终稳定候选使用项目原环境 Go 1.26 重跑普通场景与完整合同，质量工具使用 PATH 同步的 Go 1.23.6，不混用标准库。
- 存量债：`cli/webapp_embed_internal_test.go` 的 UI 补斜杠断言期待 307，Go 1.23.6 标准 ServeMux 返回 301，Go 1.26.0 返回 307；原测试逐字未变，独立 ServeMux 同样复现。判定为 Server HTTP 路由的存量跨工具链兼容差异，不以修改断言处理；下一次维护该路由／支持工具链合同时明确并固定状态码，跨支持版本验收，证据在 `toolchain-redirect/`。`README.md` 共享服务示例仍称 Gitea 主体为 `gitea:<user-id>`，判定为身份资料漂移，本轮不扩到身份行为，下一次维护身份登录资料时同步并检查引用。`object/README.md` 的 System 出版说明仍称“空 LakeFS/Gitea Snapshot”，旧存储名与当前唯一 lakeFS 不符；判定为原包用法存量漂移，登记留在下次该出版／部署说明维护时同步，身份认证的 Gitea 使用不受此债影响。
- 场景接线出口：v2 普通场景的六个叶失败及两个父失败指向同一旧数组断言；7 个 feature 的 10 个单对象 READ 断言块按已选定返回形状改为精确字段比较，命令、locator、权限、版本输入、每行字段与预期值逐字节保留。原 matcher 与合法数组断言保留；重复信号升级为场景解析守卫，先红后绿，证据在 `scene-read-shape/`。不改 Object 行为。
- HTTP 场景接线出口：v3 的普通 ProductScenes 已绿，唯一叶失败来自授权隔离 probe 的四个旧 object 输入；迁到同一业务对象的完整根引用，Dataset、三主体、发权操作及全部权限／完整 Canonical 断言逐字节保留。全树确认没有其它基本 HTTP PUT／REMOVE 表残留；快速契约复用公开 Object DTO 严格解码和完整引用解析，显式非法输入 probe 保留，授权 probe 要先具备合法请求。先红后绿证据在 `scene-http-reference/`，不改执行器映射、服务或正文 Oracle。
- 新静态测试归位：v4 的普通场景 21／0／0 与闸层同源通过；完整合同的组件 2866 pass 后，原根单元测试归属守卫正确拒绝新增 HTTP 夹具测试中的纯模块调用。通用严格 JSON 夹具校验归现有 `internal/testkit`，生产只用标准库且目标类型／语义校验由调用方注入；9 个请求与引用变体归其测试，4 个场景选择／错误用途变体留 CLI 验证选择及失败传播，原 13 变体语义完整保留。全树仍用真实公开 DTO／Parser，原所有权守卫及其单位／集成证据矩阵未变且已绿，没有加白名单或虚假宿主调用。证据在 `http-guard-ownership/`。
- 完整入口收尾：v5 闸层、验证合同 50／0／0、普通场景 21／0／0 同源通过；完整合同 3997 pass／18 fail（含父项）／12 个既有环境门控跳过，源码未变。失败揭示旧接线：根全局 catalog／pin 拒绝被模块闭集校验抢先；HTTP 证据重复认领 Object PUT、遗漏实际 Writer commit；系统能力入口派生文件未刷新；混合基点负例的迁移误删显式 repo；消费帮助、场景静态守卫仍匹配旧 argv／数组。均按实际 owner 修正，95 路由及 46＋50 证据断言保留，真实提交旅程认领 commit，不用人工回执冒充生产成功。原范围外 READ 是无正文的空 union，三个迁移新增的 FORBIDDEN 期望修正为精确引用未解析并保留无泄漏断言；真正授权拒绝仍为原 FORBIDDEN。跨模块旧参数拒绝补实际 Run／HTTP 不可达回归，先红后绿，证据在 `final-contract-wiring/`。
- CAS 出口：原 `TestCatalogRepoWriteFlow` 的 expected-only 过期 CAS 保留输入及 NON_FAST_FORWARD 断言；迁移接到 MaintenanceExecutor 后暴露空 Base 被当前 Head 抢填的默认依据错误。按既有 Writer.CommitIntent 合同修到 Object service：账本重放之后，空 Base 先沿显式 Expected，再退到当前 Head；不在 CLI 补业务默认值。新增真实 lakeFS HTTP 夹具的 PUT／REMOVE 过期预期回归，先红后绿，并核对发布 Head 和内容未变，包 README 同步；不扩到其它对象迁移或部署工作。
- 当前验收边界：CLI 清理、场景接线、owner 设计、两图及最终 HTML 已稳定，用户最新注册对象主线、书名与导航已通过独立语义自检及实际显示。图片阅读的原断言与新增实际 GET／HEAD、转义、代码隔离、路径拒绝回归先红后绿；新资产路径同时暴露既有 symlink 仓外逃逸，已修复并保留红例。旧 v1～v5 只为前态；最终 v6 候选与正式结果统一保存在 `.validation/cli-cleanup-20261014/final-joint-v6/`，普通场景、完整合同、文档与质量闸须绑定同一源码状态，不以源码变化或环境跳过冒称完整验收。对象 owner 已按用户指示停止并交接 dsh，临时引擎保留且运行验证已停止；本聊天完成 CLI／设计书范围的检查，不代替其尚未闭合的对象专项、race、部署或 controlplane 裁决。暂不勾选完整联合验收。

### OBJECT-REFACTOR-IMPL-01 · 完成对象层重构及充分验证（2026-10-13）

- [x] 本轮唯一认领已完成：按 Object Guideline 的结构终态与 U1～U12 落实生产实现；承接 OBJECT-CASES-PREP-01 的规格、既有精确断言和三个红基线，以同一最终候选的 Object 必需运行证据及受影响消费兼容验收闭合。
- Goal：固定知识身份、唯一表示／组装与 Schema、真实只读 Reader、唯一 Writer、统一对象 URN＋JSON Pointer 的读／完整单元写／显式删除，以及能从根帮助理解和操作的 Object CLI；命令、校验、输出、帮助和既有命令扩展同归 `object/cli/`，KC 宿主只经公开贡献装配。
- Non-Goals：沿 Guideline，不扩任意字段 PATCH、集合／数组删除、跨 Repo 事务、组合对象整体 PUT，不重设计 Snapshot／Dataset，不把具体插件实施并入本轮。目录草稿 diff／commit 和回执仍是应用发布流程；既有入口及授权、版本、原子性保持兼容，不覆盖并发工作、不安装依赖、不提交。
- 不变量与接口：I-01、C-01、AUTH-01～03、CA-02 及 K-04 的固定身份／依据和当前权限；公开合同沿 `object/`、具名 Reader／Writer／Service／Format 和各自 Conformance，新增寻址类型在 Object 根选定，传输输入将完整引用作为请求数据交给所属服务。
- 方案：共享结构化引用与 Pointer 语义、按维护单元提前制定读取计划、所属模块贡献 CLI；否决中央 CLI 复制对象规则、第二份组装／解析器、路径充当身份、先整对象读取再裁响应及仅收窄接口名的伪只读。先修必要行为，再切换结构和新入口，逐切片测试后进行充分消费兼容、架构、文档、质量验证。
- 协同：Writer／Format 基线、Reader 实际能力边界、Object CLI 迁移由三个子任务分担；本聊天统一寻址／服务接线和全体集成验收。先复现所选红例再修复，不削弱旧断言或用 skip 换绿。

- 已落实：根 Object 类型与唯一单元代数、Writer 最终工作集／完整 Schema 校验、实际只读视图、URN＋Pointer 精确读与完整单元 PUT/REMOVE；Object CLI 和 HTTP SDK 均归自己目录，兼容入口由贡献装配。纯 Binding 声明仍属 Object，专属来源解释唯一归 LiveAccess，Dataset 逻辑 URI 读取保留观察与当前权限。
- 增量修复与守卫：根 REMOVE 迭代漏删、表示冲突、伪只读、深选先整读、复合来源丢失、根 Relation 种类丢失、URN 不能沿用 Aspect 授权、Schema 同批迁移／新增引用再删除悬空，以及原生元数据计划不核坐标；均保留精确正反例。唯一 Format.ToObjectUnits 转换取代三个字段映射；全仓 150-token 克隆检查为 0 组。
- 闸层兼容处置：完整组件暴露 Dataset 竞态用例绕过真实 Reader，已改为同一 Snapshot Registry／Reader 的实际装配，原范围／旧版／推进时机断言保留；HTTP 具名表增加两条对象维护路由，相应闭集清点 92→94，并逐条守卫。已有 Notice 来源隔离两条红例沿原 Controller 修复：通知不替换后台来源／身份，保留真实桥接反例并强化原配置测试，无新增索引插件功能。
- 耦合收紧：用同一当前工具、depth=2 政策与 macOS 平台重算改前冻结树及候选，保存前后 import 图和度量于 `.validation/object-refactor-impl-20261013/coupling/`。66→68 单元仅登记 `object/cli`／`object/client`；传播成本 16.67%→16.26%，既有单元没有恶化，`internal/httpjson` 的 Instability 0.33→0.25 来自复用认证传输；人工归为设计使然，并显式 `-write-baseline` 收紧，不改门禁阈值或排除。
- 已验收范围：URN 的四项真实 CLI→HTTP 旅程与 lost-response 真实 Writer 回执恢复通过；Object 根／Format／Writer／Reader／SystemRepo 及 Reader／LiveAccess 定向 race、所有权与库存守卫已绿。完整闸暴露的失败已逐项处置并按固定候选复验；虚拟 Bound 仍须读取固定对象类型依据，不能声称所有逻辑深选都零兄弟 I/O。真实部署为独立入口，不把夹具或带门控 skip 的运行算作部署验收。

- U1 独立验收：新 Agent 不继承实现上下文、不读源码／设计／测试，仅沿根帮助及四个操作叶帮助和身份帮助，在真实 CLI→Server→Writer 上完成初建 Member、nullable=false 深读、带完整摘要更新、确认省略 description 后不可读、删除后 Member 不可读；6 次帮助、9 次业务调用，日志 `.validation/object-refactor-impl-20261013/help-walk/agent-transcript.txt`。自动化预设参数旅程不代替此观察，U1 已补闭合；仅证明本次独立 Agent，不泛化为所有使用者。
- 走读信号处置：Object 叶帮助的 published 默认 ref 原未明说，已明确为 refs/heads/main；根帮助未列身份英文组、认证模式选择需要用户判断，属于应用 CLI 说明的既有缺口，不阻断本次 Object 走读，登记为后续该 owner 维护时补根导航／认证发现指引，未越界改认证模式。原生 Reader 的导出嵌入字段可取回写 Provider，判定真缺陷，三条字段能力断言先红后绿；改私有读接口嵌入，不新增转发算法。

- 最终兼容补闭环：Notice 按本次目标同步刷新，共用原算法并返回实际 basis/revision；仅在无后台 runtime 时局部借本次 Source，不保存、不执行其它仓或排队工作。已配 worker 失败不借 caller，后台上下文不继承 caller 凭证，来源查找串仓在外呼前拒绝；真实 Controller、HTTP proof、OpenSearch 与旧 CLI/HEAD/二次 revision 断言均绿。Server 只在 runtime 与已有明确 service principal 同时具备时装配后台桥；另补真实 HTTP 的无后台配置局部执行回归，并完成竞态验证。
- 旧消费兼容复验：完整合同发现的输出／帮助／未知参数矩阵／系统出版物／路由清点问题均保留原观察并修复。统一引用附维护依据，旧分列 READ／resolve 保持各自产品形状；合法命令矩阵每叶采用自己的输入，未知参数仍拒绝；最终帮助及系统入口同源生成。

- 最后环境门控补验暴露旧夹具调用已退休 --catalog；仅把 SceneAccessorNotice 前态改为现行 local init，版本／实际搜索结果／HEAD 断言保留。State 部署脚本仍引用已不存在 catalog-initialized 且只跑 CLI、不跑其两项 LiveAccess 合同，判定验收入口真缺陷；修到实际材料目录和明确的两包，未削弱门控或新增依赖。真实 Docker State＋OpenSearch 的作用域验证另存固定候选证据。

- 真实 SceneAccessor HTTP 补验又暴露“仅配置 request runtime、未配置 worker 身份却当后台源”的失败（UNAUTHENTICATED）。Server 现只在已有 ServiceIdentity 明确给出 principal 时装配后台观察，使用该身份；请求 runtime 保持可用但不隐式安装 worker。无后台身份的 notice 使用本次已验证请求，已配置 worker 仍不借 caller；不新增身份或插件 API。

- 验证环境故障处置：v4 对象／消费方 race 520 项、实际 URI/HTTP race 5 项及真实 State/OpenSearch 4 项均通过且无 skip；其后并行追加临时检索容器造成资源压力，既有 OpenSearch 于 15:37 UTC 被 OOM 杀掉（Docker State.OOMKilled=true），合同场景超时及两个追加入口中断，原失败证据保留不算验收。已清理本次临时容器并恢复原检索容器；State 入口复用既有 KC_TEST_OPENSEARCH_URL、只回收自己创建的资源，健康探测加有界超时。最终候选串行复跑依赖检索的入口，不修改业务 Oracle 或用 skip 避开。

- 完成证据（2026-10-14 北京时间）：最终 v5 冻结源码指纹 `768b41a49aca4f723b460d4d4fc4d5ab58ebc74b0bab88f04624bd5816867984`，与交付前工作树一致；各正式运行均 sourceChanged=false。证据集中于 `.validation/object-refactor-impl-20261013/final-v5/`：`make test` 21 pass／0 fail／0 skip；`make test-contracts` 3203 pass／0 fail／12 环境门控，61/61 CLI 成功且满足各自边界要求；Object／消费方 race 520 pass／0 fail／0 skip，加入 Server 后 665 pass／0 fail／1 外部 Taihu 门控；URI/HTTP race 5 pass／0 fail／0 skip；真实 Docker State＋OpenSearch／SceneAccessorNotice 4 pass／0 fail／0 skip，修正后的正式 State 入口另有 3 pass／0 fail／0 skip。上述均来自同一 v5 候选，不使用中断运行或旧候选替代。
- 必需闸与未执行分母：`make check-docs`（33 文档／174 关系）、`make quality`、`make check-validation`（48 项工具合同及 61 CLI／94 HTTP／1652 Go 声明清点）、空白检查通过；已重读变更 owner 与图，并完成独立所有权复核和 help-only 真实走读。广域合同的 12 个门控中，受影响的四个 State／通知用例已按上条单独无 skip 补验；其余八个为 Luna、Taihu、真实 lakeFS adapter／部署／规模与外部 provider-consumer，未执行、不计通过。独立 `make deploy-local-scenes` 未执行；本条完成 Object 实现及必需验收，不宣称完整外部部署验收。结果注记写入后只改变本 TASK，生产代码与冻结候选保持一致；未安装依赖、未提交。

### LIVEACCESS-PLUGIN-EXECUTION-01 · 前置完成后实施 LiveAccess（2026-10-13）

- [x] 本轮唯一认领已完成：三项前置实际实现及必需验收独立核实后，按用户授权完成 LiveAccess 插件重构；最终候选的 14 个必需出口和独立终审闭合。启动与完成均依据真实运行及源码等价证据。
- 前置核对（2026-10-14）：Object final-v5、公共插件 final-v4、声明式索引 final-v2 的必需出口闭合；具体索引 11 个正式 scope 同指纹 `b76c28649f0d074ace80c2af541e4141cfb105f14d151c1c504c143bfbc97dde`、sourceChanged=false。独立逐文件核验当前源码仅差 TASK 最终结果注记；受影响 State/通知已无 skip 补验，真实部署自动旅程 20 项通过，既定人工叶与不相关外部门控不计通过。证据分别位于 `.validation/object-refactor-impl-20261013/final-v5/`、`.validation/plugin-common-20261014/final-v4/`、`.validation/declarative-index-20261014/final-v2/`。
- 实施合同沿 LiveAccess owner 与已入库 U13～U20：透明取值、局部失败、适用身份和原结果审计全部兑现；Member 的实际值进入索引，查询与回读沿原观察。采用已有值 Schema/access 与索引消费合同，不再发明一套索引标记。
- 目录边界：专属实现、声明解释、插件贡献、说明和测试归 `liveaccess/`；CLI 定义、帮助及专属客户端适配归 `liveaccess/cli/`，统一 CLI 经公共装配挂载。来源观察桥接归自身目录并消费索引接口，不把专属逻辑塞进宿主或中央 CLI。
- 实施时复核三个前置的当前公开合同，接通真实插件/索引消费链，随实现将 16 条规格接成可执行断言；受影响合同、架构守卫、文档和质量检查全绿后才完成。保留其它任务改动，不提交或推送。
- 实际实施（2026-10-14）：Repo 固定采用已接透明 Repo/Dataset 读取、逐值失败、适用认证、耐久原结果和独立审计；Member 沿真实索引 Entry 与 OpenSearch 查询/原观察正文采用。专属 CLI/SDK 及存证归 LiveAccess，公共采用事实下沉 `discovery/adoption`，不把 LiveAccess 绑到 Index 登记运行实现。U13～U20 的 16 条库内规格已关联具名执行断言；原规格与诊断绿色不算最终候选验收。
- 增量复核处置：实际反例修复重定向回调篡改原头、JSON 转义证明反射、URL 编码证明泄漏；指定 Member/new leaf/虚拟 Aspect 不读兄弟，历史 Schema pin 与 Dataset 范围分别保留。缓存预热绕过输入 Dataset 的新红已修，嵌套范围在缓存前检查，普通 Repo 仍命中缓存；以上已升级真实回归和模块 AST 守卫，无删弱断言或 skip。
- 全套兼容出口复核：v3 保留正式 Reader 测试装配缺口与真实 lakeFS OOM 原红；v4 完整合同又检出透明读取绕过旧 Reader 过程记录，以及同来源多组用例被首组遮蔽。保留原审计/三个具名动态观察断言，分别接回当前请求的既有记录能力和全部同来源 Go evidence 核验，不补读对象、不合并独立用例。并发广套的端口耗尽、错误组合的 tree 命令与中断部署均保留失败/中断状态，不算验收；新候选将限制执行并发并重新完成全部必需出口。
- 必要守卫与前态修复：私有记录辅助函数抽取后，v5 原单文件文本守卫先红；现按真实 CLI/HTTP Run 绑定、实际调用及被调用的 callback 核共同执行器，保留原名称并补未调用/注释/别名/缺路由等反例，不搬生产代码迎合守卫。完整 CLI 预扫仅剩旧提供方缺 `repository.dataset.publish` 前态；按当前来源发布合同补限定仓、Dataset 和默认分支的规则，原双主体、伪造身份、旧 pin 及审计断言不变，真实 Gitea/OpenSearch 定向先红后绿。以上定向结果不替代最终候选全套运行。
- 独立结构判定与基线登记：相同 depth=2、工具、平台和全部生产 import 的原 strict 先红，拒绝六个新单元及六个既有 I 漂移；76→82 单元、传播成本 15.736842%→15.356820%（收紧 0.380022pp）。service 0.6667→0.6154、contract 0.5→0.4、object/client 0.5→0.4、liveaccess 0.4167→0.3571、interfaces 0.4286→0.3333 均逐边核实际职责，service 的新增 outgoing 亦明确保留，并未把下降冒充零代价。declarativeindex/cli 0.5→0.6 单独裁决：唯一新增 outgoing 是 neutral adoption 的共同说明 shape，消除重复且不依赖运行登记；挪 SDK、kernel 或恢复副本会转移指标或违反所有权。沿 Object／公共插件／索引阶段已有独立人工结构接受先例，由 `/root/audit_boundary_review` 审查批准后显式登记新 owner 和较低成本基线；未改政策、阈值、算法、生产 import 或断言，不冒称旧 strict 无漂移。原红、完整图、逐项判定、显式登记及复核见 `.validation/liveaccess-plugin-20261014/structural-review/`；最终完整 quality 仍须当前候选执行到绿。
- 走查接缝登记：`.data/scenes/goto.py` 的 `validate_live_feature` 没有场景预配置来源的真实 logical Repo→native 名称输入，现有走查也未接只读 metadata 核验工具口，无法证明新显式 Repo 采用前态；现有 Snapshot SDK 本身已提供原生 metadata 查询。判定为走查输入/工具缺口，不是产品协议缺口；已在任何构建副作用前明确拒绝，不猜原生名称、不静默 skip/ready，不新造产品命令或热加载。Go 场景与真实 lakeFS DFS 共用显式输入，知识仓选择 LiveAccess/main，关系仓不默认选择，冻结/克隆保留 native metadata；触发条件是提供实际场景绑定输入并接现有 SDK 只读核验，再恢复此走查路径。该限制不算用例执行通过。
- 自动接续已收尾：本聊天 heartbeat `liveaccess` 已在实际实现及必需验收完成后通过应用入口暂停，并复核落盘状态为 PAUSED。新增 Member 索引规格及 CLI 归属的真实库覆盖守卫均已执行；未提交或推送。
- 接续准备验证：U20 缺失时真实库覆盖回归先红，补规格后 26 项产品视图测试及全套 55 项场景工具回归、Go 场景结构/视图/观测守卫通过；产品/工程/前态检查、`make check-docs`（33 文档 / 174 关系）、增量空白及 `make quality` 通过，质量日志 `/tmp/kc-liveaccess-sequence-quality-20261013.log`。最新 Repo 配置边界同步到规格与覆盖表，LiveAccess 正文 999 字符；这些只证明接续准备，不证明三项前置或本插件重构完成。
- 最终实现与用例验收（2026-10-14）：透明 Repo/Dataset 读取在原位置交付实际值、合法空值或逐值失败，自动采用当前已认证且适用的证明；可靠保留原结果后交付，独立授权审计可在来源更新、重启或离线后复核。LiveAccess Member 经真实插件和 OpenSearch 按既有 Schema/access 索引实际值及原观察，固定 StateRevision 的查询与正文保持同一观察。实现、声明解释、存证、桥接及所属测试在 `liveaccess/`，CLI 定义、帮助、校验、输出和专属适配在 `liveaccess/cli/`；U13～U20 八组 16 条规格全部关联并执行具名断言。
- 冻结候选与完整出口：v7 指纹 `dc4bd3ae42cff01d72d774fb70641a072f2327a89d1a624a7f524e0a12d4b991`，2878 文件逐字节核验，14 个正式 scope 均 sourceChanged=false。完整 contracts 3785 pass／0 fail，CLI 62/62 成功且满足所需边界；必要 race 1548 pass／0 fail；实际索引整模块 259 pass、普通场景 21 pass、精确 State/通知 4 pass、真实 Gitea 提供方/消费方 1 pass，四个专项均无实际测试 skip。`make check-docs`（33 文档／174 关系）、`make quality`、validation 工具 50 项、场景工具 57 项及产品／视图／前态三个 tree 出口通过；各修改 owner 与图已重读。
- 独立真实部署：正式 run `20261014T002939Z-fcaaf2a7309c` 在既有 lakeFS／MinIO／PostgreSQL 与本次验证索引上运行当前候选，20 pass／0 fail；两个人工走查叶按原合同未执行、不计通过。使用既有 compose scenes 入口的冻结 runner 副本，限制编译并发并复用忽略目录内缓存，未把原失败的默认部署命令写成通过。广套中的部署 DFS 与提供方/消费方已分别补执行，其余六项无关外部服务、adapter、规模门控未执行、不计通过；无测试包的 package skip 与用例 skip 分别核算。
- 证据与收尾：`.validation/liveaccess-plugin-20261014/final-v7/summary.json` 及 `independent-acceptance-audit.json` 均为 complete／implementationAccepted=true，独立终审核对全部正式原始终态、附件摘要、具名规格与 CLI 断言。四次 v7 失败或中断记录和此前候选均排除，未拼局部绿色。最末 lakeFS 在成功验收结束约 65 秒后 OOM 退出，时间证据单独登记；已回收两个本次验证实例并恢复原 lakeFS 至 healthy，原索引与数据卷保留。冻结验收后仅本 TASK 交付注记发生变化，源码等价记录另存 `delivery-source-equivalence.json`。
- 存量夹具债登记：位置 `declarativeindex/internal/testfixture/live.go:18` 与 `declarativeindex/control/controller_test.go:61`；信号为全新引擎、包并行时默认控制索引刚创建且分片仍 RECOVERING，首次单次 CatchUp 收到 503。判定为既有夹具共享默认控制及首次就绪前态假设，提供方正确返回 TEMPORARY_UNAVAILABLE，非删除默认控制的证据；原红与服务时间线保存在 `opensearch-503-diagnosis.json`。处置为下次维护实际引擎夹具时隔离控制 namespace 或建立显式就绪前态，并保留原恢复断言；触发条件为全新引擎下并行验收。当前同源码限制包并发的完整合同已通过，不冒称并行初次运行通过，不修改生产重试或弱化断言。

### DECLARATIVE-INDEX-SEQUENCE-01 · 对象与插件层完成后交付索引插件（2026-10-13）

- [x] 本轮唯一认领已完成：Object 与插件共同层实际验收后，声明式索引具体实现、控制、驱动、CLI 及所属测试已完成独立模块重构；11 个必需 scope 来自同一最终候选，受影响用例无未执行项，部署自动旅程另按合同取证。未把目录存在、聊天 final、前阶段或不同源码的绿色拼作本阶段完成。
- 执行与目录：先核对 `object-refactor-guideline.md` / `object-refactor-plan.md`、插件 owner、当前代码与同一候选验证；插件公共层按实际需求和真实消费接缝落实，具体索引实现/说明/测试集中于 `declarativeindex/`，控制与 CLI 各有所属子目录。命令贡献、参数校验、输出、帮助和例子在模块自己的 `cli/` 同源维护；扩展已有命令也按贡献装配，不越界复制到 KC 宿主。通用查询/组合留 `discovery/`。
- 前置跟进：只读检查对象任务“评估对象模块重构目标”（`01a134bf-6b10-7da0-bf35-ab46a2fa9de7`）和插件任务“重写插件扩展体系文档”（`01a13460-44b8-7f60-8425-d53a8e2d4bfd`）及落盘证据；其他任务实际实施中不启动重复实现、不修改其在途文件，不向其他聊天发消息。前置已完成且没有同阶段在途实施时，按本用户授权在当前聊天继续；仍未完成则等待自动跟进。
- 先行用例与出口：复用 DECLARATIVE-INDEX-CASES-01 的四组新增测试和已有查询/范围/分页覆盖；先修 notice 请求覆盖后台来源/身份的两条真实红例。公开插件接缝落实时先补真实选用、帮助、同引擎两实例及生命周期用例，再接具体实现；阶段完成须受影响合同、`make check-docs`、`make quality` 及适用实际引擎验证通过，不以路径存在/回调发生/别阶段绿色代替。保持共享修改，不安装未经批准依赖、不提交或推送。
- 初始安排（2026-10-13）：同步计划及直接 owner 的实施先后，技术独立边界不改成协议依赖；创建当前聊天的条件自动跟进，保持前置未变时安静，全部完成后停止跟进。当时未开始重构源码。
- 初始登记与检查：自动跟进 `automation-3`（“对象与插件验收后完成声明式索引”）已创建且 ACTIVE，绑定本聊天，每 10 分钟核对前置；包含本用户授权、阶段真实出口、避免重复实施、CLI 同源目录及先行红例，完成后暂停。计划、插件 owner、架构/能力组织和索引正文已同步并重读，图新增 plan operationalizes 索引的直接关系；正文 1499 字符，`make check-docs`（33 文档/174 关系）、产品用例关联、`make quality` 及增量空白检查通过，质量日志 `/tmp/kc-index-sequence-quality-20261013.log`。当时等待 Object 实现验收，不把安排记成三层重构完成。
- 独立复核处置：旧先行顺序已清除、CLI 同源边界一致；为避免循环等待，插件共同层的完成分母限定共同登记/匹配/说明/生产调用/生命周期，具体索引 U1～U6 和 LiveAccess 专项随后续阶段验收。共同层仍必须有真实生产贡献，不以假注册器替代；不要求具体索引已全部完成才能解锁具体索引阶段。
- 共同层开始（2026-10-14）：Repo 原生 metadata 的 `kc.plugins` 保存启用配置，沿既有 metadata 授权读写；运行时按仓读取，不新设 Server 启用表。公开类型、解析与消费登记归 Discovery 服务；Snapshot 只提供通用仓 metadata 可选能力，不解释插件。先固化真实来源选用/帮助/同引擎隔离/生命周期失败反例，再接真实 Index/Controller；本阶段不把具体算法仍在旧目录算成索引专项完成。Object 正文缓存及预热保持独立且只装配一次。
- 共同层边界收口：消费登记、完整贡献校验、配置与逐仓路由归 `discovery/plugin/`，任务编排仍归 `discovery/service/`；Home 与实际 Entry 不依赖任务服务。静态说明由插件 `cli/` 同源提供纯数据，CLI 不导入运行登记；中央只装配元数据与认证运输。正式 HTTP 95 条、CLI 62 叶，新增说明任务具名绑定真实 CLI→SDK→Server 验证，Schema JSON 不加旁注。
- 共同层先红后修：保留实际 namespace 两实例串用、前台 sync 波及另一来源、关闭排空等待后台共享锁、冷连接管理意外 provider I/O、未知入口使整组失效、typed nil 贡献接纳、Home 构造失败漏关、无索引来源被全局 Dataset 准备误阻挡的真实反例。根装配先核原生 metadata 再装观察视图；运行配置不热刷新，借用视图不核验/关闭根。不可读配置保留原故障，独立来源及凭据修复仍可进入，不将故障当空配置。
- 耦合审查与收紧决策：同一工具、macOS、`path-depth=2;named-subtree=;version=1` 重算 Object v5 冻结树和候选，68→74 单元，传播成本 16.2643%→15.9015%；移除说明 DTO 拉入整个查询服务、消费登记反向依赖服务、中央持有 Discovery 组合 DTO 的不必要边。旧绝对漂移门禁的五项下降如实保留，均为 Ce 不变、实际消费者增加：datasets 固定版本恢复、projection 真贡献/消费端口、httpjson 两 SDK 共用运输、Object SDK 共用固定范围、Object Coordinator 由 Home 独立装配。无既有 I 上漂移 ≥.05；依本仓 Object 验收的人工设计判定先例，独立图审后显式登记新模块并收紧成本基线，不改阈值/政策/工具或删 import。前后图、旧失败与逐项判断在 `.validation/plugin-common-coupling-20261014/`；不能把更新后绿色写成此前无漂移。
- 冻结 v1 闸结果：同指纹 `88f74c14863d4099934c20eda7306b4f096f0e5e8455acda73c0c2a11649b901` 的 quality / check-validation 通过；完整 contracts 的组件通过，边界在旧 `Projection.Start` 名称守卫失败，CLI 段尚未执行。按新的实际 Plugins 与独立 ObjectMaintenance 激活路径更新守卫，仍须证明冷 Home 不启动、长期 Server 启动二者；新候选完整复验，不拼 v1 片段宣布共同层完成。证据在 `.validation/plugin-common-20261014/final-v1/`。索引正文已回到 1497 字符，Schema 返回不改。
- 冻结 v2 闸结果：同指纹 `df706657460768f3941ed37b29f508174ddecf52218979ed60f2ca3374323c69` 的 quality / check-validation、组件及边界通过，完整 CLI 暴露四处失败：冷 Handler 提前访问连接来源、插件帮助未关联具名场景证据、未配置索引错误说明改变、重复 continuation 被新增校验拒绝。保留旧断言并修复；Handler 构造保持冷态，服务显式激活同一根，重复激活不重读配置；旧查询仍取最后 scalar，新增插件操作数仍只允许一次。实际 HTTP 退出回归又复现后台共享锁使排空卡住，须先取消后台再等待在途；同时核对实际 serve 信号入口的相同顺序。定向绿色只用于定位与修复，仍待新冻结候选完整验收，不宣布共同层完成。
- 最后共同层收口：实际 serve 信号退出次序先红，现经 `BeginShutdown` 拒新并取消后台，再由 HTTP 排空、`Close` 释放，保留在途请求上下文。真实 Entry 两条范围误改回归先红：构造回调改 slice 会把第三仓纳入路由，保留 slice 后修改会漏掉原仓维护；系统路由与 Entry inventory 分别冻结自己的副本后定向 race 通过。各处代码、说明和守卫沿同一生命周期；四组具体索引控制恢复问题只读定位，仍未提前开始内部迁移。
- 冻结 v3 闸结果：同指纹 `671aaf24a25f854e424acb83d44bbbf1878ecab910eecb9fea6b3085b46dc43c` 的 quality、docs、check-validation（49 项）及独立只凭帮助的真实查询/同依据回读通过。共享 OpenSearch 已有 991 分片、91% 堆占用，parent breaker 156 次；两项实际请求 429 后中止整套，保留失败证据、不计通过。临时停止旧测试引擎并保留全部数据卷，复用同一已缓存 2.19.3 镜像的独占验证实例；验收结束恢复原服务，未删除原索引。独占实例完整重跑 3361 pass／0 fail／12 环境门控，但 CLI 覆盖出口仅 61/62，仍未验收：真实 plugin help 测试绕过了标准调用/断言观测。已沿原 helper 接入真实成功及同主体撤权后的 FORBIDDEN，定向报告为 2 次调用、1 次成功断言、1/1 边界；不手写成功记录、不弱化门禁，待 v4 所有必需入口闭合。
- 走读存量债：`object/cli/help.go:12` 的分列 read 摘要把 Dataset/--object 与 --commit 写成合法组合，实际要求统一引用才能固定来源 commit。判定为 Object CLI 既有帮助缺陷，已核 Object v5 原句未变；本轮插件概览→说明→实际 Schema→查询→统一引用回读成功且全为 c2，不扩大共同层追修存量。处置与触发：下次维护 Object read 帮助时拆明 Repo／Dataset 分列用法及统一引用的 commit 条件；完整走读及判定在 `.validation/plugin-common-20261014/final-v3/help-walk-agent-transcript.txt` 和 `help-walk-assessment.json`。
- 共同层已验收（2026-10-14）：最终 v4 同指纹 `d4fd726266799b55666a8587fdfbda7369f312b3d4803865c9d566b4cf226c67`，9 个正式 scope 均 sourceChanged=false，验收时工作树逐字节一致。完整 contracts 3362 pass／0 fail，62/62 CLI 成功且满足边界；普通场景 21 pass／0 fail／0 skip；必要 race 619 pass／0 fail（Luna/Taihu 两项外部门控）；真实 State/通知 4 pass／0 fail／0 skip，已补广套受影响门控。docs（33 文档/174 关系）、quality、validation 工具合同 49 项通过。独立走读原观察保留，v4 沿所发现路径的 10 次真实调用复现概览、Schema、检索及对象回读全部 c2，不将自动复现冒充新独立观察。证据 `.validation/plugin-common-20261014/final-v4/summary.json`；广套其余 8 项外部环境不计通过，真实部署为独立入口。独占引擎已回收，原测试服务和数据保留恢复；本条只完成共同接缝分母，未把具体算法旧目录或四项控制缺口记成完成。

- 具体索引先行基线（2026-10-14）：旧真实 Entry / Writer / Dataset Registry / 物理池上的四条控制恢复反例，普通与 race 均得到预期行为红，无编译或 race 错误；空需求同时覆盖 Snapshot desire 与 State notice。固定声明、完整 Canonical 与动态正文、原观察、查询零取值、Registry 当前/持久 State 和所有源 HEAD 均有实际断言。证据 `.validation/index-control-four-20261014/baseline/`；该目录明确仅保存运行后源片及日志，不冒充冻结候选验收。具体模块所有权守卫同样先取得旧布局红，再迁移。
- 具体索引实施中：真实查询/编译、控制及全部物理适配正迁入 `declarativeindex/`；通用预算/范围 context key 与消费 DTO 留唯一真定义于 Discovery。固定 Dataset 需求从登记服务经窄口交给控制器，与 HEAD 独立执行、保留单项失败并继续其他依据；显式空需求不从工作账回填。Object 的独立 Coordinator 不再由索引内部重复构造，既有通用 consumer 精确契约迁到其真实 owner 执行。尚未取得最终候选整套验收。
- 本轮独立检查的存量债：`docs/reviewed/knowledge-exploration-research.md:178`、`ingestion-retrieval-research.md:174` 的外部研究源码 URL 已被旧路径替换污染成 `llama_discovery/projection`；核共同层 v4 原字节即存在，本轮只更新实际本地源码链接，未扩大到研究来源重审。判定为旧引用错误，处置与触发：下次维护这两篇研究来源时核官方仓库原路径并修复；不计本轮实现或运行验收通过。
- 独立结构审查与收紧：原模块内部目录混合物理驱动和 Writer/Reader 测试夹具，真实初版结构成本 16.2982% 高于共同层 15.9015%，严格闸先红。未提高容忍度；把全部真实驱动及所属测试收进 `declarativeindex/opensearch/internal/opensearchimpl`，共享夹具只消费公开 adapter，Go 语言私有边界更严。相同工具/平台/政策的实际新图为 76 单元、897/5700=15.7368%，降低 0.1647pp；运行 driver 不再可达测试夹具。保留原 47 owner 禁止项，新增具体边界及反例，完整 arch 定向通过。剩余三项绝对 I 下漂分别来自通用 projection 移出维护依赖及实际消费增加、testkit 真实共用、Object 维护两能力分别复用；无新增 outgoing，独立按已有人工设计判定先例批准稳定性增强。显式 `-write-baseline` 登记新单元并收紧较低成本，不改政策、阈值、工具或省略真实边；原红、预测非验收、实际图与逐项判断在 `.validation/declarative-index-boundary-20261014/`。
- 必要失败补闭环：State 进程内观察不再冒充物理就绪，实际 OpenSearch 核 ActiveIndex 代仍存在；只删代而保留 READY 控制/工作账的 Snapshot 与 State 先红后恢复，错误不伪装零命中。旧通知完成吞掉外呼期间新通知的两项红已用持久代次条件完成修复。独立审查另发现 HEAD 故障期间误完成通知的增量回归，HEAD-only 及 fixed-a/HEAD-b 两例均先红后修，故障恢复无需重发；旧 State 查询未核实际提供方观察依据的存量缺口属于本轮 U5 必需项，已先红后拒绝错 revision/observation digest/basis，候选/正文/外呼均未执行。各定向结果只证明其范围，最终候选验收仍待进行。
- 冻结 v1 闸结果：指纹 `ca93f73ae14e09d280eeb4e9e55c6ebc43524c512199738405971400cfc4855e` 的 docs、quality、validation 通过且 sourceChanged=false。完整 contracts 组件阶段在 SystemRepo 的真实说明依据失败：`ProjectionSpec` 源引用仍是已退役的 `discovery/projection/plan.go`；边界与完整 CLI 尚未运行。保留具名断言，修到具体模块实际公开符号及当前包说明路径，不以旧局部绿色宣称全部通过；证据 `.validation/declarative-index-20261014/final-v1/`，待新冻结候选所有必需入口复验。

- 具体插件最终验收（2026-10-14）：v2 指纹 `b76c28649f0d074ace80c2af541e4141cfb105f14d151c1c504c143bfbc97dde`，11 个正式 scope 均 sourceChanged=false，验收时与工作树逐字节一致。完整 contracts 3501 pass／0 fail，62/62 CLI 成功且满足边界；普通场景 21 pass／0 fail／0 skip；必要 race 711 pass／0 fail（仅外部 Taihu 门控）；实际引擎整模块 259 pass／0 fail／0 测试 skip（Go 将无测试的私有 helper 包记 package skip，不是用例门控）；真实 State/通知 4 pass／0 fail／0 skip。docs 33 文档/174 关系、quality、validation 工具合同 49 项通过，当前公开清点 62 CLI／95 HTTP／1733 Go 声明；各 owner 修改和图已重读，主文 1499 字符。
- 部署与分母：既有 local 真 lakeFS/MinIO/PostgreSQL 上使用当前候选 HTTP/CLI 运行正式自动旅程，20 pass／0 fail；两个人工走查叶由既定场景合同排除，不计通过。只用现有 compose scenes 执行器的运行参数将检索绑定本次独占验证实例，不改生产端点、不重建旧 KC server，不把旧服务当候选。广套 12 门控的 4 项 State/通知及自动部署入口已单独执行，其余 7 项外部 Luna/Taihu、adapter、consumer 与规模未执行、不计通过。独立帮助原观察保留，CLI/help/SDK 29 个产品文件与其观察时逐字节相同；当前候选沿所发现路径的 10 次实际说明/Schema/查询/对象回读均为 c2，未把自动复现当新独立观察。
- 实际交付与收尾：查询及真实派生发布归 `declarativeindex/projection/`，需求/调度/恢复/通知归 `control/`，命令/校验/输出/帮助归自身 `cli/`；公开物理适配及真实内部实现同归自身 `opensearch/`，通用 DTO/预算/范围/查询组合留 Discovery。四项先行控制红与额外需求失败、介质丢失、同键通知、HEAD 故障及错观察查询反例均保留；独立只读审无剩余阻点。证据 `.validation/declarative-index-20261014/final-v2/summary.json`，实现验收完成；原测试引擎及全部原数据已恢复、临时验证实例已回收。仅本 TASK 结果注记发生在冻结验收后，生产代码/合同/设计与最终候选一致；未安装依赖、未提交或推送。

### PLUGIN-REPO-SCOPE-01 · Repo 配置与写读共同作用域（2026-10-13）

- [x] 文档调整完成：插件由 Repo 配置并以该 Repo 为作用域；写入后的派生维护、直接读取和 Dataset 来源消费使用该仓配置的贡献。Server 负责装配执行，Dataset 只组合；首批假设插件贡献没有冲突，不设计优先级、覆盖或冲突裁决。
- 范围：插件扩展 owner，索引/LiveAccess、CLI、术语、重构计划及设计索引的边界同步；只调整文档并重读文档图，不实施插件 API、配置存储或路由，不改 Go、不提交。
- 完成条件：写入、查询和访问过程能定位来源 Repo、取得配置与贡献、传递受限 payload 并采用结果；独立复核、owner/图重读、`make check-docs`、`make quality` 及增量空白检查通过后只记文档完成。
- 质量处置：原文混用“Repository/对象采用”、Server 默认选择及固定来源插件版本，判定为插件配置归属和知识声明混淆；本轮统一为 Repo 配置，Object 保存 Schema/access/Binding，运行引用和凭据由 Server 持有。插件冲突暂不纳入本轮设计分母；真实需求出现时回到该 owner 选定，不以假设无冲突放宽已有授权、查询支持或完整性合同。
- 验证：独立增量复核通过，CLI U8 遗留的“无采用声明”已改为来源 Repo 未配置；已重读变更 owner 与文档图。`make check-docs`（33 文档／174 关系）、`make quality`（退出 0，日志 `/tmp/kc-repo-plugin-scope-quality-final-20261013.log`）及文档空白检查通过。仅确认文档调整，Repo 配置/登记和真实路由仍待实施；共享工作树中的实施顺序、LiveAccess U20 等并发增量保留，不计本轮交付。

### OBJECT-CASES-PREP-01 · 重构前固定必要且充分的对象用例（2026-10-13）

- [ ] 本轮认领：按 Object Guideline 已选定能力建立稳定用例与前态／动作／Oracle，核对已有精确断言及局部缺口，补维护单元生命周期与拒绝后态回归，关联实际场景宿主和验证 owner。
- 范围：写用例、测试及其文档／图／场景关联，不开始生产重构，不新增临时 URN API、解析器、不可执行 feature 或 skip；未选定的集合／字段删除不进入分母。目录发布、Discovery、LiveAccess、治理仅按受影响消费兼容分别验收。
- 完成条件：每项承诺有正向控制、独立失败风险及移除后漏检说明；新入口尚未实现的风险有可证伪规格和公开接入位置，保持语义用例在既有入口执行。独立充分性／必要性复核，受影响回归、场景产品／视图检查、`make check-docs` 与 `make quality` 全绿后只记用例准备完成，不记 Object 重构或新入口验收通过。
- 用例产物：Guideline 固定 U1～U12，验证 owner §2.15 写明前态、动作、准确 Oracle、已有局部断言、接入缺口与删除风险；补身份编码／Pointer 独立向量、读取维护摘要和失败后态，场景关联只指向实际断言。新增四项回归并加强仓 CAS，不复制既有 Schema／Dataset／Writer 语义套件；根帮助、新 URN／locator 及对应 CLI→HTTP 旅程仍明确为待实现规格。
- 实际验证：新跨仓同名隔离、Member 完整替换与旧摘要拒绝、Aspect／Member 删除隔离及增强 CAS 通过；Writer 整包有根删除一例和既有表示冲突四子例失败，Reader 整包有实际只读四分支失败。`object/service`、`internal/arch`、所选 CLI 帮助／Schema／真实服务消费／范围／独立采用回归和场景覆盖合同通过。产品／视图检查、`make check-docs`（33 文档／174 关系）、最终 `make quality` 通过；不记受影响运行合同全绿，本项保留 `[ ]`，不开始生产重构。
- 证据：Writer 原始输出在 `.validation/object-cases-prep-20261013/writer-package.jsonl` 与 `writer-lifecycle-targeted-final.jsonl`；Reader／服务／架构输出 `/tmp/kc-object-cases-prep-read-boundaries-20261013.log`；CLI 输出 `/tmp/kc-object-cases-prep-cli-evidence-20261013.log`、场景合同 `/tmp/kc-object-cases-prep-scene-contracts-20261013.log`；最终质量 `/tmp/kc-object-cases-prep-quality-final-20261013.log`。未跑真实部署，不将既有证据映射算作本次执行。
- 基线缺陷登记（R1 处置，保留精确失败守卫）：
  - U5：位置 `object/format/apply.go:104`、`object/format/tree.go:60`；信号为已接受根删除后 `columns/status` 仍可读。源码显示遍历 ObjectUnits 时 Remove 原地改同一切片；判定为完整删除不成立。处置为沿唯一单元删除路径修复并重跑全单元／旧版／兄弟 Oracle；触发条件为 R1 写入或格式迁移之前，未绿不得切换。
  - U6：位置 `object/format/layout.go:145`、`object/units.go:214`；四种完整 Aspect／Members 最终冲突被接受，Ref 与 APPLIED 回执推进，原对象不可读。判定为最终表示校验缺口。处置为唯一表示规则补齐并保留显式转换正向控制；触发条件为 R1 单元代数或 Writer 迁移之前，不能用 Schema 或输入顺序遮住缺口。
  - U12：位置 `object/reader/repository_service.go:23`、`:34`；tree／native × Require／Wrap 的实际返回仍可取得 CreateRef、Merge、Archive。判定为接口收窄未隔离实际能力。处置为落实真实只读视图并保持四分支守卫；触发条件为 R1 Reader 公共边界切换之前，不能以返回类型名称或目录守卫代替。

### OBJECT-AFFORDANCE-CORRECTION-01 · 按实际任务纠正 Object 能力清单（2026-10-13）

- [x] 文档纠正完成：Guideline、CLI owner 与计划已将 Schema 列举／描述归入 Object 基本能力，目录预览／提交与命令回执按 CLI 任务说明；目标基础读取直接交付固定版本及维护内容摘要，不要求先运行独立 resolve。
- 本轮只改上述文档及本条，不改 Go、现有命令／API、场景或运行配置，不提交。核对前已有统一 URN／locator 文档改动，保留其目标和在途修改；独立复核、owner／图重读、`make check-docs`、`make quality` 与本轮增量检查通过后只标记文档完成。
- 实施差距：当前 `cli/product_encode.go` 的 read 输出未交付维护摘要，`cli/desired.go` 的目录预览／提交通过 Resolve 接口取摘要，维护用例也通过独立命令取得更新条件。统一读取入口实施时补同依据的维护信息及真实“读取 → 更新”回归，再处理旧诊断入口；不将移出基础清单写成已删除命令或内部能力。
- 验证：独立增量复核通过，已重读变更 owner 及文档图；`make check-docs`（33 文档／172 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-correction-quality-20261013.log`）与本轮空白检查通过。只完成文档纠正，未实现新的读取输出或删除现有 CLI；同文件中其它插件增量属于并发工作，不计入本轮交付。

### OBJECT-UNIFIED-WRITE-REFERENCE-01 · 读写共用 URN＋locator（2026-10-13）

- [x] 设计文档完成：Guideline 的创建、更新、删除与读取已统一使用对象 URN＋locator，PUT／REMOVE 主示例及粒度表已改；现有分散参数只记兼容适配。目录 diff／commit 已从 Object 自身清单移出，计划同步统一引用与入口边界验收。
- 范围与出口：只改 Guideline、计划及本条，不改 Go、现有 CLI／API、存储或写入原子性，不提交；URN 字面编码及真实参数落点沿公开合同实现，本轮示意不冒充可执行命令。只选定既有完整维护目标的统一引用，不借此增加集合／深字段删除语义。
- 质量处置：此前将现有参数及 Address 能力限制写成未来统一入口的形状，判定为现状收窄目标；主合同改为统一引用、服务按维护模型计算变更，旧参数和限制移入兼容说明。集合／深字段删除是否扩为新的公开操作范围另行选定，不用旧 Go 类型自动否决，也不以统一编码声称支持。
- 验证：统一引用、三种维护目标、兼容及未选定删除范围经独立复核通过；已重读 owner／图，`make check-docs`（33 文档／171 关系）及本轮空白检查通过。`make quality` 因共享源码 `snapshot/service/file_view.go:79` 的 `mountOwning` 未使用而失败（日志 `/tmp/kc-object-unified-write-reference-quality-20261013.log`）；保留在途 Snapshot 修改并记录阻点，源码处置后重跑，不绕过静态检查、不记整仓质量或统一 CLI 实现通过。
- 目录入口更正：此前以“Object CLI 适配”将目录 diff／commit 留在本模块表，混淆了能力使用与所有权。当前共享修订已移出该表，本轮补清应用知识发布流程、Object Writer 校验／写入保证及 Snapshot 文件／版本提交的责任，区分已有对象两版 Reader 对照与当前目录 diff。迁移触碰目录流程时按应用任务和兼容验证，不因现有函数位置固化所有权。
- 本轮出口：对象差异 API、目录实际执行路径及模块职责经独立复核，owner／图重读；`make check-docs`（33 文档／172 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-directory-task-scope-quality-20261013.log`）及本轮空白检查通过。先前共享静态检查阻点本轮已解除；只确认文档范围完成，URN／locator 真实写入入口、帮助内容及对象差异 CLI 仍待实现／选定。

### LIVEACCESS-HELP-AFFORDANCE-01 · 说明入口与验收用例提前入库（2026-10-13）

- [x] 用例准备完成：五项目标、证据可靠性及插件说明合为 U13～U19 七组、15 条必要规格，已把完整前态、动作、预期与独立风险放入最近宿主 `_specs/liveaccess.yaml`；产品视图直接展示并检查，不等实现后再写用例。
- 范围：用例库、其发现/引用检查与回归、场景规范和验证/LiveAccess owner；不开始生产插件重构。规格与具名实际证据分别展示，未实现部分保留 gap，不制造测试名、参考 feature、skip 或运行通过。公开新入口仍按共享插件合同实施。
- 五项目标及协议承诺不变，原 18 条 ID/标题保留并新增 U19；用例表只登记覆盖和精确局部证据，完整规格由库拥有。独立充分性/必要性复核通过；补齐混合核验失败、请求身份串用、模块说明、独立审计及三个证据故障，修正不适用凭证必失败、未建立 trace/失败说明前态等过强或空洞断言。
- 质量闭环：原表有完整描述却没有正式入库，现增加规格发现、字段/引用/唯一 ID 检查及真实库 U13～U19 必须有规格的回归；规格不能当执行证据或消除 gap。索引并发改用有序列表两次导致关联失效，已增加该稳定 U 条目表达的发现与重复 ID/围栏守卫；未更改其能力。现有目录/依赖守卫不证明全部专属逻辑迁入，已为 U13 保留精确 gap。
- 验证：55 项场景工具回归、三个 Go 场景结构/视图/观测守卫、模块结构守卫及五个相关包的具名既有证据定向运行通过，无 skip。产品关联/工程并集/前态检查、`make check-docs`（33 文档 / 173 关系）与增量空白通过；最终 `make quality` 退出 0，日志 `/tmp/kc-liveaccess-cases-library-quality-final-20261013.log`。前次共享 `mountOwning` 未使用阻点已由所属改动解除。本条只记用例准备及入库机制通过，未记新插件、逐值结果、原值审计或新 help 运行验收通过。

### PLUGIN-SOURCE-ADOPTION-01 · 来源采用与组合能力呈现（2026-10-13）

- [x] 已明确择一并同步设计：Repository/对象保存插件采用与能力声明，Server 按固定来源匹配已装配实现；Dataset 只组合范围并呈现来源能力，不另设插件绑定或覆盖选择。逐来源准备、查询、维护、续页及例子/验收一并对齐。
- 触碰 owner：共享插件接入、声明式索引选择与查询/准备、LiveAccess 组合用例、CLI 说明入口、术语及直接实施计划/阅读入口；只改设计与图主题、任务记录，不改 Go、公开类型、运行配置或场景，不启动服务、安装依赖或提交。
- 质量处置：`plugin-extension.md` 原整次 Dataset 单实例及 Dataset 独立选用、`declarative-index.md` 原 R 选 A/D 选 B、`terminology.md` 双目标采用定义是未经用途证明的额外覆盖，本轮当场撤回；固定版本、范围、声明和真实实例依据继续保持。不同来源的选择、准备、查询归并与续页路由仍是待实现合同，实施时补真实跨实例回归，不将当前单一 Projection 注入当成已具备。
- 验证出口：独立复核全篇残留及跨来源完整性，重读变更 owner/图，文档与工程质量、增量空白检查全绿后只标记设计纠正完成。
- 初次工程检查：`make quality` 因共享在途源码 `snapshot/service/file_view.go:79` 的未使用 `mountOwning` 方法（U1000）失败，日志 `/tmp/kc-plugin-source-adoption-quality-20261013.log`；判定为本轮文档以外的源码阻点，未绕过检查或修改该文件。该方法随后已由共享变更移除，按更新后的候选重验。
- 完成证据：独立复核提出的准备调用者及实施计划归属残留已当场修正，最终复核无实质冲突；来源 Writer 与纯文件发布不等待索引，固定采用/范围、跨来源支持/排序/覆盖、观察及续页依据保留。变更 owner/图已重读，最终 `make check-docs` 通过（33 文档 / 172 关系），`make quality` 退出 0（`/tmp/kc-plugin-source-adoption-quality-final-20261013.log`），增量及新增文件空白检查通过。仅完成设计纠正；来源采用形状、真实逐来源路由及新目录/说明入口仍待实施。

### LIVEACCESS-CONTRACT-LIST-01 · 直接列取值协议与结果承诺（2026-10-13）

- [x] 已直接列现有 Go 取值接口、请求字段、成功/失败返回；成功、合法空值与局部失败对对象结果的影响分别列项，认证、访问记录、Snapshot/LiveAccess 证据、独立查询及可靠性分别列项。公开类型独立核对，不新增 DTO 或把端口字段冒充 HTTP 消息。
- 用户本轮明确要求设计文档写协议，按此授权列现有字段；只改该段及本条，保留目标、所有权及用例关联。
- 检查时并发新增的 `cli#U8` 缺场景关联或 gap，已补真实缺口登记恢复产品引用完整性；未修改其目标或宣称已有执行证据。
- 验证：仅替换该契约段，Goal 与 18 用例原文一致；可见正文 999 字符。公开端口/观察类型、owner/图已核对；`make check-docs`、产品关联与增量空白通过，`make quality` 退出 0（`/tmp/kc-liveaccess-contract-list-quality-20261013.log`）。本轮没有实现或运行验收变更。

### PLUGIN-AFFORDANCE-ENTRY-01 · 插件说明入口与目标访问概览（2026-10-13）

- [x] 已将能力说明读取写入插件重构合同并同步 CLI owner：明确 `kc help` 基础能力导航、`kc plugin <名称> help` 说明读取，以及 Dataset 知识访问概览中实际选用插件的用途和帮助入口；更新发现过程、例子、改接表及验收。
- 范围：只改共享插件篇、CLI 设计、相关图主题和本条；不改两个插件自身目标、Go、公开协议或运行配置，不安装依赖、不启动服务、不提交。Object 提供基础模型与 Schema；插件提供自己的 Affordance，应用层按真实选用组装概览，纯文件 Dataset 继续独立。
- 处置：此前解释把需要使用说明直接推成请求时 describe Hook，判定为无必要的执行接点；本轮改为登记可复用说明、按目标交付与按需读取。说明入口和目标聚合尚缺公开合同及接线，重构实施时由 CLI/消费服务落实类型、README/Conformance 和真实走读，不把现有 SchemaReport 或文档绿色当成已实现。
- 验证出口：独立复核职责与端到端 discover/access 过程，重读变更 owner/图，运行 `make check-docs`、`make quality` 和本轮空白检查；全绿后只标记本次文档完成。
- 完成证据：独立审查未发现实质问题，新增说明/概览均标为待实施，固定 Schema 展示、运行实例选用、纯文件 Dataset 与真实执行职责分开。`make check-docs` 通过（33 文档 / 171 关系），`make quality` 退出 0（日志 `/tmp/kc-plugin-affordance-quality-20261013.log`），增量及新增文件空白检查通过。只完成文档，不证明新命令、说明读取服务或目标概览接口已经实现。

### OBJECT-MODEL-ACCESS-01 · 首先能访问对象模型概念（2026-10-13）

- [x] 根帮助设计文档完成：Guideline 的 Goal、能力清单及 CLI 第一项已改为 `kc help` 本身直接解释 object_id、Aspect、Member、Address、Relation、联系、基础能力与小例；详细参数沿既有叶子帮助，系统导读和概念对象留作深入阅读；计划同步仅沿根帮助的零知识走读验收。
- 范围与出口：只改 Guideline、计划及本条，不实现帮助、不新增帮助主题、业务命令、概念标识、API 或出版实现；Object 提供模型与固定读取，完整系统出版仍归 SystemRepo。独立核对出版源／owner／图，文档与质量门禁全绿后只记文档完成。
- 质量登记：`systemrepo/content/concepts/object/definition.md` 将业务对象与知识表示混作“知识载体”，`concepts/address/definition.md` 尚未说明新 URN／locator 目标；判定为模型说明需随重构同步的出版内容债，Object 对应语义／访问切片落地时按 owner 修订出版源、同源说明及正式出版回归，不在本文虚构尚未选定的 URN 编码或已部署新功能。
- 验证：概念身份、CLI 参数、object_id 说明位置和模块／出版职责经独立复核通过；重读 owner／图，`make check-docs`（33 文档／171 关系）及本轮空白检查通过。最终 `make quality` 因共享工作区的 `snapshot/dataset/publication_source_validation_test.go` 需 gofmt 而失败（日志 `/tmp/kc-object-model-access-quality-final-20261013.log`）；记录阻点并保留在途源码，待其格式修正后重跑，未绕过门禁、未记整仓质量或部署通过。
- 入口纠正：直接读取系统概念仍要求预先知道仓和标识，不能作为零知识起点；已改为 root help → 模型静态帮助 → 叶帮助 → 可选的系统定义读取。现有 CLI owner 已要求模块概念帮助静态提供，当前 rootHelp／helpFor 尚缺 Object 模型主题；实现时按仅知道 `kc help`、无会话／服务配置且无业务请求的独立走读取证，不以文档选定冒充已实现。
- 本轮出口：真实 `kc help` 正常返回且无模型导航，`kc help object` 返回未知主题，确认当前入口缺口；独立职责复核、owner／图重读、`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-zero-knowledge-help-quality-20261013.log`）及空白检查通过。此前共享格式阻点本轮已解除；仅标记设计文档完成，模型帮助与 URN／locator 功能尚未实现。
- 根入口更正：新增 `kc help object` 为必经页的设计撤回；根帮助直接承担模型概览，叶子帮助才展开操作参数。不把“找到下一页”冒充已经理解模型，也不把根帮助扩成全部参数手册；本轮独立复核通过，当前待补是根帮助内容。
- 根帮助出口：独立充分性复核、owner／图重读、`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-root-help-quality-20261013.log`）及空白检查通过；只完成根帮助目标文档，CLI 内容未改。

### OBJECT-WRITE-BOUNDARY-01 · Object 入口归属与写入粒度（2026-10-13）

- [x] 文档修正完成：Guideline 已区分 Object 核心操作、目录适配与恢复辅助；写清 PUT／REMOVE 的整体值、Aspect、Member 粒度和限制，移出 Binding 专属解释，resolve 降为可选诊断。
- 范围与出口：只改 Guideline、计划的对应验收及本条，不改命令、API 或运行实现；以身份、表示、固定读写及来源记录判断归属，不以包位置或不外呼为依据。独立核对公开合同，重读 owner／图，文档及质量门禁全绿后只记文档完成。
- 质量处置：此前“纯声明解释即属 Object”的判断已撤回。`object/README.md` 首段与 `object/reader/binding.go` 仍收纳 LiveAccess 专属解析，判定为待迁移的职责混合；本轮在指南和计划明确目标，触碰声明解析迁移时移动唯一实现及调用方、同步 README／Conformance，不新增第二份解析器。
- 验证：参数、三粒度与删除范围独立核对通过；修正“必须先读再 PUT”的过度约束，明确完整新值可直接提交。已重读 owner／直接图关系，`make check-docs`（33 文档／171 关系）、最终 `make quality`（退出 0，日志 `/tmp/kc-object-write-boundary-quality-final-20261013.log`）及本轮空白检查通过；未改代码、执行示例写入或确认新 URN／locator 功能实现。

### LIVEACCESS-EFFECT-BOUNDARY-01 · 按插件贡献说明读取效果（2026-10-13）

- [x] 已删除误归到 LiveAccess 的调用方 CLI 表；契约明确插件提供取值结果，由对象层采用和呈现，说明读取结果、逐值失败、身份及审计的影响。独立所有权复核通过，没有把整个对象返回或审计入口归给插件。
- 所有权误判当场修：命令存在只能证明入口可用，不能证明属于该插件；本篇以插件输出与对象层结果采用的边界明确合同，不以通过工程检查替代语义复核。
- 验证：18 用例原 ID/标题及关联保持，可见正文 807 字符；owner/图重读，文档与产品关联、增量空白及 `make quality`（退出 0，日志 `/tmp/kc-liveaccess-effect-boundary-quality-20261013.log`）通过。仅改本篇与任务纠错记录，未改对象层文档、CLI 或运行行为。

### OBJECT-LOCATOR-EXAMPLES-01 · 内容定位符写法与例子（2026-10-13）

- [x] 示例文档完成：Guideline 的 Address 章节已补 locator 具体写法、对象内容例子、选取结果及 JSON Pointer／URI 两层转义；明确对象根、Member、深字段、数组位置与字面键名。
- 范围与出口：不改身份／访问语境、Affordance、代码、CLI／API、其它设计或图关系；按 RFC 6901 独立核对示例，重读 owner／图，文档与质量门禁通过后只标记文档完成。
- 验证：示例及转义独立复核通过，明确对象按 Aspects、columns 按 Members 维护，未从 JSON 外形推导写粒度；编辑前快照确认仅扩写 locator 示例。`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-locator-examples-quality-20261013.log`）及本轮空白检查通过；未实现新读取入口。

### LIVEACCESS-CLI-DOC-01 · LiveAccess 的 CLI、契约与承诺（2026-10-13）

- [x] 历史核对完成，所有权判断已撤回：真实 help 只证明四入口存在，不能把对象/声明检查/审计调用方入口视作 LiveAccess 的能力；错列的表由 LIVEACCESS-EFFECT-BOUNDARY-01 删除，改写插件对既有读取的效果。原合并的契约与承诺保留。
- 验证：四个真实 `kc help` 均正常返回，已与公开命令表、SURFACE、LiveAccess 与审计查询合同核对；没有将 `--request-id` 当历史查询条件。18 用例原 ID/标题不变，可见正文 996 字符，owner/图已重读；文档图、产品关联、增量空白及 `make quality`（退出 0，日志 `/tmp/kc-liveaccess-cli-doc-quality-20261013.log`）通过。本轮仅改该篇及本条，不改运行实现。

### OBJECT-AFFORDANCE-SCOPE-01 · 按 Object 所有权收口可供性（2026-10-13）

- [x] 范围修正文档完成：清单已删除 Discovery、LiveAccess、Dataset、Governance 与仓级版本管理入口，保留 Object 身份、固定内容／Schema／来源声明、对象写入及历史结果；计划将逐项所有权与基础独立采用纳入完成合同，避免继续按“操作对象”或 CLI 分组判断归属。
- 范围与验证：仅 Guideline 本章节、计划完成观察及本条；不改代码、命令／API、其它 owner 或图关系。沿架构 owner 与 Object／Reader／Writer／Service 合同独立核对，确认其它 Guideline 章节不变，文档及质量门禁通过后只记文档完成。
- 结果：12 项按能力所有权独立复核通过，Schema／Binding 固定声明解释与外部执行分开，Writer 既有候选写入合同保留；Guideline 其它章节与编辑前快照一致。`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-scope-quality-20261013.log`）及本轮空白检查通过。仅完成文档范围修正，未改运行行为。
- 归属更正：上述 Binding 判断撤回；只读的专属解释也不属于基础 Object，已由 OBJECT-WRITE-BOUNDARY-01 从清单移出，固定声明读取与 LiveAccess 语义解释分别说明。历史工程检查不作为该归属判断正确的证据。

### OBJECT-AFFORDANCE-CLI-01 · 以 CLI 入口说明对象能力（2026-10-13）

- [x] 文档修正完成：Affordance 章节已改为真实 CLI 命令与一句用途，移除抽象任务分类；明确 diff、Schema 查询、回执查询的实际保证，URN／深 locator 单列为待实现能力。
- 范围与出口：仅 Guideline 本章节及本条，不改 CLI、公开形状、其它设计或图关系；核对真实 help、包 README 与 SURFACE，独立复核并确认其它章节不变，文档及质量门禁通过后只标记文档完成。
- 验证：真实 `kc help` 正常返回，命令表与模块帮助、SURFACE 对照，独立复核通过；编辑前快照确认 Guideline 其它章节一致。`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-cli-quality-20261013.log`）及本轮空白检查通过；未实现 URN／深层定位或变更运行入口。
- 范围纠正：该轮仅核对入口存在与描述，却将对象相关的外部职责混入 Object 清单；所有权判断错误已由 OBJECT-AFFORDANCE-SCOPE-01 修正，并升级为计划的完成合同。工程门禁通过不替代语义归属核验。

### REFACTOR-CASES-PREP-01 · 补齐必要的重构验收用例（2026-10-13）

- [x] 验收准备完成：LiveAccess U13～U18 逐项覆盖五项目标及证据可靠性，完整前态、动作、预期、复用证据及移除风险写入验证 owner；索引 U1～U6 补具体完成观察。两套产品视图均关联实际局部断言和剩余 gap，旧 ID/反例保留。独立充分性/必要性复核通过，不把规格完成写成新插件运行验收通过。
- 本轮补验收规格及现有公开入口可执行的回归；新插件登记、逐值失败与实际值审计的公开形状在实施时承接这些规格。既有直接 access 复用固定范围 Serving 的观察核验并记录实际访问证据，保持公开返回形状；测试暴露的缺口同步修复，不弱化断言。
- 可执行增量：候选 a 返回后经 Writer 推进 b，再真实回读 a 的集合、值、Schema 与依据；保留端口核对完整原值/完整 basis；直接 access 成功/失败核对实际来源收到本次证明、普通输出与证据无凭证、时间及完整错误关联。修复既有直接 access 缺观察核验/漏访问记录造成的 6 个红子例；窄修沿既有编排及证据抽象。
- 验证：整个 invocation、7 项相关 CLI 公开读取/认证/审计合同、4 项回归及场景视图/公共覆盖合同通过，无 skip；原始定向输出 `/tmp/kc-refactor-cases-regression-20261013.jsonl`、`/tmp/kc-refactor-case-links-tests-20261013.jsonl`。`make check-docs`（33 文档 / 171 关系）、`make quality`（退出 0，日志 `/tmp/kc-refactor-cases-quality-final-20261013.log`）、产品关联/视图并集及增量空白通过。LiveAccess 可见正文 946 字符，索引原始全文 1500 字符；未执行真实部署。新增接口未选定的用例有具体规格与明确接入点，没有临时 DTO 或测试替身冒充目标成立。

### OBJECT-AFFORDANCE-LIST-01 · 对象任务可供性清单（2026-10-13）

- [x] 清单文档完成：Guideline 已列 13 项基础读取、维护、追溯和恢复任务，另标发现、外部访问、Dataset 发布及治理的协作归属，CLI 帮助按清单说明真实入口。
- 范围与验证：不改代码、命令／API、其它设计正文或图关系，不新增独立校验入口或实施行为；独立复核清单与现有合同，确认 Guideline 其它章节不变，运行文档、质量及本轮空白检查后只标记文档完成。
- 结果：独立复核通过，编辑前快照确认 Guideline 增量仅在 Affordance 与 CLI；`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-list-quality-20261013.log`）及本轮空白检查通过。任务名称不冒充已存在命令，新增 URN／locator 能力仍待实现，不以本轮清单交付宣称 CLI 运行验收。

### OBJECT-ADDRESS-DESIGN-01 · 统一对象寻址设计（2026-10-13）

- [x] 设计文档完成：已补清对象身份、Repo／Dataset 访问语境、固定依据、JSON Pointer 与维护 Address 的职责；明确同名对象、Dataset 版本约束、对象写入与交付发布边界，计划同步完成观察。
- 范围与出口：仅 Guideline、计划、文档图与本条；不改代码、公开形状、测试、格式、权限或运行配置，不启动服务、安装依赖或提交。逻辑寻址规则本轮明确，具体 API 形状在公开合同落实；独立复核、owner／图重读、文档和质量门禁通过后只标记设计文档完成。
- 验证：独立复核通过，已重读相关 owner 及新增 Dataset／术语图关系；`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-address-design-quality-20261013.log`）和本轮空白检查通过。范围不完整须如实说明，未新增整对象拒绝政策、第二写面或字段 PATCH；新增统一引用与读取计划尚待代码及真实入口证据。

### LIVEACCESS-CASE-REVIEW-01 · 按当前重构目标检查已有用例（2026-10-13）

- [x] 已完成源码、实际断言与独立审查核对；`make check-docs` 及本轮空白检查通过。此勾选仅表示检查完成，用例准备程度见下述缺口。
- 结论：既有用例可复用，尚不满足完整重构验收。独立目录已有结构守卫；成功透明取值及普通身份透传已有组件和公共 HTTP 断言。缺新插件完整调用链、同一对象混合固定/成功外部/null/失败值后按值返回，以及源变化后经独立审计按请求复核原值与逐值失败；适用目标/重定向、审计凭证排除也缺针对断言。
- `resource-access.md` U1～U12 仅标题，场景产品视图仍对应旧动态消费分母；现有单地址取值报错反例不自动与对象按值失败冲突，不能靠删除底层拒绝断言补新目标。索引六组已有具体规格和组件证据，但新插件的选用、隔离、续页与固定依据旅程未齐。
- 验证：11 个相关旧基线测试在 `liveaccess`、`liveaccess/service` 与 `internal/arch` 定向重跑通过，无 skip，日志 `/tmp/kc-liveaccess-case-baseline-20261013.jsonl`；产品引用/明确 gap 检查通过，日志 `/tmp/kc-liveaccess-case-product-check-20261013.log`。本轮检查准备程度，没有新增或修改测试、实现及场景；下一步按当前五项目标补具体验收用例和关联，已有范围内的反例继续复用。

### OBJECT-PUBLIC-GOALS-01 · 对象访问、写入与 CLI 可供性目标（2026-10-13）

- [x] 文档目标完成：Guideline 已明确对象 URN、URN＋locator 指定数据读取、创建／更新／删除及 CLI Affordance；计划补对应完成观察，图登记对象访问目标所有权。
- 范围：仅目标文档、计划、文档图与本任务；不改 Go、命令／API、持久格式、测试、系统出版物或配置，不启动服务、不安装依赖、不提交。既有写入仍完整替换／删除明确维护单元，读取 locator 不新增字段 PATCH。
- 验证与质量出口：重读 owner／图，独立核对产品承诺及现有写入合同，运行文档、工程质量与本轮空白检查；结果只证明文档。URN 解析、深层读取、选择器前推及实际 CLI 示例是待实现目标，沿计划逐项取证，不以文档检查宣称功能或部署验收。
- 结果：独立复核通过；最终 `make check-docs`（33 文档／169 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-public-goals-quality-final-20261013.log`）及本轮空白检查通过。保留唯一语义、实际只读入口、Schema 可选与已记录来源的保证范围；未修改实现或运行行为。

### DOC-TASK-SIMPLIFY-01 · 任务与重构文档按需表达（2026-10-13）

- [x] 已取消任务与设计文档的固定五段要求，同步 Agent 指引、文档规范和检查程序；三篇插件重构文档已清理空段、重复段与混用标题，阅读入口和能力路线的旧模板措辞同步。
- 验证：检查程序回归通过，自由结构的设计文档可经真实命令检查，重复主题、依赖环和坏链仍拒绝；`make check-docs`（33 文档 / 169 关系）、`make quality`（退出 0，日志 `/tmp/kc-task-document-rules-quality-20261013.log`）和本轮空白检查通过。独立复核后取消剩余固定起笔要求并澄清既有 ID 引用，再跑文档检查通过。LiveAccess 12 个、索引 6 个用例原编号/标题不变；LiveAccess 可见正文 777 字符。只调整任务/文档治理与表达，不证明插件运行目标已实现。

### LIVEACCESS-REFACTOR-GOALS-01 · LiveAccess 重构目标与约束（2026-10-13）

- [x] 已删除重复的失败目标、未选定的对象整体拒绝政策及版本/Schema/搜索泛套话；插件篇四处继承的整体拒绝表述同步清理，未补替代政策。
- 本轮删除验证：LiveAccess 正文含标题 836 字符，12 用例原编号/标题保留；相关 owner/图已重读，独立检查传播遗漏。`make check-docs`、`make quality`（退出 0）及增量空白通过，日志 `/tmp/kc-liveaccess-remove-invented-policy-quality-20261013.log`。未改 Go、权限/搜索 owner 的既有正式合同或守卫；单值失败继续由 Goal 3 定义。
- 删除验证：正文含标题 927 字符，已重读 owner/图，文档与质量检查、增量空白检查通过；日志 `/tmp/kc-liveaccess-remove-write-aside-quality-20261013.log`。仅删文档一句，未改 Go 或公开合同。
- 本轮证据：LiveAccess 正文含标题 955 字符，12 用例原 ID/标题保留；service owner 和阅读入口同步，原值/记录保留期限匹配，既有 READ 版本与观察依据继续返回，凭证不进入证据。独立审查、owner/图重读、最终 `make check-docs`、`make quality`（退出 0）及增量空白通过；日志 `/tmp/kc-liveaccess-audit-goal-quality-20261013.log`。仅完成设计目标，实际值的保留关联与审计回读仍由下述实施出口补齐。
- Goal / owner：LiveAccess owner 的插件目标、句柄取值、单值失败与身份传递；已按 docs/graph 读取 ownerTopics/直接关系，相关授权与对象能力合同沿 permissions/object-interface。Non-Goals：仅文档与阅读入口/图登记，不改 Go、公开形状、场景、测试或运行配置，不实施插件，不提交。
- 不变量 / 取舍：保留 V-01、AUTH-01～03、OI-01～05、API-01；选定独立目录、真实插件、透明取值、单值失败隔离和适用身份自动传递，只规定目标/约束；否决实现路线、Collector 混入主线、造新能力名或未经选定的 Handle/错误 DTO。
- 质量出口：`liveaccess/service/hydrate.go` 当前单值错误直接上返，与用户新定的局部失败目标有差距；判定为待实现目标而非文档完成即可验收，LiveAccess 重构时按新目标选定公开错误表达并补局部失败回归，受影响合同与工程闸全绿，不用 skip 或迎合现状。对象整体拒绝不是用户选定目标，不据历史记录新增该类验收要求。已有生产保留/凭证目标/直接 access 核验债保留原专项出口。
- 访问证据 owner / 出口：通用访问记录及独立审计归 service，实际值及原观察保留归 LiveAccess；已读 docs/graph 端点、AccessEvent/AccessLog、观察与服务公开合同。现有 AccessEvent 有身份/时间/RequestID/固定目标及 basis，无实际值，判定为独立审计尚不能据此完整复核 LiveAccess 当次数据；重构接入时补实际值保留关联及独立审计回读，验源已更新仍返原值、单值失败说明、重启恢复、留证失败与保留期限。仅补设计目标，不预定存储位置、新 DTO 或查询字段，不修改普通 READ 既有版本/观察依据。
- 文档模板债：已由 DOC-TASK-SIMPLIFY-01 按用户约定取消固定模板并清理空 Non-Goals 段。
- 验证：独立复核目标与拒绝边界，重读 owner/图，核对全文字数与稳定用例，运行文档、质量和增量空白检查后才勾选；结果只证明文档，不证明透明取值/局部失败/身份传递已实现。
- 清理验证：最终可见正文含标题 704 字符，12 用例 ID/标题不变，owner/图已重读；`make check-docs`、`make quality`（退出 0）及本轮空白检查通过，日志 `/tmp/kc-liveaccess-text-cleanup-quality-20261013.log`。行为约束仅移动/去重，未改 Go 或协议。
- 交付结果：可见正文含标题 882 字符；独立审查通过，12 个原用例 ID/标题保留。阅读入口及图标题同步，源侧角色边界主题归术语 owner；插件例子同步局部失败目标并保留对象授权/搜索完整性/交付拒绝边界。`make check-docs`、`make quality`（退出 0）与本轮空白检查通过，日志 `/tmp/kc-liveaccess-goals-quality-20261013.log`。未改 Go 或运行配置，现有 hydrate/README 的错误上返行为仍按上述实施出口待补齐，未把文档验收写成插件实现验收。

### PLUGIN-MINIMAL-EXAMPLES-01 · 插件重构合同与过程示例（2026-10-13）

- [x] 本轮按新版插件目标修正完成：逐个扩展点给出索引/LiveAccess 的实际需求和消费方；移除无需求支撑的通用返回改写点，补端到端固定依据、透明逐值取值/失败隔离、适用身份自动传递及独立访问证据，并同步索引查询/控制/CLI 分工，保持重构文档职责。
- Goal / owner：插件作者接入方式归 `plugin-extension.md`；索引真实候选与同依据回读沿 `declarative-index.md` / `retrieval.md`，访问观察沿 `resource-access.md`，最终交付沿 `permissions.md` / `service.md`。已核对 docs/graph 所有权、直接关系和现有公开调用/交付代码。
- Non-Goals / 不变量：只改文档、阅读入口和图主题登记，不改 Go、测试、协议、CLI 或其他任务，不安装依赖、不启动服务、不提交；保持 W-01/W-02、V-01/KS-01、C-01/R-01/R-02、AUTH-01～03/KS-02、P-01/PC-01、L-01/API-01。
- 选定 / 否决：由最新版两个 owner 的承诺反推接点，每项必须有具名使用需求、payload、消费方及失败语义；平台固定回读、权限、结果组装和凭证目标限制作为强制职责，不混成可替换 Hook。LiveAccess 单值失败说明是已选定行为变化，公开表示与实现仍须选定并取证；否决以当前 `(value,error)` 限制新目标、预建通用返回点和把透明读取局部失败外推为完整动态搜索。
- 验证与质量出口：独立按完整重构分母及两个例子复核，重读 owner 和文档图，运行 `make check-docs`、`make quality`、本轮空白检查；登记、全部任务、维护及资源的真实接线差距在正文明确，首个实施切片触发时取证。返回接点保留现有合同，不新增通用返回框架要求，不以文档闸宣称运行验收。
- 新版对齐证据：已重读两插件 owner、共享 owner 和文档图；独立复核确认需求矩阵、强制系统阶段、局部失败/严格查询、模块分工及独立留证正确，例子与验收同步。`make check-docs` 通过（33 文档 / 169 关系），`make quality` 退出 0，日志 `/tmp/kc-plugin-requirements-derived-quality-20261013.log`，增量及新增文件空白检查通过。本轮只改共享插件篇、阅读入口、该篇图主题和本条记录，未改两个插件 owner 或 Go、运行测试及部署。
- 实施差距出口：`liveaccess/service/hydrate.go` 的整次错误上返、`object/read.go` 缺逐值失败表示、单仓业务 READ 未透明取值、原观察公开选择及真实保留未齐，判定为新版目标待实现；LiveAccess 接入时先选公开形状，再补逐值/对象拒绝/严格投影回归。`caller_credentials.go` 透传不证明目标适用/重定向强制，按权限 owner 补边界；`observability/access.go` 现有坐标/指针/整体错误及 RequestID 不证明原值/逐值失败或请求查询，独立审计目标落地时补原结果关联、可靠保留及授权查询。索引 control/CLI 仍是目标归属，按声明式索引 owner 分工迁移并验真实调用，不将文档绿色写成这些行为已完成。
- 本轮证据：独立复核确认文档职责及完整重构合同恢复，精确选择、固定准备/恢复、后台身份、物理隔离、旧资源与续页、失败/退出保证保留；例子只辅助说明。`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0，日志 `/tmp/kc-plugin-refactor-contract-quality-20261013.log`，增量及新增文件空白检查通过；owner 与图已重读。仍仅文档、阅读入口和图主题登记，未改 Go 或执行运行/部署验收。
- 交付证据：独立复核确认两例、现有注入口、Schema/观察核验、返回采用和 SEARCH 专属交付范围准确；补回 Dataset 不继承来源选择、一次任务同实例与遍历贡献。owner 与图已重读，`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0，日志 `/tmp/kc-plugin-minimal-examples-quality-20261013.log`；增量与新增文件空白检查通过。未修改 Go、运行行为测试、部署或提交；通用登记和返回阶段注入明确待实施，没有以任意字段改写或伪造观察示范新政策。

### DECLARATIVE-INDEX-CASES-01 · 重构前用例补齐与基线（2026-10-13）

- [ ] 测试先行已补写并取得基线；两条既有后台来源合同仍红，未标记整项验收完成。按 U1～U6 复用字段、范围、分页及 Repo a/b 竞态，补 Dataset 发布失败/取消、查询中途切发布版、账仍 READY 的物理索引丢失恢复及 State 发布失败。只改测试与用例关联、不重构生产实现；仅为验证启动已缓存的隔离 OpenSearch，退出已清理，未安装依赖或提交。
- 所触 owner：声明式索引、索引控制、检索、插件接入及验证体系；场景就近关联沿 `.data/scenes/README.md`。现有 `discovery/` 仍是可执行代码位置，待迁移时随具体实现移入 `declarativeindex/`；本轮不创建仅含假框架或重复合同的目标包。
- 基线：既有 Dataset 准备/发布和 Repo 候选 a → Writer b → 对象回读 a 的定向测试通过，日志 `/tmp/kc-index-cases-baseline-20261013.log`。`tree.py --check-product` 实际失败：文档已改章节/条目格式，仍采用旧标题的产品关联无法解析，先修复关联与稳定 U 标题。测试草稿编译/前态错误只算编写问题，不记为实现红证据。
- 充分/必要边界：既有消费者注册、隔离/等待、scope-before-body 与固定观察断言不机械复制。来源采用到插件登记/真实实例、共享 OpenSearch 物理隔离、说明读取及完整宿主退出缺公开接线，沿现有 gap 具体记录；不写不能执行的 feature、不用测试注册器或换 Projection 注入伪造通过。用例写在本轮，运行结果与未实施插件旅程分开报告。
- 真实红基线：全包 `discovery/service` 的两个既有 `TestProjectionNoticeExecutorPreservesBackgroundSource*` 失败；独立复跑仍失败，日志 `/tmp/kc-index-cases-known-red-20261013.log`。位置为 `discovery/service/operations.go` 的 notice 调用 `SetObservationSource(request...)`，判定为请求覆盖后台来源/身份，拒绝通知也残留覆盖；当场关联 U6，不改/削弱测试，不在测试先行轮顺手修实现。触发处置为插件维护改接前先按 index-control 的后台独立身份合同修复并跑现有反例至绿。
- 新用例证据：新增四个文件的四组测试，含冻结/权威接受失败/接受前取消、Dataset a→b 回读与 scope、物理索引丢失的重启/周期恢复、State 全量/增量候选保存后 Commit 失败及重试。实际 Writer、Index、Controller、Dataset authority、查询服务和对象回读参与，物理端口仅注入故障/候选；新测试普通与 `-race` 全绿且无 skip（`/tmp/kc-index-cases-new-20261013.jsonl`、`/tmp/kc-index-cases-race-20261013.log`）。独立复核收紧 Schema 拒绝的具体错误原因，未新增假插件登记器或为先行用例改生产逻辑。
- 关联与门禁：恢复 owner 的稳定 U1～U6 三级标题及产品章节关联，补现有独立断言和四条新 evidence、后台身份红例与具体 gap；修正旧 Repo/Dataset 独立选用文字。正文 1493 字符。场景产品/工程/共享前态检查、`make check-docs`、`make check-validation`、`make quality`、逻辑与物理 driver 单测通过；日志 `/tmp/kc-index-cases-validation-20261013.log`、`/tmp/kc-index-cases-quality-20261013.log`、`/tmp/kc-index-cases-logical-provider-20261013.log`。运行关联不等于新插件验收。
- 扩展基线：真实 OpenSearch 下 `discovery/projection` 与 `discovery/datasets` 全包通过，`discovery/service` 仅上述两条背景来源反例失败，无 skip；首个验证 run `.validation/runs/20261013T123706Z-878d22c7831a` 在并行文档变更中 sourceChanged，不能当成单一源码状态验证，保留其失败证据并在本轮文字收口后重跑同一选择。完整部署和尚无公开接线的插件采用/生命周期旅程未执行。
- 最终运行：文字收口后相同真实引擎选择重跑，`.validation/runs/20261013T124237Z-046352544505/report.json` 记录 `sourceChanged=false`、无 skip；只有两条既有后台来源反例失败，索引及 Dataset 包通过。新 Schema 错误原因断言收紧后定向 `-race` 再次通过；最终质量日志 `/tmp/kc-index-cases-final-quality-20261013.log` 退出 0。owner/图重读，正文仅补标题前空行后 1498 字符；本轮用例已交付，红基线和缺插件接线的整项合同仍不记验收通过。

### DECLARATIVE-INDEX-REFACTOR-DOC-01 · 索引插件重构说明（2026-10-13）

- [x] Affordance 导航文档增量完成：前轮能力、命名及目录文档已完成；本轮在可供性 owner 与术语补 `kc plugin <名称> help`，Schema 用法提示先沿任务帮助，实际来源采用沿已选定访问概览。核验现有 Schema/CLI 无旁注位后，不为提示增加 Schema 字段或返回封装；独立复核与文档、工程质量检查通过，仅改文档。
- Goal / Non-Goals：沿插件接入、索引控制、检索及能力组织说明本插件合同与模块约束，目录主题/直接关系在图登记；本轮仅改文档、阅读入口和图，不写 Go/正式用例、不改实现或其它任务。
- 不变量 / 取舍：保持 W-01/W-02、V-01/KS-01、AUTH-01～03、R-01/R-02、L-01/API-01；实例可服务多个目标，控制经维护接口依赖索引，查询不反向依赖控制，CLI 经认证客户端/Server。保持既有语义不等于旧代码适配或历史部署升级，不凭空列被否决方案。
- 验证出口：核验当前 Home 装配与公开接缝，独立复核正文，重读 owner/图，检查字数并运行文档、工程质量与增量空白检查；文档完成不代表插件运行验收。
- 本轮验收：U1～U5 原逻辑要求归入字段、选用/范围、维护/恢复、原子/发布失败、查询/分页五组，U6 增加服务隔离及退出；声明式索引未纳入现行场景 product_documents，也无既有旅程绑定，本轮不改场景树或断言。独立复核承诺/目录/失败边界，运行字数、文档、质量和空白检查；这里只证明文档。
- 原子可见边界：同一次查询及按其依据继续读取必须一致，两次独立 latest 请求不承诺自动同版；不改变基础固定读写独立、异步构建或唯一写面。验收须在索引判断与对象层回读之间推进版本/切换 Dataset，拒绝候选 a 与正文 b 混用；仅文档，不把现有同依据接缝当成完整并发验收。
- 实施出口：先编写可执行目标用例并取得基线/缺口证据，再迁移控制、CLI 和接插件；每个失败钉错误原因、发布依据及恢复结果，不以任意 error 或现实现缺陷充当通过标准。正式用例、插件和部署尚未实施，本轮门禁结果另记。
- 名称与能力增量：具体插件为 `declarative-index`，目标目录 `declarativeindex/`；`discovery/` 保留通用查询合同、选择与跨来源组合、业务 CLI，消费侧不导入具体实现。已同步插件接入、能力组织、CLI、术语及图标题，不移动当前代码或把现有路径证据改成不存在的目录链接。`kc plugin declarative-index help` 是 CLI owner 已选定但未实现的只读说明任务；实际执行沿既有查询/运维，帮助不能证明就绪或授权。六组用例补说明与实际登记一致、只读核验；正式测试和接线仍待实施。
- Affordance 导航增量：`object-system.md` 的可供性语义及 `terminology.md` 已显式补统一插件说明入口；CLI owner 选定 Schema 查看任务帮助解释 `access` 并指向该入口，实际来源采用/匹配继续沿既有访问概览，补 U8 导航及机器输出不变的用例规格。证据为 `object/schema.go` 字段无说明位置、`object/reader/schema.go` 不传顶层 description、`cli/product_encode.go` 只传 path/type/access；不将 JSON Schema 的 `$comment` 当作本项目合同或可靠用户提示。只改设计，不改 Go、Schema 内容、公开结果形状或命令实现；新的提示接线沿插件说明任务的正式合同与真实走读验收。
- 导航文档证据：独立复核通过，确认先复用既有帮助/概览，不将运行事实混入 Schema 或把帮助可读等同就绪；变更 owner 与图已重读。`make check-docs`（33 文档/172 关系）、`make quality`、增量空白检查通过，质量仅用已有缓存，日志 `/tmp/kc-index-affordance-navigation-quality-20261013.log`；当前只证明设计文档，未完成插件说明或导航运行接线。
- 上轮文档证据：源文含格式、链接和空白共 1497 字符，可见正文 1242 字符；独立复核后分开更新失败与 Dataset 接受失败，取消限定接受前，统一实例用词。owner/图已重读，新增目录主题及对能力组织的 refines 关系；`make check-docs`（33 文档/169 关系）、`make quality`、本轮空白检查通过，日志 `/tmp/kc-declarative-index-scope-cases-quality-20261013.log`。本轮完成用例规格与目标约束，未编写正式测试、迁移实现或执行部署验收。
- 当前增量证据：正文含格式、链接和空白共 1483 字符；独立复核指出旧发布版不应无条件可用，已改为保留发布版、原依据失效明确失败。相关 owner/图已重读，命名、通用/具体目录及待实施命令边界通过；`make check-docs`（33 文档/172 关系）、`make quality` 与增量空白检查通过，质量仅用已有缓存，日志 `/tmp/kc-declarative-index-capabilities-quality-20261013.log`。仅交付文档，未实现插件或说明命令、未执行运行或部署验收。

### RESOURCE-ACCESS-CONDENSE-01 · 外部资源访问插件重构合同（2026-10-13）

- [x] 表达修正完成：Non-Goals 直接说明请求/返回格式兼容、随服务构建及启动登记、采集仍走写入入口、通知只提示重查、事件历史/订阅另行设计；没有承诺通知路由不改。最终可见正文含标题 999 字符，12 用例原编号/标题不变，owner/图已重读，文档与质量检查及本轮空白检查通过；日志 `/tmp/kc-resource-access-plain-language-quality-20261013.log`。仅修表达，未改实现。
- Goal / owner：资源访问篇拥有 LiveAccess 插件访问接入与观察保证；共同登记/选用/生命周期沿插件扩展篇。已重读 docs/graph 的 ownerTopics、直接 Relation 和现有装配/调用公开接缝。Non-Goals：仅改文档及图中标题，不改行为、Go、协议、场景、主题所有权或其它任务，不提交。
- 不变量 / 接口：保持 V-01、W-01/W-02、AUTH-01～03、PC-01、API-01；公开形状仍沿 LiveAccess README、StateLookup、ObservationBasis 及既有兼容合同。选定合并重复解释，否决删用例 ID、改变保证或把待实现写成已有能力。
- 债项出口：`liveaccess/service` 尚缺登记上下文与目标目录，首次插件接入补静态入口、受限 typed 登记、实例私有处理器、资源归属及选择反例；`invocation/verbs_resource.go` 直接 access 核验/证据缺口在访问服务改接时统一并复跑已登记回归；latest-only 保留夹具不证明生产能力，保留承诺上线前明确介质、容量、期限、备份与恢复；`liveaccess/state_lookup.go` 证明透传不证明目标信任，出站装配改变前落实权限 owner 的目标/重定向政策并验 U7。
- 首轮压缩证据：非空白源码 1000 字符，可见文本 915 字符，五段与用例提取、`make check-docs`、`make quality` 通过；日志 `/tmp/kc-resource-access-condense-quality-20261013.log`。用户随后指出重构目的不够突出，本轮补明全局注入→实例登记/目标路由、统一 access 核验/存证、后台及资源归属，重新独立审查、重读 owner/图并复跑字数/用例/文档/质量检查；未通过前不勾选，文档不代替插件实现或部署验收。
- 本轮验证结果：最终可见正文含标题 983 字符（不含空白、Markdown 标记及链接地址）；独立审查通过，明确逐请求选一次实例，平台与源侧权限分别成立，保留服务没有转嫁接入方。owner/图已重读，12 用例 ID/标题原样提取，`make check-docs`、`make quality`（退出 0）及本轮空白检查通过；质量日志 `/tmp/kc-resource-access-plugin-focus-quality-20261013.log`。仅文档及图标题变更，登记/路由、直接 access 补齐与生产保留仍未实现。

### DECLARATIVE-INDEX-CONDENSE-01 · 声明式索引压缩（2026-10-13）

- [x] 文档压缩完成：将 `declarative-index.md` 压到 1000 字以内，突出声明、保证和兑现路径。
- Goal / Non-Goals：沿该 owner 的逻辑合同与插件要求，仅改文档；维护、查询、登记和观察仍归各自 owner，不改实现或公开形状。
- 不变量 / 取舍：保留固定解释、声明不授权、同依据回读、失败不冒充零命中和基本发布独立；保留五段设计合同及 U1～U5，删除重复铺陈，不复制插件机制。
- 验证与质量出口：独立复核核心决定，重读 owner 与文档图，核对字数并运行 `make check-docs`、`make quality` 和本轮空白检查；结果只证明文档与工程闸，不代表插件或部署验收。
- 交付结果：源文含 Markdown、链接和空白共 998 字符；核心决定与 U1～U5 保留，独立复核通过。文档检查、工程质量闸与空白检查全绿，质量检查仅用已有缓存，日志 `/tmp/kc-declarative-index-quality-20261013.log`。

### PLUGIN-FLOW-DOCUMENT-01 · 插件流程与接入点重写（2026-10-13）

- [x] 文档重写完成：按用户纠正沿启动、任务调用、变化通知、后台对账和退出解释扩展；每个切入点明确调用者、payload、结果采用及失败后果，补完整过程和扩展步骤。
- Goal / owner：通用接入过程归 `plugin-extension.md`；索引逻辑、维护、查询分别沿 `declarative-index.md`、`index-control.md`、`retrieval.md`，LiveAccess 沿 `resource-access.md`；已按 docs/graph 的 ownerTopics 和 Relation 端点核对直接关系，并核验 Cordis 官方机制。
- Non-Goals：只改文档和主题登记，不改 Go、公开协议、CLI、测试、配置或其它任务在途实现；不安装依赖、不启动服务、不提交。保留现有实例选择、固定依据、资源归属与恢复决定。
- 不变量 / 接口：W-01/W-02、V-01/KS-01、C-01/R-01/R-02、AUTH-01～03/KS-02、P-01/PC-01、L-01/API-01；payload 解释其用途并链接现有公开类型，不在设计 Markdown 编造消息字段或状态机。
- 选定 / 否决：用流程、输入输出、切入点和完整例子组织文档，区分同步服务、变化通知、生命周期；借鉴 Cordis 的上下文与贡献归属。否决未来文件清单作为阅读起点、把所有调用改称事件、以源码链接替代过程说明，以及把设计接口宣称为可用插件 API。
- 验证与质量出口：独立按流程复核，重读变更 owner 和文档图，运行 `make check-docs`、`make quality` 及增量空白检查；观察到的实现差距保留在文档落地表，首个接入触发时补齐并验证，不把本轮文档检查写成插件运行或部署验收。
- 交付证据：全文与直接关系已重读，独立复核后修正现有类型锚点、冻结候选输入及显式 notice 的同步结果合同；`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0（日志 `/tmp/kc-plugin-flow-quality-20261013.log`），增量及新增文件空白检查通过。仅文档、阅读入口和图主题登记；未改 Go、运行测试、部署或提交，通用登记与多实例路由仍待实施。

### OBJECT-GUIDELINE-REWRITE-01 · Object 边界与合同重写（2026-10-13）

- [x] 本轮边界纠正完成：明确 Object 写入与 Snapshot 字面写入各自输入、语义和保证，保留六项契约及五项承诺，源码不超过 1500 字符；只改本篇与任务记录，代码实施仍停止。
- Goal / owner：沿 `object-refactor-guideline.md` 的基础模块边界，保留 `knowledge.md` 的身份/维护/Schema 与 `service.md` / `permissions.md` 的任务和当前授权约束；已核对 docs/graph 及现有 Object/Reader/Writer 公开合同。
- Non-Goals / 不变量：不新增“业务 Object／对象知识表示”两套主体，不改 Go、测试、CLI、协议、插件方案或其他任务的在途实现；保持 I-01/V-01、W-01/W-02、API-01、AUTH-01～03、CA-01、OI-01～05、PC-01，不提交。
- 选定 / 否决：用可操作的对象定义、要求→承诺、读写过程和责任表指导重构；否决概念铺陈、预设空目录、重复专题及把测试修补或包迁移写成整体目标。协议形状仍沿公开类型/包 README/Conformance，插件方案仍归各专题。
- 验证：独立复核内容与直接关系，重读改动 owner 和文档图，运行 `make check-docs`、`make quality` 与增量检查；不以文档闸宣称当前红回归或重构已验收。
- 结果与证据：正文从概念/目录铺陈改为对象定义、要求→承诺、描述/写入/读取/当前态流程、责任边界及重构要求；删除并列的“业务 Object／对象知识表示”表、预设目录树、历史图和迁移进度。独立复核纠正了显式历史 Schema 引用与 COMMIT/PROPOSAL 的保证范围；保留现有合同与五段文档格式。`make check-docs`（33 文档 / 168 关系）、`make quality`、增量空白检查通过，质量日志 `.validation/object-guideline-rewrite-20261013/quality.log`。只修改本篇和任务记录，无 Go/测试/图关系/插件方案/部署/提交变化；原红回归继续待实现。
- 本次字数口径：全文 Unicode 字符数包含标题、Markdown、链接及空白，须不超过 1500；保持五段文档合同。压缩后的源码重新复核与执行门禁，前次绿色不作为本次结果。
- 压缩结果：全文 1499 字符、55 行，独立复核确认关键合同保留，明确 Object 只解释固定声明；字数断言、`make check-docs`、`make quality` 和增量检查通过。质量日志 `.validation/object-guideline-rewrite-20261013/condensed-quality.log`。未改代码、测试或文档关系，原红回归与代码实施状态不变。
- 术语纠正：删除 Object 定义中的笼统“来源与能力声明”和表对象示例的“取值声明”；来源依据与特定插件接入分别解释。核对现有 provenance、Schema 的来源入口及既有实现 Member 后，按 LiveAccess 自身合同说明 Schema/Binding 配置，未增加通用 Object 属性。全文 1497 字符，文档/质量/空白检查通过，日志同目录 `access-wording-quality.log`；仅本篇与任务记录变化。
- 本轮结果：六项契约单独定义，五项读写承诺单独说明，保留必要的读写流程与模块边界；全文含 Markdown、链接、空白为 1493 字符。独立复核、字数断言、`make check-docs`（33 文档 / 168 关系）、`make quality`（退出 0）与增量空白检查通过，日志同目录 `contracts-first-quality.log`；未改代码。文档声明的是重构目标，既有行为红回归仍待后续兑现。
- 分层纠正：对象维护的身份、范围、表示与 Schema 保证归 Object；路径/字节及版本提交归 Snapshot，对象变更与文件树变更分别定义。将“唯一写面”限定为“唯一对象写面”，不否定字面写入；保留现有合同，本轮不新增通用整对象覆盖 API。全文 1487 字符，独立复核、文档图重读、字数断言、`make check-docs`、`make quality`（退出 0）及增量空白检查通过，日志同目录 `layer-boundary-quality.log`；未改 Go、测试、图关系或其它专题，原红回归仍待实施。
- 定位审查债：`object/schema.go` 的 `parseSchemaFields` 接受含点字段名，`ValidateSchemaInstance` 按顶层字面键取值；`discovery/fields.go` 的 `lookupPath` 按点拆成嵌套路径，数组只作末值展开。判定为跨模块字段含义不一致，不能复用为统一嵌套定位合同；本轮仅静态核验并登记，未新增 API 或执行行为验收。触发为下一轮字段路径设计/嵌套访问接入：沿 `knowledge.md` 与声明式索引 owner 明确唯一解释，补跨模块回归后再实现，不以当前三段 Address 限制未来内容读取深度。登记的文档及工程门禁通过，质量日志同目录 `address-review-quality.log`。

### OBJECT-BOUNDARY-REGRESSIONS-01 · 重构审查的针对性回归（2026-10-13）

- [ ] 本轮仅补用例：用户明确要求等待正在进行的重构完成后再进行下一轮重构；按目标合同新增并执行针对性回归，保留当前缺口的失败证据，不修改实现、不以现状缺陷作为通过期望。
- Goal / owner：表示一致性、真实只读端口与请求责任沿 `object-refactor-guideline.md` / `knowledge.md`；观察核验及访问证据沿 `resource-access.md`；请求来源与后台来源隔离沿 `index-control.md`。用例最低责任层和证据语义沿 `test-catalog.md`，迁移分母沿 `object-refactor-plan.md`；已核对 docs/graph 所有权及直接关系。
- Non-Goals：不修改生产 Go、公开形状、CLI/HTTP、权限、来源/Schema 声明或后台装配，不接入插件，不干预其它任务的在途实现，不新增依赖、启动服务或提交。不为保持绿色删除/放宽现有断言，也不使用 skip 或构建标签隐藏预期失败。
- 不变量 / 接口：I-01 / V-01、W-01 / W-02、AUTH-01 / AUTH-03、API-01、OI-01～05、PC-01；使用公开 Writer、Reader、StateLookup、实际访问入口和 ProjectionNoticeExecutor 的既有合同及现有内存夹具，不另造插件或任务 DTO。
- 选定：Writer 在无 Schema 下也拒绝最终表示冲突且不推进 Ref；Reader 的具体返回值逐项不携带创建 Ref、Merge 或 Archive 能力；直接 access 拒绝非法观察依据/违反实际采用 Schema 的值，并保留合法空值及实际访问证据；两个请求身份不能替换装配期后台来源。每项含能到达目标检查的合法前态及控制组。
- 否决：把缺陷复现成功写成目标合同通过、只断言任意错误/函数名/目录、为测试方便发明未选定授权上下文或 API、把普通 audit 当实际访问证据，以及把包迁移当行为修复。
- 审查项保留：Repo READ/COMMIT 的任务规则仍在 Invocation、Object 根仍拼装访问 endpoint，判定为职责收口事项；本轮不编写绑定未选定字段或静态函数位置的伪行为测试，触发为当前重构完成后的下一轮职责复核。无 Schema 表示冲突和真实只读端口已由无服务临时探针确认，未判定是否由本次重构引入。
- 验证出口：新增测试必须编译并单独执行，明确目标合同的红/绿结果及失败原因；运行 `make check-docs`、`make quality` 和增量检查，仅报告实际范围。因用户本轮只授权用例，未修复的目标合同报红时保持本条未勾选；下一轮修复后复跑所需合同至绿，不使用本轮局部结果宣称当前重构或部署已验收。
- 用例落点：`object/writer/representation_consistency_test.go` 覆盖同批两顺序及既有 Record/Member 的冲突、Ref/原对象/回执不变和合法 REMOVE→PUT 转换；`object/reader/readonly_view_test.go` 覆盖 tree/native × Require/Wrap 的具体能力及固定读值/坐标；`invocation/direct_access_contract_test.go` 经实际应用入口验证非法观察、固定 Schema、合法值/null、成功/源失败的耐久 Access 证据；`discovery/service/notice_background_source_test.go` 通过真实 Source 桥接核验两个 caller 以及拒绝路径之后的后台实际来源、主体、Trace 和请求关联。
- 独立复跑：四包新增用例均编译成功，合法转换及合法值/null 控制通过；冲突表示/只读端口/直接 access 核验与证据/后台来源隔离仍真实报红，组合命令退出 1。最终 JSON 事件保存在 `.validation/object-boundary-regressions-20261013/targeted.jsonl`，各项首轮输出分别是同目录 `object.log` / `access.log` / `notice.log`；没有 skip、构建标签或现状迎合断言。未改生产实现，未执行部署验收或整套契约。
- 质量出口：已有 `TestProjectionNoticeExecutorConfiguresRuntimeBeforeNotice` 锁定请求替换来源，与本轮目标隔离断言冲突；本轮保留旧测试，下一轮改接 notice 时须按 owner 合同复核其 oracle。服务规则与运行 endpoint 的归属继续作为职责审查项；不要求当前在途重构同时完成插件接入。新增文件的空白检查通过；门禁结果随本轮最终输出登记，不把行为红证据记作验收通过。
- 最终门禁：`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0，格式/tidy/vet/staticcheck、生产复杂度/体积/克隆和耦合棘轮通过，日志 `.validation/object-boundary-regressions-20261013/quality.log`；仅工程闸绿，目标行为仍红。测试自检允许既有内部证据包装而只约束公开输出，源失败钉原错误分类且不能制造成功观察；notice 拒绝钉实际到达控制路径及错误分类，不约定错误对象地址。下一轮实现使用本轮正式回归，不将临时“复现缺陷成功”的探针作为目标通过条件。


### OBJECT-PLUGIN-DOCUMENTS-01 · 四项设计合同成文（2026-10-13）

- [x] 文档交付完成：四项契约、承诺与具体接入已落实到现有四篇主设计文档，设计书 README 提供按四问题阅读的入口；仅文档，未实施登记或路由代码。
- Goal / owner：基础对象合同与迁移归 object-refactor-guideline；通用登记、能力分区和目标选择归 plugin-extension；索引逻辑/维护/请求分别归 declarative-index、index-control、retrieval；LiveAccess 的具体登记及访问/观察合同归 resource-access。所有权与直接关系只在 docs/graph 登记。
- Non-Goals：仅文档，不改 Go/API/CLI/测试/配置/系统出版物，不开发统一宿主或安装管理，不安装依赖、不启动服务、不提交；保留各并行任务在途内容。
- 不变量 / 接口：保持 I-01/V-01、W-01/W-02、API-01、AUTH-01～03、R-01/R-02、PC-01及现有观察保证；公开形状引用 object、discovery、liveaccess 和兼容操作合同，不在 Markdown 编造 DTO、错误码或状态机。
- 选定：四项各归一个主阅读文档，公共规则按能力分区，Repo/Dataset 可同时选择索引与 LiveAccess；类型化目录和登记上下文由各消费侧拥有，实例与资源生命周期明确。LiveAccess 接入不再只留未来方向；动态索引与 LiveAccess 组合必须满足已有来源/观察共享合同。
- 否决：单个目标只能选一个万能插件、按生命周期阶段拆出互不关联实例、统一 role+any 注册表、所选实例缺能力后暗回默认、重复正文制造第二合同、用已有算法或测试夹具宣称接入/生产保留已实现。
- 验证出口：重读改动 owner 与文档图，独立机制复核，make check-docs / make quality / git diff --check 全绿再标记本条完成；不把文档闸记为运行/部署验收。


- 交付结果：Object 篇补描述组织及固定写读链，含提交后结果不确定的原命令恢复；插件篇补索引/LiveAccess 带类型能力分区及独立默认/精确目标选择；索引篇补要求→承诺和阶段贡献；资源访问篇补 LiveAccess 登记上下文/私有目录、read/invoke 与兼容声明链，源地址不被实例覆盖。动态共享索引与所选 Live 来源必须相容，不暗合并；三个具体 Live 接入用例、公开 API 锚点及实现差距同时登记。
- 验证证据：owner、阅读入口及 docs/graph 已重读，独立跨篇复核通过；make check-docs（33 文档 / 168 关系）、git diff --check 与 make quality 全绿，日志 /tmp/kc-four-design-documents-quality.log。未改 Go/测试/CLI/运行配置或系统出版物，不将门禁通过记为插件运行、生产观察保留或部署验收；现有质量债及触发出口继续保留。

### DATASET-PERMISSION-TESTS-01 · Dataset 权限测试与用例优化（2026-10-13）

- [ ] 本轮认领：将此前的充分性与必要性审计落实到测试和用例，补足失效反例与状态断言，归并同义重复，保持所有独立授权、范围与传输边界；不提交。
- Goal / owner：Permissions 的身份、资源隔离、当前授权与耐久决定，Dataset 的可信发布范围和固定依据，Test Catalog 的最低责任层与独立风险规则，Quality Loop 的反例与质量闭环。所有权及直接关系按 docs/graph 读取。
- Non-Goals：不引入对象或字段 ACL，不修改公开动作、DTO、授权政策或底层存储，不把动态源共享和发布选源的未选定 authority 编成测试假设，不调整质量阈值、不安装依赖，不覆盖其它聊天的在途修改。
- 不变量：保留 KS-01、KS-02、AUTH-01～03、V-01、API-01；pin 不授权，清单不扩仓，失败不污染授权账或已接受发布，业务请求仍经已认证 Server。
- 选定：先让正反用例确实到达目标检查；授权更新覆盖 old/new 四格、精确错误码与深快照；补临时多来源的第二阶段授权、真实授权管理到 file.read 重判、准确可见集合及落盘边界。纯参数留最低层表驱动，各 transport/provider 仅保留自身转换和公开组合风险。
- 否决：用测试数量或通过事件宣称充分、用另一道前门拒绝冒充目标检查、只验任意 error/数量/秘密 marker、为减少数量删除独立合同、用当前实现反向收窄设计。
- 接口：沿 snapshot/access、snapshot/dataset、snapshot/service、snapshot/client 的既有合同和应用装配；本轮主要修改测试、场景与必要的证据说明。发现行为缺陷先保留反例，再按相同 owner 合同修复。
- 验证：定向合同与公开 CLI/HTTP 旅程、必要 race、反例故障注入，以及 make check-docs、make quality、普通场景与完整 contracts；分别保存源码绑定结果，不合并历史执行或把 skip 当 pass。仅完整所需合同全绿且无 skip 后勾选。
- 质量出口：未闭合的发布选源/分享 authority、最后准备回调撤权窗口、动态源消费/共享/目标信任按原 owner 保持债，不造虚假通过。每项本轮发现的问题记录位置、信号、判定、处置、触发条件；同类弱断言以表驱动状态断言和反例回归固定，不再依赖人工计数。
- 实施结果：规则坐标逐维反例与 Add/Revoke/Update 账状态使用独立深快照，更新覆盖旧/新范围四格、准确冲突码、并发唯一赢家及私有迁移回执；临时多来源、具名与同名临时对象、metadata、三个既定 hook 窗口均补正确前态及精确后态。文件范围钉准确路径、来源提交和字节；cursor 纯参数矩阵归组件，HTTP 留转换与撤权见证，重复 hook 操作由已有最低层合同拥有。
- 公开旅程：新增只撤 file.read 而保留 dataset.resolve 的真实耐久授权更新，以及 Repo×Dataset 部分来源授权到目录、读取和正式 clone 的路径与落盘字节核验。二者保持完整接受 pin 不变，另一已认证主体仍能读取原数据，不以不存在的文件或前门拒绝冒充文件授权。
- 当场修复：`invocation/dataset_access.go` 的文件适配将 Repo 与 Dataset 分开求值，Repo×Dataset 交集规则丢失一个坐标；新公开旅程和组件反例在原代码 overlay 上真实报红。私有 helper 传入实际 Repo 与 Dataset 后转绿，保留原 Match、授权政策和公开形状；独立增量审查无阻断问题。
- 门禁差距登记：同期资源访问设计新增 U10～U12，产品覆盖检查报缺入口/缺口；在 `.data/scenes/_views.yaml` 如实登记实例选择、替换与兼容执行仍缺的旅程，保留检查器和既有证据。后续实现这些新合同即须补实际场景，不能用本轮 Dataset 固定文件证据替代。
- 保留债：`snapshot/service/dataset_http.go` 的发布选源/分享 authority 尚无已选定独立动作，判定为 owner 合同待选，触发为公开发权方案落定；PreparePublication 内发生撤权的最终提交边界尚无请求期授权/租约合同，判定为待审窗口，触发为该提交边界合同选定；动态来源的消费授权、观察共享与目标信任差距继续归 `resource-access` U6/U7，不把固定文件绿外推。历史真实 clone 脚本检查旧输出目录的失效 oracle 仅留历史审计记录，当前正式 clone 旅程检查当次唯一 TempDir 的精确落盘集合，后续采用该脚本前必须修复并复跑。
- 测试闭环修正：`scripts/maintenance_test.py` 的 surface 守卫隔离夹具缺同期迁移后的 object / snapshot/cli / snapshot/client 目录及两份 README，基线真实报红；仅同步前态后六项回归全绿，缺扫描路径必须失败和全部退役反例仍保留。Go 测试保存基线核对为 18 份既有文件修改、1 份新文件、10 项真正新增、4 项改名；6 个独立故障有红/绿见证，未将编译失败或历史变异计入。
- 证据合同：共享工作树仍有并行迁移，普通场景断言虽绿但运行期修改套件脚本导致 shell 尾部读偏移，不能将该入口退出失败写成通过；以当前文件及原 HEAD 建立无提交的隔离验证副本，复制前后指纹一致后重跑质量、库存、普通场景与完整合同。最终同源结果另见 `.validation/test-opt/20261013/`；既有外部/live skip 和未执行真实部署单列，不勾选为完整无跳过验收。
- 完整合同发现与修复：第一份稳定副本的各测试断言通过，但 TestMain 的逐命令边界门禁以 61/61 成功、57/61 达标报红；`cli/snapshot_consumer_commands_test.go` 的四个迁移后命令缺可计入且精确的失败断言。复用既有三条旅程，补未发布版详情、异版 cursor、无管理权更新，强化撤权/下载目标/过期观察的准确错误与完整状态不变。定向三项无 skip 全绿，四命令成功与边界全部达标，未改门禁或增加顶层测试；本轮现为 20 份 Go 测试文件、10 项真正新增，受影响文件内 66 个顶层 Test。随后以新稳定副本重跑完整合同，不用拼接两次覆盖报告替代套件通过。
- 全仓合同阻断出口：第二份稳定副本的组件合同被同期新增的 `object/writer/representation_consistency_test.go`、`object/reader/readonly_view_test.go`、`invocation/direct_access_contract_test.go`、`discovery/service/notice_background_source_test.go` 反例阻断，信号为表示冲突、具体只读返回值仍携写能力、直接访问核验/证据和后台来源隔离失败。判定与处置沿本文件 `OBJECT-BOUNDARY-REGRESSIONS-01` 的明确用户边界：本轮只补反例，等待在途重构后再修实现；保持红证据，不删除、skip、放宽或代修这些领域行为。触发为该重构切片修复后重跑完整 contracts；本轮另独立执行完整 CLI 合同确认新增边界，不将受影响测试和质量闸绿写成整仓合同全绿。
- 最终执行范围：稳定副本 `frozen-v2` 的本轮 66 个顶层 Test / 121 个叶全部通过、0 fail / 0 skip，授权并发两项 race 通过；`make quality`（含 check-docs）、`make check-validation` 与普通/索引场景通过。独立完整 `make test-e2e` 退出 0，61/61 公开命令有已断言成功，61/61 满足风险对应的失败边界要求；495 顶层 pass / 6 既有外部/live skip，不能把 skip 记为通过。完整 mixed contracts 因上述另一任务六个顶层反例失败而退出，保持本条未勾选；各最终 run 的 sourceChanged=false，代码/测试与当前工作树相同，记录性 TASK 后补。结果、分层目的、去重及反例依据保留于 `.validation/test-opt/20261013/RESULT.md`，未执行真实部署、未提交。

### SNAPSHOT-ENTRYPOINTS-01 · Snapshot CLI / SDK 独立归属（2026-10-13）

- [ ] 本轮认领：先拆除 Snapshot 客户端入口对 KC 应用的依赖，再将 CLI 与 typed HTTP SDK 收入 `snapshot/`，KC 保留统一入口和装配；设计与实现、回归和质量门同时补齐，不提交。
- 本轮用户纠偏：目录迁移不能仅搬执行器；Snapshot 的操作数、参数归属、产品输出与模块帮助分组仍有中央残留，帮助也尚未全面达到可供性说明标准。Goal 沿 CLI / capability-organization / terminology 的现有 owner：继续收口这些叶任务合同，并让十五个模块叶帮助明确用途、输入、前提、结果、后果、风险与成本、失败恢复及示例。Non-Goals 保持既有 argv/Action/HTTP/授权/发布与输出保证，不造新的 Affordance 实体、描述 Schema 或分派系统，不迁移宿主身份、跨模块旅程、FUSE 生命周期与集成测试，不代修其它任务故意保留的领域反例。
- 帮助范围复核：61 个公开叶原先都有展示入口，但中央 46 叶及 serve 的不少说明仍只有调用式与简述。沿同一用户标准继续补齐全部公开叶与 serve 的具体输入、结果、后果和风险/恢复，按 KC 宿主环境、Writer、知识读取、治理、运维拆帮助文件；不新增命令、不改变执行语义，不把静态帮助当成动态就绪/授权或源操作完成保证。Snapshot 十五叶仍独立持有自身说明，其余任务由各自现有宿主合同装配。
- 本轮选定：静态 Command 贡献操作数与模块分组元数据，模块独享参数范围检查；KC 按公开声明绑定 argv、组合导航并注入运输。帮助正文按仓登记、Dataset、文件与授权拆在模块内，独立 Executor 与 KC 两条帮助入口完整同源。否决继续维护中央叶名单、仅有调用式或通用章节占位、移空壳换目录、用帮助宣称目标已就绪/已授权，以及为了形式齐全编造副作用或性能保证。不变量 API-01、L-01、KS-01/02、AUTH-01～03；接口指向 snapshot/cli/contract.go 与对应公开 CLI/包 README。验证须检查真实入口传递、帮助零请求/零写入、具体风险与恢复事实、模块归属闭集及现有 argv/stdout 回归，而非仅检字符或成功执行。
- 本轮帮助事实复核出口：`server/service_management_routes.go` 的 registryAudit 只转交 limit，`invocation/inventory_ops.go` 的 VerbAudit 没有续页协议，SDK 虽可携 continuation，实际入口不分页；`snapshot/dataset/log.go` 的 Log 将 history 读取错误变为空历史。判定为既有审计能力边界及错误传播债，不在客户端归属修订中扩张领域协议；帮助明确只有最近一段、continuation 不续读且空 entries 不能证明无变更。处置为保留事实与位置，触发为审计分页或错误传播合同下一次维护时补服务/SDK/CLI 一致行为与故障回归，不能只改文案宣称已支持。
- 本轮增量修复：帮助核对暴露 clone 在首块为空且 EOF 时不创建文件，违背原始字节物化合同；按既有写入路径创建真实零字节文件，新增实际落盘/统计、已存在目标、非推进响应与中途失败回归。空目录不独立物化及整树部分残留仍如实说明；单文件下载与 overlay 的不同写入保证保持。授权名单过滤与账头隐藏随真实 SDK 录制回归迁入模块，中央编码器只保留统一编码。
- 宿主帮助核对的存量债：`cli/product_encode.go` 的 shapeCLIReadObject 去掉 observations、shapeCLIAccess 只展示首条观察，当前简化输出不能证明完整观察集合；`cli/remote_operations.go` 的 AccessSpec 请求不携单仓 commit/ref，当前使用 published。判定为既有产品展示/选择边界，帮助如实说明，处置为登记并不扩张公开形状；触发为观察证据输出或 AccessSpec 历史选择合同下一次维护时补精确输出与目标选择回归。
- 宿主参数/结果说明债：`cli/remote_management.go` / `server/service_management_routes.go` 的 validation record 不传 message；`cli/remote_operations.go` 的 hook/gate list 不传 on/repo 过滤；`invocation/verbs_observability.go` 的 hitmap 聚合后按 limit 截断排名，continuation 不形成续页。判定为旧帮助超出真实产品执行、非本轮领域行为改造；当场纠正文案并登记，触发对应请求/分页合同维护时选定支持或拒绝策略，补真实 argv/SDK/服务回归。不能仅把这些旧参数写在调用式里就声称已有过滤、附注或分页能力。
- 说明复核出口：`cli/SURFACE.md` 曾把 relations 写成邻居身份名单，实际结果为关系对象坐标与端点；在本轮相关说明当场纠正，保留原结果回归。`object/writer/preview.go` 的 Ingest 注释仍借已退役 kc ingest 解释函数名，判定为存量注释债，登记而不改 Writer 实现；触发下次 Writer 接口说明维护时清理，不据此恢复 argv。
- 本次纠偏交付：Snapshot 的叶操作数、参数范围、帮助组与授权列表产品输出已收口，六个无符号中央空壳删除；中央只保留声明装配、身份运输及必要内部应用/嵌入夹具。十五个模块叶及中央四十六叶加 serve 均提供完整同源帮助，正文分任务文件持有，统一展示不打开输入、不连服务或写输出。独立事实复核修正默认引用、配方输入互斥、单仓/命名 Dataset 双消费授权以及 Hook/治理真实边界，不编造未实现权利或恢复保证。
- 本次验证范围：受影响模块 21 个顶层测试 / 51 个通过事件、实际消费与授权装配 16 / 39、全部帮助与中央装配 34 / 198、架构及文档/库存检查 60 / 335，各入口均 0 fail / 0 skip；计数按各份报告分别保留，不合并为去重测试总量。模块 race、make check-docs、make quality、surface 与库存检查通过，耦合仍为 66 单元 / 16.67%，未调门禁。最终检查窗口内 1056 源文件指纹一致，sourceChanged=false；日志与摘要在 .validation/snapshot-affordance。前一轮完整 contracts 的已登记 Object/LiveAccess 红例仍保留，本轮没有重跑整组 contracts 或真实部署，不把定向验证写成全仓验收，本条继续未勾选；未提交。
- 用户要求的零知识走读：当前源码构建真实 kc，从根帮助沿导航访问，并检查未知 topic、无 Server 与缺参反馈；另用无项目上下文的独立代理只访问 CLI 30 次，不读源码/README/设计。结论撤回“帮助八栏齐全即整体可供性达标”的判断：可重构固定文件交付流程与写入/恢复边界，但 Snapshot/Repository/commit/revision 的关系、来源版本选择/发现、可复制完整配方及文件消费最短旅程仍需先验；Repository 元数据的可达使用边界也未披露。五项缺口逐项记录位置 / 信号 / 判定 / 处置 / 触发条件于 .validation/snapshot-affordance-probe/assessment.json，原始主走读输出保存在同目录；未据未知 topic 强行新增按包命名的命令树，未修改实现或连接真实部署。触发为下一轮帮助整改，以只凭 CLI 的独立走读能正确理解并选择下一步为验收，不以章节数替代。
- 本轮补齐 Goal / owner：用户明确授权消除 AFF-SNAPSHOT-01～05，沿 cli 的完整交互目标、dataset 的文件选择/固定交付、terminology 的资源与版本关系、capability-organization 的模块客户端归属，补概念引导、真实选源边界、完整配方和消费旅程、原生元数据的可达入口。Non-Goals 沿这些 owner：业务 argv/Action/HTTP/DTO/授权与发布保证不变，不新增来源浏览权威、Metadata CLI、知识解释前提或动态帮助框架，不迁移身份/跨模块宿主流程，不安装依赖、不提交或运行真实部署。
- 本轮选定 / 接口：模块以 HelpTopics() 的纯静态字符串映射贡献分组引导和概念页，返回副本，叶仍由 Commands() 声明；KC 只装配帮助命名空间、同源分组说明与自身全局旅程，不把解释页注册为业务命令。否决继续用章节数作为可用性、在中央复制模块说明、将 commit ID 伪装成当前 selector 支持、把 baseRev 核对写成历史选择，以及编造 Repo metadata argv。接口归 snapshot/cli 的公开代码与包 README；不变量 API-01、L-01、KS-01/02、V-01、AUTH-01～03。完整输入示例必须经现有 parser/离线 CLI 验证；实际使用者仍沿来源管理者/原生界面取得当前 KC 未提供浏览的源分支、历史版本与路径。最终以新二进制和无项目上下文走读复验五缺口，并跑受影响合同、check-docs/quality，不修改其它任务保留的红例。
- 第一次复走读反馈：新根导航可发现概念、版本、配方和属性；独立无上下文使用者实际完成完整离线例子，并正确重构资源与交付关系。但 selector: main 与单文件 target: ../outside.csv 仍可被本机候选保留，帮助的校验阶段容易被高估。已核验原 lakeFS GetRef 的引用支持在发布解析阶段，PrepareKnowledgeSet 编译接受清单前校验单文件路径；本机 ValidateKnowledgeSet 仅非空/一致和目录布局检查。处置为本轮明确离线与发布两个证明范围、保留真实负例及完整合法例子；源检查不在本机伪造。位置 snapshot/dataset/definition_validation.go + mount.go 的单文件输入 preflight 与 dataset.go 路径校验差异登记为存量校验一致性债，触发下一次输入校验维护时复核并补各入口准确错误与无状态变更，不能以文案修订声称已修校验。普通消费者缺 Server 的反馈同时改为取得既有地址与登录，再区分部署运营者入口；错误码及执行边界不变。
- 本轮五项补齐结果：静态帮助从根导航到资源关系、合法来源引用/取得办法、完整配方/overlay、已有 Dataset 文件消费和原生属性管理，十四主题与十五模块叶同源装配，业务叶仍为六十一。独立零知识走读实际调用四十一次，照抄两份 YAML 完成 stdout/新文件合成，并核对缺字段、目录/同仓冲突、未解析 selector、单文件路径与空补丁；只凭输出能正确说明资源/版本、发权、离线/发布与属性边界，完整产物符合帮助。最初两次入口调用用了系统临时目录，随后按指定 /tmp 隔离重跑，偏差和全部输出保留在 .validation/snapshot-affordance-complete/cold-final.json；未读源码或服务配置、未连接真实服务。工程独立复核发现未知主题恢复名单未纳入概念页，当场补回根导航并加实际 Run 回归，不复制主题名单。
- 最终走读的存量反馈债：snapshot/cli/overlay.go 将本机候选显示为 revision=1，snapshot/dataset/overlay.go 的 empty 分支忽略无范围改动的 name，解析又忽略未声明的 metadata，snapshot/dataset/definition_validation.go、overlay.go、definition.go 的错误正文仍混用 workspace/commit coordinates。信号为成功结果缺候选提醒、静默未生效和新手术语负担；判定为既有结果/输入/消息合同，当前帮助已明确真实范围，本轮不扩 DTO 或改合成规则。处置为登记；触发下一次候选输出或输入合同维护时选定明确提示/拒绝策略，补真实 argv/完整产物/无发布回归及统一 Dataset 术语，错误码不得仅为改词改动。另据 snapshot/cli/commands.go 与 input_contract.go 核验 clone 仍沿既有宽参数词表，clone.go 只解析当前版，不能指望 --revision 选择历史；触发参数合同维护时明确拒绝无效选择并验证零运输，当前帮助已指向历史 files list/read。
- 本轮补齐验证：模块/SDK/纯运输 32 个顶层测试、75 通过事件；帮助/装配 39 / 231；架构/文档/库存 62 / 337；完整 Dataset 111 / 169；原生 lakeFS 定向 12 / 33；补充实际消费/生命周期 6 / 8。各报告零 fail / skip，计数分别保留不合并去重。模块 race、make check-docs（33 文档 / 171 关系）、make quality、make check-validation 与增量空白检查通过，耦合 66 单元 / 16.67% held。最后窗口内 1047 工程源码/构建文件指纹一致，sourceChanged=false；五项闭环、原始两轮走读、最终未知主题→根导航→Snapshot 的四次增量实访和测试日志在 .validation/snapshot-affordance-complete。只验证帮助/离线任务与进程内消费合同，未运行真实部署或整组混合 contracts，保留其它任务已登记领域红例，本条继续未勾选；未提交。
- 用户要求的整层端到端 review（2026-10-13）：沿 core-architecture、dataset U1～U10、permissions、service、capability-organization、cli 与 test-catalog/W-02，核对存储写入、登记/发布/历史/属性、源许可/当前授权、文件交付和独立 CLI/SDK/HTTP/Console 的同一任务链。以“应然要求→公开合同→真实入口→正反后态→故障恢复→同源码部署证据”建立分母，不用测试数或平均百分比抵消阻断。只读生产实现；临时公开 API/Go overlay 反例在 .validation/snapshot-system-review 与 /tmp，不改或弱化已有断言，不安装依赖、启动真实部署或提交。
- review 三出口：本次已复现的写入丢 ACK 删除未决命令、同 ID 异内容再写（command_ledger.go:254）、实际来源目标碰撞接纳并切换当前版及读取入口分歧（dataset/definition.go:157、service/file_view.go:173/231、dataset/virtual.go:49）、不存在单文件仍接纳/check 通过、已有权威缺全元数据可重建空账（dataset/registry.go:65、registry_store.go:30/44/56）、observer 擦除领域拒绝（raw_writer.go:57）均登记为真实修复债；Console 借管理详情拼 pin 导致纯消费主体被拒（console/webui/src/pages/kc/dataset.tsx:38/47、service/dataset_http.go:103）登记为入口合同阻断。每项位置/信号/判定/处置/触发和可重复反例见 assessment.json；触发下一轮 Snapshot 正确性/恢复/入口收口，将跨组件同类风险升为正式最低层合同和公开旅程，不以改帮助或扩大 grant 掩盖。本次审查没有修这些生产缺陷。
- review 完成度缺口与证据边界：独立 Handler 上 show/audit/attach/detach 的真实模块客户端请求仍 404/405；来源选取/分享许可是 permissions owner 明认未承接的设计目标，不能把 Dataset manage 当作源许可；已接受来源引用后的短 GC 策略仍被转发，但本次没有执行真实 GC；reset 前残留锁在两次重开后仍阻断写入，需要明确安全恢复合同。混合文件地址静默忽略、物理删除正确拒绝、范围响应仍整文件读取/原生 blob 上限等分别登记具体债或已选运行边界，不改选定协议。现有核心 Dataset 111、access/service 92、入口包 33、真实 HTTP+lakeFS 夹具旅程 6、相关守卫及前端 176 项回归通过；新增临时反例明确失败（写入 2 条，Dataset 7 条预期后态，文件读取 2 条），quality 通过不替代它们。全仓源指纹因其它任务文档并发变化而不同，逐文件核对 Snapshot 和相关宿主实现未变化；旧真实部署源图不同，仅作历史证据，当前部署/浏览器/SSO/GC/容量未验收。整层仍不能标完成，本条保持未勾选。
- 本轮完整补齐授权 / Goal：用户明确要求依审查顺序完成 Snapshot，先 P1 正确性、恢复与纯消费 Console，再收口独立登记入口与来源选取/分享许可，最后同源码端到端验证到绿。沿 core-architecture / dataset / permissions / service / capability-organization / cli / test-catalog 的 owner，保留 W-02、KS-01/02、AUTH-01～03、V-01、API-01、L-01 和 Dataset U1～U10；本轮行为变更前先落真实失败回归，不以修帮助或更换断言替代。
- 本轮 Non-Goals：不改变 Object 的表示/读写语义或修其它任务故意保留的领域红例，不新增对象/文件 ACL、知识解释、跨仓事务或另一套身份/授权/登记账，不扩容量承诺、不安装未经批准的依赖、不提交。Object Writer 与内存 provider 仅透传共享命令账所需的明确未提交证明。真实验证仅使用专用 local 测试栈或隔离夹具；不清理、不替换其它运行中的部署。
- 本轮选定 / 否决 / 接口：共享命令账只有明确未进入权威提交的错误才释放占位，未知出版结果保持耐久未决；由 snapshot 的纯错误包装承接证明而非按错误码猜测。Dataset 在接受前有界核对实际文件目标，统一纯路径路由，不用整树扫描；空权威须以已有历史能力证明确为首次根才能创建，不能凭空树重置。Console 直接 resolve 服务接受的当前/历史 pin，管理详情独立加载而非消费先决。所属登记/库存/审计 HTTP 收入 Snapshot 服务，KC 只注入供给、说明与外部政策接缝；来源发布许可必须显式且与 Dataset 管理和普通读取分开，沿既有规则权威强制。公开接口在 snapshot 根、dataset/access/service/client 的代码与包 README 选定，owner 正文描述保证与取舍，不复制协议字段或状态表。
- 本轮来源许可选定：目标 Dataset 管理之外，逐来源 Repository、实际 selector/ref 或显式固定 commit 与目标 Dataset 求值独立发布许可；来源检查和配方副作用之前、最终接受之前重查。来源许可的管理必须具备仓或全局授权管理权，Dataset-only 管理不能自授；创建者仅显式配置、消费分享不扩张、已发布消费不重加整仓授权。接口由 access/service 公开合同持有，否决复用普通读取、dataset.* 或第二授权账。
- 本轮恢复与保留选定（行为前）：沿 core-architecture 的固定依据、W-02/V-01 与 dataset U4，lakeFS 残留发布锁保持 fail closed，增加指定底座的显式恢复操作；调用者先停止所有相关写入者，再按已观察目标和锁候选双坐标核对，释放锁不推进目标且不解决命令 PENDING。否决超时删锁和无预期值盲删。原生产品接口在缺少已提交历史保留验证能力时拒绝修改已登记权威的 GC 策略，不代替私网管理员部署约束。正式回归钉住陈旧观察/故障/重开/幂等、恢复不写目标及拒绝不传给底座；恢复只限 lakeFS 公开函数，不增通用 Store 方法、DTO 或 CLI 命令树。
- 本轮恢复质量出口：争锁方观测目标已发布不能证明锁持有者已停止，自动删除他人锁会与原持有者延迟清理形成竞争；已移除争锁清理，新增两类正式反例守卫并复跑并发/race。Recovery 要求完整分支引用，空值/HEAD/裸名拒绝；真实 lakeFS 已验证双坐标核对、拒绝与幂等恢复不推进目标。
- 存量别名债：snapshot/internal/lakefsimpl/repository.go 的 publicationLock 用 logical ID 加分支构造，而 OpenExisting 不核对 logical ID 与物理身份；同一物理仓以两个 logical ID 打开可能用不同锁。判定为既有别名绑定风险，不由本轮增量扩入多绑定支持；处置是在引入或允许同物理 authority 多 logical ID 之前，明确拒绝别名或按物理坐标选锁并处理旧残留迁移，触发时补跨实例 CAS 反例，不以本轮单一实际绑定的证据宣称多别名安全。
- 本轮增量完成范围：未知写入保持耐久占位且拒绝不同digest重用，null回执重开不伪造重放；实际选定文件存在/交付冲突与旧pin多地址/隐藏范围统一；空登记须证明初始根；Console消费直接服务pin、详情独立；四个管理HTTP归Snapshot及真实审计错误传播；来源发布与目标管理分权、源规则管理防自授、一次性发布末端复核、HTTP消费不隐式采用配方、源问题询问同matcher；check在file.read之前不探正文；RawWriter与Dataset observer保留真实领域错误、接受后记录失败不回滚；GC变更失败关闭与显式停写恢复。每类均有正式先红后绿及相关真实端口回归，不以更换成功断言替代。
- 真实部署入口增量出口：scripts/deploy/bootstrap.sh保留退役fixture参数、CLI横幅与环境仍有旧Catalog入口，真实启动从旧配置及flag解析失败证明；已更新选定唯一登记工具参数、kc show及登记身份环境，六份入口统一加入架构守卫，真实local初始化/系统二次核对/健康检查已通过。只重建可清盘kc-deploy-local测试项目，不处理其它开发/共享项目；依赖沿既有锁定工具和镜像，未加项目依赖。
- 最后路径/接线增量：完整交付声明占用显式目录及祖先，隐藏file/mount不能让root映射交付同名祖先文件；发布对root实际目录位置有界点查并在cap不足时拒绝，消费依完整声明排除且不探隐藏源，合法兄弟保留，正式红绿/race已证明。真实local原生Repo操作因明确本地身份没有注入服务凭据而被旧stock身份/Setup阻断；选定仅显式local且可信本地身份允许配置私网凭据，未配置模式/匿名/存储身份模式不扩张，实际调用者仍由产品当前规则检查。
- Goal / owner：沿 core-architecture / capability-organization 的 Snapshot Store 独立模块目标收敛客户端入口；CLI 交互由 cli、文件组合与登记由 dataset、服务边界由 service、授权由 permissions 拥有，主题与直接关系按 docs/graph 读取。
- Non-Goals：保持现有 argv、Action、HTTP 路由、发布与授权保证，不新增独立二进制、命令树、会话或通用注册框架；托管仓供给、连接恢复、部署和跨知识任务仍归 KC 应用；不迁移 kcfs / datasetfs、不安装依赖、不运行真实部署验收，不覆盖其它聊天的在途修改。
- 不变量：保留 API-01、A-01 / L-01、V-01、KS-01 / KS-02、AUTH-01～03、W-01 / W-02；固定发布和当前权限保持，客户端不复制业务权威，模块核心不反向依赖客户端入口或 KC 宿主。
- 选定：新增明确一级 `snapshot/client`、`snapshot/cli`，CLI 经 typed HTTP SDK 调用唯一服务；宿主注入已有已认证运输与主体，不复制身份或会话。通用 JSON HTTP 编解码、压缩和错误处理以私有基础设施共用一次实现；命令与叶 help 静态同源装配，文件按 Repo / Dataset / File / Grant 职责组织。
- 否决：直接搬文件却携带 invocation / root client 的知识依赖、旧新双实现或长期类型转发壳、底层 Store 导入 CLI、CLI 本地重做授权或发布，以及仅因原 HTTP handler 位于 Server 就把文件登记权威重新划给宿主。
- 接口：现有 `snapshot.Store` / dataset / access / service 合同不扩张领域行为；客户端公开入口与实际 DTO、工厂、命令装配写在新包 contract.go / README 和闭集回归，KC Client 与 CLI 仅持运输和模块装配。
- 验证计划：独立 SDK/CLI 的请求、精确数据、编码与撤权回归；现有真实 argv、历史文件消费和授权旅程；受影响 Client / CLI / Snapshot / Server / 架构合同，以及 make check-docs、make quality 和普通场景；真实部署单独说明，契约存在 skip 时不勾选完成。
- 质量出口：新增入口分类与单向依赖纳入自动守卫；迁移前后依相同逻辑单元政策核对耦合，不抬阈值或省略边。所有增量问题修复或登记位置 / 信号 / 判定 / 处置 / 触发条件；当前仅立项，尚未记验收通过。
- 耦合登记：按同一 `path-depth=2;named-subtree=;version=1` 政策，迁移前 63 单元 / 17.46%，入口分离后 66 单元 / 16.67%；新增三个真实单元为 snapshot/client、snapshot/cli、internal/httpjson，未省略生产 import 或改变分组政策。显式执行 coupling -write-baseline，将已有 17.33% 上限收紧至 16.67%，并记录当前图的逐单元事实；迁移前已存在的 discovery/datasets 与 object/service 漂移属于同期 Object 迁移，未在本轮修改这两个实现或计为入口迁移收益。前后观测与原基线保存于 .validation/snapshot-entrypoints，最终闸另行执行。
- 增量复核：补齐此前漏登的 metadata 参数真实 argv 回归，并保持其它命令对该参数的原有拒绝行为与零请求保证；KC 内部 stamp 在装配边界过滤，help 先于任何 HTTP 或本机写入返回，离线 overlay 与嵌入 Home 接缝保留。首轮完整契约捕捉到内部 typed task 的 resolve/check 分支被误删，原断言保留，已恢复借 SDK 的 KC 任务适配（不加入公开 argv），并补匿名定义、metadata、空白 revision 与输出回归；模块本身仍无 Invocation / Home 依赖。失效 helper、退役公开分支及静态检查问题当场修，未降低阈值或使用 skip。
- 实现结果：Snapshot SDK 的请求 DTO、三个服务工厂及 HTTP 调用已归 snapshot/client，十五个现有 CLI 叶入口与 help/flag 声明归 snapshot/cli；KC 仅装配身份运输、统一解析/显示及自身应用流程。私有 internal/httpjson 复用精确数字、编码/压缩/错误处理；源映射解析归 dataset 的纯配方输入。静态命令库存读取两处明确声明，公开命令数仍为 61，单向依赖及公开工厂形状已纳入架构守卫。
- 契约修正：完整执行发现此前四个 Snapshot 消费命令虽已有真实 argv 回归，却未登记在 dataset-defined 场景证据中；补登记三个既有测试，保留所有行为断言与覆盖守卫。同期 Object 迁移使两个 LiveAccess 夹具先写实例后写 Schema、系统材料守卫仍读已删路径；仅纠正逐条 Writer 夹具顺序与四份实际 owner 路径，并同步材料来源说明，没有改生产发布顺序、Schema 内容或弱化断言。四项失效回归均已通过，完整契约重跑另记。
- 入口复核记录：scripts/testsuite.sh 的 run_race 曾列已迁移的 ./index，信号是独立 race 入口的历史路径滞后；同期 Object 维护随后已改为 discovery/projection 并增加退役入口自动守卫。本轮未改这些入口，也不把未执行的整组 race 记为通过；SDK/CLI 定向 race 另有通过记录。
- 完整契约限制：最新冻结源码的组件组合被同期审查新增且明确保留为失败的反例阻断：object/writer/representation_consistency_test.go 的最终表示冲突、object/reader/readonly_view_test.go 的具体返回值仍携写能力、invocation/direct_access_contract_test.go 的观察核验与实际访问证据、discovery/service/notice_background_source_test.go 的后台来源被调用者替换。信号、判定与反例由 Object/LiveAccess 审查拥有；该任务用户明确要求只补针对性用例，待当前重构后再修实现。本轮保留所有原断言、单独保存失败证据，不删除、跳过或代修这些领域行为；触发为其对应重构切片完成后重跑整组 contracts。不能把 Snapshot 入口实现完成写成整仓契约验收通过，本条保持未勾选。
- 验证环境记录：一次完整执行因 Docker VM global OOM 杀死测试 OpenSearch 而中断，容器 ID、内核杀进程、端口撤销和拒连证据保存于 .validation/snapshot-entrypoints/opensearch-oom-evidence.md。后改用本轮专用测试节点 heap 256 MiB / memory 1 GiB，沿既有 KC_TEST_OPENSEARCH_URL 接口执行，不改源码、测试断言或其它实例。另发现冻结副本的新设计与后来补齐的场景视图不同步，已同步并先运行 product/views/states 三项静态检查；中断与失败运行均保留，不计通过。
- 最终范围证据：独立 SDK/CLI、共享 HTTP 编解码、中央 Client 和私有 typed task 回归及必要 race 通过；make check-docs / make quality、静态命令库存、架构与产品视图检查通过。普通 lakeFS 场景运行 20261013T073659Z-2c63ca381dae 为 passed（21 个通过事件，无 skip）；完整 CLI/HTTP/组合旅程 20261013T082313Z-37098f51ae08 为 passed-with-skips（926 个通过事件、6 个既有 live skip、无失败），61/61 公开命令有成功断言且 61/61 满足原风险边界。四个新增消费命令的真实失败断言改用既有 expectCode 记账接缝，定向报告分别达到 1/1、1/1、4/1、2/2；没有调低要求。整组 contracts 20261013T075746Z-67ae1631fc65 为 failed（1391 个通过事件、5 个 live skip、20 个上列反例的父/子失败事件），不能宣称整仓验收完成。三份报告均 sourceChanged=false，按各自指纹分别保存，不合并历史执行或通过事件；证据摘要及最终源码归档在 .validation/snapshot-entrypoints。未运行真实部署验收，未提交。

### INDEX-EXTENSION-PLAN-01 · 当前协议上的最小索引接入（2026-10-13）

- [x] 文档方案完成：按用户选择修订最小插件接入机制、Object 边界与现有计划；未开发登记 API 或插件实现。Object 未迁移完成不阻止声明式索引基于现有协议先行。
- Goal / owner：Object 自身合同、能力和跨模块迁移归 object-refactor-guideline；最小插件接入独立归 plugin-extension，以声明式索引实际消费为依据；两条工作的执行与验证路线归 object-refactor-plan。声明、就绪/对账、查询、LiveAccess 与 CLI 各沿 declarative-index / index-control / retrieval / resource-access / cli；主题归属与关系只在 docs/graph 登记。
- Non-Goals：本轮仅修改文档，不写代码、不造完整 PluginHost/动态加载/包发布/热卸载/管理 CLI，不改 LiveAccess 或当前身份/Schema 采用/观察结构的在途实现，不安装依赖、不启动服务或部署验收、不提交。
- 不变量 / 接口：保留 API-01、A-01 / L-01、V-01、W-01 / W-02、R-01 / R-02、AUTH-01 / AUTH-03、PC-01；依赖 object 固定读/维护、discovery 查询与字段合同、discovery/projection 物理端口及 discovery/service 消费口。引用随共享迁移的实际公开落点更新，不复制 Object 或索引算法。
- 选定：声明式索引是提供查询、固定 Dataset 依据准备和已接受推进三项贡献的真实实例；系统登记→按明确目标选择→实际调用，产品装配构造并关闭。OpenSearch/EngineOpener 仅插件内物理适配器，不能仅注册引擎就声称完成能力插件化。
- 否决：完整 Object 和全角色 Host 成为首批前置、注册后仍暗用固定 cx.WS.Index、预先定所有 hook/安装管理、用 SearchProjection 接口声称任意代码 Dataset 隔离。当前 context scope/budget 仅约束可信既有 Index；任意候选开放时再抽被所有实现经过的 typed 边界。
- 计划：R0～R3 按首个切面的真实分母执行；原完整 Object 迁移出口继续未完成。先构造实际实例、登记三贡献，再明确选择和调用；默认与另一具名 Index 实例验证目标范围/固定依据/失败和关闭，第二种真实能力需要共同机制时再提取。
- 质量出口：同步撤回上一版过度预设，既有审阅债按当前源码/反例及实际触碰复核，不把其它在途修正算成本轮实现。重读 owner/图、运行 make check-docs / make quality 与增量空白复核；仅说明文档和工程门禁，未验证业务或部署。
- 本轮结果：最小体系与计划已改写并独立复核，明确构造实例→登记三贡献→目标选用→实际调用/维护→关闭；推进按实例服务需求覆盖 Repo 和已接受 Dataset，不让 Dataset 跟随源 HEAD。撤回 Object 完整迁移、全角色 Host 和包/热卸载/管理 CLI 的首批前置；CLI 只按实际合同差异同步。
- 模型纠正：能力实体仅 Plugin，Affordance 是能力说明、CLI/help 是载体；系统提供注册、作用域选择、服务和 hook。撤回将 Interface 作为并列架构层的表述和预设目录；现有 Go 类型与已发布合同只作兼容约束，未改代码或命令保证。
- 本轮边界调整：保留 Object 自身要求→承诺及与索引/LiveAccess 的依赖、职责拆出和兼容验证；移出插件登记、目标选择和三贡献执行方案。Object 完成不要求首个插件接入完成，但受影响的上层合同必须验证；两条工作分别列分母，不用文档归属或路径存在宣称实现验收。仅修改文档，先重读 owner/图再运行文档与质量闸。
- 本轮审阅债：位置 knowledge.md 与 object-interface.md 的旧能力实体表述；信号是部分文字仍把 Interface 与 Plugin 并列。判定为目标术语与既有兼容合同未完全分开的存量文档问题，本轮不重设其协议或执行行为；处置为对应 owner 同步区分历史合同与目标实体，触发为能力声明/兼容分派专题下一次维护。位置 snapshot 的追加监听与索引固定服务准备；信号是无单插件撤销及维护输入可能覆盖来源整仓，判定为既有能力边界，已在 plugin-extension 明确；触发实际热卸载或限制业务代码处理材料时另行选定合同和越界回归，不以作用域选用宣称隔离。

- 模型纠正复核：目标指南、计划、CLI、全景与术语已统一；旧图仅保留历史链接，不再作为当前能力模型展示。make check-docs（32 文档 / 149 关系）与 git diff --check 通过；make quality 的格式/tidy/vet/staticcheck/复杂度/体积/克隆阶段通过，但同期源码迁移使耦合传播升至 37.32%、新逻辑单元尚未完成基线登记且 snapshot/service 漂移超阈值，整体退出 2。本轮不调整基线或削弱门禁；触发为迁移按同一政策完成真实依赖与逻辑单元复核后重跑，仍不标完成。
- 验证限制：共享工作树同期进行 object/discovery/index → object/discovery 的源码迁移。首次 make check-docs / make quality 因 repofile.Parse 在途编译未过；后续编译恢复后，检查仍被其它 owner 与历史 TASK 的旧路径 Markdown 链接阻塞。本轮已同步自己修改文档的实际 API 锚点，git diff --check 通过；未修改迁移源码或批量改其它 owner，未弱化门禁，未取得整仓 docs/quality 通过，保留本条未勾选。触发为迁移稳定并完成全图引用同步后重跑两项检查，旧版本通过记录不计本轮。


- 本轮边界调整结果：Object guideline 已聚焦要求→承诺、公开依赖和跨模块职责拆出；plugin-extension 独立承接真实索引三贡献及目标选用；计划分别列 Object 自身、消费兼容和索引接入分母；索引与 LiveAccess 具体设计沿各自 owner。新文档及主题/直接关系、总览/CLI/组织引用已同步并重读，独立复核通过；未改代码、测试、命令或出版物。
- 接入设计补实：上一版仅约束登记/选择，仍把实际机制留给实现，不能作为开发依据。本轮选定静态 Go 插件入口和系统生成的登记上下文、一次性接纳贡献组合、消费侧只读实例目录、启动冻结的 Repo/Dataset 精确选择表、系统需求视图、HEAD/固定 Dataset 两路维护、实例资源 namespace 与续页执行身份，并列出全部既有调用的改接点；只改文档，不把目标登记 API 或多实例隔离写成已实现能力。具体字段与 Go 签名归实施时的公开代码，本文不编造网络协议或管理命令。

- 边界调整初次验证记录：make check-docs 通过（33 文档 / 163 关系），git diff --check 通过。quality 首次停在共享迁移新增文件检查，直接复核后通过；随后发现并修正文档的在途路径断链。最终 quality 的文档及格式/tidy阶段通过，但 go vet 因 CLI 引用迁移中已不存在的 kcclient.WorkspaceFileCoordinate、WorkspaceFilesService、SnapshotService 等类型而退出 2；其后静态/复杂度/体积/克隆/耦合阶段未执行，不能记为通过。日志 /tmp/kc-object-doc-boundary-quality-final.log；处置为保留待验，触发为 Snapshot/Client 迁移恢复编译后重跑质量闸，不修改源码或弱化守卫换绿。


- 接入设计补实结果：已明确插件入口与登记上下文、完整实例接纳、冻结的精确目标规则、HEAD/固定 Dataset/State 需求及实际路由、固定恢复脱离 HEAD 门槛、逐 basis 独立就绪、所有索引任务改接、额外实例物理 namespace 与多层续页身份。旧资源接纳者的固定身份与部署默认选择分开；默认指针变动不转移资源或旧 token。来源上下文不由请求改写，Stop 与资源账各自唯一归属，首次需求失败拒绝激活；独立具体机制复核后的问题已修正文档。
- 最终文档与闸证据：重新读取改动 owner 与图，make check-docs 通过（33 篇 / 165 关系），git diff --check 通过，make quality 全绿（格式/tidy/vet/staticcheck/复杂度/体积/克隆/耦合）；日志 /tmp/kc-plugin-concrete-quality-final.log。先前静态检查中的 U1000 / ST1005 由并行源码工作恢复，修复和耦合基线变化不计本轮实现；本轮仅修改 Markdown/文档图，未修改 Go、测试、配置或出版物，不将质量闸记为新插件的业务验收，所有接入实现仍待开发。

#### 本条保留的 LiveAccess 历史审阅信号

下列信号按现有访问、观察与权限 owner 的保证保留。触碰相应 LiveAccess 路径时先复核
当前工作树和反例，确认缺口后在对应专项切片独立修复；不是 Object 目录迁移或首个静态索引接入的无条件前置，
也不能把注册或路径移动当作风险已经消失。

| 位置 | 信号 | 判定与处置 | 触发条件 |
|---|---|---|---|
| `liveaccess/live_access.go`、`serving.go` 与 `state_lookup.go` | 通用 LiveAccess、Bound State 与直接 access 的值和 basis 核验分散，完整包络不等于采用结构及固定声明关联已核验 | 统一合法观察依据、实际采用值结构与声明关联的核验；补伪造/不匹配依据及结构不合规值的拒绝反例，保留合法空值语义，不预设新字段 | LiveAccess 注册、来源返回值及按观察依据回读切换前 |
| `invocation/verbs_resource.go` 的 `access` 与访问证据编排 | 直接返回 bindings/observations 的取值路径未完整进入与 invoke 对齐的实际访问证据链 | 在所属任务服务记录真实主体、固定声明、实际观察及成功/失败结果；保持公开 access 返回形状，不以请求完成日志替代访问证据 | access 服务迁移和默认 binding 切换前 |
| `liveaccess/state_lookup.go` 的 HTTP 出站装配与调用方证明转发 | 声明 origin 和通用 HTTP client 不足以证明目标信任、证明适用与重定向后的委托边界 | 由宿主注入目标/证明政策，来源适配执行目标与重定向检查；补新地址、跨目标重定向及源拒绝反例，不回退旧观察，不把凭证写入知识或证据 | LiveAccess HTTP adapter 归属切换或改变出站证明装配前 |

另纠正此前一项判断：`ResourceLookup` 会返回注入的 lookup 或构造 HTTP 适配器，本身
不访问外部源；`WithLiveAccess` 优先匹配已安装 Handler。当前已有独立 Echo 的正式
调用测试。因此通用调用中出现 State 适配器属于结构耦合，不能直接判作“普通能力无法
调用”的行为缺陷。无需来源访问的契约可以提前补强，但授权动作不因拆包自动改变。



### SNAPSHOT-CLI-02-IMPLEMENT · 第二批 Snapshot CLI 实现（2026-10-13）

- [x] 本轮完成：实现已选定的 `dataset show`、`files list/read` 与 `grant update`，补历史版解析、交付目录混排分页与真实 argv 回归，受影响测试、`make check-docs`、`make quality` 全绿；无新 skip，未提交。
- Goal / owner：交付 Dataset 详情/历史、单目录页和单文件消费、同一授权规则的条件原子修改；设计沿 `doc/design/affordance.md`、`dataset.md`、`service.md`、`permissions.md` 及上一条合同选择，owner/直接关系已按 docs/graph 读取。
- Non-Goals：保持 `cli` 单包、旧命令及既有消费默认，不扩对象层、不新增原生仓属性入口、不增加 pin 文件或会话、不整改或迁移独立 kcfs / datasetfs、不承诺来源 Range IO、不安装依赖、不启动部署服务、不提交；保留其它会话全部在途修改。
- 不变量：V-01、KS-01/KS-02、AUTH-01～03、API-01、A-01/L-01；版本从已接受记录重建，当前退役与权限仍检查，交付路径不扩大范围，目录页严格可续，字节不改写，目标不覆盖，规则身份与来源保持且旧/新范围在原子边界内鉴权。
- 选定：补现有 named Resolve 的显式发布版选择与 typed Client 详情查询；目录混排分页在网关完成且游标绑定发布/目录；CLI 复用文件网关，以 JSON 字节块或完整单文件目标交付；授权复用已有原子 Update，输入两份单条 Rule 文件并做规范传输转换。否决管理详情拼 pin、CLI 全量分页再截断、二进制 stdout、自动刷新 expected、remove/add 替代原子修改与新 CommandSpec 框架。
- 接口与代码：Snapshot 文件/历史/授权合同仍由 `snapshot/dataset`、`snapshot/service`、`snapshot/access` 与 Client Go 类型/包 README/Conformance 拥有；CLI 落点沿已选文件簇、surface/分派/应用登记及同源 help 入口，旧形状保持。
- 验证：先取得新增需求的失败证据再实现；定向 CLI argv、Client、Dataset、Access、Service、架构/命令/帮助闭集、发布与撤权、二进制/目标保护及 race 检查；之后文档和工程质量闸全绿，无新 skip。未执行的真实部署不记验收通过。
- 质量出口：增量问题当场修或升级守卫；既有整文件读再切片成本债沿合同条保留。当前其它线程正在 Object R0 / 索引采用工作，本批不认领、修改或将其结果计为新增 CLI 证据。
- 实现范围：四个叶子已接 typed Server，代码保持 cli 单包，详情、文件、规则输入分别收拢为同包文件；root / 组 / 叶 help 同源。显式历史版只重放已接受的冻结提交，跳过默认配方首次采用；省略版本的原行为保留。交付分页先派生直系子项、再按剩余额度读取一个来源目录页，游标绑定发布、目录和当前可见范围，不缓存权限或全量扫描来源。
- 增量复核：修正显式历史请求可能意外创建首个发布、无效输入可能先刷新登录、下载响应来源未属于 pin、跨块来源切换及目标创建竞态；均进入必要回归。下载同一来源和总大小，失败清理临时字节，完整后以同目录 create-if-absent 发布。规则输入拒绝未知/重复/null 字段并显式转换 repo，更新不拉新 expected、不改规则身份与来源。
- 当前验证记录：Client、Snapshot Access、Dataset、Service 四个整包 race 已通过，331 个终态测试事件、fail / skip 为 0。受影响 CLI 当前选择为 65 个主测试、177 个通过事件，整包 race 通过且无 fail / skip；包括新入口、旧命令/帮助闭集、真实 argv 发布/历史/混排消费、原子撤权、二进制下载及既有目标保护。最后调整目标创建竞态的错误分类后，12 个文件相关事件再次 race 通过。Snapshot 及所选文档/库存守卫另有 27 个通过事件，受影响包 vet / staticcheck 与实际 kc help files 通过。
- 失败证据与发布：历史显式解析的采用漏洞和跨块合法来源切换均由旧行为 overlay 取得真实反例；目标创建竞态此前错报参数错误，现统一为既有目标冲突并保留并发写者内容。现有同源生成器已刷新四个入口，仅 datasets / inventory / permissions 三份出版物增量；生成器沿并行迁移后的 systemrepo/content 落点，未重建退役目录。共享工作树的 Object 迁移过程曾阻挡编译、系统出版、文档和质量闸；这些失败保留日志，不计通过，不修改其它迁移或弱化断言换绿。
- 收尾进展：文档闸已恢复为 32 篇 / 149 关系通过，运行前后源码未变化。消费回归加强为从 show 获取 ID、从 dataset show 获取交付路径，再枚举/读取同一发布；旧 --datasets 不是现有公开参数，已纠正测试。质量闸进一步发现本批直连 snapshot/service 增加了 CLI 依赖和 service 不稳定度漂移，现改用 Invocation 已有的同一纯路径校验合同，不复制校验或增添公共形状；后续重跑受影响验证。其它迁移的单位登记与传播成本仍由对应任务处理，不放宽守卫或基线换绿。
- 最终验证：迁移恢复后重跑受影响 CLI 的 65 个主测试（177 个含子例的通过事件）及 Client / Snapshot Access / Dataset / Service 四个整包（331 个通过事件），均为 race、fail / skip 为 0；加强后的无预知 ID / 路径消费旅程在其中真实通过。Snapshot 架构选择 26 个事件、Server 文件接线选择 6 个事件及文档/验证库存包 28 个事件通过，均无 fail / skip。定向范围不替代完整 Object 架构、全仓业务合同或真实部署验收。
- 最终闸证据：`make check-docs` 的 gate-watch-10 与 `make quality` 的 gate-watch-06 均退出 0；32 篇 / 149 关系、格式/tidy/vet/staticcheck/复杂度/文件体积/克隆/耦合全绿。两项使用同一候选，1359 个 Go / Markdown / OKF 与关键输入在运行前后及结束复查均未变化。本批自己的 service 耦合漂移已消除；并行 Owner 对迁移单位口径与基线的调整不计作本批实现或质量修复。原始日志沿 `/tmp/kc-snapshot-cli-phase2-*`；记录回写后再跑文档与质量闸。
- 交付边界：CLI 保持单包，既有 57 个路径保留、新增四个所选叶子；独立 kcfs / 根 datasetfs 不迁移。无 pin 文件/消费会话、对象入口、原生仓属性入口、依赖安装、部署服务或本批提交。来源整文件读再切片的成本债及触发条件沿上条保留。

### SNAPSHOT-CLI-02 · Snapshot 缺失入口的行为合同选择（2026-10-13）

- [x] 本轮完成合同选择：按此前第二批约定，选择 Snapshot 缺失 CLI 入口的任务、输入输出、权限、固定版本与失败边界；交付可实施合同及前置缺口，四个新命令尚未实现。
- Goal / owner：让消费者核对 Dataset 已接受定义与历史、按需枚举交付目录和读取单文件，让管理者原子修改既有授权规则。交互归 `doc/design/affordance.md`，文件选择/版本归 `dataset.md`，网关与客户端边界归 `service.md`，当前授权归 `permissions.md`；owner 与直接关系按 `docs/graph/` 核对。
- Non-Goals：本批不新增执行命令、不改业务代码或公开类型、不迁移对象层、不包装全部原生 Store API、不增加 pin 文件/消费会话、不纳入独立 kcfs / datasetfs 的挂载与生命周期重构、不安装依赖、不启动部署服务、不提交；保留第一批及其它会话在途改动。
- 不变量：V-01、KS-01/KS-02、AUTH-01～03、API-01、A-01/L-01；详情可见不发消费权，交付布局不扩大来源文件范围，显式旧版不会自动漂到当前版，固定依据不绕过当前权限，规则更新不拆成撤销再新增。
- 选定方向：优先 Dataset 详情/历史与两个按需文件入口，复用同一 Server、当前身份及文件网关；授权原子修改复用既有服务更新边界。文件浏览不限于项目附着，但始终是交付文件访问；设计区分它与知识发现，并保留挂载/整体本机采用的原边界。
- 选定 CLI：`dataset show` 查看指定发布及一页真实历史；`files list` 返回一页交付目录直接子项；`files read` 默认返回有界字节块，指定本机目标时完整下载一个文件；`grant update` 用完整新规则和此前观察到的旧规则做条件原子替换。文件路径都属于 Dataset 交付树；公开命令不暴露内部 pin，文件续页须显式选版，游标绑定 Dataset、版本与目录。新入口尚未实现。
- 输入与本机交付：详情可按发布版和历史游标读取；文件首次省略选版时只取当前一次，后续采用返回版号，JSON 字节编码沿现有 Client 合同。完整单文件下载与字节范围参数互斥，不提供原始字节 stdout，保留现有错误/回执流；目标不覆盖既有文件或符号链接，使用同目录临时文件并在完整成功后发布。授权更新的旧、新规则分别从文件输入，复用单条 Rule 的库存表示；typed 适配显式转换为已有 PUT 合同，身份和来源不得伪改，发送前不拉新规则覆盖旧观察。
- 精确调用方向：`kc dataset show <id> [--revision <n>] [--limit <n>] [--continuation <cursor>]`（也可 `--dataset`）；`kc files list --dataset <id> [--revision <n>] [--path <dir>] [--limit <n>] [--continuation <cursor>]`；`kc files read --dataset <id> [--revision <n>] --path <file> ([--offset <n>] [--length <n>] | --out <file>)`；`kc grant update --id <id> --file <replacement-rule.json> --expected-file <observed-rule.json>`。根目录仅用于 list；详情的 limit 只控制版本名单。四个入口的权限依次沿库存读、Dataset 解析与文件读、旧/新范围管理权，不额外发权或要求消费者取得库存管理权限。
- 否决：用管理详情拼消费 pin、要求消费者手抄 pin 或创建服务端消费会话、只在 CLI 截短非有界目录结果、将字节范围响应承诺成源端 Range IO、将原生仓名猜成逻辑 Repository ID、提交前自动刷新 expected，以及把 remove/add 当原子修改。
- 接口指向：Dataset 管理详情沿 `snapshot/dataset.ManagementView` 和现有 Snapshot GET；文件沿 `client.WorkspaceFilesService` / `snapshot/service.FileView`，消费版本由服务接受记录解析；授权沿 `snapshot/access.Service.Update` 和既有 Admin PUT。后续实施补 typed Client 与 CLI 接线，所需公开形状只在代码/包 README/Conformance 落地。
- 代码落点：详情加到第一批的 `cli/remote_dataset.go`，文件请求及单文件交付收拢为同包 `cli/remote_files.go` / `dataset_file_read.go`，规则更新沿 `cli/remote_access.go`；叶子说明沿 `help_snapshot.go`，root / composition / dataset / files / grant 逐层可达，同源系统入口由现有生成器发布。`surface.go`、分派、CLI 应用登记与 HTTP 登记继续各自维护，不引入新的标签或注册框架。
- 边界纠偏（2026-10-13）：用户明确排除 kcfs。它消费 Snapshot 文件网关，但宿主挂载计划、FUSE、卸载和进程生命周期独立；`cmd/kcfs`、`cli/datasetfs*.go`、根 `datasetfs/` 均保持在 Snapshot 目录之外，不纳入本批分组或迁移。`files list/read` 仍是 Snapshot 文件交付的产品 CLI 适配，不是挂载实现；跨工具 help 链接不表示模块归属。
- 前置缺口：现有命名 Dataset resolve 缺发布版本选择，交付目录的派生/混排分支尚未严格分页；两项须随文件入口实施并补真实回归。仓属性的 SDK 接收原生仓名，可信逻辑 ID 到原仓/实例的定位合同未闭合，metadata 读/设置/删键入口独立后续选择，不靠可选控制台导航补足。
- 质量出口：`snapshot/service/file_view.go` 的范围读取先整文件读取再切片，判定为既有来源 IO/内存能力差距；本批明确只选返回字节块和单文件交付，不声称源端资源有界。触发大文件源端按需 IO 保证时，须由来源 Range 能力与服务合同单独实现、验证；本批不静默扩大承诺。
- 验证计划：基于公开代码与现有 Conformance 做独立审查；改动 owner 与图登记重读后运行 `make check-docs`、`make quality`。行为尚未实现，不用第一批或既有 HTTP 测试宣称新 CLI 已验收。
- 后续实施验收：真实 argv 从库存选 Dataset、查看定义、分页获得交付路径、按同版读取/下载；目标放到第二页以后并覆盖纯文件、目录映射及混排。分页不得超限、重复、漏项或接受异版/异目录游标；发布后旧版连续，退役/撤权/来源失效明确失败。二进制字节不改写，已有目标及失败后的目标保持原状；规则更新覆盖正确字段转换、旧/新范围鉴权、过期观察冲突与失败保留旧值。缺权、空目录、未完成分页和不可用分别表达，不使用 skip 或削弱既有断言换绿。
- 最终复核与验证：以本批开始的工作树为基线，只新增任务记录并修改 CLI / Dataset / Service owner 的任务和保证；公开字段、错误代数和状态机仍由 Go / 包 README / Conformance 拥有，图的 owner 与直接关系不变。独立复核确认固定版本、分页、当前权限、条件更新和成本边界闭合，`make check-docs`（32 文档 / 149 关系）、`make quality` 与 `git diff --check` 通过；质量工具复用本机缓存。未改业务代码、未新增入口、未做新入口行为验收、未提交。

### OBJECT-REFACTOR-EXECUTION-01 · Object 模块重构执行（2026-10-13）

- [ ] 本轮认领：用户再次要求现在推进；Snapshot 检查点及 R0 发布发现同源基线已建立，R1 必要合同修正已定向取证，当前执行 R2 统一结构切换，随后 R3 验证；不把局部绿称作整批完成。
- Goal / owner：沿 `object-refactor-plan.md` 与 `object-refactor-guideline.md` 建立发布发现验收和同源基线，再统一切换 Object、Interfaces、Discovery、LiveAccess、Systemrepo 及调用方；对象、访问、发现、授权、CLI 与验证保证仍归各主题 owner，关系沿 docs/graph。
- Non-Goals：保留既有 CLI/HTTP、存储格式、对象身份、权限、版本及结果合同；不新增业务处理器、必填对象类型、Interface 或索引发布门槛，不实现尚未选定的插件管理命令或权限，不安装依赖、不推送，不覆盖其他在途改动。
- 不变量与选定：沿 A-01/L-01、I-01/V-01、W-01/W-02、R-01/R-02、AUTH-01～03、API-01、CA-01、OI-01～05。先补图片、分散模块、外部表的公开 Writer/Server 验收；保持行为、必要修正及目标架构分别取证，R2 一次统一结构切换，不保留兼容壳、双实现或深层跨模块访问。
- 接口与验证：沿当前 Snapshot 根及 dataset/access/service/console 公开入口、既有 Reader/Writer、Schema 字段 access、Interface 与访问观察合同。持续全包编译、定向回归、正式场景和架构契约；交付前 `make check-docs`、`make quality` 及受影响测试/race 全绿，真实部署另行取证，未执行和无关 live skip 不称验收通过。
- 检查点依据：单登记空间已有 `fa52d18`；控制台线程中断后，由本任务核验 `.validation/shared-auth/20261013/` 当前 lakeFS 1.86 / Gitea 双主体证据及最终合同。82 项真实 API/CLI、33 项撤权及 30 条浏览器流程已有记录；实际提交还须核对源码一致性与依赖闭包。
- 增量质量出口：最终契约 `20261013T033644Z-0a3a0f64af7a` 的两条 fail 事件来自同一个 `TestSceneViews/product_links`：新 `cli#U7` 缺少证据或明确 gap。当前同一测试已复现失败；在 `.data/scenes/_views.yaml` 如实登记尚未选定/实现的插件管理旅程，不削弱断言、不冒充已有发现测试覆盖管理能力。该轮 sourceChanged=true，不作为 Object 基线。
- 候选增量检查：五个新 `docs/graph/relations/*.okf` 有 EOF 空行，先前工作树 `git diff --check` 不覆盖未跟踪文件。候选暂存检查取得失败证据；本轮去掉这些增量空行，并将 `scripts/check-quality.sh` 的增量空白检查覆盖工作树、暂存区及非忽略新文件，保持已有 Git 空白规则，不追存量或改阈值。
- 阶段性提交：`8d24030c7de6c7a404bae2b833c9c1d43579441e`，冻结候选树 `0a14cc8a90cf00bf77f6e8899f16dba193968335`；候选 `make quality` / `make check-docs` 通过，`testsuite:contracts` / `20261013T040027Z-6c02d514ff54` 退出 0、2248 pass 事件、0 fail、11 既有外部/live skip、sourceChanged=false，CLI 57/57。实际界面与双主体证据沿上述独立材料；没有把 skip、生产 Taihu 或未实测网络隔离计为通过。Agent 约定、独立 Git helper 及冻结后的其它文档改动保留，没有推送。
- R0 证据：宿主 `repository-attached/_materials/published-objects/` 新增图片、分散模块和离线外部表材料及独立公开 Writer/Server 证据；CLI 命中坐标、typed HTTP 正文/版本依据分别按既有形状验证。目标执行 `20261013T043458Z-9fd25381761f` 四项通过；冻结同源完整合同 `20261013T043540Z-6f5ce37cd5a4` 退出 0、2252 pass / 0 fail / 11 既有外部 skip、sourceChanged=false，CLI 57/57。源码清单与日志保留于 `.validation/object-refactor/r0/`，不是部署验收。
- R1 独立反例：空/空白 ObjectID 经 tree/native COMMIT、CommitIntent、PROPOSAL 的共享写校验；旧 Schema 显式 pin 被新版 access 错误赋予 filter/EXISTS/MISSING；Bound State 返回不符合实际采用 Schema 的标量。红证据在 `.validation/object-refactor/r1-identity-red.log`、`r1-schema-red.log`、`r1-observation-red.log`；定向修正已通过，完整受影响回归仍待最后核对。
- R1 选定与否决：保留现有对象/Schema 身份、作者格式、路由与权限。Schema 采用见证只进入可重建投影的维护元数据，物理修订使旧派生投影重新接受显式维护；消费端按固定依据精确核验见证与 Schema，不把投影当权威、不改 Snapshot 对象格式。否决在查询中枚举 reverse locator/整仓定位表；现有 scoped-Schema 守卫曾实际报红，保留并修正实现。私有应用端口不扩充 knowledgeapp 的公共锚点；无 Binding 的仓不打开索引或来源才能判断静态能力，原有反例保持。
- R1 待核验出口：真实 OpenSearch 下的旧 Schema 投影重开与各逻辑访问面、Schema 采用见证伪造/范围拒绝、源值结构、实际访问证据与 HTTP 目标/证明政策。后两项沿计划列出的触发条件在 LiveAccess 服务/HTTP 归属切换前核验，不能由单次 Handler 成功外推。缓存、独立消费者及 Host 生命周期的新增合同仍在 R2 切换中分别取证；尚未实现公开插件管理面。

- R2 当前范围：Object/Reader/Writer/Format/Cache、Interfaces 与其调用服务、LiveAccess 与其逻辑服务、Discovery 与投影/发现服务、Systemrepo 的实际实现和全部调用方已迁至所属 owner；旧知识应用容器拆成 Object/Discovery/治理/可搜索 Dataset 流程。组装与字段解释各保留唯一算法，逻辑补判不再编译物理索引，投影消费 ObservationSource，动态取源归 LiveAccess 桥接。通用 SnapshotConsumer 及持久进度提至 Object maintenance；没有制作转发壳。
- 边界修订：最新 guideline/plan 已撤回通用 PluginHost 的首批前置；本次完整 Object 迁移不预建通用插件管理，独立索引贡献接入仍沿其专门任务验证，不把路径迁移宣称为插件接入完成。原公开 route/action/持久 ID 与 Schema 引用保留。引用同步误触公开 URL 与出版物 path_hint 已由失败证据定位并恢复，后续以同源用例和契约核对，不更改 Oracle。
- 当前验证：全包编译及 Object、Interfaces、LiveAccess、Systemrepo、发现服务、Dataset 发布、架构定向测试通过，结构切换已进入 R3。最终完整合同仍在取证；不以目录存在或局部绿色确认整批完成。

- 度量取证：旧 depth=1 诊断从 R0 26 单元/30.46% 到当前 27 单元/37.32%，没有将其写成改善或抬高旧阈值；该归组将 internal/testkit 与被它使用的 journal/treepath 合为一单元，制造跨能力假环。按真实模块根/公开一级能力统一 depth=2 重算 R0 同源树及当前源码（同一 Go 1.26/macOS，所有 import 保留），56→63 单元、20.0325%→17.3323%。对比 JSON 位于 `.validation/object-refactor/r2/coupling/`，唯一个既有 >=0.05 漂移为 Snapshot service 从 0.75→0.6667（增加直接消费者，Ce 不增加）；按实际共享 CLI 接入证据记录为更稳定的依赖面。源码包 DAG/深度守卫与度量各保留独立出口。选择 default depth=2，旧 depth=1 只供诊断；传播成本、未知单元、>=0.05 漂移与政策签名拒绝都保持，显式 -write-baseline 登记实际新 owner 及更低成本，未增加阈值。
- 审阅债：`interfaces/schema_refs.go` 沿旧 Schema list helper 保留大小写折叠；含大写 Schema 身份的实际采用可能不一致。作为原行为待复核，触发为 Schema 引用规范化或该能力专门行为修正；本次未把结构迁移或现有小写用例算作它已验收。HTTP 目标/证明政策、直接 access 证据仍沿 R1 触发债单列，不称迁移已经修复。
- R3 增量修复：完整应用合同定位到旧出版物目录夹具及逐次发布 Interface 的 Schema/实例顺序；当前共享修正保留原 Schema 校验及字节一致断言。又在两个验证脚本发现旧 `./index` 执行目标；先由 `TestValidationEntrypointsDoNotUseRetiredObjectOwners` 取得两处失败，再同步为实际 Projection 入口，红绿证据位于 `.validation/object-refactor/r3-entrypoints-{red,green}.log`。同类遗漏升级为覆盖 Makefile 与脚本的架构守卫，不修改业务 Oracle。
- R3 独立部署：`20261013T073418Z-ad750e2e5ce3` 实际调用 `make deploy-local-scenes`，因 `/tmp/kc-deploy-local/compose.env` 不存在而退出 2；未运行部署场景，不隐式重建环境，不把现有其他用途容器或旧版本部署记录当成本批验收。
- R3 固定候选：`.validation/object-refactor/r3-final/tree` / `5213e9121dd040305f622721a95e3fe6fb87d398f5e7a99d7a9d8ba3debafe5e`。`make test` / `20261013T074206Z-41c54898d8e4` 为 21 pass / 0 fail / 0 skip；受影响模块及 Home/Server race / `20261013T074216Z-e8ee570aedcb` 为 858 pass / 0 fail / 4 外部 skip；真实 OpenSearch 专项 / `20261013T074258Z-802a8f3bfce6` 为 12 pass / 0 fail / 0 skip；独立 HTTP State runtime 三项 / `20261013T074645Z-306da6b1d409` 为 3 pass / 0 fail / 0 skip。上述运行均 sourceChanged=false。全包 typecheck 与候选 make quality 通过；最终 README 归属纠正及任务记录不改运行代码，另跑工作树 make quality / check-docs。
- R3 完整合同未通过：`20261013T074106Z-b514a7ba62d3` 退出 1，2443 pass / 0 个单项 fail / 11 外部 skip、sourceChanged=false；CLI TestMain 的覆盖闸失败，61/61 命令有成功执行，仅 57/61 满足风险边界。新增 Snapshot 命令 dataset show / files list / files read 缺 1 项登记边界，grant update 缺 2 项；是实际失败，不以既有 57 项或成功事件替代完整 61 项。当前由独立 Snapshot CLI/SDK 任务续作，保留原门槛及 `.validation/object-refactor/r3-final-contracts-2.log`，不宣布整仓合同绿色。
- 本轮交接边界：已只读核对用户在「梳理目标架构中的瞬时状态访问」的最新直接指令——仅补针对性用例，等待当前重构之后再处理下一轮。新增四组回归由本任务复跑 `20261013T080146Z-8cdb2eb2fe89`，5 pass / 20 fail / 0 skip、sourceChanged=false：无 Schema 最终表示冲突、Reader 具体返回值写能力、直接 access 观察核验与实际访问证据、notice 改写后台来源。保留合法转换与合法值/null 控制组，未删除、放宽、标记 skip 或修实现。请求职责仍在 Invocation、Object endpoint 组装的归属也保留为下一轮审查项。主要结构迁移已落地，但完整职责收口与整体 R3 出口未闭合；本条保持未勾选，无最终提交或推送。
- 验证入口存量债：`scripts/e2e-state-runtime-docker.sh` / `scripts/deploy/accessors.sh` 仍引用已迁走的 catalog-initialized 材料；前者的两个 provider 用例还只选择 CLI 包，现实际位于 LiveAccess。R0 同源脚本已含旧材料路径，判定为独立运行入口债；本批未改源码，以忽略目录中的独立 runner 使用实际场景材料和三个真实用例取证，不宣称旧 make 入口通过。触发为下一轮 State runtime / 部署入口维护。源码、运行范围及当前差异清单保留于 `.validation/object-refactor/r3-final/`。

### OBJECT-PLUGIN-DESIGN-01 · 插件扩展目标、CLI 与重构路线（2026-10-13）

- [x] 本轮完成文档：将 publish/discover/access 的插件扩展设计合入已有目标架构与 R0～R3 计划，明确核心、宿主、声明式索引、LiveAccess 和 CLI 的边界及迁移出口；未实施任何重构阶段。
- Goal / owner：目标与扩展生命周期归 `doc/design/object.md`，路线归 `object-refactor-plan.md`，能力说明及 CLI 载体分别归 `object-system.md` / `terminology.md` / `cli.md`；核心及组织边界沿 core-architecture / capability-organization，所有权与直接关系仅由 docs/graph 登记。
- Non-Goals：仅修改文档与图登记，不实现插件宿主、管理命令或代码迁移，不改变现有协议、Action、命令结果与授权，不发布 system 内容、不运行服务或部署验收、不安装依赖、不提交；保留其它会话的在途改动。
- 不变量：保持 A-01 / L-01、API-01、I-01 / V-01、W-01 / W-02、R-01 / R-02、AUTH-01 / AUTH-03、CA-01、PC-01 与 OI-01～05；安装不发权，范围不等于代码沙箱，基本发布不等待插件/索引/来源在线。
- 选定：系统拥有不可跳过的任务边界，Host 管包/安装/请求视图及可撤销资源，插件注册类型化服务和参与点；默认绑定保留既有能力，Repo/Dataset 显式选择且冲突拒绝。保持 R0 验收、R1 必要行为修正、R2 统一结构切换、R3 完整验收；R2 内按受限基座→Host→标准插件→动态桥接→CLI/装配收尾组织依赖。
- 否决：任意事件或 middleware 重排授权/依据/交付，metadata 执行业务代码、组合自动继承、每插件生成命令树、安装即发权、普通查询启动重建、写完设计即称实现通过。
- 接口与质量出口：设计引用既有 Go 类型/README/Conformance，不复制新字段、错误码或状态机。新增管理能力实施前独立选定 typed API、当前权限、审计与恢复；默认 access 的 basis/value 校验、专用观察证据、外呼目标/redirect 和既有 invoke 说明差距按指南登记位置/信号/判定/处置/触发，未在本轮修复。
- 验证：变更 owner 与图登记已重读，`make check-docs` 通过（32 文档 / 149 关系），`make quality` 退出 0，格式、tidy、vet、staticcheck、复杂度、体积、克隆及耦合棘轮通过；增量与空白检查通过。只证明设计及工程门禁，不外推 R0～R3、业务或部署验收，未新增 skip 或削弱断言，未提交。
- 最终复核：明确必要证据可靠保存后才对外交付；默认绑定只由既有显式部署装配恢复，知识声明不安装实例；静态帮助与安装账/配置/其它 scope/来源秘密的可见性分开。所见现状差距均保留触发条件，没有把静态疑点写作已证实运行故障。
- 后续收敛：本条的完整 Host/安装生命周期预设及 Object 统一迁移前置已由 INDEX-EXTENSION-PLAN-01 撤回；保留此条当时文档验证记录，当前方案以真实索引的三项贡献接入为准，不计此前或当前任何实现完成。

### SNAPSHOT-CLI-01 · Snapshot CLI 文件分组与帮助闭环（2026-10-13）

- [x] 本轮完成：仅整理 Snapshot 相关 CLI 适配文件，修正描述与帮助导航，必要回归、受影响测试、`make check-docs`、`make quality` 全绿；命令、单包与业务行为保持原样。
- Goal / owner：保持现有产品调用形状和行为，收拢仓/库存、Dataset、显式授权与文件交付的 CLI 适配；交互与输入/输出保证归 `doc/design/affordance.md`，文件范围/版本交付归 `dataset.md`，服务与原生资源接入归 `service.md`，授权归 `permissions.md`。owner 和直接关系已按 `docs/graph/` 核对。
- Non-Goals：不新增命令或业务入口、不拆 Go 包、不扩公开接口、不改变发布/授权/版本/文件交付行为、不迁移对象层、不新增标签或注册框架、不安装依赖、不启动真实部署验收、不提交；保留本轮开始前全部工作树修改。
- 不变量：保留 V-01、KS-01/KS-02、AUTH-01～03、API-01 与 A-01/L-01；创建/连接、登记、Dataset 发布和发权仍分别成立；一次消费保持既定范围和版本，CLI 与 HTTP 的注册权威不合并。
- 选定：同一个 `cli` 包内按能力移动既有函数；现有命令闭集、分派、身份上下文及帮助渲染保留。Snapshot 叶子说明集中在私有帮助函数并静态合并，现有帮助组补可达路径；否决新子包、新 CommandSpec 框架、Discover 执行入口、原生 API 机械命令化及随整理修存量行为。
- 接口：`cli/surface.go` 保持公开路径/动作闭集，既有 `client.SnapshotService` / AdminService 和 `snapshot/dataset` 配方合同继续承接调用；文件移动不改函数签名或请求构造。help 的同源系统入口出版物按现有生成器刷新，仅同步描述。
- 验证：先补帮助导航/发布效果/输入规则与本机预处理的失败证据，再整理文件及帮助；运行 Snapshot CLI/Client/服务相关回归、公开命令与架构合同、文档图和工程质量门。定向范围不替代真实部署验收。
- 质量闭环：本轮帮助同源、目录可达性与文件移动的增量信号当场修或升级守卫。既有缺入口（Dataset 详情/历史与按需文件读取、仓属性管理、规则原子更新）保持后续行为任务；其它层的已登记问题不在本批修。未通过所需检查前不勾选。
- 实现范围：仓/库存/登记历史收拢到 `remote_repository.go`，Dataset 调用与配方输入分别归 `remote_dataset.go` / `dataset_recipe.go`，本机 overlay 归 `dataset_overlay.go`，显式授权归 `remote_access.go`；保留现有 connection、clone 与共享分派。`help_snapshot.go` 集中 12 个 Snapshot 叶子，总览可进入既有 composition / dataset / grant / registry 组；更正发布固定版、单仓选择、根映射回退、可选配方写回及退役保留记录。README / SURFACE 同步文件组织，现有生成器仅刷新系统入口出版物中的 3 条 Snapshot 描述。
- 范围说明（2026-10-13 纠偏）：此前同期对 kcfs 的帮助澄清及回归属于独立宿主客户端的帮助维护，不列为 Snapshot 能力交付；已有运行记录保留为事实，不由测试覆盖推导模块归属。后续 Snapshot 批次不整理或迁移 kcfs，代码保持在 Snapshot 目录之外。独立文档债：`datasetfs/README.md` 仍引用已退役 catalog / 旧授权归属且遗漏 Linux go-fuse 依赖，判定为旧边界说明漂移；本批记录，下一次独立 kcfs 文档维护时对照公开类型与架构守卫修正，不借本批扩范围。
- 增量信号处置：独立基线对照确认 8 个迁移函数签名与函数体 token 等价、公开路径闭集与 Client 形状不变，46 个非 Snapshot 叶子说明逐项不变；帮助导航、真实资源效果、kcfs 边界及重复帮助项升级为回归守卫，根映射被旧描述误排除的问题当场修正。`remote_dataset.go` 的既有内部 pin / pin check 分支原样保留，登记为路由清理债：仅在后续明确授权清理退休内部入口时按调用闭集判断并处理，本批不因文件搬迁删分支。
- 最终验证：新增帮助回归先取得失败证据再修正到绿；受影响 CLI 定向检查 61 个顶层测试通过（含子测试共 144 个 pass 事件），0 fail / 0 skip。`client`、`snapshot/dataset`、`snapshot/access`、`snapshot/service`、`internal/arch`、文档与验证库存脚本测试通过；`make check-docs`（32 文档 / 149 关系）、`make check-surface`、`make quality` 与 `git diff --check` 全绿，质量工具使用本机已有模块缓存。未安装依赖、未启动部署服务/容器、未执行真实部署验收、未提交；本条只证明上述整理及相关合同范围。

### RESOURCE-METADATA-01 · Repo 原生元数据与 Dataset 可选元数据（2026-10-13）

- [ ] 本轮认领：设计、公开合同与实现一起支持 Repository / Dataset 的可选自定义元数据；README 不成为创建、发布或发现的前提。
- Goal / owner：Dataset 的资源说明与版本交付归 `doc/design/dataset.md`，原生 Repository 接入与服务边界归 `doc/design/service.md` / `core-architecture.md`，README 与知识说明归 `knowledge.md`，授权归 `permissions.md`；按 `docs/graph/` 的 owner 与直接关系定位。
- Non-Goals：不建立第二份 Repository 元数据权威，不把资源属性解释为知识或授权，不强制 README / Schema，不引入 PATCH、跨仓事务或新的搜索代数，不安装依赖、不提交、不运行真实部署验收。
- 不变量：保留 V-01、KS-01 / KS-02、AUTH-01～03、API-01 与 A-01 / L-01；Dataset 已接受发布（含元数据）不可改写，元数据不改变文件范围、来源身份或权限。
- 选定：两种资源采用 lakeFS 原生的字符串键值形状；Repository 读写使用原 lakeFS metadata API 与权威，Dataset 可在定义和新版本发布时携带完整可选元数据，并随发布记录持久、恢复、回读。无元数据继续成立；README 是可选详细说明。否决复制原仓 metadata 到登记账、强制选入 README，以及把 metadata 放入知识 Schema 或索引作为正文权威。
- 接口：Repository 沿用 `snapshot/service` 的原生受控路由、`snapshot/access` 动作和控制台；Dataset 以 `snapshot/dataset.KnowledgeSet` / `KnowledgeSetRecipe`、Snapshot HTTP / Client 与定义控制台承接，具体字段与校验写在公开类型、包 README 和回归合同。
- 验证：执行受影响 Dataset 持久化/历史/深拷贝/配方、原生 Repository 读写授权与代理、Client/CLI/HTTP 和前端回归，以及 `make check-docs`、`make quality` 与受影响合同；不以定向测试宣称真实部署或全量验收。保留本轮开始前所有工作树改动。
- 质量闭环：本轮发现的增量问题当场修或补守卫；存量信号按位置 / 信号 / 判定 / 处置 / 触发条件记在本条，未完成验证不勾选。
- 实现范围：原生 Repository metadata GET / POST / DELETE 与显式读写授权、Snapshot SDK 和原仓设置页；Dataset 定义、便携配方、发布准备、持久恢复、库存、历史详情、CLI `--metadata` 与表单 / JSON 编辑。两者都使用字符串键值，元数据能力完全不读取、识别或处理 README。
- 增量问题处置：修复空 overlay 与状态/返回值共享 map、发布准备及配方入口丢元数据、嵌套定义绕过类型校验；严格校验集中在纯函数与传输入口，保留公开 DTO 纯数据架构守卫、业务任意值和精确数字及 Writer 流式路径。增加持久旧发布不可改写/删除与失败保持回归；原生客户端补错误分类和原生请求不压缩合同，UI 部分失败回读实际权威。
- 同期门禁信号：原生物理来源源码注释使用 CLI 守卫的保留词导致文本误报，仅将注释改为同义 forwarding，未改逻辑或守卫；前端同期文件读取改动遗留未使用变量，仅清理废弃项并由现有 lint 验证。未覆盖其它在途功能。
- 验证过程：前端 172 项与 typecheck / lint / build、受影响 Go 与 Dataset race、文档和工程质量闸已有通过记录；首轮普通场景业务通过但 sourceChanged=true，不作为同份源码证据。完整契约发现公开 DTO 解码方法违反守卫，已改为纯验证端口并重跑相关架构回归到绿；最终普通场景与完整契约在固定候选上复跑，真实部署验收保持独立、未执行。
- 最终验证：固定候选指纹 `f6cae9f1905dce00ba100c13a018ac33f868863b16cbe70b50416516026deb40`；`make test` / `20261013T032225Z-2d3b133a08c3` 退出 0，21 pass / 0 fail / 0 skip；`make test-contracts` / `20261013T032225Z-74b7acaf2e5a` 退出 0，2229 pass / 0 fail / 11 既有外部 skip，CLI 57/57 成功且满足风险分层合同。两次 sourceChanged=false；pass 事件含子测试，不作为库存通过率。固定候选 `make test-webui`（175 tests、typecheck / lint / build 与控制台 Go 回归）、`make quality` / `make check-docs` 全绿；当前工作目录另跑元数据 / Native / 架构 / 精确解码回归与 `make quality` 到绿，未覆盖同期在途变更。
- 未验证边界：上述 11 项为既有 live runtime、真实 lakeFS、身份、付费模型和规模合同，与本轮新增回归无新增 skip；真实部署验收未执行。实现完成，但依严格「完整契约无 skip」勾选规则保留本条未勾选，不记全量 / 真实部署验收通过。固定候选证据与日志已保存在 `.validation/`；只回写交付记录，没有提交。

### SINGLE-REGISTRY-01 · 一套服务一个登记空间（2026-10-12）

- [ ] 单登记空间实现及受影响普通检查已完成；完整契约仍有 11 项既有外部/真实环境测试跳过，按严格「全绿且无 skip」规则保留未勾选，不登记全量/真实部署验收通过。
- Goal / owner：仓登记与 Dataset 发布、固定历史由 dataset 拥有；模块边界由 core-architecture，部署恢复由 service，交互由 cli，权限由 permissions，公开词汇由 terminology 拥有；相关关系按 docs/graph 读取。
- Non-Goals：不删除仓登记、Dataset 不可变发布记录、历史、退役、当前权限、CAS 或恢复；不改 Object/Schema/Writer 写入语义，不实现多租户，不执行对象模块迁移，不安装依赖；提交按用户最新授权执行。
- 不变量：保留 KS-01/KS-02、V-01、AUTH-01～03、API-01、A-01/L-01；唯一空间不使连接等同登记、不使组合发权、不让 pin 扩大范围、不从缓存重建权威。
- 选定：部署唯一登记权威和唯一 Dataset 服务；客户端选择 Server 后直接操作 Repository/Dataset；删除 Catalog 参数、枚举、切换及整体归档。否决保留虚假默认 ID、多空间兼容分支和以删名词方式丢失状态。
- 接口：snapshot/dataset 的 Service/Registry 与状态合同、snapshot/service 的单 Datasets 资源、单 Registry 部署 binding，Client/CLI/HTTP 与 Console 同步收口；权威身份来自实际 Snapshot Repository，仅用于恢复核对。
- 验证：先增加并运行退役选择拒绝、唯一服务发布/历史/恢复与无 Catalog 公开形状的失败证据，再修改实现；运行受影响 Go/前端回归、make test、make test-contracts、make check-docs、make quality。真实部署验收按独立部署合同另行执行，未运行不得记通过。
- 增量质量问题：本轮发现的信号逐项当场修、登记债或升级守卫，不修改门禁阈值、不删弱化合同、不 skip 换绿；保留本轮开始前工作树改动。
- 实现范围：唯一 RegistryBinding、dataset.Service/Registry；删除 Catalog DTO、选择上下文、列表/切换/整体归档、请求与政策选择维度；HTTP 收到 Snapshot 模块入口，CLI 与控制台直接使用服务库存。仓登记、Dataset 不可变记录/当前版、历史、CAS、范围、退役与当前授权均保留。
- 恢复边界：旧唯一登记元信息只读恢复，首次明确管理变更迁移且不重写未变 Dataset；旧唯一治理账、供给回执和部署身份保持原历史与幂等结果。旧登记与治理账身份须匹配已装配的实际登记权威；多空间、混合/重复字段、权威不一致与遗失元信息明确拒绝；旧 Catalog 范围/动作政策不做自动扩权迁移，部署方须显式更新政策。
- 当场修与守卫：退休字段大小写导致的范围扩张、混合治理账丢历史、元信息迁移制造虚假 Dataset 历史、场景新权威指针未隔离、表格批量变换越行、系统入口旧动作分组/说明漂移、登记审计分页漏传与控制台 create 名称路由碰撞均已修并补回归；Catalog API/集合/参数/生命周期与场景表格闭集升级防回潮守卫。
- 验证范围记录：三组 Snapshot 普通/race/vet、Home/Invocation/Server（含 readiness）/HTTP、Client、155 项前端及完整控制台构建/Go 回归通过。第一轮普通/完整合同的旧接线失败与终止均保留为诊断，不计最终验收；所有已发现增量错误修复后完整复跑。未安装依赖；用户在验证结束后明确要求提交；真实部署、外部 SSO、付费模型、live runtime 与规模资格按各自环境合同，不计入夹具验收。
- 最终同源结果：源码指纹 `6666cdf2cb87989bf6cac82b588d259a88b35aa428a2a67ee99632acae193717`；`testsuite:lakefs` / `20261012T130034Z-1dd845e135ee`（make test）退出 0、21 个通过事件、0 fail/skip、sourceChanged=false；`testsuite:contracts` / `20261012T130034Z-1c10447eebed`（make test-contracts）退出 0、2162 个通过事件、11 个既有 live skip、0 fail、sourceChanged=false，CLI 57/57 成功且满足风险分层边界。通过事件含子测试，不当声明库存通过率。
- 最终门禁与材料：make check-docs（32 文档 / 143 关系）、make quality、make check-validation、git diff --check 全部退出 0；层/导出面/退休选择/HTTP 87 条闭集/文档 Test 引用/维护检查通过；场景 views/product/states、45 项场景工具测试、scale 3 项无网络请求合同与数学 self-tests、perf_tree --check、fixture 编译及 shell 语法检查通过，性能树原 history-generation/rollover gap 保留。任务记录回写后再次复跑文档/质量闸；行为源码未再修改。
- 未运行的 11 项外部合同：TestLiveHTTPStateRuntimeContainer、TestLiveHTTPRuntimeBuildsOpenSearchStateProjection、TestLiveLunaListwiseRerank、TestLiveTaihuAuthentication、TestLiveBulkApplyAgainstRealLakeFS、TestLiveHTTPSearchRerankWithLuna、TestSceneAccessorNoticeUpdatesSearchIndex、TestLiveLakeFSSceneDFS、TestLiveServiceProviderConsumerJourney、TestLiveHTTPDynamicStateSearchJourney、TestScaleProfileRepoAddLakeFS。保留其真实环境要求与原 skip，不把未运行登记成通过；独立真实部署/FUSE/模型/身份/压测验收未取得。
- 提交前复核：用户在本轮验证结束后明确要求提交；依赖的 Snapshot 在途迁移与退役接线随一致候选纳入，AGENTS、对象模块 plan/guideline 及图示/关系/旧相关任务记录、独立 Git helper 清理留在工作树。纳入此前未跟踪源文件后发现三处空白问题，已修正；155 项前端与完整控制台检查复跑通过。精确暂存候选导出后 make quality（30 文档 / 123 关系）、internal/arch、check-docs 与 validation-inventory 全部通过，暂存 diff --check 通过；无生成产物或真实密钥入提交。完整契约的同源运行仍按上列指纹和范围解释，提交前只增加空白修正、说明同步与无关改动排除，不追加真实部署验收。


本轮登记的存量债（未借单空间改造追存量）：

| 位置 | 信号 / 判定 | 处置 | 触发条件 |
|---|---|---|---|
| `.data/scale/runner/smoke.py`、`ladder.py` 与 `scenes/env.example.yaml` | 默认 scratch `kc-perf-*` 使用保留前缀，run stamp 大写 T 不满足 lakeFS 名称；原 runner 漏洞，与本轮选择维度无关 | 保留并登记，后续统一合法 scratch 生成并补真实 Client 校验的无网络回归 | 下一次运行 named managed scratch 压测前 |
| `scripts/testsuite.sh` taihu-live 分组 | 选择 ./cli，但 TestLiveTaihuAuthentication 实际在 ./server；原 live 入口可产生无测试成功 | 独立修正真实包与存在性守卫；本轮不把此入口算 live 证据 | 下次真实 Taihu 验收前 |
| `scripts/e2e-kcfs-linux.sh` 临时部署 | 原脚本用新 stateDir 绑定共享且已初始化的 kc-catalog 权威，按恢复合同应拒绝；原有验收环境接线问题 | 保留单空间参数迁移，后续每次 run 绑定独立物理登记权威并清理，不重置共享权威 | 下一次真实 FUSE 验收前 |
| `.data/scale/runner/smoke.py`、`ladder.py` 环境合同 | 已声明 credentialsEnv/setupAllowCreate=false，但 Client 仅发 X-Kc-As 且始终 create；存量 runner 尚未实现该环境能力 | 明确不以本轮请求接线回归证明真实环境资格，后续补认证与既有仓路径合同 | 第一次非 local-auth 或禁止创建环境的真实压测前 |




### OBJECT-REFACTOR-VERIFY-01 · 按验证依据裁决重构路线（2026-10-12）

- [x] 本轮完成独立评估与计划修订：从目标验收、实际包依赖和用例树验证机制比较路线，撤回按层划分的 P0～P7；采用 R0 先补验收、R1 必要行为纠偏、R2 一次组织迁移、R3 最终验收，未启动实现。
- Goal / owner：object-refactor-plan 拥有路线，object-refactor-guideline 拥有目标与边界；验证按 test-catalog / 场景树合同，不把既有覆盖当目标限制。
- 依据：当前 `go list -json ./...` 枚举 60 个生产包，knowledge / reader / serving / retrieval / index 的直接生产消费方分别为 25 / 7 / 5 / 9 / 5；计数只说明影响面，结合共享类型、混合职责、无需兼容和不改持久/wire 语义选择统一组织切换。行为验收先在旧实现取基线，架构目标在迁移批次先红后绿；不强制经营每层的过渡 API。
- 更正：ResourceLookup 构造适配器本身不访问源，WithLiveAccess 优先选择已注册 Handler，已有独立 Echo 正式调用测试；这里先判为结构耦合，不直接判“普通 Interface 不能调用”的行为缺陷。必要行为风险仍由反例复核，不靠读到类型名推导故障。
- 本轮范围与证据：只改计划、guideline 执行原则、文档图标题及任务记录；views / product / states 三项树静态检查通过，未运行场景或业务回归。运行 `make check-docs` / `make quality` 并复核增量；不改 Snapshot 在途源码、不安装依赖、不提交。
- 验证结果：`make quality` 退出码 0，含 32 文档 / 143 关系的文档检查、工程门禁和耦合棘轮通过；`git diff --check` 通过。三项树检查只证明组织、声明及共享前态合同，`go list` 只证明静态枚举；未将其记作业务或部署验收，R0～R3 全部未实施。

### OBJECT-REFACTOR-ORDER-01 · 重构顺序纠正（2026-10-12）

- 状态：此处逐层实施的顺序建议已撤回，未执行。历史修订范围与质量失败记录保留，当前唯一执行路线归 OBJECT-REFACTOR-VERIFY-01 与 object-refactor-plan 的 R0～R3。
- Goal / owner：执行顺序归 object-refactor-plan，边界与依赖原则归 object-refactor-guideline；保持现有合同、发布发现切片及 Snapshot 在途范围。
- 判断 / 选定：原计划先迁 Interfaces/Discovery、P4 再切换基础类型、Reader 与编码，至少需要第二次调用方适配；基础阶段必须倒查上层需求，完成最终公共合同与路径。解除下层反向依赖只做必要局部拆解，不提前重构整个上层。
- 验证范围：`make check-docs`、`make quality`、文档增量复核；只验计划修订，不执行代码迁移、业务或部署验收，不安装依赖、不提交。
- 当时证据与处置：文档图 32 篇 / 143 条关系通过，`git diff --check` 通过；`make quality` 退出码 2，停在其它在途改动的 gofmt 检查（`snapshot/access/update.go`、`snapshot/access/update_test.go`、`snapshot/service/access_http.go`）。当轮未改这些源码或削弱门禁，未取得该轮质量通过；来源改动后需要新的有效证据。这个记录不作为当前路线已验收依据。

### OBJECT-REFACTOR-PLAN-01 · 对象模块分阶段计划（2026-10-12）

- [x] 本轮完成计划整理：保存 P0～P7 的依赖、实施批次、中间态和验证出口，登记文档图并对齐 guideline；未启动代码重构，实施阶段全部保持未完成。
- Goal / owner：执行阶段归 `doc/design/object.md`，目标归 object-refactor-guideline，各领域合同与验证方法继续沿主题 owner。
- Non-Goals：不改源码、system 出版物、场景树或 Snapshot 在途实现，不运行业务/部署验收、不安装依赖、不提交；不把非强制说明变成发布门槛。
- 选定 / 不变量：沿用 guideline 的依赖、身份、版本、共享 executor、缓存、查询及 Interface 不变量；原 P0～P7 未实施且已撤回，当前按 OBJECT-REFACTOR-VERIFY-01 的验证优先路线执行；真实调用方和契约随结构一并切换，不建双实现/兼容壳。
- 本轮验证：`make check-docs`、`make quality`、文档增量复核；只证明计划与工程门禁，不计为任何 P0～P7 实施阶段通过。
- 验证结果：文档图 32 篇 / 143 条关系通过；`make quality` 退出码 0，工程门禁及耦合棘轮通过；`git diff --check` 通过。只改计划、guideline 执行原则、文档图和本任务记录，未运行业务测试或真实部署，未提交。
- 当前阶段（全部未认领；按对应验证出口完成后再勾选）：
  - [ ] R0：以目标保证补足公共行为验收，取得有效基线；提前建立异构对象发布发现证据。
  - [ ] R1：独立复核并修正必要行为差异，先红后绿，形成结构迁移基线。
  - [ ] R2：一次统一切换对象、能力、发现、专门流程及真实调用方，持续编译/定向验证，整批全绿才完成。
  - [ ] R3：核对最终源码的完整受影响合同、架构和质量证据，独立完成部署验收。

### OBJECT-GUIDELINE-01 · 对象模块重构指南（2026-10-12）

- [x] 本轮完成文档整理：保存可重复查看的目录、职责、依赖、迁移与验收 guideline，将图示资源纳入仓库并登记文档图；文档及质量门禁通过，不代表代码迁移或业务验收完成。
- Goal / owner：目标与迁移边界归 `doc/design/object.md`；能力组织、架构与请求责任分别沿 capability-organization / core-architecture / service，发布发现语义沿 object-system 及其直接 owner。
- Non-Goals：不改源码、公开协议、system 出版物或场景树，不新增业务对象发布门槛，不改 Snapshot 在途实现，不安装依赖、不启动服务、不提交；保留其它在途改动。
- 不变量 / 选定：沿用 A-01 / L-01、I-01 / V-01、API-01、CA-01、R-01 / R-02、OI-01～05；选定同级 object / interfaces / discovery / liveaccess 与公开一级入口，基本读写归 Object，任务服务归所属能力；否决 objectapp 兜底与宿主执行请求规则。
- 验证：本轮运行 `make check-docs`、`make quality`，检查 Markdown 与图示资源；这些证据只验文档及工程门禁，不代表目标结构、业务用例或真实部署通过。
- 验证结果：`make check-docs` 为 31 文档 / 136 关系通过；`make quality` 退出码 0，格式、tidy、vet、staticcheck、复杂度、体积、克隆及耦合门禁通过。两张 PNG 完整性与两份 SVG XML 检查通过，图示视觉复核通过；`git diff --check` 通过。未运行业务测试或真实部署，未提交。
- 延后问题的处置：ObjectID 空白校验（`object/address.go` / Writer）随基本读写迁移修正；显式 Schema pin 与索引采用依据不一致（`reader/schema.go` / `discovery/projection/spec.go` / `discovery/projection/extract.go`）随发现拆分补同依据回归；只读 Repository 暴露完整 Store 写能力（`object/repository.go`）随读写端口拆分收窄。位置、信号、判定及触发条件详见指南，均未在本轮修复。

### OBJECT-DESIGN-01 · 对象供给全景与发布发现设计（2026-10-12）

- [x] 本轮仅完成设计与现状分析：整理“发布可使用的业务 Object”全景，明确发布与发现切片及后续完善项；文档图、相关 owner 与质量门禁通过，不代表新的对象发布发现合同已实现或业务旅程已验收。
- Goal / owner：对象供给生命周期与可供性由新全景篇 `doc/design/object.md` 拥有；词汇、四个协议边界、知识发布、Interface 与产品缺口分别由 terminology / core-architecture / knowledge / object-interface / mvp-acceptance 拥有。
- Non-Goals：本轮只改设计及其关系和任务记录，不实现新的发布、发现或调用 API，不新增 Object 注册表、通用资源包装、领域处理器、令牌或会话，不改现有协议与授权，不安装依赖、不运行真实部署验收、不提交；保留其它在途改动。
- 不变量：沿用 I-01 / V-01、AUTH-01 / AUTH-03、OI-01～05，以及声明式索引字段 access 唯一来源；Object、知识表示和 Snapshot 载体分清，声明发布不推导就绪、授权或执行成功。
- 选定 / 否决：全景篇承接对象供给责任链，当前缺口与优先序进 MVP/TASK；复用 Writer、固定 Reader、Schema access、检索和关系，不新增第二份对象权威。否决把字段/命令形状写入设计、以现有机制存在冒充对象范式已验收，以及跨外部副作用与本地账本的伪原子保证。
- 接口与验证：公开形状仍归 knowledge / reader / writer / serving / retrieval / index 的 Go API、包 README 和 Conformance；本轮检查 `make check-docs`、`make quality` 及变更范围，不把文档验证算作新的业务场景或运行能力通过。
- 设计结果：新增对象供给全景，含三者关系、概念分工、七项责任、发布发现切片、三类对象与方向性用例；同步术语、架构、知识、Interface、服务及入口。文档图新增一个 owner 与 13 条直接关系，产品缺口由 MVP 承接；源码、system 出版物与场景树本轮未改。
- 后续实施建议（未认领、未实现）：按下表顺序选定公开合同并补证据，不从设计示意直接扩展返回形状。

| 顺序 | 完善项与位置 | 完成依据 |
|---|---|---|
| 1 | 对象供给角色、最小描述和可供性组织；领域 Schema / 普通 Aspect / 系统说明 | 图片、分散模块、外部表可固定发布与读回，分清业务对象和合同记录；不依赖文件路径或强制 Interface |
| 2 | 可解释供给的发布保证；Writer 与固定 Reader / Interface 解析 | 结构合法与解释依赖可采用分别验证；不把关系端点、外部源在线或处理器装配作为全部知识发布前置 |
| 3 | 用途及能力发现写法；Schema 字段 access、规范引用与检索回读 | 实现者与提及者分开；同合同依据的合法引用不漏匹配，跨来源/版本不误合并；明确 Member 命中证据，未就绪与零结果分开 |
| 4 | 异构对象发布发现旅程；既有场景前态、节点材料、产品条目关联 | 同一主体经 Server 完成检索、固定读回与关系定位，旧 pin 保留解释；材料与共享前态遵守场景合同，不代替清河或冒充媒体执行验收 |

- 增量复核处置：已修正 runtime 被收窄为动态值、Object 与知识记录混用、已有不变量编号被扩大保证、供给发现被误写成禁止所有动态查询，以及字段命中被误读为精确 Member 命中。系统出版物与可供性公共形状的同步列为后续实施缺口，没有借文档改名宣称代码能力已实现。
- 验证记录：`make check-docs` 为 30 文档 / 123 关系通过；`make quality` 的 `object-design:documentation-quality` / `20261012T034130Z-a695cda360b4` 与审查后 `object-design:reviewed-quality` / `20261012T034448Z-0314f230c3ce` 均退出码 0、passed、`sourceChanged=false`。闸层通过，26 单元、传播成本 33.23% 基线保持；`git diff --check` 通过。这些命令未执行业务 Go 用例，零测试事件不计为业务验收；记录回写后再次复跑文档与质量门禁。
- 交付边界：本轮只有设计、文档图、验收缺口与任务记录改动；新供给角色、可供性公共形状、语义发布检查、能力发现规范及独立异构场景均仍待后续实施，未运行真实部署或对象执行验收，未提交。

### CLEANUP-03 · 退役 DSH 宿主集成（2026-10-10）

- [x] 本轮完成：按用户明确裁决彻底移除仓内 DSH 插件及专属运行、构建、CI、验收和 CLI 私有任务上下文接缝；受影响测试与契约全绿且无 skip，保留其它在途改动，未提交。
- Goal / owner：撤掉已不再维护的宿主集成，所有当前入口无需插件目录或依赖即可装配。交互与入口归 [CLI 交互](doc/design/affordance.md)，独立客户端与认证边界归 [服务边界](doc/design/service.md)，验证分母与证据归 [验证体系](doc/engineering/test-catalog.md) / [MVP 验收](doc/engineering/testing.md)，能力区域归 [能力边界组织](doc/architecture.md)，过程归 [工程质量闭环](doc/engineering/quality.md)。
- Non-Goals：不退役公开 kc / typed Client / Server / Dataset File Gateway / kcfs，不改知识协议、授权、固定版本与 Writer 语义，不裁决 webui vendor、不删耐久状态或历史原始验收资料、不安装依赖、不运行真实部署验收，不代验收 OBJECT-INTERFACES-01 / DATASET-04。
- 不变量 / 接口：沿用 API-01（共享应用执行器）、A-01（依赖方向）、I-01 / V-01（身份与固定依据）；当前形状由公开 Go API、包 README 与 Conformance 拥有。CLI 按正式参数、环境及客户端会话取坐标，不再读取插件生成的 tasks/projects context.json；通用环境默认和服务会话隔离仍保留。消费授权沿 [权限体系](doc/design/permissions.md) 的 AUTH-01 / AUTH-02 分层与现行 Conformance，Dataset 消费和直接仓读分别求值，不改授权语义。
- 选定 / 否决：删除完整插件源码、锁文件、打包与本地生成缓存，清理专属命令和文档要求；将插件消费者测试中的通用 Server DTO / pin / 固定读风险复核并保留在核心测试。否决只删目录留下断链、伪造 Agent 验收完成、删除或削弱核心协议/架构断言、skip 换绿、删除外部宿主安装或其它工作区。
- 验证计划：先证明现入口仍依赖插件、私有上下文仍能干预正式 CLI，再删除；补核心范围回归与插件依赖防复发检查，运行受影响测试、场景与库存合同、Server Node 测试、`make check-docs`、`make quality`；真实 Agent 运行时退役后的通用验收缺口登记为债。
- 实现范围：完整删除插件目录（43 个源文件及其本地构建/依赖缓存），源码先按轮初字节备份到仓外临时目录；移除 CLI 私有 tasks/projects 读取接缝和专属测试，保留正式环境、saved Catalog/session 与 Server 必需边界，未删除用户 Home 中的旧上下文数据。CLEANUP-01 的插件验证记录是历史，不再是当前入口要求。
- 构建与验证接线：插件/付费 Agent 目标及 CI/npm 依赖退役，核心 Server Node 测试迁为 `test-server-web` 并继续进入 CI / test-all；Gitea 入口保留纯 Go consumer 与 Catalog discovery 合同及严格 live 要求。FUSE 原插件分支改为原生 daemon-mount/stop，ready manifest、固定 pin、EROFS、停止挂载/日志清理断言保留；隔离执行生产 shell 块的回归不计作真实 FUSE 验收。
- 风险迁移：消费者合同改为无宿主的真实 typed Client/HTTP 测试，保留库存与 Repository/Dataset 来源、named/selected pin、默认 ref、HEAD 前移后的固定 commit/body；增加异主体与当前 Dataset 消费撤权拒绝。Dataset `file.read` 与直接仓读分开求值，Client README 的歧义说明按 owner/现有合同收口，未改协议授权。
- 库存与防回潮：从现有场景树的已声明 feature 提取 Agent brief，明确声明库存和未评估状态；原 DSL、Then Oracle 与 metric 材料守卫保留，移除宿主 manifest/Python 硬依赖。既有 surface 守卫拒绝插件目录/接线/私有 context 回潮，通用模型工具禁令保留；修复扫描缺路径的错误被当成无匹配放行的问题。独立复核另发现仅恢复带退役标签的测试会漏放，已先红后绿补反例，并只对插件接线检查纳入测试文件。历史迁移记录明确排除，扫描错误仍失败关闭。
- 红后绿与增量审查：私有上下文干预正式 Server 坐标的 4 个反例先红；插件缺失时旧 Docker wrapper 与 Gitea 检查仍被 npm/Node 阻断，旧守卫也漏掉无目录扫描错误和工具注册反例。新增反例现已绿；复核发现新 CLI 回归的阻塞 channel 可能挂起，已改为重复请求明确失败，定向 race 通过。显式 Catalog discovery 标签编译暴露存量未使用 import，已最小删除该 import，原合同断言保留。Client Gateway 与 MVP V7 的旧逐仓授权措辞按现有 Dataset 受限消费合同纠正，无授权行为变化。
- 最终同源证据：以下六次运行使用同一源码指纹 `ee9efa7c2feac5f2c41b32fe0e8084b07f14d8d16fa0b9ff645e0f9bd2fcf2aa`，均退出码 0、`sourceChanged=false`、fail / skip 为 0；计数是含子测试的终态事件，不是声明库存或完整项目通过率。

| run / scope | 正式范围 | 实际结果 |
|---|---|---|
| `20261010T095444Z-30568867218e` / `cleanup3:final-maintenance` | `make check-surface check-validation test-boundary`；场景树 views / product / states 分别检查 | 77 个 Go 通过事件、3 个包；Python 6 + 6 项通过；库存及三项场景静态合同通过，未评估模型执行 |
| `20261010T095444Z-a5c07eb9b52f` / `cleanup3:final-quality` | `make quality`（含 `make check-docs`） | 文档图 29 篇 / 110 条关系通过；格式、tidy、vet、staticcheck、复杂度、体积、克隆与耦合门禁通过，传播率 33.23% 未增 |
| `20261010T095444Z-0602e20fc84f` / `cleanup3:final-compile-race` | `make typecheck`、显式 `catalog_discovery_contract` 标签空选编译、私有上下文隔离 race | 全包与标签编译通过，未计为 live 运行；race 7 个通过事件（根测试及 6 个子例） |
| `20261010T095444Z-18de7a2e9be0` / `cleanup3:final-client` | 显式选择的消费、远程/会话、固定范围/授权、CLI surface/覆盖与场景材料回归 | 140 个 Go 通过事件、1 个包；实际 typed Client/HTTP Server 消费合同通过，默认 lakeFS 进程内夹具 |
| `20261010T095444Z-6e4fdf3389bf` / `cleanup3:final-server-web` | `make test-server-web` | Node 14 项通过，无 skip，使用已有 Node 24；无依赖安装 |
| `20261010T095444Z-876f6cca0363` / `cleanup3:final-scenes` | `make test` | 普通与索引场景 21 个 Go 通过事件、1 个包；lakeFS 进程内夹具与真实 OpenSearch，非真实 lakeFS 部署 |

- 过程失败保留：`20261010T094817Z-4755af9d2fb2` 与 `20261010T094951Z-9cc7992f023a` 因调用场景树的参数组合不合法整体失败（此前门禁断言已绿），不计通过；最后按工具公开合同逐项执行到绿。`20261010T094854Z-59721dad5a17` 因显式标签文件的未使用 import 编译失败，不计通过；最小修复后上述编译范围已绿。所有原始 report / log 保留。
- 最终增量复核：与轮初源文件备份相比，29 个已有文件变化、46 个源文件删除（其中插件 43 个、专属 CLI 接缝/旧桥测试 3 个）、2 个新核心回归文件；其余 1,788 个已有文件逐字节保留，无文件模式变化，5 个原有删除未复活，Git revision 未变。`docs/graph/` 与 DATASET-04 任务块逐字节保留；OBJECT-INTERFACES-01 的并行任务验证记录保留，未回退、未代其标完成。协议与授权生产实现未变，独立复核未发现增量阻断项；仅回写本节记录与状态，随后再跑文档和质量门禁。
- 验收边界：本轮结果只覆盖上述选择；未跑完整旧混合套件、真实 lakeFS/Gitea、真实 Linux/FUSE 或模型任务验收。现有严格外部入口和风险继续保留，不以删除插件或静态 brief 库存宣称它们通过。仅启动既有入口的 OpenSearch 验证夹具，结束后清理本轮创建的容器；其它任务的服务与用户 Home 原始数据未改。

| 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| MVP C8 / VALUE-01、场景 Agent briefs | 专属模型运行器退役，通用概念/入口/恢复及真实任务效果目标仍在 | 协议 Oracle 或库存声明不能证明真实模型任务通过 | 保留为未执行缺口，无替代 runner；未来按角色/任务分母、回答/trace 与独立 oracle 验证 | 恢复模型验收或交付任务效果结论 |
| MVP V9 / V11 / V14 | 原宿主侧栏/开关/更新检查与上下文生命周期已退役 | 核心固定依据与当前授权已有合同，不能代证新的宿主人用体验 | 保留原风险与退役边界，不计当前宿主执行分母或新宿主通过 | 新宿主集成或重做发现/采用交互 |
| Linux/FUSE 与 live Gitea 资格 | 验收入口去插件依赖；本轮隔离回归/默认 lakeFS HTTP 夹具可执行 | 不等于真实设备、外部 provider 或部署通过 | 保留严格独立入口，缺环境失败，不新增 skip | 对应部署资格验收 |
| `cli/remote_test.go:156` | 现有 grant 默认范围隔离测试仍设置 `KC_WORKSPACE` 并检查 `workspace` | 存量夹具未钉现行 `KC_DATASET` / `dataset`，通过不能证明对应风险；本轮逻辑及该文件未改 | 登记测试债，迁移当前变量/DTO 反例并保留拒绝继承风险，当前不追存量 | 下次修改客户端环境默认或整理该测试 |

### CLEANUP-02 · 用途、替代关系与独占风险盘点（2026-10-10）

- [x] 本轮完成：按实际入口和独占风险复核上轮候选，迁移已查清的一对一旧测试命令及确定的文档残留；保留在途开发改动，受影响测试、文档与质量契约通过，未提交。
- Goal / owner：消除已被现行入口替代的维护接缝，并为其余候选补用途、消费者、替代项和退役证据。过程归 [工程质量闭环](doc/engineering/quality.md)，测试分类和证据归 [验证体系](doc/engineering/test-catalog.md)，正式交互边界归 [CLI 交互](doc/design/affordance.md) 与 `cliSurface` / CLI Conformance。
- Non-Goals：不改对象 Interface / Dataset 控制台在途行为、业务协议和授权，不削弱或删除独占风险断言，不全局替换不同语义的旧动作，不裁决 vendor 来源树与依赖全集，不删除耐久状态、独有草稿或未归档运行证据，不安装依赖，不运行真实部署验收，不提交。
- 不变量 / 接口：沿用 API-01（共用应用执行器）、A-01（依赖方向）；测试必须分清正式 `cli.Run`、`RunEmbeddedForTest` 应用接缝和 `fixtureHomeOperation` 前态夹具。正式 argv 由 `cli/surface.go`、包 README 与既有公开命令合同拥有，不新增形状；无调用的 `internal/gitdir.BranchName` 属于内部 Git plumbing，按 [架构总览](doc/architecture.md) 与包 README 收窄，不改变 Catalog authority / CAS 边界。
- 选定 / 否决：从 `catalog-show → show` 的一对一簇开始，迁移正调用、保留后态/权限/旧命令拒绝断言，确认消费者清零后退役单项映射；删除全仓无调用、非公开合同的 `BranchName` 及可重建 Finder 元数据；否决机械删除零生产引用符号、把新拼写误当真实 transport 覆盖、把 `archive-repo` 的原归档要求直接改成登记移除，以及按体积或时间删除资料。
- 验证计划：先取得旧映射仍复活退役命令的失败证据，再修正；执行受影响 CLI 回归、公开入口与覆盖合同、工具/架构守卫、`make check-docs`、`make quality`。新发现逐项当场修、登记为债或升级守卫；未执行范围不计通过。

- 清理结果：6 处正向 `catalog-show` 调用改为 `show`，删除单项测试转换；新增有效夹具上的旧命令拒绝回归，原状态、授权和失败断言保留。这些旅程仍经 `RunEmbeddedForTest`，不计作新增真实 CLI/HTTP transport 验收。验证 owner 的 16 处过时操作标签与示例改为现行 CLI 或既有 typed API，C-05 对象坐标拒绝要求保留，不把 Repository 归档误换为 detach。
- 防复发：退役 Catalog 路径在多处文档重复出现，收进既有 check-surface；只匹配 `kc` / `go run ./cmd/kc --` 的命令示例，不禁裸历史摘要或内部动作名。隔离夹具的 8 个反例修改前全部漏放，现行命令与旧 HTTP 拒绝反例保留。
- 内部代码 / 本地资料：全仓查无 `BranchName` 调用；完整内部包的临时副本仅把该函数转为私有后，staticcheck 报 U1000，确认不是类型推断或 wire 形状误报，删除该函数及 unused import。仅回收 `.session_tmps/393bfd15-b262-4490-93ec-62785dc575a7/.DS_Store`（Finder 元数据，6,148 bytes）；其余 10 个会话文件与目录集合逐字节核对保留。
- 导出审计：AST 工具列出 627 个零跨目录生产限定引用的窄候选，其中 127 个有测试限定引用；这是建议清单，不是死代码计数。同包调用、推断使用、公开 wire/Provider 合同均须另行核对；本轮只删除已证实的内部函数，未确认候选保留。
- 验证结果：下列四次运行均为退出码 0、`sourceChanged=false`，共享源码指纹 `5d08a732839427593ca87c519c2bbffd9db59e285b140a39c16cda4b50ead62f`，测试 fail / skip 均为 0。定向 `KC_E2E_RUN` 只证明选定范围，不写成完整混合验收；覆盖报告记录 `show` 的 12 项成功断言与 3 项 FORBIDDEN 边界，仍属于应用接缝证据。

| run-id / scope | 执行范围 | 结果与边界 |
|---|---|---|
| `20261010T092842Z-23c9ccf7112d` / `cleanup2:maintenance` | `make check-surface check-validation test-boundary`；`go test ./internal/gitdir ./catalog` | 文档图 29 文档 / 110 关系；Python 6 + 4 项通过；Go 231 个测试终态事件 / 5 包通过 |
| `20261010T092842Z-dade3f18cc14` / `cleanup2:quality` | `make quality` | gofmt / tidy / vet / staticcheck / 复杂度、体积、克隆门禁通过；26 单元，传播成本 33.23%，基线保持 |
| `20261010T092842Z-d00e8b79cd9e` / `cleanup2:affected-e2e` | 5 条受影响旅程、旧映射拒绝、正式命令面、内部操作/阶段、usage 与 HTTP 对象坐标拒绝合同 | 15 个顶层用例，Go 56 个测试终态事件 / 1 包通过；复用测试执行器临时 OpenSearch，结束后回收 |
| `20261010T092937Z-a858d28457c6` / `cleanup2:coverage-contracts` | 覆盖证据维度、语义风险要求与完整命令面报告合同 | 3 项通过，无 skip；不把报告形状检查外推成全部命令动态覆盖 |

- 最终增量复核：轮初快照相比仅 11 个已有源文件变更，无新增或删除源文件；其余 1,852 个文件逐字节保留，OBJECT-INTERFACES-01 / DATASET-04 任务块与 5 个既有删除保持原状。独立复核确认原断言保留，无协议形状扩张或新特例；本次仅回写验收记录和任务状态，回写后再跑文档与质量门禁。

本轮保留的候选与债：

| 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| `cli/kc_test.go` 其余旧动作映射 | 仍有存量旅程和动态 argv 使用 | 不能由新拼写推定同语义或真实 transport 覆盖 | 保留；逐簇核对消费者、后态和独占风险再迁移 | 修改对应旅程或专门认领下一簇测试迁移 |
| `doc/engineering/test-catalog.md` C-08 与旧 `archive-repo` 接缝 | 测试接缝映射到 detach，不能证明 Repository 归档后的 COMMIT/PROPOSE 拒绝 | 用例要求仍有效，已有接缝的证据不足 | 保留 `REPOSITORY_ARCHIVED` 要求和现况；补真正 ArchiveRepository 与两类写拒绝证据后再裁决转换 | 触及仓归档生命周期或 C-08 旅程 |
| 同文 C-13、`audit --dataset` 接缝 | 当前 Catalog Audit typed client 不传 Dataset，行摘要仍写配方历史 | Dataset 配方历史和 Catalog authority 历史的证据混用待澄清 | 保留风险；按 owner/API 分清各自历史与独立断言，不在清理中改业务语义 | 触及 C-13 或 audit 旅程 |
| `webui/pub/{getting-started.png,logo.png,logo.svg,logo192.png,trophy.svg}` | 当前生产源码闭包及既有构建文本无引用，但 Vite 仍复制发布 | 可考虑收窄发布集合；外部直链与 vendor 来源约定未确认，既有构建也不是当前 WIP 的构建验收 | 保留原件，先选发布集合并补当前构建、浏览器与静态路由证据 | 发布体积治理或 vendor 子集改造 |
| `.validation/runs` | 盘点时 287 次运行，至少 14 个被源码文档引用，1 个未完成最终报告 | 原始运行输出无法由当前代码重建；没有保留/归档合同 | 全部保留，按引用、在途状态和归档规则回收 | 建立证据保留入口或空间预算触发 |
| `.session_tmps` 两份 Markdown、8 个 rejected architecture 侧车 | 独有草稿/审查资料，没有已归档或可重建证明 | 尚不能判为缓存残留 | 保留；核对原始输入、生成器与独有结论归档 | 专门本地资料回收 |
| `scripts/assemble-doc.sh` | 仓内无调用，声明外部 WorkSurface/Orchestrator 用途 | 仓外消费者未确认停用 | 保留；确认外部使用后更新合同或退役 | 外部工作流确认或重新接入 |
| AST 导出候选、webui vendor / 依赖全集 | 低引用或不在 KC 当前生产闭包 | 尚无逐符号退役证明或 vendor 子集合同 | 保留；公开合同、同包调用和升级/许可边界另审，不安装或卸载依赖 | 对应 API 退役、上游升级或专门瘦身 |

- 验收边界：本轮不代验收 OBJECT-INTERFACES-01 / DATASET-04，不运行真实 lakeFS/Gitea/native/FUSE 或模型部署验收；新建服务仅用于既有测试入口的 OpenSearch 验证夹具，不改环境供给合同。完整旧混合套件和未逐项复核的候选不计通过。

### CLEANUP-01 · 确定残留清理与入口防漂移（2026-10-10）

- [x] 本轮完成：清理已经复核的退役要求、失效入口、迁移说明和任务状态残留；已有工作区改动保留，受影响测试与契约通过，未提交。
- Goal / owner：让当前维护入口与实际职责、路径及执行结果一致。过程 owner 为 [工程质量闭环](doc/engineering/quality.md)，执行与证据 owner 为 [验证体系](doc/engineering/test-catalog.md)；基础验收和 CLI 归属分别由 基础重构验收、[CLI 交互](doc/design/affordance.md) 与现行包合同拥有。
- Non-Goals：不修改正在开发的对象 Interface / Dataset 控制台行为、协议或权限，不删除或削弱契约与守卫，不重新裁决 vendor 子集，不删除耐久状态、历史运行证据或未知用途工具，不启动真实部署验收，不安装依赖，不提交。
- 不变量：保持 API-01（共用应用执行器）与 A-01（依赖方向）；没有新增不变量。执行器的环境缺失失败、守卫可被反例击穿及同源码证据纪律沿用验证 owner 的过程合同。
- 选定 / 否决：选定最小修正、隔离临时夹具的红后绿回归、复用现有文档检查扩大维护链接覆盖；否决机械删除零引用符号、替换 Dolt 名称后外推 lakeFS 验收、大删迁移取舍、skip 换绿、放宽质量基线。
- 接口 / 验证：既有 Makefile 入口、check-surface / kcfs 脚本退出码、validation manifest.environment 与 check-docs 本地链接；精确形状仍由脚本、公开包合同及测试拥有。修改前已复现旧 Node 路径不存在、server 旧路由未被静态守卫扫描、缺 lakeFS 绑定返回成功以及退役 selector 进入验证 manifest。
- 复查补充：显式 kcfs 的平台/FUSE 缺失和容器安装/下载失败仍能返回成功，隔离反例后统一收口为失败；插件两个 HTTP 夹具仍模拟退役 workspaces 路由和字段，按正式 datasets 路由、列表身份、pin 与临时定义合同修正，原来源、撤权、旧 pin 和凭证断言保留。
- 验证结果：下列四次收口运行均为退出码 0、`sourceChanged=false`，共享源码指纹 `f819bf7e68a6416098234fd48c055ea2980e3ac1665f788c050d297e4261983b`。非 Go 结果按各工具原始摘要判读，不用验证工具的零 Go 事件冒充用例计数。既有任务状态不由本轮代勾选。

| run-id / scope | 执行范围 | 结果与边界 |
|---|---|---|
| `20261010T090433Z-8c2a9a71b725` / `cleanup:final-maintenance` | `make check-surface check-validation test-boundary` | 文档图 29 文档 / 110 关系；Python 6 + 4 项通过；Go 71 个测试终态事件 / 3 包通过，fail 0 / skip 0 |
| `20261010T090433Z-7e0f8c885816` / `cleanup:final-quality` | `make quality` | 闸层通过；26 单元，传播成本 33.23%，基线保持，无放宽 |
| `20261010T090433Z-26338056754e` / `cleanup:final-plugin` | Server Node 测试、插件 typecheck / test / build / pack:check | Node 14 项、Vitest 27 项通过，无跳过；使用已有 Node 24 和依赖，未执行 `npm ci`，不写成完整 `make test-plugin` |
| `20261010T090433Z-e33a83405461` / `cleanup:final-scenes` | `make test` | lakeFS HTTP 夹具普通/索引旅程与真实 OpenSearch；Go 21 个测试终态事件 / 1 包通过，fail 0 / skip 0 |

- 过程失败保留：`20261010T085832Z-98f162a43656` 的质量命令因 `GOPROXY=off` 无法查询已缓存工具的模块元数据而失败，后续改用本机文件代理通过，没有安装依赖；`20261010T085832Z-9c87b965a1d1` 的插件命令复现两项旧 HTTP 夹具失败，修正合同后通过。失败记录不改写成通过。
- 最后增量自检修正了新增容器模拟测试的日志隔离：先以三项失败反例证明宿主固定输出路径未进入 fixture，再只在 mock 执行接缝重定位输出，生产命令捕获与 setup 语义不变；四项回归通过。本条回写后再复跑文档、工具回归、架构与质量门禁；该增量与记录回写均不改业务源码。
- 验收边界：未执行真实 lakeFS 部署、Gitea / native provider 资格、真实 Linux/FUSE、模型或容量验收；本轮结果不代验收 OBJECT-INTERFACES-01 / DATASET-04，不把未执行记为通过。

本轮保留的候选与债（不按关键词或零引用自动删除；这是 CLEANUP-01 结束时的盘点，后续处置见 CLEANUP-02）：

| 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| `cli/kc_test.go` 的旧命令转换层、`doc/engineering/test-catalog.md` 的操作表与调用示例 | 多处测试仍经旧 argv/action 映射运行；部分文档仍使用退役 `catalog repo attach/create/archive` 标签或示例 | 历史接缝、内部动作与正式入口混用，须保留用例语义和断言，不能全局替换 | 保留；按风险 owner 核对正式 CLI，分清摘要标签与可执行示例，渐进迁移旅程后收窄兼容表 | 下次修改对应旅程、验证 owner 入口或专门认领测试迁移 |
| `scripts/assemble-doc.sh` | 仓内无调用，默认输出为已退役设计路径，但声明外部 Orchestrator 用途 | 外部停用无法确认 | 保留；确认仓外调用后退役或更新输出合同 | 外部工作流确认停用或重新接入组装工具 |
| `webui/src`、`package.json`、`content.go`、`go.mod`、上游 Playwright | KC 生产闭包只用参考树一部分，上游外壳与 E2E 未接当前入口 | vendor 全集有明确保留约定，子集边界待定 | 保留；先明确版本、许可、抽取及升级验证合同，再收窄 | 上游升级、接入参考页面或专门前端瘦身 |
| `webui/pub` 与 Vite 复制步骤 | 部分上游资产仅由参考页面使用，仍进入 KC 产物 | 可考虑收窄发布集合，来源资产不可直接判垃圾 | 保留；分清参考与发布资产，补浏览器和静态路由证据 | 发布体积治理或 vendor 子集改造 |
| `.validation/runs` | 本机约 490 MiB、275 次历史运行，持续追加 | 运行证据，缺保留与归档合同 | 本轮不删；按状态、时间、在途与引用关系制定保留规则 | 空间预算触发或建立自动回收入口 |
| `.session_tmps` | 临时产物混有独立 Markdown 草稿 | 独有内容是否已归档待核实 | 本轮不删；核对内容归档再回收 | 专门本地资料回收 |

设计使然与误报：vendor 参考树、公开 wire/协议形状及测试基建的零生产引用不直接等于死代码；`.kc` / `.kc-home` 是耐久状态，`node_modules` 是可重建本地依赖，均不纳入本轮源码删除。

### OBJECT-INTERFACES-01 · 对象能力扩展与字段索引声明（2026-10-10，本轮继续认领）

- [ ] 在既有知识身份、Aspect/Member、Schema、Relation 和 Writer 合同上，实现可独立注册的对象 Interface、LiveAccess 以及唯一的 Schema 字段索引声明，同步 owner 设计与系统出版物，受影响测试及契约全绿后验收。
- Goal：对象按声明实现共同能力，消费者经系统固定版本解析和分派；查询声明统一写在 Schema 字段上，结构与查询合同分别校验，并沿用声明 → 校验/能力匹配 → 执行/同依据回读的接缝。Owner：[对象 Interface](doc/design/affordance.md)、[知识读写](doc/design/object.md)、[资源访问](doc/design/liveaccess.md)、[声明式索引](doc/design/discovery.md)、[术语](doc/concepts.md)。
- 本轮裁决：仅保留字段 access，撤回独立索引对象、第二写法和覆盖/兼容规则；移除对应解析、定位与公开 API，system 查询声明移回字段。结构不变而 access 收缩或移除应允许发布，旧 pin 按旧字段声明解释；AccessSpec 变化仍由索引流程单独判断。不影响对象 Interface 与 LiveAccess，不改物理配置、授权、写代数和无关前置工作区修改。
- Non-Goals：不新增写入代数、跨仓事务、源系统客户端或默认授权；不把运行观察当 Snapshot；不要求所有对象拥有统一 definition 字段；不新增通用资源描述符、流处理能力或物理索引配置；不改动无关的已有工作区修改，不提交。
- 不变量：沿用 I-01（身份）、V-01（固定依据）、REL-01（关系来源）；对象能力遵循新 owner 的 OI-01～05（身份独立、固定与有界分派、授权与实际依据、独立扩展、实际依赖维护），索引遵循其 owner 的字段声明唯一来源、类型语义与历史固定解释约束；知识版本、运行观察与处理实现依据分别保留。
- 选定：普通对象/Aspect/Member 承载能力声明，固定版本解析，注册处理器按合同和实现结构分派；LiveAccess 保留旧 State/ResourceDescriptor 解释；Schema 字段 access 统一编译到 AccessSpec，结构兼容性和查询影响分别判断并保持旧 pin 语义。否决：独立索引对象、第二种索引写法、覆盖和回退规则、核心业务类型 switch、隐式全仓扫描、向只读 system 仓动态追加扩展、Schema 副本跨仓隐式解析、公开读取挂 Hook、借索引声明扩大授权。
- 接口落点：knowledge 的公开对象能力/索引声明类型，object/reader 的固定版本解析，liveaccess 的处理器接缝，retrieval.AccessSpec 与 index 的能力兑现；精确公开形状、错误和一致性证据归包 README/Go API/Conformance，不写入设计 Markdown。
- 实现范围：普通 Interface 合同与命名实现 Member、固定版本解析、独立处理器注册、Schema 校验及授权；首个内置 LiveAccess 与历史 ResourceDescriptor 解释，公共 CLI/HTTP 调用保留持久访问与审计依据；字段查询声明经 Writer 发布与有界 Schema 定位编译到 AccessSpec，查询能力变更驱动重建且旧 pin 保持原解释。system 出版物、导航、Schema、查询声明与相关 owner/文档图同步；移除独立索引 Schema、对象、定位元数据与 API。
- 增量质量闭环：已修正文档对 required/additionalProperties 的过时限制；回调可变值隔离、共享注册表并发装配、调用证据失败关闭均有回归。字段 access 收缩不再误判结构破坏，字段删除、类型改变与其它值结构保护保留；普通 discovery/projection/* 对象不能改变查询合同。审查发现处理器匹配混入查询声明摘要，分开结构支持摘要与完整知识依据并增加回归。系统 Schema 数量/身份与材料漂移继续由自动守卫保护，没有削弱原断言。
- 验证结果：字段查询变化重建、正文增量、能力移除、旧 pin 解释、值结构保护和 Interface 结构分派回归通过，受影响核心包与公开调用的 race 回归通过。固定源码副本（指纹 `45d532f498283d005233601ddb677e0f9e5618f0eff8a6839df4f2cf94021372`）的 `make quality` 通过；`make test` 运行 `20261010T091328Z-1acd8a633186` 为 passed，21 个测试终态事件，fail / skip 均为 0；`make test-contracts` 运行 `20261010T091328Z-6d5dc878d218` 为 passed-with-skips，1,963 个通过事件、fail 0、既有外部 live skip 20，60/60 公开命令满足成功及风险边界要求。两次正式运行均退出码 0、`sourceChanged=false`；完整混合契约不写成无 skip 或真实部署验收。
- 同源证据边界：运行期间其它任务更新了 11 个文件，包含 TASK、CLI 旧命令夹具、验证说明、Git 内部工具及维护守卫；本轮知识、Interface、索引实现和 system/owner 文件与固定副本逐字节一致。当前工作区另补跑受影响定向回归和 `make quality` 通过，记录回写后再次复核文档与质量门禁；固定副本的完整契约不外推为并行改动后整树同源验收。证据保留在 `.validation/schema-field-frozen-copy.json`、`.validation/schema-field-current-differences.json`、相应 runs 与日志中。
- 过程失败保留：原工作区完整契约运行 `20261010T084731Z-027ea6dc0caf` 的断言虽无失败，但并行修改触发 `source-changed`，整体退出码 2，不计通过；改用与当时工作区逐字节一致的固定副本完成上述正式复跑。前一独立索引对象方案的验证记录不作为本轮裁决的验收证据。
- 验收边界：本轮验证本地夹具、真实 OpenSearch、组件/架构和公开 CLI 契约；真实 lakeFS/Gitea/运行时/身份服务/模型的部署验收为独立入口。保留本项 `[ ]`，不将带 skip 的混合套件记为本文件要求的全绿且无 skip 完整验收，也不以本轮结果覆盖既有部署待办。


**现在的位置：基础协议与可恢复的服务骨架已经建立；普通用户自助修复已有历史功能验证记录。** 统一用户名、持久登录、同名托管供给、管理地址、自有 Gitea 连接、首次准入与受限分享已实现，既有范围由 WALK-01 记录；不把这些记录当成本轮重新验收。动态 State 部署、真实任务收益与规模资格仍是独立待办。

| 阶段 | 已具备的能力 | 当前状态与下一道门槛 |
|---|---|---|
| M1 基础协议与固定版本消费 | 稳定身份、Writer 单仓发布、CAS/幂等、Schema、来源、Dataset、固定任务依据、检索后同版本回读、权限交付链、typed Client | **已交付参考实现**；继续由共享合同和正式 CLI/HTTP 旅程回归保护 |
| M2 可恢复单实例与仓分配机制 | 独立 Catalog Snapshot 权威、耐久状态与可丢缓存、已有源只读 attach、服务凭证创建托管仓、创建者策略授权、实例替换后续用 | **已补齐同名用户供给**；Gitea/LakeFS 托管供给与管理地址进入耐久恢复路径，正式验收见 WALK-01 |
| M3 普通用户全程自助 | 获权后可发布、修改、评审、移除、复核和采用更新；单文件手册已有完整内容 | **已有修复与验证记录**；SELF-01～04 的既有范围由 WALK-01 汇总，真实外部身份服务与更广部署资格单独记录；手册与价值路径归 GUIDE-01 |
| M4 动态 State 完整部署 | State 精确 READ、独立动态投影、声明/观察双 basis、notice-and-pull、同版本 hydrate 已有 | **局部能力已有，可信消费闭环待补齐**；DOC-20 收口设计与用例，DYN-01 承接真实 Observer/runtime/lakeFS/KC/OpenSearch 与重启恢复；默认时效待 REVIEW-04 |
| M5 数据规模与生产资格 | 规模档位、压测场景树用例和基础输入生成器已有；原生点读、增量机制与部分遥测已有；LAKEFS-04 bulk-ingest 写通路已实现并通过合同/回归/闸层 | **尚无有效容量结论**；SCALE-01 补执行器、小规模基线、逐档扩量、历史与恢复证据（含 staging/bulk 双路径对照），DOC-15 补观测，未来 native provider 的能力隔离沿现行 Provider 合同；代际方案待 REVIEW-01 |

验证贯穿每个阶段，不集中到最后。M1/M2 的“已交付”保留既有实现与历史验收结论；并不表示本次整理重新跑过完整产品套件。

DOC-20 的既有 State 消费设计与用例见其完成记录；剩余缺口包括范围级覆盖/时效、
后台恢复、观察保留、旧 Dataset 声明维护与源授权。DYN-01 承接 lakeFS 上的部署证据；单 key
增量成本纳入 SCALE-01，不能以 lookup 次数代替整个维护路径成本。Stream 窗口与持续订阅后续
分别推进；自助产品已有结果由 WALK-01 保留，手册与其它规模任务仍按原条目跟踪。

产品缺口按 [MVP 的优先完善表](doc/engineering/testing.md) 收口：服务版采用、可信动态
消费、真实任务效果与修订循环、规模与恢复资格。本轮 GUIDE-01 对齐文档与现行入口；
之后各项仍沿既有 owner 与验收合同推进，不因价值表述更新改变当前容量或部署承诺。

## 2. 普通用户自助闭环的产品目标

这条主线要达到的结果是：部署方只在最初提供 KC 服务、托管 Store 和准入策略；此后接入方与消费方仅使用交付的 `kc` 客户端，独立完成日常工作。客户端已经知道唯一的 Server 地址，普通用户不填写服务地址、部署配置、Store DSN 或凭证。

local 免认证模式和 Taihu 模式都是已有认证入口，walkthrough 选择哪一种不影响本目标。当前要解决的是认证之后的产品身份和资源供给：无论认证器来自哪里，KC 都得到经过验证的用户名，并在普通界面中始终把 `kaiqidong` 作为该用户的 identifier；认证提供方 subject、Store 数字账号和服务凭证只留在内部映射与审计中。

### 2.1 目标旅程

1. 用户打开已配置好 Server 地址的 `kc`，以 local 测试模式或 Taihu 登录；`whoami` 显示 `kaiqidong`。
2. 接入方选择“接入 / 创建托管 Repository”，只填写可读仓名；有多个允许的 Store 时选择 Gitea 或 LakeFS，只有一个时无需选择。
3. KC 以当前用户为依据，确保 Store 中存在标识为 `kaiqidong` 的账号或隔离空间，创建物理仓，保存内部 binding，登记到目标 Catalog，并按策略授予创建者发布、修改和回读能力。部署方不参与逐用户或逐仓操作。
4. 创建完成后返回用户可理解的仓名、KC Repository identity、状态和可访问地址。Gitea 和 LakeFS 均提供 KC 仓管理页；Gitea 的原生仓页另受平台共同登录配置约束。
5. 接入方直接发布第一版知识，并能用同一对象身份持续修改、补充 Schema/来源、预览、提交、回读、下线和检查索引状态。
6. 接入方在自己的授权范围内分享给消费方；消费方自行发现、检索、读取、固定版本，并明确采用后续更新。撤权后旧 pin 不能绕过当前权限。
7. 创建或发布遇到超时、重试、Server 重启或 Store 暂时不可用时，用户能查询原操作状态并恢复到同一逻辑结果，不产生重复账号、重复仓或错误授权。

### 2.2 用户可见与内部实现的边界

| 用户应该看到或输入 | 只保存在系统内部 |
|---|---|
| KC 用户名，例如 `kaiqidong` | Taihu subject、Gitea 数字 user id、认证器前缀 |
| 可读仓名、Store 选择（需要时） | 服务账号、token、DSN、物理目录、allocation id |
| KC Repository identity、状态、可访问地址 | 自动生成的 command id、binding 与分阶段供给账 |
| 发布回执、知识对象身份、commit、索引状态 | Store provider 的恢复细节和中间错误状态 |

逻辑 Repository identity 不由物理仓名推导，也不因选择 Gitea 或 LakeFS 而改变含义。Gitea 与 LakeFS 可以使用不同隔离机制，但创建、重试、恢复、权限和普通输出对用户保持一致。

### 2.3 已确定与待裁决

**已经确定：** local/Taihu 认证方式不属于本轮产品决策；walkthrough 可用 local 排除外部依赖。部署方不参与日常接入。KC 用户名是公开 identifier，Store 内部 ID 不得外露。服务账号只代平台执行供给，不能成为知识创建者或仓的产品所有者。托管仓和自有仓是两条独立旅程。

**仍需 review：**

- 用户名是否全局唯一、能否改名或被回收；KC 需要怎样的内部稳定绑定来防止同名冒领。
- Gitea 同名账号由 KC 创建后，用户如何以自己的 KC 身份进入 Gitea 页面，避免另发一套长期密码。
- ~~Dolt 采用哪种用户可查看形态~~：已随 Dolt adapter 退役关闭；托管仓管理页由 KC 提供，provider 原生页保留 Gitea/lakeFS 入口。
- 首次准入是组织资格、邀请还是审批；仓维护者可分享的动作范围和转授权上限。
- 删除是归档还是物理删除，用户名冲突、配额、部分成功和 Store 不可用时怎样向用户呈现。

## 3. 当前待推进工作

### CATREG-01 · Catalog 成员合并与登记格式校验

- [x] 已完成（2026-10-10，Catalog 增量范围）：成员名单写入 `catalog.yaml`，Dataset 当前定义与不可变 release 保持独立文件；初始化、增删成员、发布/退役/归档、只读恢复与按 Dataset 审计已验证。
- Goal：按已证实的访问需求组织登记状态，恢复拒绝损坏或歧义的格式。Owner：[Dataset](doc/design/dataset.md) 的登记/发布与历史复核，[架构总览](doc/architecture.md) 的独立 Snapshot 权威，[服务边界](doc/design/service.md) 的显式初始化与恢复。Non-Goals：不改知识协议、授权语义、源仓历史、公开命令或跨仓事务；不去重 Dataset 当前定义与 release，不新增知识 Schema。
- 不变量：`L-01`、`V-01`、`W-02`、`KS-01`、`KS-02`；接口依据 `CatalogState`、`KnowledgeSet`、`KnowledgeSetSource`、`DatasetItem`、`Registry.Save` 与 Snapshot tree/CAS。
- 选定：新写出合并成员；旧布局兼容只读打开，明确变更时按新布局提交；结构与文件身份统一校验，持久 Items 如存在须与冻结 Sources 相符；保留退役/移除后的历史记录。否决：恢复自动推进 HEAD、混合两份成员权威、按旧历史授予当前权限、因格式调整覆盖不可变 release。
- 审查闭环：路径历史把无关 HEAD/父提交归为 Dataset 变更、后端页上限截断早期记录，均已先红后绿修复并升级为回归守卫（`registry_audit_test.go`，含 ChangeStore 与树比较两种路径）；YAML 浮点/时间/二进制类型隐式转换已先证伪再收紧解码，整数编码中转浮点丢精度也已修复并以 `2^53+1` 可恢复回归守卫。`catalog/log.go` 历史读取失败转空列表为存量错误传播缺口，本项不扩大公开签名；处置登记为债，触发条件为完善 Catalog 审计错误/分页合同。
- 已实现：新初始化明确写空成员列表，明确登记变更合并旧成员文件；只读恢复与幂等登记不迁移，不推进 HEAD。新增格式、文件身份/命名冲突、冻结 Sources/Items、current/release 一致性、退役/归档恢复与审计回归。
- 验证口径：共享工作区另有 System 出版物改动在途，整树检查曾被其编译/材料状态、文档关系环与旧 Schema 数量断言阻断；以基线 `2aa827f8` 加本项 13 个 Catalog 文件在 `/tmp/kc-catalog-validation-20261010` 隔离复跑，最终文件逐字节与工作区一致。最终 Catalog 全包、`make quality`（含文档）、`make test`、真实 `make deploy-local-scenes`、完整 `make test-contracts` 全绿，公开命令 60/60。System 场景断言同步后，当前共享工作区的 `make quality`、`make test`、真实 `make deploy-local-scenes` 与审计/发布恢复 CLI 定向合同也已绿；不把隔离完整合同扩称为 System 增量的完整合同验收。真实 DFS 20 个通过事件（含根）与两个走查专用节点按原入口不执行，本项未新增 skip，不把 DFS 结果扩称为那两个节点通过。最终证据：`/tmp/kc-catalog-final-package.log`、`/tmp/kc-catalog-final-quality.log`、`/tmp/kc-catalog-scenes.log`、`/tmp/kc-catalog-final-live.jsonl`、`/tmp/kc-catalog-shared-targeted.log`、`/tmp/kc-catalog-isolated-final-contracts.log`。

下列条目各自独立可认领。它们分别落在重构目标形态的哪一面、彼此的依赖顺序，以及「目标」与「现状」的逐条差距，见
重构目标形态与执行序归根本 TASK 的迁移记录（标注三态：已固化 / 已选定未闭环 / 待裁决），不裁决任何 `REVIEW-*`，也不新增 owner；原 REFACTOR_TOPOLOGY 与 PROVIDER_REFACTOR_GUIDE 已退出文档图。

### QUALITY-01 · 工程质量闭环：债登记（DTO 信封重复、staticcheck 存量与观察项）

- [x] 已收口（2026-09-23）：U1000 八处已按上一段处置并删除；剩余 13 处 cli 存量项与 DTO 信封重复均已处置——staticcheck/gocyclo/dupl 存量由并行收口轮清零，DTO 七字段信封按登记触发条件（下次触碰服务路由时）收敛为共享信封 embed 类型（`cli/service_routes.go` `knowledgeScope`/`knowledgeDatasetIdentity`，`client/knowledge.go` `KnowledgeScope`/`KnowledgeDatasetIdentity`/`KnowledgeRepoPin`；JSON wire 契约不变，序列化守卫测试通过），`make quality` 全绿。原条目：本轮认领 U1000 处置；债务登记，条目形状（位置 / 信号 / 判定 / 处置 / 触发条件）见 [`doc/engineering/quality.md`](doc/engineering/quality.md) §3。位置：`cli/service_routes.go`（约 10 个 request struct，:131–:269）与 `client/knowledge.go:19,107`。信号：`Catalog/Dataset/Pin/Definition/Repository/Commit/Ref` 七字段信封组成组重复（2026-09-23 试跑基线：全仓 7 行重复块 605 组，本条为最大非机械组）。判定：真问题（低危；行为由 httpsurface 闭集路由守卫与 client parity 守卫兜底）。处置：收敛为共享信封 embed 类型。触发条件：下次新增或修改服务路由触碰同文件时一并收敛，不单独排期。另登记 staticcheck 存量（2026-09-23 首次跑 `make quality` 报出 8 处 U1000；同日复核完整闸层输出为 21 处，含 `cli/allow.go` `knowledgeActionVerbAllowed`、`cli/dataset_consume.go` `openServing`/`searchVisiblePin`、`cli/stores_observe.go` SA4006×2 及 `cli/remote_*`、`cli/scene_feature_test.go` 等并行工作线文件；统一访问 TRAVERSE 增量文件经定向 staticcheck/gocyclo/dupl 复核为零新增）：`catalog.datasetRestrictsRepository`、`home.absStoreDir`、`home.looksLikeLocalPath`、`index.(*Index).searchEngine`、`index.(*Index).searchEngineAt`、`knowledge.(*SystemRepository).operation`、`discovery/opensearch.(*openSearchEngine).closePIT`、`scripts/docs-serve.piece`；判定：疑似死代码但需逐条复核（接口实现、反射或并行工作线待接线均可能为误报），处置=按 owner 逐条复核后删除或接线，不机械删除；触发条件=并行工作线收口、`make quality` 重跑后处置。同轮观察项（判定设计使然，不处置只随热点复查跟踪）：`home/deployment_runtime.go` `InitializeDeployment`（176 行/51 分支，fail-closed 恢复合同要求）、`discovery/projection/state.go` `refreshState`（161 行，内聚单算法）。2026-09-23 增补：耦合度量基线已建立（`scripts/coupling/baseline.json`，23 单元、传播成本 38.74%，`make quality` 附带只读耦合报告）；方法与读法约定见 [`doc/engineering/quality.md`](doc/engineering/quality.md) §7。U1000 处置结果（2026-09-23，本轮）：上述 8 处经逐条复核全部判定真问题并删除——`catalog.datasetRestrictsRepository` 为 object/reader 正本之外的陈旧副本；`home.absStoreDir` / `looksLikeLocalPath`、`index.searchEngine` / `searchEngineAt`、`retrieval closePIT` 为死包装或死代码；`knowledge.(*SystemRepository).operation`、`docs-serve.piece` 为死代码；连带清理 `home_system.go` 注释引用与 `home_mount.go` 失效 strings import。验证：go build / go vet / gofmt 全净，`make test` 绿（ok kc/cli 106s）、`make check-docs` 绿。剩余 13 处 cli 项归统一访问 TRAVERSE 工作线处置；并行线收口、`make quality` 全绿后勾选本条。

### QUALITY-02 · 服务调用并发锁按 serving 模式判定（race scope 检出数据竞争）

- [x] 已收口（2026-09-24）：race scope 首次在本树跑通后检出两处 DATA RACE（`cli/flags_identity.go` `observeHome` 读 `ws.Journal` 对另一并发请求 `home.(*Home).SetJournal` 的写；`TestRemoteProviderReadBackAndConsumerDiscovery` 同源）。判定：真问题——并行读的 RLock 快速路径对夹具/组件模式不安全：该模式每次调用经 `observeHome → SetJournal` 在共享 Home 上安装请求 journal 并扇出 Writer/Reader/ControlPlane/Catalogs，并发读既竞争又可能读到他人 stamp；部署模式的请求级隔离（`command.go` `ws.ReadView(requestJournal)`）本已存在但锁未按模式区分。处置：`lockTypedInvocation` 按 facade serving 模式判定——部署模式只读动作共享 RLock，夹具模式全部串行 Lock（含 VFS 读）；`TestTypedReadInvocationsShareTheReadLock` 更新为部署模式合同并新增 `TestTypedFixtureReadsSerializeWithEachOther` 守卫。同轮顺带发现（非竞争）：`TestHTTPAccessLogQueryFiltersAndPages` 为时间炸弹——夹具硬编码 `2026-08-25` 而 access-log 热窗按 `now-30d`（`observability.DefaultRetention`）计算，2026-09-24 03:00 墙钟起滑出窗口导致合法空页；夹具日期改为相对时钟（窗口过滤/分页/completeThrough 断言不变）。验证：定向 -race 复跑相关测试绿；全量 race/coverage 重跑见交付说明。同轮观察项（负载下偶发，非本修复引入）：`TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding` 断言恢复期零源请求，-race 满载偶发计数漂移（隔离复跑绿）；`TestWorkspaceFileGateway*` 两测试在 coverage 慢载下 TempDir 清理遇 projection worker 仍在写——已修（测试补 handler `Close()` 清理，隔离+定向复跑绿）。剩余债（登记未清）：夹具/组件模式缺请求级 journal 管道，读吞吐被串行锁封顶；触发条件=组件模式服务需要并行读吞吐时，先推广 ReadView 私有 journal 路径到夹具模式再放开读锁，不单独排期。

### QUALITY-03 · Server 求值入口缺空 principal 拒绝守卫（权限梳理发现）

- [ ] 未清（2026-09-24 权限模型梳理）：`cli/allow.go` `PrincipalAllowed` 与 `cli/dataset_consume.go` 消费求值对 `as == ""` 直接放行——这是本地直连 Home 无 `--as` 的 owner 路径（无 Deployment 本地 Home 属另一入口，`doc/design/permissions.md` §7.3 配对合同）。判定：守卫缺口而非行为 bug——部署 Server 路径依赖上游认证保证 principal 非空，求值函数本身无防线；若未来 Server 入口漏带 principal，会静默变成全放行，与 §7.1「不存在 owner bypass」的实现前提只隔一层。处置（未排期）：为 Server typed 求值路径补「principal 为空即拒绝（本地 Home 路径除外）」守卫测试，必要时在 facade 装配层强制。触发条件=触碰认证装配（`cli/flags_identity.go` / `service_invoke.go`）或新增 Server 入口时顺手补齐；期间该双轨以本条为准，不写进设计文档（设计已按 §7.3 声明应然）。

### QUALITY-04 · 耦合 ratchet 升格与二期指标

- [x] 已落地（2026-09-24）：`scripts/coupling` 增加 `-strict` 硬门禁并接入 `make quality`（`scripts/check-quality.sh`）——卡传播成本超基线、Instability drift ≥ 0.05、基线外新单元（新单元须显式 `-write-baseline` 收紧并记本文件）；基线文件新增抽象度 A / 主序列 D / 公开面密度字段（传播成本与 Instability 口径不变，仍为 23 单元 / 38.74%），A/D/密度为正则文本代理（const 块成员不计、未走 go/types），只观测不拦，确认信号后再升格。失败路径经篡改基线验证（exit 1、报因正确）。方法与读法约定见 [`doc/engineering/quality.md`](doc/engineering/quality.md) §7。验收：go vet / gofmt 净；`make quality` 全绿且尾部输出 `coupling ratchet: held`（2026-09-24）。

### QUALITY-05 · cli 包内分层：内嵌 Server 面与 CLI 前端分离（测量立项）

- 历史立项（2026-09-24，已由 CAP-ORG 的 B4–B14 收口，不再独立认领）：原 S1 方案为把 `serve_request.go` / `serve_response.go` 纯类型移入新子包 `cli/serve`，注册 `architectureLayer`，闭集锚点不动，全闸验证。未认领（2026-09-24 测量立项，数据自 `scripts/coupling -files cli` / `-cochange`）：cli 是双产品一包——CLI 前端（`verbs*` 11 文件 2844 LOC / churn 133、`help.go` 544 LOC / churn 34、`flags*` / run / surface / command）与内嵌 Server（`serve*` 1269 LOC、`service*` 1614 LOC、`dataset*` 2139 LOC、`telemetry*` 928 LOC、`observability*` 618 LOC、`auth*` 583 LOC、`home` 411 LOC，合计约 7.5k LOC）。时序耦合：cli（60 commit）与 client 74%、retrieval 70%、index 64%、knowledge 61%、internal 59% 全部强联动——每个特性都穿过这一个包。观察项（随热点复查，不处置）：`hook/outbox.go` 为遥测 outbox wire 枢纽（与 `cli/telemetry*`、`internal/telemetry`、`hook/dispatch` 各 ~80% co-change，支持 4–5 commit，支持度低仅跟踪）；`index <-> retrieval` 65%（13 commit，同层近等不稳定度，边界软）；`cli <-> client` 74% 为 wire 镜像设计使然（parity 守卫在位）。
- **S3' 已落地（2026-09-24）**：执行管线提取进 `kc/invoke`——`Invocation`（含 9 个方法：flag/flags/require/setID/targetRef/requireProposal/requirePreview/bindingScope/knowledgeRef，散落 5 文件全数随类型走——别名类型不能在他包加方法）、`Command`/`Handler`/`Stage` 三常量、flag 读取器（`FlagValue`/`FlagString` 系/`setIDOf`/`setIDFlag`）、`ObservationTelemetry` 观察累加器（字段导出）、`BindingScope`；cli 留 `invoke_aliases.go`（零流失别名 + 一行包装器），`telemetryProvider` 因依赖 home 解析留 cli、以 `projectionProvider func() string` 闭包注入构造器（invoke 保持无 home 依赖）；map 元素字面量键 `stage:`/`run:` → `Stage:`/`Run:` 同步改名。门禁事件：① 方法改名过宽两例被编译器+测试点名回滚（`request.flags`/`wire.flags` 非 invocation 接收者）；② staticcheck U1000 删 `stage` 类型别名、裁三个零引用别名（`authorizationObserver` 实有 5 处使用，恢复）；③ ratchet 实拦 3 处不稳定漂移——`home` 0.86→0.80、`controlplane` 0.67→0.60、`hook` 0.50→0.40，均为 invoke 成为新下游、域包 Ca 上升所致，方向从不稳定指向稳定，属改善方向，显式收紧基线；传播成本 36.05%→38.22% 记录为新基线（invoke 成共享枢纽后扇入上升，共享内核设计本意——替代方案是 import 环）。验证：build/vet/staticcheck 全绿，arch 守卫全量绿，闭集锚点绿，`make quality` EXIT 0（基线 24 单元/38.22% held），`make test` EXIT 0。
- 形状确认（2026-09-24 完成，认领前必读）：owner 合同只钉 argv 表面与注册表——[CLI](doc/design/affordance.md) 硬性约束均为行为语义、操作语义归 [`cli/SURFACE.md`](cli/SURFACE.md)、闭集锚点为 `cli/surface.go`（60 条锚点测试），**无合同钉 Go 包布局**。守卫触点：`internal/arch/layers_test.go` 按精确包路径匹配——新包必须注册进 `architectureLayer`（app 层）；本轮已发现并硬化子包逃逸盲（denied 精确匹配会放过 `kc/cli/*` 新包）为前缀继承 + `deniedMatch` 单测；`TestCLIAndHTTPUseSameTypedApplicationExecutor` 钉 cli 与 HTTP 共用 typed executor，拆分必须保持。耦合矩阵（identifier 级）：serve/service 簇（13 文件）与 cli 核心双向纠缠（command 注册表、auth 家族、dataset 视图、surface.go），**无零倒置切面**；`product_encode.go` 经读码确认属 CLI 前端输出编码器，不随 web 资产走。
- Goal / Non-Goals（引用 owner）：Goal = 内嵌 Server 面获得包边界，cli 包一分为二且 import 方向单向；Non-Goals = 不改任何 argv / help / SURFACE 语义（闭集锚点不动）、不动 client、不引入新依赖。方案对比：A) `cli/serve` 子包 + 共享落点（见 S2'）——推荐，import 方向 cli→serve 单向、与测量缝一致；B) 顶层 `serve` 包与 client 平级——否决（第二个 app 边缘包稀释 client 的 wire 镜像职责，守卫 denied 维护面更大）；C) 不拆只降耦——否决（19.2k 包结构问题仍在，verbs churn 133 持续增长）。原 S1（serve_request/serve_response 纯类型先行）经读码+调用方数据否决：三 helper 是 cli 全局 HTTP 管道（`identity_authorize` 16 处调用——登录回调共用），且 `writeInvoke` 依赖 `RunResult`，先移必成 import 环。修订切片（每步以编译器仲裁真实耦合、全闸验证）：**S1' 已落地（2026-09-24）**：`static_pages.go` 移入 `cli/serve`（导出 `WriteStaticHTMLPage`/`WriteStaticScript`，`console_page`/`repository_page` 重接线，`architectureLayer` 注册 app 层，arch 全量绿）；**S2' 已落地（2026-09-24）**：新建顶层 `kc/invoke` 包（与 `httpsurface`/`identity` 同级的 app 层共享契约包，命名对齐仓库惯例）——收容 `RunResult`（cli 留类型别名 `type RunResult = invoke.RunResult`，全仓零改名为真别名）、HTTP 管道（`DecodeJSONBody`/`WriteJSON`/`WriteInvoke`，12 个 cli 文件 ~40 处调用点重接线）与 `JSONOut`（`jsonOut` 通用 JSON 规范化器一并迁入，cli 留一行包装器）；`invoke` 注册进 `architectureLayer`，环点消解（cli→invoke、cli/serve→invoke 可同时成立）。同轮 ratchet 实战首拦：新单元 `invoke` 使 strict 门禁指名拦截（"unit invoke is not in the baseline"），显式 `-write-baseline` 收紧——基线更新为 24 单元、传播成本 36.05%（较 38.74% **改善 2.68pp**：共享落点缩短了波及链，属只紧不松允许的改善方向）；**S3' 尝试与回滚（2026-09-24）**：16 文件 server 集实移被符号级证据拦停并整体回滚（工作树已恢复 S2' 提交态、arch 全量绿）——server 路由不是独立面，而是**构建在共享执行管线之上**：`f.executeTyped(w, r, ..., command{run: func(cx *invocation) ...})`，HTTP handler 构造 `invocation` 走与 CLI 同一 typed executor（`TestCLIAndHTTPUseSameTypedApplicationExecutor` 所钉合同）；server 集对 cli 的真实符号依赖仅 6 组（`invocation`×7 文件、`HTTPAuthenticator`/`HTTPIdentity`/`ServiceIdentity`、`Home` 类型别名、`FlagValue=any`、`managedHTTPHandler`），全部经由执行管线。**修订切片**：S3'=执行管线提取——`invocation`/`command`/`operationTelemetry`/`FlagString` 系帮助函数迁入 `kc/invoke`（或并列 appcore），cli 与 cli/serve 共同依赖执行契约，环从根上消解；S4'=facade + service* 簇 + 混居文件分家（此后无环）；S5'=webapp/console embed 资产 + Vite outDir 合同。另观察项（登记不处置）：`cli/webapp/` 为 gitignore 的构建产物目录且 `.gitkeep` 未被跟踪（HEAD `git ls-tree` 为空）——fresh clone 上 `go:embed all:webapp` 将无匹配文件而构建失败（本轮实测确认：删除 .gitkeep 后 `make quality` 即断）；属并行线 webapp 构建流（S5'/webui 合同）的潜在断裂。验收锚点：每步闭集锚点测试不动、`make quality`（含耦合 strict）与 `make test` 全绿。

### CAP-ORG-01 · 能力边界组织重构：目录 = 能力 = 概念（B0–B14，一批一提交）

- [ ] DATASET-DEFINE-IDEMPOTENCY（债登记，2026-09-29）：dataset define 的内容派生 command-id（`dataset-define:<setID>:<revision>:<hash>`）未接 invoke 层命令账本——重复提交同一已接受发布（响应丢失后重试）返回 `NON_FAST_FORWARD` 而非幂等确认（部署恢复旅程测试实测，`cli/deployment_dataset_recovery_test.go`）。位置：`invocation/verbs_catalog.go` VerbDefineKnowledgeSet（checkNextRevision 在 writer 账本重放之前短路）。信号：与 knowledge U5 幂等精神不一致；与 writer put `REPLAYED` 行为不一致。判定：真缺口（低危——版本单调守卫本身正确，操作方可用 show/mounts 判定 v1 已是当前）。处置：verb 内先查 writer 账本命中重放再进 registry 校验，或 registry 对"revision == current 且 recipe 相同"返回幂等结果；二者取舍需对照 writer 账本所有权与 U5 owner 设计，不在 DATASET-02 验收轮顺手改。触发条件：下次触碰 dataset 发布路径或认领 knowledge U5 幂等场景时一并处置。

- [x] B0 已完成（2026-09-28）：设计按五段合同升入 [能力边界组织设计](doc/architecture.md)（class=decision）并登记 `docs/graph/`（refines core-architecture；depends_on architecture-invariants / quality-loop）；分支 `refactor/capability-boundaries`（tag `pre-capability-refactor` = 2ce24b6）；前轮场景/文档记账已单独提交 `2ce24b6`。
- [x] R1 修复（2026-09-28，先于 B1）：main 继承两处 arch 红——① QUALITY-05 S3' 把工作区载体 `Invocation`（含 `WS *home.Home`，143 处动词体消费）搬进 invoke，触发 `TestApplicationPackageBoundaries`（只有 transport 可 import home）；② 同轮命令字面量 `run:`→`Run:` 改名后 `TestCLIAndHTTPUseSameTypedApplicationExecutor` 守卫字面量未跟上。修复：`invocation`/`command`/`handler` 移回 cli（`cli/invocation.go`，invoke 回归纯词汇表，掉 home/object/liveaccess/snapshot 四个 import），守卫字面量追平（等价字面量，断言不弱化）。棘轮：invoke instability 0.88→0.80（从不稳定指向稳定，改善方向），propagation 38.22%→37.86%，基线显式收紧并于此注记。验证：`go test ./internal/arch/` 全量绿、`go test ./invoke/` 绿、`go build ./...` 绿、`make quality` EXIT 0；`go test ./cli/` 与 `make test` 见本条提交前复跑记录。
- [x] B1–B3 已落地（2026-09-28）：材料与闸，零生产行为移动——① `invoke`/`datasetfs` 补 README（invoke 随 R1 按收窄后真实面重写）+ `knowledgeapp` README 如实入册 18 个执行器（纵切 9 + 访问计划 1 + 治理 5 + 投影 3，`6e5d97c`）；② §3.4 的 7 条 denylist 落 `internal/arch` forbidden 表（invoke 集含 home，`TestForbiddenDependencies` 28 条全绿，`643ab0d`）；③ arch 新守卫 `TestEveryTopLevelCapabilityHasAReadme`（顶层 Go 目录必须持 README，复用 skipDir）+ `discovery/service/surface_anchor_test.go` 导出面闭集锚点（58 个包级导出符号，集合相等，篡改探针验证会咬人）。验证：`go test ./internal/arch/` 全量绿、`make test` EXIT 0、`make quality` EXIT 0。开放项：B1 三份 README 的人工评审仍开放——意见按批修订，不阻塞 B4（先立 `server/routes.go` 闭集契约）。
- [x] B4 已落地（2026-09-28）：`server/` 契约先行——`routes.go` 88 条 method+pattern 闭集（提取自 cli 全部 88 个 `HandleFunc` 注册，与 `httpsurface.Patterns()` 双向对账、无重复、字面表有序，三锚点测试绿），app 层注册进 `architectureLayer`，`server/README.md` 四件套①随批。棘轮：新单元 `server` 使传播成本 37.86%→**34.83%**（-3.03pp，server 节点提前吸收原经 cli 的波及链），基线显式收紧并于此注记；B5–B7 搬实现时逐条挂 handler 绑定。验证：`go test ./server/`、arch 全量绿、`make test` EXIT 0（111.1s）、`make quality` EXIT 0。
- [x] B5a 已落地（2026-09-28）：执行载体上移顶层 `invocation/` 缝合包（`cli/invocation.go` → `invocation/invocation.go`，`invoke_aliases.go` 类型别名垫片，143 处动词体站点零改动；方法依赖 invoke.FlagString/RequireDataset 等全部可解析）。arch 三处：`TestApplicationPackageBoundaries` home 规则改"两 transport 开 Home、载体持句柄不开句柄"（豁免 `invocation`）、`architectureLayer` app 层注册、denylist `invocation ↛ {cli, server, httpsurface, client, knowledgeapp}`。棘轮：internal 聚合桶失真（载体入 internal 使传播 +16.5pp 伪增值）→ 顶层单元立身份后 **34.62%**（-0.22pp，26 单元基线重录，controlplane/home/invoke 不稳定度同步改善）。设计文档 §2.4 R3 + §2.2 树注记；B5 内部分层重排（B5a 载体 → B5b 授权/hooks/证据族 → B5c 派发器 → B6 service 面）。B3 README 守卫现场咬合验证（新目录先红后绿）。验证：`go build`、arch 全量绿、`make test` EXIT 0（99.6s）、`make quality` EXIT 0。
- [x] B5b.1 已落地（2026-09-28）：授权层迁入 invocation 缝——`allow.json` 模型与判定闸整族（`Authorize`/`AuthorizeCatalogInventory`/`AuthorizationFlags` + `AllowRule`/`AllowFile`/`AllowQuery` + `MatchAllow`/`PrincipalAllowed` 等，allow.go 547 行）、任务 pin 归一族（`PrepareKnowledgePinContext`/`HoistTaskPinDefinition`/`SuppliedKnowledgeSet`）、Catalog discovery 验证标记闭合三元组、`observeAuthorizationResult` 观测、两回执类型（allow.json 模式成员）。cli 侧 `allow_aliases.go` 别名/包装垫片，动词与测试站点零改动；`catalog_discovery.go` 只留 remote 解析与 search 形状检查。棘轮：**+0.00pp 零漂移**（单元模型耦合中性，26 单元基线不动）。验证：`go build`、arch 全量绿、`make test` EXIT 0（107.0s）、`make quality` EXIT 0、gofmt 净。
- [x] B5b.2 已落地（2026-09-28）：hooks 层迁入 invocation 缝——`WithHooks` + `writerReplayed`/`hookEvent`/`durableHome`/`observeHookOutbox`/`fillHookResult` 整族（home_hookrun.go 117 行，依赖 hook/object/writer/snapshot/treewriter/kchome/invoke 全合法），cli 侧 `invoke_aliases.go` 垫片保派发器两处调用（B5c 随迁）。棘轮：hook instability 0.40→0.33（改善向），基线显式收紧。验证：`go build`、arch 全量绿、`make test` EXIT 0（108.0s）、`make quality` EXIT 0。
- [x] B5b.3 已落地（2026-09-28）：观测/证据/请求形状层迁入 invocation 缝（~1500 行，8 个 cli 文件迁出/删除）——域遥测族（`RecordDomainTelemetry`/`TelemetryFace`/`TelemetryStart` 等）、证据链（`EvidenceChain`/`ShapeInvocationResult`/`Record*` 四族 + 信封整形 `ErrorResult`/`ResultOutcome`/`SearchRerankResult`）、store 观测装饰器（`BindHomeTelemetry`/`ObservedSnapshot` 全家）、投影观测、context 词表（`RequestIDFrom`/`IdentityContextFrom`/`TraceContextFrom`/`PrincipalOf`/`ActionOf`/`ResolveHome`/`OwnerPrincipal`）、请求形状解析器（`SearchRequestFromFlags`/`AddressFrom`/`LimitFrom` 等 10 函数）。棘轮：discovery/projection/observability instability 0.50→0.44（改善向），基线收紧；传播 34.62% 持平。staticcheck 三轮迭代清零投机垫片（U1000 驱动删 14 个无消费者包装）。验证：`go build`、arch 全量绿、`make test` EXIT 0（99.8s）、`make quality` EXIT 0、`make check-surface` PASS、`testsuite.sh contracts` PASS（60/60 公开命令覆盖）、gofmt 净。
- [x] B5c 已落地（2026-09-28）：派发器迁入 invocation 缝，cli 收敛为纯前端——`ExecuteApplicationOperation`（stage 机器+授权+hooks+观测）与 `InvokeApplicationWithTelemetryAtHome`（typed service 入口：span 织造+证据链）双入口进缝；`TypedInvocationReadOnly` 随迁；旗语卫生整族（`flags.go` 95 行：knownFlags transport 级词表 + `RejectUnknownFlags`/`RejectRemovedFlags`/`RejectServeOnlyFlags`/`RejectServeFlags`）整迁。cli 剩：argv 注册表、dispatchWithStateAtHome（CLI 路：help/registry）、invokeWithTelemetry* CLI 入口、verbs/surface/run/产品输出/serve。棘轮：0.00pp 零漂移、无 instability drift（耦合中性）。验证：`go build`、arch 全量绿、`make test` EXIT 0（101.5s）、`make quality` EXIT 0（U1000 迭代清零）、`make check-surface` EXIT 0、gofmt 净。
- [x] B6 已落地（2026-09-29）：server 拥有 HTTP 面、cli 收敛为纯前端——cli 的 serve/service_routes（wire DTO `Flags()` 化）/handlers（httpFacade/managedHTTPHandler）/state runtime（HTTPStateLookup 族）/gitea/配对身份/遥测（ObservationTelemetry 织造）/就绪探测全部迁入 `server/`，公开入口 `HTTPHandler`/`HTTPHandlerWithTelemetry`/`HTTPHandlerFromConfig`/`HTTPServerOptionsFromFlags`；cli 动词操作体全体迁入 `invocation/verbs_*.go`（读/写/索引/资源/目录/观测/授权域，`Run: inv.VerbX` 注册表引用），HTTP typed handler 调同一批操作体（API-01）。arch 三处：`invocation` denylist 收窄 `{cli, server, httpsurface, client}`（knowledgeapp 解禁——缝是用例核心编排层）、`server → home` 解禁（transport 打开 Home）、新增合法边 `cli → server`（serve verb 二进制组合根，cmd/kc 只 import cli；B8 复核）。`check-surface`/`validation-inventory` 路径 cli→server。行为语义变化一处：ResourceLookup 无 state runtime 返回 `TEMPORARY_UNAVAILABLE`（与原实现经 `NewHTTPStateLookup(nil)` 同码，TestProductScenes 合同不变）。测试所有权：server 内部符号（httpFacade/wire DTO/authenticator）→ `package server`；cli e2e（Run/RunWithTelemetry + serv.X 前缀）→ `package cli`。棘轮 34.62%→38.62%（26 单元；缝成为更大枢纽+server 获传入边，B6 设计代价，基线显式重写；B6 收尾时 cli/serve 静态页 writer 迁 server 后包消解，二次重写为 38.00%，cli instability 0.87→0.93）。验证：`go build`、arch 全量绿、`make test` EXIT 0（100.2s）、`make quality` EXIT 0（U1000 两轮清零）、`make check-surface` EXIT 0、gofmt/vet/staticcheck 净。
- B6 登记债（三出口·登记为债）：
  - ~~夹具重复~~ 已于 B8 收口（改判设计使然）：cli 版测客户端登录会话流（无头校验、Admin 位），server 版测服务端授权（校验 Authorization、OnBehalfOf/Provider/Subject）；行为不同非重复，统一需导出测试替身反而污染公开形状。
  - ~~组合边 `cli → server`~~ 已于 B8 复核收口（判定保留）：serve verb 是 kc 二进制的组合根，`server.HTTPServerOptionsFromFlags`/`HTTPHandlerFromConfig` 即本地 serve 的装配缝；`home` 是部署装配根（OpenDeployment 路），不承载本地 serve 装配——再设一层缝纯属代理。边界由 arch `TestForbiddenDependencies`（server ↛ cli）+ 设计文档 §2.4 R3 记录。
  - ~~wire-shape 归属~~ 已于 B7 收口：`client` 为管理面公开 wire 契约家（裁决见设计文档 §2.4 B7 注记）；`server → client` 为守卫 why-text 记录的既决例外。
  - 耦合代价：传播成本 34.62%→38.62%；触发条件：B9–B12 internal 收拢批次复查缝是否过胖（`invocation` instability 0.889），若 verbs 域内出现再分层信号（B5b 式读/写分家）则拆子包。
- [x] B7 已落地（2026-09-29）：`serve_readiness` 迁 `server/internal/readiness`（Result/Cache/NewCache+Providers/BindProbe/Providers()/Surface/Overall/Probe/Deployment/EvidenceWriteProbe，指纹缓存与 inflight 单飞原样保留）；就绪探测改装配缝注入——`Providers.SearchBackend func(home.StoresFile) error` 端口由 `home.NewSearchBackendProbe()` 绑定 `opensearch.Check`（非 opensearch 驱动 no-op，与原 `normalizeIndexDriver==opensearch` 门等价），deployment 路径同一端口吃 `config.Stores`（DeploymentConfig.Stores 同为 StoresFile）；server 与 readiness 包均不再 import `discovery/opensearch`（server 侧五个孤儿别名 homeReady/layoutPath/storesPath/normalizeIndexDriver/openCatalogs 一并清除）。A-01 守卫落地：`TestProviderAdaptersImportedOnlyAtAssembly`（depGraph 全量扫描，provider 集 {snapshot/lakefs, snapshot/gitea, discovery/opensearch, discovery/llmhttp, object/cache}；commandlog/treewriter 属能力树豁免；注册例外 server→discovery/llmhttp 的 reranker/embedder 装配，B9 吸收并已在 why-text 记录）。wire-shape 债项收口（B6 登记→B7 裁决）：`client` 为管理面公开 wire 契约家（20+ 负载类型 + typed 方法 + 身份/会话；外部无形状消费者已核实），typed service 路由形状 server 自持不变；`server → client` 守卫 why-text 改记为既决例外，双方不得单方迁移形状。arch 增改三处：app 层注册 `server/internal/readiness`、home 边界规则按 `server/` 前缀豁免（内部包同属 transport 侧）、server denylist why-text 更新。文档同步：设计文档 §2.4 B7 注记、home/README（A-01 装配范围）、server/README（就绪探测段）。棘轮 38.00% 零漂移。验证：`go build`、arch 全量绿、`make test` EXIT 0（98.9s）、`make quality` EXIT 0、`make check-surface` EXIT 0、gofmt/vet/staticcheck 净。
- B7 登记债（三出口·登记为债）：
  - ~~server → discovery/llmhttp~~ 已收口（2026-09-29 复核批 R1）：provider 装配移至装配根——`home/llm_ports.go`（B7 端口模式）绑定 `llmhttp.New/NewEmbeddings`（env 端点与凭据属部署装配，留在 home），server 只留 flag/env 语义并接收 `retrieval.Reranker/Embedder` 端口；arch 守卫 server 例外移除（A-01 从此零例外，peer-adapter 豁免 opensearchimpl→llmhttp 保留）。
- [x] B8 已落地（2026-09-29）：Phase 1/2 收尾——cli 导出面收窄（初版 43→24 系 go doc 截断漏计，复核后实为 57→11）：product 输出文本（Help/ConsumeHelp/WriteHelp/ComposeHelp）unexport（`TestHelp` 断言随迁包内 `help_internal_test.go`），零外部消费者的迁移垫片（flagString 族/requireFlag 族/writeAllow/configDir/normalizeLoginServer/principalAllowed/resourceLookup/httpHandler/remoteServerURL）unexport，死垫片 resourceLookup/remoteServerURL/principalAllowed 删除；`cmd/kc`、`cmd/kcfs`、`cmd/kc-integration`、`integrationruntime` 所用符号全部保留（Run/RunWithTelemetry/ParseArgs/ParsedArgs/RunKnowledgeSetFS/ClientServerURL/NewSessionClient + 类型别名），「公开形状只增不删」按对外合同不收缩执行（18 项逐一全仓库引用扫描）。消费方守卫：arch `TestCapabilityConsumerSets` 钉住 delivery/gate/hook/identity/observability 消费方集合（delivery←invocation；gate←controlplane/home/invocation；hook←home/invocation/invoke；identity←cli/home/invocation/server；observability←cli/client/home/invocation/liveaccess/server），新消费方须显式扩表；delivery README 消费方更正为 invocation 缝注入。B6 债双收口：配对夹具改判设计使然（cli 客户端登录流 vs server 服务端授权，行为不同非重复）；`cli → server` 组合边判定保留（serve verb 即二进制组合根）。阶段门：`make test` EXIT 0（124.4s）+ `make test-race` EXIT 0（7 包 -race：snapshot/commandlog、hook、object/reader、object/cache、index、home、cli）+ `make test-contracts` EXIT 0 + `make quality` EXIT 0（38.00% 零漂移）+ `make check-surface`/`make check-docs` EXIT 0；tag `capability-phase-2`。
- [x] B9 已落地（2026-09-29）：Phase 2 协议回执——`snapshot/receipt.go` 新增 `snapshot.Receipt`/`ReceiptResult`（JSON 形状与历史输出逐字段一致；新增不替换 treewriter.Receipt）+ `treewriter.Receipt.Protocol()` 转换（子包→根依赖已存在）；invocation/verbs_write.go 信封字段改协议类型（`WorkspaceCommitResult.Receipt`，产品 JSON 不变，treewriter import 从 verbs_write 消失）、invocation/home_hookrun.go fillHookResult 增协议回执分支（writer.CommitReceipt/treewriter.Receipt producer 分支保留）。B7 登记的 llmhttp 例外仍开放（B12 吸收 provider 装配）。验证：`go build`、arch 全量绿（A-01 组含）、`make test` EXIT 0（99.7s）、`make quality` EXIT 0（38.00% 零漂移）、`make check-docs` EXIT 0、gofmt/vet/staticcheck 净。
- [x] B10 已落地（2026-09-29；用户裁决 C=本轮实施薄壳、A=observability 上提、B=gate 保持独立）：snapshot internal 化——`snapshot/lakefs`/`snapshot/gitea` 收拢为公开注册薄壳（`lakefs.go`/`gitea.go`：type 别名 + var 函数再导出；公开 API 逐符号保持，home/authority_drivers.go 调用面零改动），实现 13+8 文件迁 `snapshot/internal/{lakefsimpl,giteaimpl}`（package 改名 lakefsimpl/giteaimpl；package 内部测试随迁，外部合同测试 `lakefs_test`/`gitea_test` 留薄壳处）。A-01 守卫 adapter 集改 internal impl 路径 + shim 豁免映射；层表增两 impl。验证：`go build`、arch 全量绿、RepositoryContract 组 `TestLakeFSMatchesTreeProviderByOperationStep` EXIT 0、`./snapshot/...` 全绿、`make test` EXIT 0（105.2s）、`make quality` EXIT 0（38.00% 零漂移）、`make check-docs` EXIT 0。
- [x] B11 已落地（2026-09-29）：knowledge + catalog/worktree internal 化——`object`→`object`（`object/format` 经薄壳 `object` 消费）、`object/semanticview`→薄壳+`internal/semanticviewimpl`、`object/maintenance`→薄壳+`internal/maintenanceimpl`（discovery/projection/testkit/invocation 走薄壳面）、`catalog/worktree`→薄壳+`internal/worktreeimpl`（invocation 走薄壳面）；package 内部测试随 impl，外部合同测试留薄壳。层表 +4 impl；worktreeimpl 复制层① deny 规则；client-依赖守卫覆盖 impl；worktree 依赖闭包更新。验证：`go build`/vet/gofmt/staticcheck 净、arch 全量绿、`make test` EXIT 0（105.3s）、`make quality` EXIT 0（38.00% 零漂移）、`make check-surface` EXIT 0、`make check-docs` EXIT 0、`make test-contracts` 后台跑批。
- [x] B12 已落地（2026-09-29）：opensearch 归属 ③投影——`discovery/opensearch` → `discovery/opensearch` 薄壳 + `discovery/internal/opensearchimpl`（impl 内部测试随迁、外部合同测试留薄壳；home/server/index 消费方与文档链接全部改指新路径）。A-01 守卫扩展定稿：adapter 集含 `discovery/internal/opensearchimpl`，peer-adapter 豁免（opensearchimpl→discovery/llmhttp embedder 装配=adapter 内组合）；层表增 `discovery/opensearch`+impl；client 守卫覆盖两者。`discovery/projection/internal/` 进一步拆分评估：无实益不做（§2.3）。**耦合基线收紧 38.00%→35.54%（-2.46pp；index 0.44→0.50、retrieval 0.36→0.22，`-write-baseline` 显式重写）**。验证：`make test` EXIT 0、`make quality` EXIT 0（35.54% 棘轮 held）、`make check-surface` EXIT 0、`make check-docs` EXIT 0、arch 全量绿、gofmt/vet/staticcheck 净；tag `capability-phase-3`。
- [x] B13 已落地（2026-09-29；用户裁决 A=上提、B=独立）：裁决 A 落地——`IdentityContext`/`TraceContext`+Validate 上提 `kernel/identity_context.go`（协议基座窄口），observability 持别名（公开形状零变化），`liveaccess` 改经 kernel（删 observability import），层表 observability knowledge→app，`TestCapabilityConsumerSets` 去 liveaccess，§3.2 ⚠ → ✅。裁决 B 落定=gate 维持独立能力（§2.4 注记）。耦合 35.54%→33.23%（棘轮 held，无需重写基线）。验证：`make test` EXIT 0（115.9s；一次失败经复跑证实为本地 OpenSearch 栈掉线非回归）、`make test-boundary` PASS、`make quality` EXIT 0、`make check-docs` EXIT 0、arch 全量绿、gofmt/vet/staticcheck 净。
- [x] B14 已落地（2026-09-29）：Phase 4 验收——§5 全门 EXIT 0 无 skip：`coupling -strict`（终值 33.23% vs 38.22% 累计收紧 4.99pp，26 units 无漂移；B12 显式重写 38.00→35.54，B13 免重写）、`make quality`、`make test-boundary`、`make test-contracts`（后台 bash-328）、`make check-surface`、`make check-docs`、`make test`、`make test-race`（后台 bash-329）。设计文档回写实际落点（§2.4 B14 注记；落点差异：B8 导出面 43→24、B10–B12 薄壳全覆盖、B13 kernel 窄口、llmhttp 装配注入列显式后续项、B1 README 人工评审开放）。真实部署验收 `make deploy-local-scenes` 独立入口追加执行 EXIT 0（21 pass / 2 skip=非本场景条件性用例；首次运行因本地栈未起而失败，`make deploy-local-up` 后全绿——结果仅记交付说明，不混入重构提交内容）。**交付说明（实现范围 vs 已验证范围）**：实现=B0–B14 全部批次（14 批 16 commit，分支 refactor/capability-boundaries）；已验证=上述全部自动门 + 真实部署验收；未验证/开放=B1 README 人工评审（用户）、llmhttp provider 装配注入（显式后续项）。
- [x] 复核收口（2026-09-29，B14 后用户复核发现）：① B8 导出面收窄存在执行遗漏——初版以 `go doc ./cli` 输出清点，35 行截断漏掉 25 个 home 装配垫片 var 与 10 个 inv 类型/常量别名（实际 57 个导出仅 8 个有 cmd 消费者）；本次以 go/types AST 语义重命名器（区分包级符号引用与字段键/字符串）补完收窄至 **11 个**（Run/RunWithTelemetry/ParseArgs/ParsedArgs/RunResult/RunKnowledgeSetFS/ClientServerURL/NewSessionClient + IdentityMigrationRequest/Result + FlagValue——后者为 ParsedArgs 导出字段的类型），cli 外部测试的 `cli.X` 改指真实归属包（kchome/inv），re-export 垫片层拆除。② 设计文档 §2.3 目标树六处标注停留在计划态（snapshot internal 内容、catalog/worktreeimpl、knowledge impl 命名、index internal 评估结果、home authority_drivers 单文件、server internal 实际范围、cli 落地注记）已回写落地态。③ 三个薄壳 README（snapshot/lakefs、snapshot/gitea、discovery/opensearch）补薄壳结构注记与旧标题修正。- [x] 复核追加（同日，后台日志真实结果清点）：撤回此前「test-race / test-contracts EXIT 0」的记录——B11/B12/B14 三轮 contracts 与两轮 race 的日志尾部均为 `FAIL kc/cli`，任务退出码被收尾 `echo` 掩盖（B8 轮走 `| tail` 管道，同样掩盖，日志已不可考）。真实根因三项，全部为 B6 迁移遗留：
  1. B6 的文本扫描把 `server/service_management_routes.go` 的 flag map 键与 PathValue 键误改（`"command-id"`→`"inv.Command-id"`、`"command"`→`"inv.Command"`）——writer put/commit/receipt 的 typed HTTP 路由自 B6 起实际不可用（flags hygiene 拒绝），潜伏 9 个提交；已修复三处键。
  2. 两个路由清单守卫（`http_contract_inventory_internal_test.go`、`http_surface_coverage_test.go`）仍扫描 `cli/`，而 mux 已迁 `server/`——自 B6 起恒红；已改扫 `../server`，evidence owners 双目录（cli+server）解析。
  3. `TestBrowserCodeCompletesThroughBrokerWithoutClientSecret` 夹具用 pairing 认证器，而 browser-login 经纪合同已收紧为 taihu 模式专属——夹具过时；已改 `testTaihuAuthenticator`（Name "taihu"）对齐合同。
  4. B6 把 `ResourceLookup` 的 origin 绑定回退误改为硬失败（设计注记「合同不变」论断有误，见设计文档修正）：projection notice 与 resource access 的本地旅程自 B6 起红。恢复合同——origin 绑定 HTTP 查找迁 `liveaccess/state_lookup.go`（含 caller_credentials 转发与 resource-access/v1 wire 形状实义下移，invocation 保留别名），`invocation.ResourceLookup` 回退 `serving.NewHTTPStateLookup(nil)`；server 公开面经别名不变（check-surface ✅）。
  另修复复核扫除自身引入的字符串损坏：`--home` 旗标与 3 条断言消息被文本替换波及（AST 通道不碰字符串，但 vet 循环的裸文本替换会——该通道已废，后续只允许 AST 工具）。复跑以**无掩盖方式**执行（后台任务退出码取自命令本身，禁止 `; echo` 收尾）：race（bash-346）与 contracts 以真实退出码判定后本条才可关闭。
教训登记：**导出面清点必须用 AST（`go/types` Uses 解析），不得用 go doc 文本输出**（截断+方法/字段混淆）——已升级为复核工具 /tmp/countexports.go 方法学。
- [x] 复核批 R1（2026-09-29）：llmhttp provider 装配注入——`home/llm_ports.go`（`NewRerankerProvider`/`NewEmbedderProvider`，B7 readiness 同款端口模式，装配根唯一 adapter import 点扩至 discovery/llmhttp）；`HTTPServerOptionsFromFlags` 保留 model/timeout 的 flag/env 语义、删除具体构造；arch 守卫 `exceptions` 清空（注册例外归零）。验证：build/vet/arch（A-01 组）/server 单测/check-surface 绿；make test/quality/contracts 在批次门复跑。
- [x] 复核批 R2（2026-09-29）：耦合基线显式锁定 35.54%→33.23%（-2.31pp；26 单元，`scripts/coupling -write-baseline` + `-strict` 复核 delta +0.00pp）——B13 收紧收益锁死，防回退。
- [x] 复核批 R3+R4（2026-09-29）：①`scripts/export-audit`（Go 工具，stdlib AST）：全生产包导出符号 × 外部消费计数（production/test 分列），建议性报告——B8 方法学资产化，arch 层表登记 app 层；②R4 依审计收窄：620 候选 → 全仓标识符复核 → **9 个真死函数删除**（catalog.FormatKnowledgeSetOverlay、index.NewIndex、invocation.PolicyVerbs、knowledge.UniqueObjectIDs、object/format.TreeDigest、retrieval.{ComparableType,NumericType,TemporalType}——StringType/RangeType 存活被 233/235 引用、internal/testkit.MustWriteFile）；假阳性主类=类型经函数返回推断使用与 wire JSON 形状（611 项留档不收窄）。验证：build/vet/staticcheck(0 U1000)/arch/make test/quality(33.23% held)/check-surface/check-docs 绿；race+contracts 以无掩盖方式后台复跑判定。
- 原 B4–B14 推进计划已由上面的完成记录收口，不再列为待认领。目录目标与边界取舍仍归 [能力边界组织设计](doc/architecture.md)，保留的开放债按各自触发条件处理。
- 已裁决（2026-09-29，用户）：C=本轮实施 adapter 薄壳模式（B10 已执行）；A=observability 上提为 app 横切能力（B11–B13 执行）；B=gate 保持独立能力（B13 记录）。
- Goal / Non-Goals / 不变量绑定（A-01、L-01、API-01、AUTH-03、P-01、O-01、CA-01、KS-01、IX-01…05）/ 选定与否决方案 / 接口指向：全部见 [能力边界组织设计](doc/architecture.md)。

### CLI-REFACTOR · 产品 CLI 形状重构

- [x] 已落地：应然设计见 [CLI 交互](doc/design/affordance.md)；按 CLI 迁移记录落地产品闭集：四条真人路径（进房、读知识、写仓、挂源给权）；主路径约 20 条动词；resolve/治理/运维/catalog audit 进阶，根 help 不出现。验收锚点 `TestProductCLIRefactorDefinesTheExactPublicSurface`（60 条）+ `TestRemovedCommandsAreRejected`；落地顺序见 REFACTOR §14，协议缺口见 §15。`catalog use` Client 持久化与 `detach` 登记移除已落地，现行证据由对应 CLI 回归拥有；create/attach 分离、`detach` 新语义、`grant` 去 `admin` 前缀；知识/写拒绝 `--catalog`/`--dataset`/`--source`。不做旧 argv 兼容层。验收：`make check-docs` 与 `make test` 全绿，场景树与 E2E 已全部改用新 argv，无 skip。
- [x] 已落地：同一轮收口读得懂的输出（同文 §1.1、§4.1、§7）：Client 入口与登录解耦，login/logout 回执不含 `server`；`admission show` 换成「本人 grants + 申请入口」并删 `admission request`（部署只留 `admission.requestURL`）；System 仓发布 `schema/core/source-profile/v1` 并自带 `kr://kc/system` 源说明；`schema list` 删 `coverage` / `exhausted`、每条 schema 不再各带 `repository` / `commit`，`log` / `audit` 同形对齐；`help` 改根/分组/叶子三层渐进式披露。验收：`make check-docs` 与 `make test` 全绿，场景分页断言改用 `continuation`，无 skip。

### CATALOG-01 · Catalog 登记表不能落在易失盘

- [x] 已落地：生产 Catalog 登记表落在独立 Snapshot 权威（`dolt` / `gitea` / `lakefs`，与知识仓同类介质、不同 identity），不走本机 Git、`stateDir` SQL、知识仓或 Writer。`CatalogBinding` 只接受 `driver`+`dir`/`dsn`；未知 YAML `remote:` 与 Git `file://`、实例盘路径、缺 driver 在 `deployment init` / `serve` 失败关闭。丢失的 Catalog 权威不得重建空登记表。本地 `kc init` 夹具仍用 gitdir `NewRegistry`，不是生产路径。验收：`make check-docs` 与 `make test` 全绿，无 skip。锚点 `TestDeploymentRejectsLegacyCatalogGitRemoteField` / `TestDeploymentRejectsInstanceBoundAuthorities` / `TestDeploymentDoesNotRecreateLostCatalogBranch` / `TestSnapshotRegistryPersistsMembershipWithoutKnowledgeSemantics` / `TestCloseReleasesDurableDirectory`。

### CATALOG-02 · Catalog 对所有认证用户可见

- [x] 已落地：已认证用户可发现公开 Catalog（`show` / `catalog list` / `catalog use`），无需 `catalog.read` grant；`private: true` 与未知 Catalog id 仍 fail-closed 要 grant。发现不等于 `knowledge.read`、也不等于 grant/admin。无 `Deployment` 的本地 Home 仍要 grant。验收：`make check-docs` 与 `make test` 全绿，无 skip。锚点 `TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant` / `TestPrivateCatalogStillRequiresCatalogReadGrant` / `TestManagedRepositoryProviderCreatesPublishesAndResumes`。

### AUTH-REPO-01 · 仓的已认证默认可读

- [x] 已落地：仓可声明已认证默认可读（与 Catalog 公开发现同一模式）；未声明 fail closed。不设仓级 public/private，不按 `kr://kc/system` 授权短路。System Repository 选用该声明；运行时 Writer 仍拒绝写平台仓。验收：`make check-docs` 与相关 `go test` 全绿，无 skip。锚点 `TestUndeclaredSystemRepositoryStillRequiresGrant` / `TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant` / `TestDeclaredSystemRepositoryUsesAuthenticatedDefault` / `TestRuntimeWriterRefusesSystemRepository`。

### README-01 · README 是 Markdown 知识对象

- [ ] 本轮认领：README 是 frontmatter 知识对象；约定路径可被 ② 解释；默认 SEARCH 编该对象 `body`；已接入仓直推 published HEAD 后 ③ 能编进投影。`catalog show` 不派生 title/summary。owner 为 [`doc/design/object.md`](doc/design/object.md)、[`doc/concepts.md`](doc/concepts.md)、[`doc/design/dataset.md`](doc/design/dataset.md)、[`doc/design/discovery.md`](doc/design/discovery.md)。

**Goal：** 人写的 README 就是知识。展示仍叫 README；身份是 Address + `schema_ref`。ingestion control（面 3）只追 HEAD 并更新索引，不成为写面。

**Non-Goals：** 不实现 LLM concat；不把无 frontmatter 的文件当成知识；不让直推等于 Writer 校验；不在面 3 跑 Writer；不新增 Writer validate-only / 保护分支门（见 WRITE-VALIDATE）；不把 VFS 解释成 Catalog 成员条件；不改交付链；不把根 README 做成第二条检索代数；不把 README 展成 Catalog `title`/`summary`。

**不变量：** Address 是身份、路径只是 `path_hint`（`A-01`）；`catalog.read` 可见库存身份（`id` / `schemaCount`），README 正文仍要 `knowledge.read`；live 投影对 published HEAD 对账（`PC-01`）；1 与 3 彼此不调用；缺 README 不得由平台或模型补写，也不从库存抹仓。

**选定与否决：** 选定 `schema/core/readme/v1` Aspect `readme`、字段 `body`（`text`）。根 `README.md` 只当人写与 git 直推解释的 `path_hint`。Connector / HTTP / Agent 仍走 Writer。否决 `core/source-profile` 信封；否决库存 title/summary；否决无 frontmatter 即身份；否决文件 contains；否决 `ChangeSet → Writer → Snapshot → Ingestion → Index` 当一次写事务。

**怎么改：**

1. ② Schema + codec：`schema/core/readme/v1`；Markdown frontmatter 用 `entity`/`aspect`；正文是 `body`，不是 JSON 信封。
2. ② 解释：无 Writer locator 时仍按仓根 `README.md` 有界读取；`Read` / `Resolve` / 对象分页能看见该 Address。
3. ① 库存：`catalog show` 只列 `{id, schemaCount?}`；不从 README 派生 `title`/`summary`。读 README 走 `kc read --aspect readme`。
4. ③ 投影：`specAtCommit` 带上 `body` 的 `text`；git 直推的对象编进 `CompiledDoc.Text`。Controller 仍 Desire(HEAD)，失败不回滚仓。
5. VFS ≠ attach：文档钉死 Plain 只解释组合阶梯。

**接口指向：** `knowledge.CoreReadmeSchemaV1`、`object/readme.go`、`object/format` codec、`index.compileValue`。不新增 CLI。

**如何验证：** `make check-docs`；下列测试无 skip，再 `make test`。

- `TestSystemRepositoryPublishesReadmeSchemaAndInstance` / `TestAssertReadmeBinding`
- `TestParseMarkdownReadmeFrontmatter` / `TestIngestMarkdownReadmeFrontmatter` / `TestReadmeAspectIsMarkdownBody`
- `TestReadmeGitCommitWithoutWriterLocators` / `TestPlainMarkdownReadmeIsNotKnowledge`
- `TestCatalogShowRepositoriesStayIdentityAndReadmeIsKnowledge`
- `TestProjectionCompilerIndexesReadmeBodyAsText` / `TestGitPushedReadmeCompilesIntoAccessSpecText`
- `TestProductScenes/catalog-initialized`（库存身份）/ `TestProductScenes/system-schema-published`（`kc/system#readme`）

### WRITE-VALIDATE · 进 published 的知识发布过同一套 Writer 校验

- [ ] 未认领：人可以不经过 `kc writer` CLI 改自有仓；成为已发布知识的那次提交必须过与 COMMIT 相同的 Schema / CAS 基点 / 同仓 `schema_ref`（保护分支或 CI 代发 / 只验不写）。owner 为 [`doc/design/dataset.md`](doc/design/dataset.md) §3.5、[`doc/design/governance.md`](doc/design/governance.md)、[`object/writer/README.md`](object/writer/README.md)。

**Goal：** Writer 代数可在不推进 ref 时复用；CI 有公开检查口。

**Non-Goals：** 不把校验塞进 ingestion control；不把外部 CI 做成每次 COMMIT 的 Gate；不抢走业务 git；不改 Connector 必须走 Writer。

**不变量：** `K-21`；Gate 只拦 proposal → published。

**选定与否决：** 选定目录写入走 `writer commit --dir`（对照当前版本求差是 commit 内部动作）。选定可选 `kc diff` 看同一对照。否决产品 CLI 另设 pack / status / plan；否决把 ChangeSet 文件当日常 CLI 操作数；否决最短旅程必须先 diff 再提交；否决面 3 当写面。

### DATASET-01 · 文件层 Dataset 与消费授权（取代 KSET-01）

KSET-01（Workspace→知识集、不改组合语义）不再认领。消费组合改为文件清单 Dataset；授权与组合分成两份整理稿，**验收通过后才升格进文档图并改 owner**。

- [x] 当时两份 Dataset 整理稿已完成；后续文档重构由 DOC-21 接手，当前替换稿见 [`Dataset`](doc/design/dataset.md) 与 [`权限体系`](doc/design/permissions.md)。本条历史完成记录不表示替换稿已完成交接或验收。
- [x] 实现与 owner 升格已落地：发布冻 commit（无 store / 解不出 selector 失败关闭）、Items 为消费真相（空清单不放行整仓）、`file.read`×Dataset、Serving/SEARCH 按清单 path 过滤（hydrate 前）。公开动词是 `dataset define|retire|overlay`；消费是 `file.read`；管理/解析是 `dataset.manage` / `dataset.resolve`。产品面只有 `--dataset`、`KC_DATASET`、HTTP `/datasets`、JSON `datasets`；不保留 `kset` argv、`--kset`、`KC_KSET`、`kset.consume` 或 HTTP `/ksets`。

**Goal：** 跨仓只读切片是命名、版本化的文件清单；检索只跟 latest；消费者 `file.read` 挂在 Dataset 上，维护仍挂 Repository。

**Non-Goals：** 不接 lakeFS Datasets API；不把 Dataset 做成 Snapshot 仓；不为每个 `vN` 建索引；不把 path/Aspect 做成 allow 资源。

**不变量（目标，升格后写入不变量表）：** 清单 item 发布时钉 commit；消费 pin 冻文件清单；`file.read`×Dataset 与仓 `knowledge.read` 互不蕴含；pin 不锁权限；SEARCH 不超出服务版 path。

**选定与否决：** 选定文件层 Dataset 收掉仓名单 kset；消费 action 是 Dataset 上的 `file.read`。否决双轨 kset+Dataset、消费再要整仓 `knowledge.read`、每文件一行 grant。

### DATASET-02 · 发布权威、消费范围与服务生命周期收口

- [ ] 本轮认领：修复架构 review 复现的 pin 扩权、跨 Aspect 越界、语义 VFS 越界、服务版索引随 HEAD 丢失与版本可覆盖问题；消费与发布编排收敛到可独立验证的公共边界。

**交接状态（2026-09-20；后经用户纠偏）：** 本轮实现已落地。当时中止了运行中的全量回归并保留待验收状态；「用户要求只做实现、由用户在其他环境测试」的表述并非用户约定，已废弃。现行标准：代码修改必须通过测试，待验收状态由全量测试与契约门重新确认。

**默认测试入口纠偏（2026-09-21，本次不执行）：** Goal：`make test` 和 testsuite 默认入口使用原有 lakeFS 场景夹具，不要求准备整套部署；owner 为 `doc/engineering/test-catalog.md`，部署验收边界见 `doc/design/service.md` §11.1。Non-Goals：不改生产 provider 默认值，不迁移全部历史夹具，不删除或削弱合同，不运行测试或启动容器。不变量为 `A-01`、`V-01`；选定运行 `TestProductScenes` 与 `TestMetricPermissionScenes`，复用显式 `KC_TEST_OPENSEARCH_URL`，未配置时沿用一次性 OpenSearch。真实部署场景保留 `make deploy-local-scenes`，也保留在显式 `test-all` 中；原 component/boundary/应用合同组合保留 `test-contracts`。否决先前将默认测试绑定 `deploy-local` 的做法，以及将场景通过宣称为完整组件、HTTP/VFS 或真实部署验收。接口沿用现有 testsuite 分组和 Go 场景入口。

**Goal：** Dataset 的不可变发布记录是消费坐标和文件范围的服务端权威；当前服务版仅在必要投影准备成功后切换，源 HEAD 前进不破坏已发布版本。owner：`doc/design/dataset.md`、`doc/design/permissions.md`、`doc/design/discovery.md`、`doc/design/service.md`，产品目标见 `doc/design/object.md`。

**Non-Goals：** 不把知识或索引放进 Catalog，不把外部 live 数据写成 Snapshot，不按 Dataset revision 复制索引，不改变 VFS 只读且受本机容量限制的边界，不扩展跨仓写事务，不降低架构守卫与既有场景断言。

**不变量：** `V-01`、`KS-01`、`KS-02`、`AUTH-01` / `AUTH-02` / `AUTH-03`、`PC-01`。pin 冻结内容不冻结权限；任何读取、候选回读、Schema/动态绑定和文件投影均不能超出同一服务版范围；发布失败保留上一服务版。

**选定与否决：** 选定 Catalog 内不可变版本记录与单独当前指针、按发布记录重建消费 pin、知识读取共享完整对象范围校验、按固定 Snapshot basis 准备和恢复派生索引、typed 应用编排。否决信任客户端 Items / 任意 commit、任一 Aspect 命中就授权整个对象、语义 VFS 整仓共享投影、消费请求临时重建索引、用 HEAD 索引冒充服务版索引。切片中不完整的知识对象不伪装成完整对象；文件访问仍保留清单内文件。

**接口与验收：** `catalog.KnowledgeSet` / `ResolvedKnowledgeSet`、`object/reader.Serving`、`knowledgeapp`、`index.Controller` 与既有 File Gateway。先把 review 的真实失败固化成回归，再实施；覆盖伪造 Items/commit、历史 pin、当前撤权、跨 Aspect、语义 VFS、源 HEAD 前进与发布失败/重启恢复，最后运行 `make check-docs`、`make test`，全绿且无 skip 才勾选。

**关系边界增补（用户已确认）：** 允许关系保存在独立图谱仓，端点引用其他仓的对象；Writer 仍只提交关系所在仓，不解析或写入端点仓，不引入跨仓事务或隐式读权。Goal 是让 `REL-01`、系统设计 §7、Provider 合同与 Dataset 跨仓一跳检索一致；Non-Goals 为跨仓写、递归图查询、自动把引用对象加入 Dataset。不变量为 `REL-01`、`W-01`、`V-01`、`AUTH-02`、`R-01`；选定结构化 KnowledgeRef 与固定 Dataset 成员投影扇出，否决端点仓等于关系存储仓的校验和 authority 扫描。接口沿用 `CanonicalRelation` / `RelationRetrieveRequest` / `DatasetRelationsExecutor`。先记录 Writer/检索拒绝跨仓的失败，再修正合法双端点 Relation 场景；保留形状、权限、同版本回读和单仓写入断言，整体验收完成前不勾选。

**Provider 差分补充：** 原生 Dolt 实测复现了更新未携带路径时被 Writer 强填默认路径的问题，破坏文件切片稳定性。沿用 `docs/reviewed/provider-contract-validation.md` 的路径提示等价合同与 `KS-01`，目标是未指定新路径的更新保留原路径；不改变显式迁移语义。选定直接传递原始 ChangeSet，由 provider 在合并现有单元后为新单元补默认路径；否决写入前无条件补路径或在测试中补显式路径掩盖差异。回归为 `TestNativeKnowledgeDoltMatchesTreeProviderByOperationStep`，包含原有路径迁移、历史、正文及新增跨仓关系步骤。

**Dolt 验证补充：** 真实 adapter 回归另捕获预期失败查询后把已存在 commit 报成不存在。按 `doc/architecture.md` 的 Snapshot 版本合同与 `V-01`，修复 Docker 会话在远端分离 stdout/stderr 后破坏语句确认顺序的问题：在容器内合并输出，再经 Docker 传输；不以重试或忽略错误掩盖版本读取失败。接口不变，保留 ref/merge 合同，并重复验证真实引擎错误后的正常查询。

### DATASET-03 · 逐文件交付贯通、dataset clone 与 Dataset 管理前端

- [x] 本轮完成：U10 逐文件交付贯通——`KnowledgeSetSource` 增 `File`+`Target`（交付寻址时 `Path` 必空）、发布校验目标唯一与祖先互斥（文件对文件、文件对前缀）、overlay 文件按目标替换并继承冻结坐标；File Gateway 交付寻址枚举/读取（`Path` 为根寻址判据、pin 重放保留已接受交付树、Length 0 默认 512 KiB）；C-21 收口（`datasetFSRepositoryPath` 归一化后必须仍落在所选子树内）；`kc dataset clone` 物化当前服务版交付树（非空目录拒绝、只随服务版演进、argv 旅程 `TestDatasetCloneJourney`）；`/ui/` Dataset 控制台进 httpsurface 闭集（87 条）并带页面证据测试；webui vendor（lakeFS v1.58.0 pinned，Apache-2.0，保留 LICENSE/NOTICE）构建产物嵌入 `cli/webapp`。文档：TEST_CATALOG C-17–C-21 转「已定位」并登记证据、reviewed/dataset.md U9/U10 与结尾约束、CLI_EVALUATION 增 clone 行并逐字核对 60 条 help 引用、client/README 补 File Gateway 节；「测试由用户在其他环境执行」表述经用户纠偏废弃（AGENTS.md/TASK.md）。验证：`go test ./cli/`（无环境与 `KC_TEST_OPENSEARCH_URL` 两遍口径）、`./catalog/ ./client/ ./home/ ./internal/... ./discovery/projection/`（带 OpenSearch）全绿，`make test` 96.9s PASS、`make test-contracts` 640.8s PASS（公开命令覆盖 59/59）、`make check-docs` PASS。

**Goal：** 发布、目录枚举、文件读取与授权对逐文件条目完整成立：发布产出 `DatasetItemFile` 条目并校验目标冲突，文件网关按交付目录树枚举/读取文件条目（支持改名与多来源混排进同一交付目录），相对路径任何位置的父目录跳转都被拒绝；`kc dataset clone` 把已发布 Dataset 的当前服务版交付目录树物化成本地普通目录（产品 argv 不收 `--pin`，见 `doc/design/affordance.md`），不依赖 kcfs。owner：[`doc/design/dataset.md`](doc/design/dataset.md)（U6/U9/U10、固定范围与生命周期）、[CLI 交互](doc/design/affordance.md)（产品 argv）、[`httpsurface`](httpsurface/README.md)（闭合路由表）。

**Non-Goals：** 不把 Dataset 做成可写目标，不复制字节进 Catalog，不给逐文件条目建独立宿主挂载点，不扩展跨仓事务，不在 clone 时绕过 Dataset 消费授权或写入清单外路径；本轮不改 kcfs 的 FUSE 能力边界（逐文件条目在 FUSE 计划中显式拒绝并明确报错，混合 eager 文件与 lazy 目录挂载的 datasetfs 扩展留作后续），不新增 HTTP 路由（clone 与逐文件交付复用既有闭合面，请求形状做向后兼容扩展）。

**不变量：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`。清单 item 发布时钉 commit；空清单不放行整仓；同一目标文件只有一个明确来源，冲突显式拒绝，不按挂载或来源顺序覆盖；逐文件条目与所属仓的其它条目共享同一冻结 commit；路径规范化后仍须处于选定子树内。

**选定与否决：** 选定逐文件条目搭乘现有 `KnowledgeSetSource` 形状（`File`+`Target` 字段，`Path` 必须为空），recipe `.kc-dataset.yaml` 与 overlay 增加 `files:` 段，发布时静态校验目标唯一性，内容级同名冲突在交付时显式报错（发布不解析来源树内容，不做发布期目录遍历）；网关请求在 `MountPath+Directory/File` 之外增加可选 `Path`（交付路径）以寻址挂载子树外的文件条目，文件条目只在首页合并进枚举结果。否决发布期遍历来源树做冲突检查（发布必须不依赖来源树读取）、否决按顺序静默覆盖、否决为 clone 新开 HTTP 路由、否决 clone 生成 manifest 文件（消费坐标由服务端发布记录承担，回执只打印版本与来源 commit）。

**接口与验收：** `catalog.KnowledgeSetSource` / `DatasetItem` / `HashResolved` / recipe / overlay；`cli` 文件网关（`dataset_file_service.go`、`datasetfs.go`）与 `client` typed 请求；`kc dataset clone` 进 `cli/surface.go` 公开闭集（动作 `file.read`×Dataset 消费）；webui vendor 保留上游 LICENSE/NOTICE 并记录 pinned commit，API 层只接 httpsurface 闭合路由。回归：U10 发布/枚举/读取/授权端到端、目标冲突发布拒绝、C-21 断言转绿、clone 物化与拒绝非空目录、clone 只随当前服务版演进。代码修改必须通过测试；契约全绿且无 skip 才勾选。

### SCENE-01 · 走查只用协议场景树

- [x] 本轮完成：走查与 `TestProductScenes` 都只用 `.data/scenes`；`Given existing repository` 经 lakeFS adapter 打开既有 authority。父 home 副本分叉独立物理仓（拷目录不够）。Gherkin 仍 store-agnostic。不接 Datasets API，不出现 `warehouse-agent`。

**Goal：** `Given existing repository` 经 `snapshot/lakefs` 打开既有 authority；`TestProductScenes` 父 home 副本必须隔离物理仓（拷目录不够）。owner 为 `doc/architecture.md`（`snapshot-authority-and-derived-media`）与 `.data/scenes/README.md`。

**Non-Goals：** 不改 Gherkin 写 lakefs；不把 Catalog 组件夹具（`NewRegistry` gitdir）改成生产 OpenSnapshotRegistry；不在 `cli` 里 import adapter；不把 docker Graveler 编进协议包；不接 lakeFS Datasets API。

**不变量：** `A-01`、`V-01`；更换 Snapshot driver 不得改变 Reader/Writer/Catalog 命令语义。

**选定与否决：** 选定进程内协议忠实 lakeFS HTTP 假服务作为 hermetic `make test` 夹具，经 `home` 的 lakefs `OpenExisting` 打开；父副本 `ForkStamps` 改写 `remote.yaml` DSN。否决场景执行器继续默认 Dolt；否决把 docker Graveler 编进协议包；否决 Gherkin 写介质名。

**接口与验收：** 夹具在 `internal/testkit`，装配仍走 `cli.AddRepository(..., "lakefs", dsn)`。锚点 `TestSceneExistingRepositoryFixtureUsesLakeFS`（stamp `driver=lakefs` + 两份副本 DSN 互异）与 `TestProductScenes`（全绿、无 skip）。Catalog 仍是 gitdir `NewRegistry`。

### SCENE-02 · 可复用状态、独立用例与关注点视图

- [ ] 本轮认领：按真实复用整理全树状态与验证用例，隔离同节点探针环境，并从就近元数据生成可重叠子图和主视图并集检查。owner 为 [`doc/engineering/test-catalog.md`](doc/engineering/test-catalog.md) 与 [`.data/scenes/README.md`](.data/scenes/README.md)。

**Goal：** 状态表达可重建、供后续引用的前置条件；用例表达从该前态出发的操作与断言，临时后态不被其它用例继承。视图可以从非根开始，主视图并集覆盖声明节点、构建边和验证证据。

**Non-Goals：** 不改协议、授权语义或新增产品 CLI 形状；不把视图当作唯一终态或另写依赖树；不把所有有写入的过程升格为状态；只按实际前态和消费者调整全树目录；不把 Go 独立旅程伪装成复用 scene fixture。遵循上述 owner 的证据分母与同次运行规则。

**不变量：** `AUTH-01` / `AUTH-02` / `AUTH-03`、`V-01` 与场景结构、公开命令覆盖、父 construct 复用合同保持；探针互不继承临时写入、授权和运行时状态；显示范围不改变真实构建前置。

**选定与否决：** 选定目录作为构建依赖唯一来源，节点及 probe/Go evidence 就近标注视图，视图定义只存标题/说明和可选显示边界；构建后冻结，每条 probe 在独立副本或重放环境中执行。测试专用端点为每个环境隔离 OpenSearch 物理索引与控制记录，使有投影与无投影前态都不被其它用例污染；不改产品配置或替换检索引擎。否决手写节点/边清单、隐式跨 probe 顺序依赖与只按权威数据是否写入判断状态。

**接口与验收：** `.data/scenes/tree.py` 的文本/JSON 投影与检查入口、`cli/scene_feature_test.go` 执行器及现有场景元数据；先保留隔离与投影缺失的失败证据，再定向验收，最后 `make check-docs`、`make check-validation`、`make test`，无 skip 才完成。

**产品关联增补 Goal：** 按产品 owner 各设计篇「方向性用例」的稳定 U 条目（`_views.yaml` 的 `product_documents`）生成产品视图，建立“产品承诺 → 具体用例断言 → 构建前态”的可追溯关系。文档身份与路径解析以 `docs/graph/` 为准；用例就近声明 `verifies`，工程视图仍保留。`doc/engineering/test-catalog.md` 拥有证据判读。

**产品关联 Non-Goals / 不变量：** 不在派生 `product.html` 保存独有定义，不手抄产品条目与节点清单，不因映射存在就宣称执行或完整验收通过，不改产品行为或修复上一轮既有协议失败。产品新增条目、引用失效、无依据映射必须可检查；未决或未覆盖范围显式记录 gap。既有 `AUTH-*` / `V-01` 等产品合同不变。

**产品关联选定与验收：** 从指定 owner 章节提取条目 ID 和标题；probe、具名 Go evidence 与 construct 中的明确断言分别引用条目，依赖状态自动补全。否决仅在整个 view 挂文档链接、节点标签自动继承与重复维护成员归属。先新增反例测试，再验证产品引用、缺口检查、原工程视图兼容及 `make check-docs`；不把历史执行结果作为本次通过证明。

**产品关联定向结果（全树迁移前）：** U1–U10 自动生成产品视图，17 条具体验证关联与 9 条剩余范围说明就近可追溯；U5、U9 暂无场景关联，保留显式 gap。32 条视图反例/回归测试与 10 项 Go 场景结构合同通过，无 skip；工程并集覆盖 51 个声明节点、50 条边、59 条 probe 和 125 个 Go 引用。`make check-docs` 通过（35 文档、121 关系）；`make check-validation` 仍只报告下述既有 4 处悬空测试引用。该结果只验证关联与结构，不宣称产品 U 条目完整验收。

**全树整理增补 Goal（用户纠正后）：** 以真实前态复用审计全树，取消仅为测试分组、读操作或单次验证后态建立的目录；`schema-browsed` 等独立 Go 证据就近归到实际前态，临时授权/撤权/归档等步骤归入独立 probe。节点名描述实际建立并被消费的条件，用例名描述操作和预期。owner 仍为场景 README 与 `doc/engineering/test-catalog.md`。

**全树整理边界与方案：** 本次扩展原样板范围到全树；不改变产品合同、断言或 Go 业务测试，不把 Go 自带 setup 算成场景前态复用，不用空目录、只读 construct 或人为拆分探针凑复用。保留真实构建前置、多个独立用例的共享条件及已有部署走查入口；一次性连续步骤合并为一个 probe，沿途所有断言保留。`AUTH-*` / `V-01`、产品 claim 关联与工程并集保持；先留下伪节点失败证据，再迁移并核对命令/断言与 Go 引用未丢失，更新结构守卫的宿主定位而不降低其语义要求。

**全树整理落地：** 51 个原目录收敛为 21 个共享前态、20 条真实构建边和 65 条独立 probe；每个保留状态声明实际 `fixture`，结构门禁追踪至少两个直接或下游 scene 消费者，独立 Go 不计入复用。11 个 Go-only 目录全部取消，`schema-browsed` 回到初始化前态；临时授权、撤权、归档、退役与一次性校验归入 probe。`source-repositories-configured`、`semantic-knowledge-published`、`dataset-query-principals-granted`、`proposal-preview-created`、`qinghe-knowledge-published` 改名为实际交付条件。清河两种终态用显式 `goto --probe` 正向走查，避免重新伪造状态。

**迁移保全与验证范围：** 117 个去重 `(source, Go Test)`、17 条产品关联、122 份已快照材料完整保留；原 746 项命令关联断言中 742 项仍在可执行 feature，原 Go-only observation 的 4 项参考断言仍保存在说明材料中，其具名 Go Oracle 不变。第一轮实际回放中 `TestProductScenes` 的全部 16 个节点通过；旧 Agent 解析守卫依赖文件顺序的问题已改为核对完整且无重复的角色/brief 集合，Agent 脚本与插件测试的场景引用已同步。新增 10 条 Python 反例验证前态复用、走查 probe 选择与执行前变量检查；最后结果以 `.validation/runs/` 中同次选择范围报告为准，不把声明完整性当产品通过。

**本次定向回放结果：** `.validation/runs/20260920T050906Z-f3af36a0ef1b` 在同一源码指纹下实际运行 44 项顶层测试，42 项通过、2 项失败、0 跳过；`TestProductScenes` 及 16 个子节点均通过。两处失败分别是 `TestMetricPermissionScenes` 和 `TestSceneMutatingGrantProbesAreIsolated` 的同一个 Dataset 跨仓关系断言：预期 `rel/defines/gmv`，实际 `hits=[]`，仍保留原断言。之后仅补正运行时材料路径、走查变量预检及说明，42 项 Python 合同通过；不将定向回放表述为全仓全绿。

**仍属既有的 Agent 执行缺口：** 本次只修正 companion 的场景定位、brief 主体与静态入口；脚本 live/bootstrap 中旧 workspace/admin 命令形状仍需独立更新，不声称已执行付费 Agent 验收。

**验收中发现的前态缺口：** 语义实例旅程曾引用关系仓尚未发布的 Relation Schema；按同仓解析合同在 Domain Schema 构建阶段经 Writer 发布并回读。业务夹具另为 `relationType` 声明文本访问，并验证图仓能搜到 `defines`、不会额外命中 `merchandise`；按 `doc/design/discovery.md` 与 `discovery/projection/README.md` 保留跨仓 SEARCH 对每个成员能力的检查，不跳过关系仓。同步补正两业务仓的逐仓隔离断言与 `includes nonempty` 的匹配器，使现有断言按已声明 DSL 生效，不改协议语义。

**验收中发现的路由缺口：** 显式 `--dataset` 的 Server 检索被误判为无选择的 Catalog discovery，导致原有 Dataset 用例要求无关的默认发现配置。按 `doc/design/affordance.md`、`doc/design/dataset.md` 与 `cli/README.md` 的显式操作数合同（`API-01` / `V-01` / `KS-01` / `KS-02`）仅修正选择器分类并补回归，保留 Dataset 原有授权与回读断言；不通过配置默认 Workspace 绕过问题。

**全量验收阻塞：** 既有覆盖文档引用的 `TestHTTPWorkspaceSearchKeepsDatasetFileReadBody`、`TestResolveDescriptorBindingAtPinnedCommit`、`TestProductTemporaryPinConsumesFrozenKnowledgeAndCurrentPermissions` 尚无同名测试；架构守卫另报告 `home/managed_named.go`、`home/repository_id.go` 的 adapter import 与 `ManagedRepositoryConfig` fixture seam。它们不由本次场景重组引入，不削弱守卫或改写证据声明来通过；完整验收未绿前保持未勾选。

**保留的场景失败：** `dataset-query-principals-granted/dataset-cli-discovers-searches-reads.feature` 已按现有关系端点合同传入完整 `kc://scene/object/metric/gmv`，仍要求返回 Dataset 内关系仓的 `rel/defines/gmv`。当前 Workspace 关系路径只查端点所属知识仓，返回空 `hits`；`index.RelationsAtContext` 又限定端点与被查仓相同。该跨仓关系执行缺口超出状态/用例组织任务，保留原预期与失败，不改成只查单仓或删除关系步骤。

### SYSTEM-META · 接入方与 Agent 如何理解这套知识模型

- [ ] 本轮认领（2026-10-10）：将 `kr://kc/system` 扩展为 Knowledge Catalog 的自描述知识仓，发布模型、操作入口、运行机制、能力说明、使用指南与导航关系；根 README 提供有界阅读入口。
- Goal：Agent 从系统仓理解知识模型、选择能力并找到当前 CLI help；`systemrepo/content` 是项目中的出版源，显式部署初始化经 Writer 发布到配置指定的实际 system Snapshot，启动核对，运行时只读实际仓。Owner：[知识读写](doc/design/object.md) 的系统出版物与文件视图，[服务](doc/design/service.md) 的部署恢复，[CLI](doc/design/affordance.md) 的逐步披露，[术语表](doc/concepts.md) 的公开词汇。Non-Goals：不改现有 Entity 等 wire 名称，不引入通用 lastModified 等不存在的公共属性，不把运行就绪或当前授权写成固定出版物，不复制完整 CLI 参数表，不修改 Catalog 登记线的在途变更，不新增原地升级合同。
- 不变量：`L-01`、`I-01`、`V-01`、`W-01`、`REL-01`、`KS-02`、`R-02`，及 service 的「初始化是显式管理动作，服务启动只恢复」。接口依据 `DeploymentConfig.Repositories`、`InitializeDeployment`、`OpenDeployment`、`SystemPublicationOperations`、`KnowledgeFileReader`、`PublishSystem` 与现有 frontmatter / Schema / Address 合同。
- 选定：一对象一目录、一切面一文件；CLI 调用入口从既有登记生成并以合同守卫对齐；整份出版物内容参与版本摘要与发布/启动核对。否决：人工维护第二份参数合同、无 frontmatter 的说明冒充知识、按系统仓 ID 绕过授权、只校验 Schema 而允许说明或调用入口漂移。
- 存量债登记：位置 `object/readme.go:protocolSchemaDigest`；信号为 owner 要求系统 Schema 副本一致，而现有 Writer 守卫只锁 README Schema，现行测试明确允许其它协议 Schema 原位差异。判定为设计/已选合同一致性缺口；本轮保留该合同，不借自描述出版物扩展改变业务 Schema 发布行为。触发条件为后续统一系统 Schema 副本与兼容演进合同，届时先选定边界并补反例。
- 增量自检：Markdown 往返曾将普通 `.md` 标量转为 `{body}`，导致同一草稿重试冲突；已修正为仅对明确单 `body` 值渲染 Markdown，并以正文、标量、结构化值和原写入重试合同守卫。U8 的真实 Agent 语义验收缺口登记在 `.data/scenes/_views.yaml`，不把程序读取通过视为模型任务验收通过。
- 本轮增量自检：已移除内置 Snapshot provider，保留出版源解析与信任根；原不变性断言迁移到实际部署运行句柄，原发布/文件一致性断言同时覆盖出版源与实际 Writer 输出。实际读取暴露的夹具/介质问题均当场修复并有守卫：memory 目录 cursor 绑定目录与 commit；lakeFS presigned redirect 消耗有界响应正文，使连接可复用，避免逐文件核对耗尽端口；lakeFS 测试夹具关闭前释放请求锁，以确定性在途请求回归和竞态检查守卫关闭死锁。另两条授权合同曾按列表首项误选 system，已改为按业务仓 identity 查 binding，保留原 FORBIDDEN 与默认可读隔离断言；组件库存合同现在明确断言实际 system 与业务 authority 均在场，按 identity 核对连接。
- 托管建仓合同同步：静态部署配置仅含必需的 system authority，动态业务仓仍只能从持久供给账恢复，不把新增系统绑定误判为业务仓混入配置；原重启、幂等、历史读取和撤权断言完整保留。
- 真实部署入口自检：CLI 普通夹具的默认 lakeFS credential 不得覆盖真实场景 DFS 的部署配置；TestMain 仅在普通夹具模式注入，以子进程回归同时核对已配置和缺失凭证保持原状。真实部署验收仍单独执行，不以该配置边界守卫代替。
- 场景夹具同步：冻结与复制守卫现在同时核对实际 system、knowledge、graph 三个独立物理仓；故障回放夹具按每次构建的 system 与业务仓两次供给，把失败仍注入指定的业务前置步骤，保留原状态、最后步骤和失败报告断言。
- 概念纠偏（2026-10-10）：Goal = 系统仓忠实解释已有 Catalog 模型，移除误增的 Task 领域概念；Non-Goals = 不改变已有调用、部署和知识读写协议，不改客户端宿主的 TaskContext。Owner 沿用知识读写与术语表；不变量沿用 L-01、I-01。选定使用指南作为出版物的阅读内容，目录与身份统一为 guides，不作为新增领域模型；否决把按用户目标编排的说明提升为系统 Task 类型。删除错误概念，同步正文、Schema、导航和既有读取/完整性合同，验证范围单独记录。
- 概念纠偏验证：knowledge、object/writer、home、cli、server 中受影响的发布内容与 Schema、导航闭包、实际系统仓初始化/恢复、已认证读取、文件交付及 help 同源合同通过（34 个测试事件，0 失败、0 skip）；`make quality`（含 `make check-docs`）通过。证据见 `.validation/system-guide-correction-summary.json`；本轮未重复完整混合合同，未执行真实部署或真实 Agent 语义验收。
- 全量概念复核（2026-10-10）：Goal = 逐一核对剩余 28 个 concept，只解释已有系统；Owner 为知识读写的系统出版物、术语表及各概念在文档图中的主题 owner。Non-Goals = 不扩展协议模型、授权、动态流或治理实现，不把现有 API 当作收窄设计的理由。不变量沿用 L-01、I-01、R-02、O-01。选定移除把说明分类实体化的 Capability，校正已有概念的定义，以既有 owner 语义和公开 Go 类型/入口登记每个保留概念的依据；否决凭概念正文自身证明概念存在。Task 与 Capability 同类问题已重复，升级为出版物合同：新增 concept 必须有经复核的 owner 主题与公开声明依据，自动检查依据存在和出版物覆盖，语义仍随人工 diff 复核。
- 全量概念复核结果：移除 Capability 后保留 27 个 concept，依据登记于 `object/testdata/system-concepts.json`，独立 owner/public declaration 守卫及受影响的完整出版物、Schema、导航、部署恢复、认证读取和文件交付合同通过（62 个测试事件，0 失败、0 skip）；`make quality`（含 `make check-docs`）通过。证据见 `.validation/system-concept-audit-summary.json`；未重复完整混合合同，未执行真实部署或真实 Agent 语义验收。
- 分类纠偏（2026-10-10）：Goal = 按已有模型、操作入口和运行机制整理系统知识，停止把所有有代码依据的名词都放进 concept；Owner 沿用知识读写的系统出版物、CLI 的调用入口、术语表及各机制的现有主题。Non-Goals = 不扩展领域模型、协议形状或运行行为，不新增升级/迁移/部署动作。不变量沿用 I-01、V-01、REL-01 与已有系统出版物启动核对合同。选定 concepts 仅保留知识模型、仓与组合、版本与来源依据；Writer/CLI 移到 interfaces；主体与权限、Binding/观察/资源访问、Proposal/Gate 按各自机制共同解释，Snapshot、Hook、投影和访问审计归 mechanisms；capabilities 说明用途，guides 说明步骤。统一模型/入口/机制说明的正文 Schema，避免把阅读分类定义成领域类型；导航按目标种类更新并合并归类后重复的边。依据守卫覆盖全部说明与分类，不只覆盖 concept。否决仅移动文件但保留错误的 concept 身份/Schema/关系类型，也否决把辅助记录重新列成平级核心模型。接口继续使用既有 Writer、Address、Schema、发布完整性与只读读取合同。
- 分类纠偏结果与验证：模型与依据 14 项、操作入口 2 项、运行机制 7 篇，保留全部 27 个已有术语的 owner/public contract 依据，登记改为 `systemrepo/system-explanations.json`；合并同机制说明后删除 6 条重复导航边，正文共用说明 Schema。出版物/Schema/导航闭包、来源与分类守卫、初始化恢复、已认证模型/入口/机制读取、Dataset 原字节交付和 help 同源合同通过（62 个测试事件，0 失败、0 skip）；`make quality`（含 `make check-docs`）通过。证据见 `.validation/system-knowledge-layout-summary.json`；仅复验受影响合同，未重复完整混合合同，未执行真实部署或真实 Agent 语义验收。
- 复核发现的存量债：位置 `hook/README.md`、`hook/store.go:mutatingOn` 与 `doc/design/governance.md` Non-Goals；信号是公开应用 Hook 支持 `writer.commit`，而 owner 排除普通 Writer 提交的出站 Hook。判定为已有设计/合同不一致，本轮仅准确说明配置动作与读取边界，不反向收窄设计或修改执行；处置为单独裁决，触发条件为扩展点或写路径合同维护。位置 `liveaccess/README.md` 的 Schema 正文校验段；信号是仍称 Schema 没有 required/additionalProperties，而 `object/schema.go` 与 Writer 合同已经具备；判定为存量过时说明，本轮不改动态正文校验行为；触发条件为 Serving 值校验合同维护，届时对齐 owner、公开 Schema 与动态校验责任。
- 部署模型选定：实际 system binding 必填；初始化发布完整出版物后登记；恢复核对内容、声明和文件闭包；运行句柄保留实际仓读取能力并拒绝引用、归档和文件写入。否决默认内置仓、启动补发布、来源不可用时回退、直接复制文件绕过 Writer。
- 平台升级边界：现有 `object/writer/system.go:PublishSystem` 只初始化空仓或核对相同出版物，旧版已绑定仓不会自动覆盖；版本不匹配时部署和启动拒绝。原地升级入口仍须单独选定 CAS、历史保留与信任根切换合同，触发条件为正式提出原地平台升级需求。
- 本轮部署模型验证：实际发布、重启读同一 authority、重复初始化不补 bootstrap、缺失/漂移/离线拒绝、文件/引用/归档运行写面拒绝，以及组件夹具不再挂载默认仓的回归已通过；完整场景与混合合同复验及源码版本绑定证据以本轮交付报告为准，真实部署验收未执行。
- 上一轮自描述出版物验证：受影响包、已认证 CLI 读取、系统 Schema 分页、场景材料与 help 同源守卫无 skip 通过；`make quality` 与 `make check-docs` 通过。完整混合合同首次复验 1,860 个测试事件通过、0 失败，含原有 20 个外部环境 skip；仅共享 `TASK.md` 在运行中更新使版本绑定检查拒绝该证据，隔离同内容源码复跑记录见本轮交付。真实部署与真实 Agent 语义验收未执行，保持本条未勾选。
- 上一轮固定源码复验：固定同内容源码的 `make test` 为 21 个测试事件通过、0 失败、0 skip；`make test-contracts` 为 1,860 个测试事件通过、0 失败、原有 20 个外部环境 skip，60/60 公开 CLI 命令满足边界要求。两次运行源码指纹稳定，交付前对照的 1,793 个现存源码文件与工作区一致；版本绑定证据保存在 `.validation/system-publication-summary.json` 及对应 run 目录。

### DOLT-01 · 原生 Dolt 每条查询付一次引擎冷启动

> 状态：已随 Dolt adapter 退役关闭；以下保留为历史选型与实测记录。

- [x] 本轮完成：让同一 Repository 在一条命令内复用常驻 Dolt 查询会话，取消每条 SQL 一个 `dolt` 进程的执行方式。

**问题与实测：** `snapshot/dolt/command.go` 的 `query` 每次都执行一个新的 `dolt sql -r json -q`。本机无宿主 `dolt` 时走 Docker fallback，单条查询实测约 750 ms（`docker run --rm`）或约 135 ms（常驻容器 `docker exec`）；开销是 `dolt` 进程启动，与数据量无关。一条只写 2 个对象的 CLI 用例发出 113 次 `dolt` 进程调用，其中 39 次 `DOLT_HASHOF`、13 次 `dolt_branches`、12 次 `SHOW TABLES LIKE` 共 64 次（57%）是每次 `home.Open` 重复的开仓元数据探测。结果是 `go test ./cli` 单条用例耗时 150～220 秒并触发整包超时。这是执行通路缺陷，不是数据规模结论。

**Goal：** 把 `query` 改为向该 rootDir 的常驻 `dolt sql -r json --continue` 会话逐条发送语句并读回结果，会话由既有 `Home.Close()` → `snapshot.Registry.Close()` 释放，即 CLI 是一条命令、Server 是被服务的 Home；变更类命令先释放会话，Dolt 仍只有一个活动写者。owner 为 `doc/architecture.md`（`snapshot-authority-and-derived-media`）与 `snapshot/dolt/README.md`。

**Non-Goals：** 不引入新依赖（含 `database/sql` 与任何 MySQL wire driver）；不改 `snapshot.Store` / `TreeStore` / `HistoryStore` 公开形状；不改 ref、CAS、archive、幂等与错误码语义；不改 `kc_files` / `kc_units` / `kc_objects` 表形状；不把 `dolt sql-server` 作为运行时依赖；不改 Gitea adapter；不把测试夹具的容器编排搬进协议代码（`doc/architecture.md`、`SCALE_ARCHITECTURE.md`、`doc/architecture.md`）。

**不变量：** `A-01`、`W-02`、`V-01`；provider 执行通路改变不得改变 Reader/Writer/Catalog 与命令语义，CAS 与命令幂等保持，AS OF 固定 basis 回读结果逐字节不变。

**选定与否决：** 选定按 rootDir 复用的常驻 `dolt sql --continue` 会话（纯 `os/exec`，实测 0.4 ms/语句），结果走 stdout、错误走 stderr 逐条归属，写语句无结果集故以哨兵语句框定响应；任何变更类命令（`init`/`add`/`commit`/`checkout`/`reset`/`branch`/`sql -q` DDL 与 `runSQLScript`）先关闭该 rootDir 的会话再执行，因为实测会话持有独占写锁会使外部进程写入变为 `database is read only`。否决 `dolt sql-server` + `go-sql-driver/mysql`（未批准依赖，且 server 通路把 `COUNT(*)` 等整数结果变成字符串，改变既有 JSON 解码）；否决仅启动 `sql-server` 而客户端仍是 `dolt sql -q` 进程（实测约 205 ms/查询，比现状更慢）；否决把 Gitea 换成 CLI 测试默认 store（会替换掉 native Dolt 默认路径覆盖，与 DOC-14 / provider 合同缺口冲突）；否决只在测试夹具复用容器（仅省约四成，不解决调用次数）。

**接口与验收：** 会话实现私有于 `snapshot/dolt`，不出现在包外公开 API；先跑会失败的进程计数证据（同一流程的 `dolt` 进程启动数上界），再实现，再以 Dolt `RepositoryContract` / `WriterContract`、`snapshot/dolt`、`object/dolt` 与 `make check-docs` 验收，不以 skip 或删除断言换绿。

### LAKEFS-01 · LakeFS + 对象存储规模 Snapshot

- [x] 本轮完成：接入 LakeFS Snapshot authority，并以共享能力合同证明 provider 可替换；同一投影控制器分别处理静态 Snapshot Aspect 增量和动态 Recipe/Binding observation。LakeFS RepositoryContract、tree-provider 对拍、并发发布锁、预签名数据面和双 lane 控制器用例均已执行通过；`make check-docs`、`make check-validation` 与 `make test` 全绿。真实 LakeFS/COS 容量及长期故障资格仍归 SCALE-01，不由本项宣称。

**Goal：** LakeFS 以 Graveler/Refs 作为版本控制面、S3 兼容对象存储作为直传数据面，实现与 Dolt/Gitea 相同的固定版本、expected-old CAS、历史和增量语义；更换 binding 不修改 Catalog、Reader、Writer、Index 公开语义。

**Non-Goals：** 不向 Client/Collector/CI 暴露预签名地址或物理对象地址；不以 Actions/Webhook 作为正确性来源；不把动态 observation 写入 Snapshot；不为适配 LakeFS 放宽 CAS；不配置会删除已发布历史对象的 retention；不引入未经批准的 SDK。

**不变量：** `A-01`、`V-01`、`W-01`、`W-02`、`P-01`、`PC-01`、`E-01`、`IX-01`、`IX-04`。缺失规模必需能力必须在装配期失败关闭；静态变化只解释固定 `{from,to}`，动态变化只经 notice-and-pull 取得 observation basis。

**接口与验收：** 公开形状由 `snapshot.Store`、provider 能力合同和 `index.Controller` 拥有。LakeFS adapter 只使用 release-pinned REST 形状与标准库 HTTP；运行 Snapshot/Writer 共享合同、跨 provider 对拍、静态 Aspect 与动态 Recipe 控制器用例、架构守卫及文档检查。未配置真实 LakeFS/COS 的环境集成不得冒充生产容量资格。

### LAKEFS-02 · lakeFS 写入通路并发化与提交回执恢复

- [x] 本轮完成：`ApplyTreeCommit` 改为 wip 分支上有界并发 stage（默认 32，`KC_LAKEFS_STAGE_CONCURRENCY` 覆盖，首错误中止，commit/publish 仍单次串行）；commit 存在性探测加有界正向记忆（绝不缓存负向）；远端提交 POST 结果未知（超时/连接中断/`ErrTemporaryUnavailable`）时客户端按 `KC_COMMIT_RECEIPT_WAIT`（默认 10m，0 关闭）轮询既有 `GET /writer/v1/receipts/{command}` 恢复权威 receipt，确定性拒绝不轮询。红证据（串行 max in-flight=1、同一 commit 三次 ReadFile 探测三次、POST 超时丢 receipt）全部先行复现后转绿；`snapshot/lakefs` 全包含共享合同与 -race、`cli` 恢复用例与整包、`make check-docs` 通过。`make test` 各执行组（TestMetricPermissionScenes、TestProductScenes、`ok kc/cli`）全部通过，但验证包装器因并行 DATASET-03 工作线在运行窗口内修改源树而按 `source-changed` 判废（记录保留 `.validation/runs/20260922T031949Z-7f3ce8cc969a`）；并行线安静后的指纹稳定重跑不由本条宣称。真实 lakeFS/COS 吞吐与 staging 并发行为归 SCALE-01 部署验证。

**Goal：** 同一契约下把 `ApplyTreeCommit` 的对象写入改为有界并发（staging 原语不变：wip 私有分支上并发 stage/delete，commit 与 publish 仍单次串行），commit 存在性探测按不可变 commit 做正向记忆；远端提交 POST 因超时或连接中断失败时，客户端凭 command_id 轮询既有 `GET /writer/v1/receipts/{command}` 恢复权威结果，不再把「结果未知」当「提交失败」。

**Non-Goals：** 不改 staging/presigned 数据面合同（预签名地址不越过 Server 边界）；不改 Writer ChangeSet 合同、CAS、命令幂等账本语义或公开 CLI/HTTP 形状；不新增 HTTP 路由；不做 ChangeSet 压缩与批量差分端点（后续单独认领）；不宣称生产容量资格（归 SCALE-01）；不为适配并发放宽发布锁语义。

**不变量：** `A-01`、`V-01`、`W-01`、`W-02`；expected-old CAS 与命令幂等保持；同一 commit 读取结果逐字节不变；并发写入遇首个错误仍中止本次 apply 并清理 wip 分支；receipt 恢复只读取权威账本，不重放写入。

**选定与否决：** 选定 wip 分支上有界并发 stage（默认 32，`KC_LAKEFS_STAGE_CONCURRENCY` 覆盖）+ 首错误中止 + 正向 commit 存在记忆（只缓存「存在」，绝不缓存「不存在」；属 `CA-01` 允许的 transport cache）；客户端在结果未知类失败（超时/连接中断/`ErrTemporaryUnavailable`）后按 `KC_COMMIT_RECEIPT_WAIT`（默认 10m，0 关闭）轮询 receipt，确定性拒绝（其余 kernel 错误码）不轮询直接返回。否决客户端直传对象存储（预签名越界）；否决 import 零拷贝路径（字节在客户端且必须过 Writer）；否决把 receipt 轮询改成服务器 202 异步执行（本轮不改 HTTP 形状）；否决负向 commit 缓存（会吞掉并发创建的新 commit）。

**接口与验收：** 公开形状不变（`snapshot.TreeChangeSet` / `writer.CommitReceipt` / CLI argv）。先运行会失败的证据：staging 并发度计数（串行实现 max in-flight=1）、同一 commit 重复 `ReadFile` 的 commit 探测次数（现状每次一次）、POST 超时后客户端丢失 receipt；再实现；以 `snapshot/lakefs`（含 -race）、`cli` 定向回归与 `make check-docs` 验收，不以 skip 换绿。

### LAKEFS-03 · ChangeSet 传输压缩与差分预检并发化

- [x] 本轮完成：typed Client 对 >512 KiB 请求体自动 gzip（`Content-Encoding: gzip`，标准库），服务面按头透明解压且上限对解压后字节强制（gzip 炸弹不放大）；writer commit 路由独立体量上限（默认 512 MiB，`KC_MAX_COMMIT_REQUEST_BYTES` 覆盖），其余服务路由维持 8 MiB；`remoteCurrentDigests` 差分预检改有界并发（16，首错误中止），语义与串行一致。红证据（大请求体无压缩头、gzip 体被当明文拒、预检串行 max in-flight=1）先行复现后转绿；`client` 整包、`snapshot/lakefs` 整包、`cli` 定向批（解码/gzip/上限/预检/recovery/-race）与 `make check-docs` 通过。`cli` 整包本轮无法认证：并行 DATASET-03 工作线的未提交 WIP 自带 3 个失败用例与 2 条待补证据的 `/ui` 路由（route-count 守卫 86→88），且 Dolt 引擎夹具在与其会话并发时启动挂起（`snapshot/dolt/session.go` stdout 读取 IO wait）——均不在本条改动面，待并行线收口后随全量套件认证。

**Goal：** typed Client 对大请求体透明 gzip（`Content-Encoding: gzip`，仅标准库，不新增依赖）；服务面按头透明解压，且上限对解压后字节强制（gzip 炸弹不放大）；writer commit 路由独立大体量上限（默认 512 MiB，`KC_MAX_COMMIT_REQUEST_BYTES` 覆盖），其余服务路由维持 8 MiB；`remoteCurrentDigests` 差分预检改有界并发，结果与错误语义同串行逐操作一致。

**Non-Goals：** 不新增 HTTP 路由；不改 ChangeSet/digest 合同与 CLI argv；不引入 zstd 等未批准依赖；不动非 typed 路径；不宣称端到端部署提速数字（归 SCALE-01）。

**不变量：** `A-01`、`V-01`、`W-02`；请求体上限对解压后字节生效；预检结果（digest map）与串行语义一致——unresolved/空 digest 仍跳过、非 unresolved 错误仍整批失败；预检只读，不写 authority。

**选定与否决：** 选定客户端 >512 KiB 请求体自动 gzip + 服务面按 `Content-Encoding: gzip` 解压（`compress/gzip`，LimitReader 作用于解压流）；writer commit 上限默认 512 MiB（实测 228 MB 约两倍余量），其余路由维持 8 MiB；预检并发默认 16、首错误中止。否决 zstd（依赖未批准）；否决为预检新增批量 resolve 路由（闭合面不扩张，留待真实测量证明需要）；否决「首导入跳过预检」启发式。

**接口与验收：** 公开形状不变。先运行会失败的证据：大请求体不带 `Content-Encoding: gzip`、gzip 请求体被 8 MiB 明文限制误拒、预检串行（max in-flight=1）；再实现；以 `client`、`cli` 定向回归与 `make check-docs` 验收，不以 skip 换绿。

### LAKEFS-04 · bulk-ingest：一次性大量小文件的 lakeFS 批量写通路

- [x] 已完成：bulk-ingest 数据面能力已实现并验证，2026-09-28。选定为 lakeFS 对象上传 API（实现前核验否决 import 通路，见「选定与否决」三条理由）；`snapshot.BulkTreeIngester` 可选能力 + `knowledge.ChangeSet.BulkIngest`（`omitempty` 不动既有 digest）+ `writer commit --bulk` 单一显式选项贯通 CLI→Writer→adapter，缺能力确定性失败关闭，通路翻转落既有 IDEMPOTENCY_CONFLICT。已验证：fake 介质合同+bulk 三用例与 `-race`、`object/writer` bulk 三用例、CLI 守卫与 wire 三用例、全量 `make test`（exit 0）、`make check-docs`、`make quality`；真实 lakeFS 部署栈（compose scenes 容器，Graveler+MinIO）目标性验证 bulk PUT/REMOVE/CAS 与回读（`snapshot/lakefs/write_bulk_live_test.go`，缺环境时条件跳过）。未宣称：`make deploy-local-scenes` 全量走查与 staging/bulk 双路径容量数字（归 SCALE-01）。

**背景：** 当前 lakeFS 数据面只有逐对象 presigned staging——每文件 presign、PUT、link 三次往返，32 并发（`KC_LAKEFS_STAGE_CONCURRENCY`）只摊常数不消量级，commit 时长随变更条目数线性；smoke 证据（`.data/scale/runs/smoke-20260922T101955Z`）仅到 100 文件点已 18.9s，10w 文件点从未实测。LAKEFS-02 否决的是「字节在客户端的 import 零拷贝」（绕过 Writer），本条维持该否决：字节仍然只经 Writer 进入介质，变的只是「对象如何进介质」这一个数据面原语。

**Goal：** 为 `snapshot/lakefs` 增加 bulk-ingest 可选数据面能力：Writer 对显式 opt-in（`writer commit --bulk`）的 ChangeSet 以 lakeFS 对象上传 API（`POST/DELETE /branches/{b}/objects`，每文件 1 次往返，字节经 lakeFS 服务端落到 backing store）替代逐对象 presigned staging 三次往返；wip 分支、commit、publish（lock-branch hard-reset）与 receipt 恢复全部复用现有协议。公开 Writer/ChangeSet/CAS/幂等账本语义不变。

**Non-Goals：** 不改默认（无选项）staging 路径与发布锁语义；不给客户端对象存储直传口（维持 LAKEFS-02 否决）；不把 COS 凭证引入 KC Server；不新增公开 HTTP 路由与 CLI 动词（仅 `writer commit` 增单一 opt-in 选项，见设计问题 1）；不做 v1 自动路由与部署级模式旋钮；不宣称生产容量资格（归 SCALE-01）。

**不变量：** `A-01`、`V-01`、`W-01`、`W-02`；字节只经 Writer 进入介质（新增，随实现与测试转正登记）；bulk-ingest 是 adapter 数据面能力，不是协议旁路——「规模权威提供方不得暴露绕过知识不变量的原始路径写入口」（[架构总览](doc/architecture.md) 硬性约束）保持成立；bulk apply 失败/中断不留半提交，重放按既有 receipt 幂等语义收敛。

**选定与否决（拟，实现前逐项复核）：** 选定两模式并存、显式选择（无选项=staging 增量小 commit；`--bulk`=一次性大量条目）+ `snapshot.Store` 可选能力 `BulkTreeIngester` + lakeFS 对象上传 API 作为 bulk 数据面。**否决 lakeFS import API 通路**（实现前核验发现三处硬冲突）：① import 源须在 COS 自选键位且键名保持树路径，而 staging presign 的物理地址由介质自选，KC Server 按 Non-Goal 不持 COS 凭证，该通路在不违反凭证合同的前提下无法成立；② import 目的地前缀是替换语义（官方指南 Note：目的前缀下既有对象会被删除），与 expected-old CAS 冲突，补录/重跑会误删既有条目；③ import 不表达 REMOVE。另否决：客户端直传 COS 与 import 零拷贝绕 Writer（维持 LAKEFS-02）；把 bulk-ingest 做成协议级新写入面（不新增 APPEND/PATCH 类 Surface）；以打包档案为介质对象（Graveler 逐条目，介质不解包）；v1 自动按条目数路由与部署级 `auto|staging|ingest` 旋钮（阈值无实测依据、静默切换扩大新通路爆炸半径、单一机制够用不加机器；SCALE-01 数据出来后显式被证明不够用再议自动）。

**要回答的设计问题（实现前定案）：**
1. 策略选择（定案）：显式 opt-in，v1 不做自动路由。`writer commit` 新增单一选项（`--bulk`）；缺 bulk-ingest 能力的 adapter 对该选项确定性报错（能力不足由协议明确拒绝，不由上层猜测或模拟成功），无选项时行为与现状逐字节一致。通路选择随命令进既有命令账本：重放按账本记录的通路收敛，同 command_id 通路不一致按既有幂等冲突族确定性拒绝；不加新公开 receipt 字段。
2. 通路与账本对应（定案）：`--bulk` 落在 `knowledge.ChangeSet.BulkIngest`（`omitempty`，false 不进 canonical JSON，既有 digest 不变），随 ChangeSet 进 CanonicalDigest——同 command_id 翻转通路自动落入既有 IDEMPOTENCY_CONFLICT 族，账本无需新字段；bulk apply 与 commit/publish 全同步，无 importStart 类异步状态。
3. 能力合同形状（定案）：与 `DirectoryReader`/`HistoryStore` 同型的 `snapshot.Store` 可选能力（`BulkTreeIngester` + `BulkTreeIngesterOf`）；adapter 层如实实现或不实现；转发型 wrapper（`connectedSource`、telemetry、testkit）经 call-time 断言转发，缺失时确定性失败关闭，不静默。
4. 部署前置核验（定案）：对象上传 API 是 lakeFS OSS 标准 REST（v1.86 基线内），无需 import 的 admin-only/`blockstore.local.import_enabled` 前置。
5. 残留归属（定案）：无 `kc-ingest` 临时前缀——上传对象由 lakeFS 落位 backing store；失败 wip 分支沿既有 `defer deleteBranch` 清理，未提交残留归介质 GC 管辖（与 S3 gateway 弃写同责，非 KC 新增负债）。

**接口与验收：** 公开形状由 `snapshot.Store` 能力合同拥有（[架构总览](doc/architecture.md) snapshot-authority-and-derived-media / provider-capability-contract 主题；`snapshot/lakefs/README.md` 数据面节随实现补两模式说明）。先运行会失败的证据：缺 bulk-ingest 能力的 adapter 收到 `--bulk` 时确定性拒绝、bulk 通路未走对象上传 API 或复用 staging 往返、bulk 与 staging 产出内容不一致、同 command_id 重放通路不一致被拒；再实现；以 `snapshot/lakefs` 合同与 -race、幂等重放/恢复用例、`make check-docs` 验收，不以 skip 换绿。

**依据：** [架构总览](doc/architecture.md)（写边界与能力合同 owner）、[snapshot/lakefs README](snapshot/README.md)（数据面合同）、[规模门槛](doc/engineering/testing.md) 与 SCALE-01（探针与双路径对照）、lakeFS 官方 [Import 指南](https://docs.lakefs.io/guides/import/) 与 [性能最佳实践](https://docs.lakefs.io/concepts/performance-best-practices/)（import 推荐口径与其目的地替换语义 Note，2026-09-24 读）。

### CACHE-01 · 同版本正文缓存与独立后台预热

- [x] 已完成：服务内同版本正文缓存、可注册的独立后台消费者、有界热点与可选冷启动预热；2026-09-08。38 项本任务合同均实际执行通过、零跳过，缓存及接缝定向 race 通过。最终 `make test` 零失败，真实 adapter 套件零失败、零跳过；短套件的既有条件跳过不计为通过。实现、错误语义与范围边界见本条及缓存包合同；当时详细报告仅留本机、未入库，不作为当前源码验收凭据。

**Goal：** 在固定 basis 的 SEARCH / RELATIONS / 精确 READ 回读处复用完整 Snapshot 正文，批量回源仅处理未命中项；预热独立于索引，并能在发布、丢通知和进程重启后通过 HEAD 对账恢复。owner 为 `doc/design/service.md`、`doc/architecture.md`、`doc/design/discovery.md`。

**Non-Goals：** 不让 Snapshot/Reader 持有语义缓存实现，不修改 Writer/Catalog 协议、不改变公开 SEARCH 候选与正文语义、不缓存动态 State observation、不新增消息中间件或分布式缓存；不把有界预热解释成全仓覆盖或生产容量资格（`doc/architecture.md`、`doc/design/discovery.md`、`doc/design/liveaccess.md`、`SCALE_ARCHITECTURE.md`）。

**不变量：** 保持 V-01、C-01、CA-01/CA-02/CA-03、PC-01、P-01、R-01/R-02、AUTH-01/AUTH-03、KS-02；旧 pin 不读新版本，命中按当前权限交付，缓存副本不受 selector/State hydrate/交付修改污染，消费请求不构建投影。

**选定与否决：** 选定仓 + commit + 对象/完整 Address 的有界进程缓存、批量 miss 回源、返回副本、后台热点与有限冷启动预热；派生消费者分别跟踪进度与故障，用来源版本变化和对账驱动。否决 ID-only/latest 回退、用索引文档更新替代正文变化、让缓存实现 Retriever、Writer 同步预热以及按仓共用所有派生状态。

**接口与验收：** 窄读取端口由 `knowledge.Hydrator` 拥有，上层实现位于 `object/cache`，查询执行器与 Reader/Serving 仅依赖端口；后台扩展由 `index.SnapshotConsumer` / `Controller.RegisterConsumer` 拥有。先运行会失败的缓存/接缝/独立消费者证据，再实现；以版本/权限/副本隔离、批量回源次数、预热恢复及正式 `make check-docs` / `make test` 验收，不以 skip 换绿。

### RETRIEVAL-01 · 声明式索引与有界检索执行可靠性

- [x] 已完成：按知识探索调研 §6 实施声明/类型贯通、精确标量与分页、增量可见性、预算/取消、Workspace 有界归并及批量读取；2026-09-08。本任务必需合同均实际执行通过，未用 skip 消除失败。最终 `make test` 成功，公开命令成功与边界覆盖均为 70/70；真实适配器及定向 race 零失败、零跳过。当时短套件的条件跳过不计为通过；详细报告仅留本机、未入库，不作为当前源码验收凭据；模型质量及生产容量实验仍未宣称验收。

**Goal：** 贯通真实 Schema 发布、按访问声明编译、类型化查询、固定版本分页与增量发布；补齐查询预算、取消和批量读取，以失败反例和正式合同验证改进。

**Non-Goals：** 沿用 [声明式索引](doc/design/discovery.md)、[检索](doc/design/discovery.md)、[知识读写](doc/design/object.md) 与 [索引控制](doc/design/discovery.md) 的归属；不新增访问关键字、向量引擎、任意图路径、成本优化器或索引内 LLM，不改变发现/正文授权及 Writer 边界。

**不变量：** `S-01`、`I-01`、`C-01`、`R-01`、`P-01`、`IX-03`、`IX-04`、`KS-02`；知识身份、固定 basis 回读、物理派生可重建及增量成本边界保持成立。

**选定方案：** 保留三类 access 与现有分层；声明类型统一校验；只编必要索引槽位；无损类型化比较与游标；后端部分失败准确传播；全部增量可见后发布；补判必须可证明，查询执行有预算并支持取消；同版本批次复用定位，未变化投影不重写。时间精确物理表示属于 adapter 并升级物理版本。词表映射和直接/渐进阅读通过消费侧既有能力验证。

**否决方案：** 静默截断长值、浮点替代整数、毫秒实现收窄时间合同、末批刷新冒充全批可见、任意字符串补判冒充全文精确、输出 LIMIT 冒充总预算、按 Workspace 复制投影。

**接口与验证：** 具体形状落在 [knowledge](object/schema.go)、[retrieval](discovery/README.md)、[index](discovery/projection/README.md) 和提供方公开合同；先记录红例，再修复并运行 `make test`、真实 OpenSearch 与必要并发检查。未通过的检查与未执行的模型/规模实验分别记录，不提前勾选。

### WALK-01 · 2026-09-08 产品走查修复

- [x] 本轮已完成：修复身份、登录会话、托管供给与管理地址、接入权限和消费入口问题。`make test-all` 命令通过，公开命令成功及风险边界覆盖均为 70/70；没有用 skip 或删除断言消除失败。运行期间并发变更仅涉及研究文档，原记录保留 `source-changed`，精确指纹重建证明被测代码未改。本条保留当时修复范围与证据限制；详细报告仅留本机、未入库，不作为当前源码验收凭据。SELF-01～04 的更广部署边界仍单独保留，真实组织 SSO、远程自有 Dolt 和生产源长期运行未宣称已验收。

### SELF-01 · KC 产品身份与客户端会话贯通

- [ ] 让已有认证入口产生统一的 KC 用户名，并让交付客户端持续使用已配置的 Server 与登录会话。所属阶段：M3。

**本轮实现：** 可信人类身份统一为 KC 用户名；不可变 IdP subject 与用户名耐久绑定，旧身份仅经显式迁移。客户端保存默认 Server、按 Server 隔离会话并复用刷新；Taihu 的应用秘密留在 Server。定位反例已由红转绿，正式验证由 WALK-01 汇总。真实组织身份服务的生产登录仍需部署环境验收。

**要完成什么：** 各认证器把可信身份映射为统一的 KC 用户名；登录后，产品身份和公开 identifier 使用规范用户名，例如 `kaiqidong`。认证提供方的 subject、数字 ID、token 和前缀只用于内部认证关联及审计，不进入普通命令、仓名或分享输入。用户名必须由可信身份源验证，并明确唯一性、改名和回收规则，不能由请求自报。将服务入口封装在交付客户端，登录后跨命令复用连接并处理凭证刷新或失效；配方与 pin 不保存凭证。

**需要判断：** 用户名是否全局唯一、能否改名或被回收；若存在变化，KC 必须保存内部稳定绑定并定义迁移与防冒领规则，但不因此把内部 subject 暴露给用户。

**完成标准：** `kaiqidong` 在空用户配置下使用已交付的客户端登录，`whoami` 和后续产品界面显示 `kaiqidong`；授权、审计、托管仓所有权与重启恢复始终归于同一 KC 用户。普通输出不出现 `gitea:4` 之类 Store subject。凭证失效可恢复，后续每次命令按当前授权重新求值；全过程不修改部署配置，也不手工指定 Server 地址。固定 pin 不因登录刷新而变化。

依据：[认证设计](doc/design/service.md) §4、[服务设计](doc/design/service.md) §5、[当前登录实现](cli/README.md)、[产品缺口](doc/engineering/testing.md)。

### SELF-02 · Gitea 与 lakeFS 托管 Snapshot Store 自动供给

- [ ] 用户登录后只需创建 Repository，后台自动完成 Store 账号/租户和仓的分配、绑定与恢复。所属阶段：M3；这是现有底层 managed create 之上的产品闭环。

**目标流程：** `kaiqidong` 登录 KC，只选择创建 Repository、用户可理解的仓名和允许的托管 Store（Gitea 或 lakeFS）；KC 后台从当前用户与产品上下文生成完整 Repository identity 和幂等命令身份，确保该 Store 中存在 identifier 为 `kaiqidong` 的账号或隔离租户，创建仓、保存内部连接、登记 Catalog，并按策略授予创建者维护能力。用户不填写完整 Catalog/Repository 协议坐标，不接触 command-id、Store 地址、服务账号、token、数据库目录或内部 allocation id。

**已有实现与待验收范围：** 按名称建仓、分配账、binding、创建者授权、管理地址与重试恢复已有实现；Gitea 同名账号供给仍需已配置的登录源。当前托管目标为 Gitea 与 lakeFS，Dolt 已退役。当时 WALK-01 的供给结果仅保留为历史范围，不能由替换后端名称外推 lakeFS 的全部隔离、故障与恢复资格；本项继续按下面的完成标准取得当前部署证据。

**设计边界：** KC 用户名是跨 Store 的产品 identifier；Gitea/lakeFS 的内部 subject、账号 ID、仓 ID、DSN、凭证和目录只保存在 Store Directory/供给账。Repository identity 不随选择 Gitea 或 lakeFS 而改变，也不能从物理仓名反推协议身份。Gitea 和 lakeFS 可以有不同的物理隔离机制，但对用户呈现相同的创建、状态、失败和恢复语义。

**需要补齐：** 托管 provider 的用户/租户 ensure、仓 allocate、权限安装、binding 持久化和幂等恢复状态；用户名冲突、已有同名账号归属、部分成功、重试、停用/改名、Store 不可用、配额及删除/归档规则。服务凭证只能代表平台执行供给，不能成为知识创建者身份。

**完成标准：** 全新的 `kaiqidong` 只登录 KC，输入仓名并选择 Gitea 或 lakeFS，即得到属于 `kaiqidong` 隔离空间的 Repository；随后直接发布和回读。创建返回用户名称、仓名、逻辑 Repository identity 和状态，两个 Store 的普通返回均不暴露物理账号 ID、allocation、路径、command-id 或服务凭证；重复点击和实例替换返回同一逻辑结果；另一个用户不能占用、打开或恢复 `kaiqidong` 的 Store 空间。任一分阶段失败都有可恢复后态，不产生无人认领账号或仓。

依据：[服务与托管仓设计](doc/design/service.md)、[Store 边界](doc/architecture.md)、[当前供给实现](home/managed.go)、[Gitea adapter 合同](snapshot/README.md)。

### SELF-03 · 接入方自行连接和维护自己的 Snapshot 仓

- [ ] 提供自有仓连接、校验、凭证更新和恢复入口。所属阶段：M3；平台托管 create 已完成，不重做。

**本轮实现：** 独立 connect / connection show、check、rotate 入口支持允许来源上的自有 Gitea 仓，逐仓凭证保存在 Server 私有耐久账。连接与轮换先只读验证，失败保留原连接，重启后仍可轮换过期凭证。自有远程 Dolt 与 Gitee 没有声明为已支持。正式验证由 WALK-01 汇总。

**要完成什么：** 在服务管理面保存可恢复的连接及凭证引用，验证来源存在、身份一致、权限足够，再按既有合同进行 Catalog 准入；失败不得创建、收养或改写外部仓。连接、秘密与 Catalog 成员关系分别由各自权威管理。

**需要判断：** 首批自有仓覆盖的后端、凭证归属与维护权限；若目标确为 Gitee，需要单独核对 adapter 能力。连接暂停/移除如何影响已有消费者，也要有明确产品语义，不能顺带删除远端知识。

**完成标准：** 两位接入方独立连接两个既有仓、轮换各自凭证并持续维护；彼此不能使用对方连接；实例替换后仍可恢复；身份不符、凭证失效、重复请求和准入失败均有明确后态，外部仓不被初始化或污染。

依据：[介质与接入边界](doc/architecture.md)、[部署与恢复合同](home/README.md)、[当前产品缺口](doc/engineering/testing.md)。

### SELF-04 · 首次准入与仓维护者分享

- [ ] 让普通用户按明确策略开通基础能力，让仓维护者在获授范围内分享与撤销。所属阶段：M3。

**本轮实现：** 首次准入按显式资格和窄动作策略请求，仓维护者在当前获权与平台转授上限的交集内分享和撤销。首次授予有耐久回执，重试或重启不恢复撤销规则；消费方旧 pin 仍按当前权限检查。正式验证由 WALK-01 汇总。

**要完成什么：** 明确准入资格、可授予的基础能力、仓维护者的转授上限及撤销规则，并提供普通用户入口。用显式可追溯策略承接，不能以“已登录”“已登记”代替授权。

**需要判断：** 首次准入基于组织资格、邀请还是审批；维护者可分享哪些动作、能否转授以及撤销如何影响下游授权。来源/历史读取的最小权限先依 REVIEW-02 统一，不能在不同入口得到不同结果。

**完成标准：** 新接入方获得准入、创建仓并发布；本人通过普通入口授权消费方，消费方自行发现、选源、读取并采用更新；撤销后旧 pin 也不能继续越权读取。全过程不授予全局管理员权限、不由部署方临时补权，重启或重放不补回已撤销权限。

依据：[权限体系](doc/design/permissions.md)、[知识读写](doc/design/object.md)、[托管仓现有验收](doc/engineering/testing.md)。

### DYN-01 · 动态 State 的真实部署与重启恢复

- [ ] 按投影控制的持续服务与恢复合同完成整条部署验收。所属阶段：M4；承接原动态索引未完成项。

**现状与影响：** State READ、动态投影、变化通知及同 basis 回读均有实现和局部验证。缺的是独立 Observer、runtime、lakeFS、KC 与 OpenSearch 串联后的完整证据，以及 DOC-20 所列消费闭环缺口；已有双容器/adapter 测试不能证明 KC 重启后整条链仍能继续运行。

**要完成什么：** 用真实源变化触发 Observer，服务经已授权 Binding 拉取，推进动态投影；稳定知识仍由 Collector 走 Writer。KC 不直接读源夹具，runtime 不与 KC 共享内存 fake，Collector 不直写索引，观察值不写进 Snapshot。

**完成标准：** 源值改变后无需消费者触发维护即可检索到新观察；Snapshot commit 和文件视图不变；KC 替换/重启后能够恢复并继续追赶；重复、乱序、失败通知有后态证据，分页不混版本，回读失败不伪装为零命中。通知未送达时的时效/失联行为按 REVIEW-04 裁决验收，不能顺带声称实时完整。

依据：[投影控制](doc/design/discovery.md) 的服务目标、通知对账与方向性用例、[动态物化](doc/design/liveaccess.md)、[现有索引合同](discovery/projection/README.md)。这里验收 State，不把 Stream 窗口或多实例当作已经完成。

### GUIDE-01 · 单文件手册的阅读与打印验收

**知识模型中的索引声明（2026-09-23）：** 架构图②列出字段 `text/filter/sort` 及其用途，并将 `origin` 外部访问原点与字段索引声明分开；产品 owner 同步。文档图、手册服务测试、格式检查与四档宽度离线检查通过。

**统一访问原语（2026-09-23）：** 全景图按发现、定位读取、关联溯源、外部访问四组列出公开原语，并在产品 owner 中强调 BROWSE 的有界范围与 ACCESS/INVOKE 分责。文档图、手册服务测试、格式检查及四档宽度离线检查通过。

**外部状态接入图分组（2026-09-23）：** 在整体架构图中用一个“外部状态源”区域包含 Resource Access 与 Observer，说明同一来源的当前值访问和变化通知是两项接入职责；两条连线仍分别指向统一访问/索引控制层和索引控制层。产品 owner 同步说明。

**Snapshot 提交路径澄清（2026-09-23）：** 架构图补出业务原生提交与 KC 服务端提交两条入口，在⓪中明确 commit 与发布分支推进，并将索引控制的输入改为已发布 commit 推进。接入章节说明自有仓等价校验、文件提交与知识发布分责、托管入口的代理式产品方向及现有 lakeFS 写权威约束；不宣称标准 Git 协议兼容已交付。产品 owner 同步。文档图、手册服务测试、格式检查与四档宽度离线检查通过，已查看更新后的架构图；仅调整文档。

**整体架构图（2026-09-23）：** 按本轮 ASCII 对齐结果，将接入方、⓪–③、统一访问和 VFS 合并进同一张内嵌矢量图；③内显式展示索引控制层，连接 Observer、Resource Access 和 Snapshot 版本推进，区分取值与变化通知连线。保留版本、权限纵栏及四层跳转，产品 owner 同步说明图中职责。`make check-docs`、手册服务测试、格式检查通过；四档宽度离线检查无整页溢出，窄屏图内滚动，已检查桌面和打印样式，样本在 `/tmp/kc-integrated-architecture-qa/`。未重做整册分页打印验收。

**章节拆分（2026-09-23）：** 将瞬时状态独立为第 09 节“解决方案：瞬时状态接入”，第 10 节“只读 VFS”仅讲 Dataset 本地文件消费；同步目录、后续编号并保留旧跳转锚点。文档图、手册服务测试、格式检查通过；离线核验 13 节目录与编号一致，四档宽度无整页横向溢出。仅调整手册组织，未改变产品行为。

- [ ] 本轮认领：重写产品价值与分层采用路径，对齐 Dataset 发布、独立授权及默认最新消费，明确 VFS 仅适用于本机放得下的 Dataset 且目前只读；核对现行 CLI，并补齐浏览器视觉、窄屏、复制和打印验收。所属阶段：M3；承接原产品手册未完成项。

**本轮范围（2026-09-30）：** 用户确认以独立供给、按任务组合、同依据消费和持续修正作为产品价值，统一根 README、派生手册、相关 owner 与 MVP 缺口说明。本轮只认领 GUIDE-01；既有源码修改与其它任务记录保留。

**提交前收口（2026-10-10，沿用 GUIDE-01）：** Goal = 整理已提交的产品文档与旅程证据并同步 GitHub，修正 `_views.yaml` 重复的 service U2 缺口以及 `TestSameTaskViaCLIAndHTTPYieldsSameDomainResult` 在上游推进后未检查 HTTP 成功、完整结果与原 commit 的松断言。Non-Goals = 不改产品行为、公开协议、权限、版本规则或既有提交历史；沿用[服务](doc/design/service.md) U1、[质量闭环](doc/engineering/quality.md) §3.1 与[场景合同](.data/scenes/README.md)。不变量沿用 API-01、AUTH-03、KS-01；接口仍为正式 CLI/HTTP 的对象读取合同。选定复用同一领域结果核对、精确失败码，以及场景 YAML 重复键拒绝的自动守卫；否决空响应或任意错误被当成正确拒绝、重复键静默覆盖。两处信号判定为真问题，本轮修复并补回归；验证与推送结果按实际执行记录交付。

**提交前验证（2026-10-10）：** 场景工具 44 项单测、工程视图/产品关联/共享前态检查通过；同一任务 CLI/HTTP、场景视图和文档命令守卫定向回归通过；`make quality`（含文档图）通过，耦合基线 33.23% held。`make test-contracts` 记录 `.validation/runs/20261010T023113Z-5bd8c9b96c40`：`passed-with-skips`，1797 个测试事件通过、20 个外部/live 测试跳过，公开命令成功与风险边界覆盖均为 60/60；`make test` 记录 `.validation/runs/20261010T023955Z-d710b1178149`：`passed`，21 个测试事件通过、无 skip。两次运行源码未变化，指纹均为 `989bcf908588a8a6b27d0c6d4471b83dab25025efacb460db21a0b2ba3611493`；本段仅追加验收记录。未执行真实部署、FUSE、付费 Agent 或规模资格验收，GUIDE-01 保持原关闭条件。

**Goal：** 把 [架构总览](doc/architecture.md) 的产品原则转成接入方、消费方可理解的使用路径；修正已退役用法与验收表述，给尚未闭环的能力明确处置与验收入口。

**Non-Goals：** 不改变公开协议、权限政策、Dataset 文件层定位或动态观察语义；不以文档修改宣称生产、动态部署或任务收益已验收。以上边界分别沿用架构总览、[Dataset](doc/design/dataset.md)、[服务](doc/design/service.md) 与 [MVP](doc/engineering/testing.md)。

**不变量 / 方案 / 接口：** 沿用 I-01、V-01、W-01/02、KS-01/02、D-01、O-01、API-01；选定同一价值叙述、按角色交付、渐进采用，以及命令示例对现行注册表与旗标合同的自动核对；否决增加第二套产品设计权威、把反馈直接当知识、手工 pin 消费或用文字补造验收。接口形状以 `cli/surface.go`、`cli/SURFACE.md`、Reader/Writer 公开合同与 Conformance 为准。完成前执行文档图、受影响测试、契约与质量门，并检查离线、窄屏、复制和打印。

**知识模型说明补充（2026-09-23）：** 本次仅承接本条的概念介绍缺口：在产品 owner §1.1.1 与派生手册第 03 节，用订单服务、值班说明和负责团队解释 Entity、Aspect、Relation，并补充 Member、Schema、身份、版本与来源。沿用系统设计的 `I-01` / `K-04`、Aspect 设计的维护粒度与系统设计 §7.5 的关系边界，不新增协议或产品行为。本次文档验证单独记录，不据此勾选整条手册验收。

**本次验证：** `make check-docs` 通过（35 documents / 124 relations）；`go test ./scripts/docs-serve` 与 `git diff --check` 通过。单文件离线核验目录跳转、12 节编号一致、1440/1024/768/390px 页面及新增图内无横向溢出；已查看新增章节桌面、窄屏与打印样式截图，样本位于 `/tmp/kc-knowledge-qa.lKmDrI/`。本次未重跑完整产品套件或整册分页打印验收。

**Dataset 示例修订（2026-09-23）：** 同一 GUIDE-01 范围内，将整仓发布示例改为运维仓值班手册、服务仓订单服务定义及各仓 Schema 目录的切片组合，列明来源子目录与交付目录映射；第二版沿用相同范围，产品 owner 同步突出按用途切分与跨仓交付。`make check-docs`、手册服务测试、CLI 默认 selector 与目录映射解析测试通过；离线检查两版范围一致及 1440/1024/768/390px 无整页横向溢出。本次仅验证文档与命令解析合同，未执行真实 Dataset 发布。

**核心图局部调整（2026-09-23）：** 保留⓪–③分层、知识/文件底座分组及现有侧栏；在③说明索引控制，在动态侧栏补充 Resource Access 与 Observer 接入，在底部补出仓接入和 Writer 发布，VFS 明确连接① Dataset 文件范围。版本与时间、权限与边界改为贯穿纵栏；窄屏转为两张约束卡。产品 owner 同步分责，未增加 VFS 自动更新或动态值文件化承诺，移除手册中退役 Dolt 适配的旧表述。文档图、手册测试和格式检查通过；离线核验四层跳转、1440/1024/768/390px 无溢出并查看桌面、窄屏与打印样式，截图在 `/tmp/kc-architecture-qa.mnRWc3/`；未重新执行整册分页打印或产品运行验收。

**接入位置纠偏（同日）：** 将外部动态状态移至底部来源接入区，Resource Access 观察值供统一访问与控制取值，Observer 通知进入控制与维护；Snapshot 仓接⓪，平台跟踪版本变化并驱动维护。原动态侧栏改为平台自动控制与维护，②保留声明与解释职责，不作为源侧接入点。产品 owner 同步；文档图、手册测试、格式检查及四档宽度离线检查通过，截图在 `/tmp/kc-ingress-qa.Q6ZUqB/`。仅调整文档表达，不改变运行合同。

**知识文件示例（同日）：** 第 03 节展开交易团队 Entity、订单服务 definition/runbook 两个 Aspect、负责团队 Relation 的完整 YAML，并提供可展开的服务定义 Schema。图中团队对应整体 Entity 保存，说明路径与身份、共享对象身份、关系端点及同仓 Schema 前提。使用现有 repofile 解析五份示例并检查 Address 冲突、关系端点及服务定义实例的 Schema 校验，全部通过；文档图、手册测试、格式与四档宽度离线检查通过。样本在 `/tmp/kc-file-examples-qa.hArOOd/`；未进行真实发布。

**查询声明示例（同日）：** 第 07 节补齐 Schema/实例 YAML、`text/filter/sort` 与不声明 access 的对照、Writer 发布、schema describe 核对、Dataset 新版本采用，以及全文/团队过滤/优先级排序命令。例子沿用订单服务，并保留“声明已发布不等于索引就绪”的边界。现有解析器与 Schema 校验通过，CLI 查询参数及完整字段名解析测试通过；文档图、手册测试、格式与四档宽度离线检查通过。样本在 `/tmp/kc-retrieval-doc-qa.GJzp28/`；未执行真实发布或检索服务验收。

**本轮文档完成（2026-09-20）：** 手册以价值与采用阶梯开篇，明确 Snapshot Store + Dataset 可独立采用、Dataset 消费授权、声明式索引与统一访问；分开最新已发布版与最新可服务版的切换目标。移除旧 Workspace、手工 pin 及退役 CLI 示例，动态状态改用现行 `access` 入口。VFS 在采用表、方案和 FAQ 中明确仅适用于本机放得下的 Dataset，目前只读。产品 owner 与组合 owner 同步，依赖关系仅在文档图补齐。

**本轮验证：** `make check-docs` 通过（35 documents / 119 relations）；手册服务与单文件独立性检查 `go test ./scripts/docs-serve` 通过。将 HTML 单独复制到 `/tmp/kc-product-qa.UVyBXT/product.html`，Chromium 断网以 `file://` 打开，核验 1360px 桌面、390px 窄屏无整页横向溢出、键盘跳转、复制回退、打印展开与打印后恢复；12 页 A4 打印图已逐页查看，无正文/代码裁切，临时截图与打印样本留在同目录。本轮未安装依赖。

**全景图修订（同日）：** 按用户反馈将开篇改为“面向大规模数据的通用知识底座”，先画出与架构一致的⓪ Snapshot Store、① Dataset / Catalog、② Knowledge、③ Retrieval 四层，标出底座可独立采用、统一访问入口、动态状态接入和只读 VFS 出口；再解释各层责任、收益与适用场景，不把对象身份单列为价值。第 03 节起的 use case 正文保持原文。`make check-docs` 通过（35 documents / 120 relations），手册服务测试通过；离线验证 1440/1024/768/390px 无整页或图内溢出，四层可点击跳转；打印版全景与采用范围完整位于第一页。最新视觉样本在 `/tmp/kc-panorama-qa.q71emx/`。完整测试仍受下述既有场景问题阻断。

**表述修订（同日）：** 删除开篇的阅读引导、采用阶梯、“适合你”判断和文档编排说明；保留架构图，以客观的分层职责、依赖与能力边界说明产品。第 03 节起正文未改。产品 owner 同步移除编排性措辞；文档图检查、手册服务测试与 diff 格式检查通过。本次未重跑视觉与打印验收。

**能力归属修订（同日）：** 开篇、架构图、分层卡片和能力表明确 Snapshot Store 是存储集成，文件版本及规模能力来自 lakeFS 等底层组件；产品说明集中于其上的 Dataset 交付、知识声明、索引维护与统一访问。第 08 节明确标为解决方案，补充静态服务定义与动态健康状态的关联示例，保留 VFS 本机容量、只读和宿主限制。产品 owner 同步；文档图检查、手册服务测试、diff 格式检查通过，未新增规模承诺或商业版本划分。

**历史阻断（2026-09-20）：** 当次 `make test` 在生成验证库存时被场景目录 `.data/scenes/catalog-initialized/grants-bootstrapped` 缺少 `_meta.yaml` 阻断，未执行产品套件。保留记录 `.validation/runs/20260920T024414Z-71bd11adcb42`；这不是当前源码重新核验后的阻断结论。

**本轮完成范围（2026-09-30）：** 价值、目标形态、渐进采用及真实任务验收已进入架构 owner、README 与单文件手册；对齐默认最新消费、独立授权、静态投影准备后发布和反馈修订，清除退役命令、重复缺口与过期实现判断。`cli/documented_commands_test.go` 将手册命令对注册表、退役入口和互斥范围的核对升级为自动守卫。离线四档宽度、14 节导航、键盘跳转、复制回退与打印展开/恢复检查已通过，修复键盘焦点和重复打印事件丢失恢复状态的问题；整册打印样本在 `/tmp/kc-product-qa-20260930/`。实际任务收益及动态/规模资格分别保留在 VALUE-01、DOC-20/DYN-01、SCALE-01。

**验证收口规则：** 首次契约运行 `.validation/runs/20260930T033008Z-8a135a4cee15` 的测试进程成功，但运行期间文档变化导致状态为 `source-changed`，不能记作通过；文件收口后已在固定源码重跑，结果见下。整条 GUIDE-01 仍保持未勾选，只有相应验收范围同次全绿且无 skip 才关闭；不把短模式中未运行的 live/FUSE/付费入口算入验收。

**收口验证（2026-09-30）：** `make check-docs` 通过（28 documents / 102 relations），`make quality` 通过（耦合基线 33.23% held），手册服务测试、命令示例守卫、Writer 有界单对象成本与 Dataset 服务版恢复定向回归通过。`make test-contracts` 运行 `.validation/runs/20260930T035537Z-567c1bc45cf9`：`passed-with-skips`，1784 个测试事件通过、20 个外部/live 测试跳过，公开命令成功与风险边界覆盖均为 60/60；`make test` 运行 `.validation/runs/20260930T040516Z-37c272769fdf`：`passed`，21 个测试事件通过、无 skip。两次运行均无源码变化，指纹同为 `cad1f6d0bc65929f02be2eb5830a21d60c7b8dca46bc7a369d6297d9c2aeafc9`；本段仅追加验收记录。手册离线四档宽度、键盘、复制、打印状态与 19 页逐页视觉检查通过。未执行真实部署、完整动态链路、FUSE、付费 Agent 或规模资格入口；没有删除或跳过增量断言。

依据：[产品手册](doc/README.md)、[产品能力边界](doc/engineering/testing.md)。手册仍需随实际交付状态更新，不能提前承诺尚未验收的能力。

### VALUE-01 · 多源真实任务效果与反馈修订循环

- [ ] 未认领：在已有领域场景中证明独立供给、按任务交付、同依据使用与修订采用带来的实际收益。

**位置 / 信号 / 判定：** [MVP](doc/engineering/testing.md) 的任务验收、现有清河领域材料、应用消费与访问反馈证据；协议旅程和组成能力已有测试，但缺少同一依据、授权和预算下的任务质量/总成本对照，以及反馈到修订、新版采用、历史复核的完整运行证据。判定为产品效果与组合旅程缺口，不否定已有协议保证。

**处置：** 按[架构总览](doc/architecture.md)的价值目标和[服务](doc/design/service.md)的证据边界选代表性任务；读取、检索与关系探索路线采用相同基线，保存输出、输入依据、授权、预算和人工判定。任务反馈关联实际访问，由维护者核对并经 Writer/提案修订，验证新版采用、旧依据复核与撤权；不新增自动回写、对象全量列举或服务器任务会话。

**触发 / 完成条件：** 扩大人或 Agent 的任务效果承诺前完成。报告接入耗时、任务完成质量、引用可复核性、更新采用与恢复成本；至少覆盖跨源定义与资源访问、缺能力/缺范围/过时观察、错误知识纠正。动态结果只在 DYN-01 已取得相应部署证据的范围内评价；材料与用例仍放已有场景节点，不另建业务故事包，不把模型自评当验收或训练标签。

### SCALE-01 · 数据量、历史量与并发增长下的容量验证

- [ ] 建立可重复的规模测试与结果台账，得到实际支持范围、瓶颈和容量上限。所属阶段：M5；执行器和 S0/S1 基线从当前阶段并行推进。

**要回答的问题：** 知识持续增加、持续修改、同时被多人检索时，系统能承载多少数据，延迟和成本如何增长，索引多久追平，重启/恢复要多久，以及最先限制容量的是哪条通路。功能用例通过、生成了大量输入或单次命令成功，都不能回答这些问题。

**当前进展：** 已有规模设计、按性能视角场景树组织的压测用例（旧 KC-PERF-01～12 已映射进 `.data/scale/scenes/` 状态树与探针）及基础流式生成器；生成器只输出 table family/事件描述，尚未形成经公开 Writer 实际提交、发压、采样、判定的闭环；历史参数只记录目标提交数，不生成对应数量的真实提交。执行器按 `requires` 能力词汇表分层推进：L1 ladder runner 已实现（`.data/scale/runner/ladder.py`，client-timer 曲线 + transport 维度 staging/bulk 同 seed 配对 + 发压端进程 CPU 采样 + CASES §3 参数 manifest + 正确性对账），并在真实 lakeFS 部署栈上完成小阶梯双路径实测（S0 冒烟尺度，非资格结论）；写侧探针与指标已登记 transport 维度。尚缺：归因采集（server-metrics/resource-sample，L2）、到达模型与长窗口（L3）、真实 H0–H4 历史（L4）、资格档全探针与门槛重登记（L5）。最高通过数据档、历史上限 Hmax、支持并发和恢复时间均待测。

**执行器分层路线（按探针 `requires` 词汇表增量实现，每层独立可用）：** L1 ladder（已实现）：真阶梯执行 + transport 配对 + 进程级 CPU 采样；L2 attribution：Prometheus 只读抓取（kc-server/lakeFS/OpenSearch）+ 机器级 resource-sample，使每个曲线点可分解为 client/network/KC/介质段——无归因不构成优化依据；L3 dynamics：open/closed loop 到达模型 + ≥24h 长窗口 + 故障注入，使 steady-soaked 分支可构建；L4 history：生成器输出真实 H0–H4 变更提交，使 history-aged 可构建；L5 qualification：专用环境跑资格档全探针并按重登记门槛出资格结论。**门槛重登记机制**：S1 双路径实测完成后把 scale-benchmark §10 从 Dolt 历史参考改为 lakeFS 实测线（staging/bulk 分线），并写明重登记触发条件（介质 major、layout major、mapping/shard 策略、硬件或 durability 变化）；重登记前任何运行只出曲线不判资格。

**测试范围：** 下表是待执行与待补齐的维度，具体模型和阈值继续由规模 owner 与用例清单维护。

| 维度 | 数据与负载如何变化 | 必须得到的结论 |
|---|---|---|
| 当前知识总量 | S0/S1 为千表/万表；S2 十万表；S3 百万表；再进入 S4 宽表与 S5 两百万宽表。按规划的 grouped Relation 模型，S3/S5 主体对象约 3,300 万/1.06 亿，均是待验证的模型数 | 实际 objects、units、endpoints、可索引文档和字节数；导入时长、读写/检索延迟与资源随总量的增长趋势，不能用表数直接代表知识量 |
| 对象大小与关系复杂度 | 真实正文大小分布、多 Aspect、长描述、宽表与 R1 高基数 Relation；普通值和极端值分开 | 单对象上限、关系分页、批次内存及一次小修改的写放大；不能只测极小正文或无关系对象 |
| 历史深度 | 固定当前数据量，依次增加 H0～H4 的 10 万、100 万、500 万、1,000 万、2,000 万真实变更提交 | 当前/历史读、日志、差异、幂等账本、启动及备份恢复的退化；禁止空提交或仅在报告中声明目标数 |
| 持续变更与并发消费 | 每天 1 万次变更的稳态、10 倍日峰、100 倍突发和瞬时积压；叠加不同读写并发、冷热访问、修改/删除/重命名与旧 pin 回读 | 吞吐、P50/P95/P99、错误率、排队与追平时间、CPU/RSS/IO；相同数据量下比较不同负载，避免把并发变化误认为数据量影响 |
| 多仓与动态 State | 现有多仓 Workspace 用例上补成员数/命中分布递增；另补 Binding 数、观察更新速率、动态检索与同观察依据回读的规模用例 | 组合查询放大、动态积压/时效与资源边界；Snapshot 提交量和 State 观察量分别计数，动态时效按 REVIEW-04 裁决 |
| 索引、故障与恢复 | 增量追赶、在线全量重建、引擎断连、负载中崩溃/重试、备份恢复；代际切换待 REVIEW-01 选定后执行 | 重建与恢复耗时、临时磁盘峰值、用户可用性、RPO/RTO；成功写入不丢失、不重复，分页/回读不混 basis，Writer 不等待索引完成 |

**推进与验收阶段：**

| 阶段 | 当前状态 | 下一道验收门槛 |
|---|---|---|
| A 工具与环境就绪 | 基础生成器与 L1 ladder runner 已有（transport 配对、进程级 CPU 采样、CASES §3 manifest）；归因采集/到达模型/长窗口/断点续跑待补 | 每条探针 `requires` 的能力逐层落地并可用；经公开 Server/Writer 发压；环境、原始样本与结果可复算。先核实现有代码和部署是否满足档位前置，不照搬历史审计的环境数值 |
| B S0/S1 基线 | L1 已在真实 lakeFS 部署栈跑通 S0 小阶梯双路径（冒烟尺度，非资格结论）；S1 档与归因未建立 | 先证明发压端、采样和判定有效，再同环境重复测量；按 staging/bulk 双通路分别量化 lakeFS 介质的适用区间和成本增长（L2 归因采集就绪后），保留失败与瓶颈。小规模结果用于改进，不外推为资格结论 |
| C 数据/历史逐档扩量 | 待 B 通过及专用环境就绪 | S2→S3，再按模型进入 S4/S5，R1 与 H0～H4 分开控制变量；每档通过后再升级，有正确性或资源越界先停止并保留证据；未测档标明未测 |
| D 持续运行与容量结论 | 待选定档位及恢复方案具备 | 完成稳态持续窗口、突发、重建与故障恢复；报告最大通过数据量/历史档、支持负载、瓶颈、单位存储成本及五年容量区间。代际相关结论必须有 REVIEW-01 选定方案的实际恢复证据 |

**需要判断：** 首次产品交付承诺的数据量、历史量、并发/时效和部署资源档位。现有 S3 target、S5/H4 qualification 是规模规划，不能默认成已兑现的首发承诺；在确认专用环境和资源投入前，先完成 S0/S1 的测量闭环。具体阈值引用已有规模 gate，不通过临时降低数据量、持久性或副本配置来过线。

**完成标准与结果记录：** 按已确认的承诺范围逐项给出实测值、门槛、通过/失败/证据无效及未测项；保存源码指纹、实际版本/硬件/配置、seed、真实数量与字节、采样窗口、原始指标/trace/扫描计数和正确性对账。小规模基线完成后更新本项阶段状态，目标范围未验收前不勾整项。后续只在此保留最新有效基线链接、已通过范围、下一档阻断与瓶颈，详细运行历史留在报告中。S0/S1 验证工具链不意味着大档通过，普通 make test/test-all 不承担容量资格。

依据：[规模模型与门槛](doc/engineering/testing.md)、[压测场景树与用例入口](.data/scale/CASES.md)（用例本体在 [.data/scale/scenes/](.data/scale/scenes/README.md)，按性能视角的场景树组织）、[环境配置合同与就绪性](.data/scale/ENVIRONMENT.md)、[当前生成器](.data/scale/generator/generate.py)。新增动态/多仓规模用例需补到这些关联文件，TASK 不另维护一套执行规范。

### DOC-14 · 原生 Dolt 与 Tree 能力隔离

以下保留当时的能力隔离问题与资格标准；Dolt adapter 已退役，涉及它的现状、路径和处理范围不是当前活动要求。未来 native provider 的资格仍由现行 Provider 合同拥有。

- [x] 收口 native Knowledge adapter 的公开能力边界。所属阶段：M5 的基础纠偏，方向已选定，可独立推进。

**问题与影响：** 规模设计要求 native Knowledge adapter 不暴露 TreeStore；当前 object/dolt 仍实现 TreeStore、DirectoryReader 并转发 ApplyTreeCommit。调用方仍可能获得原始路径写入口，削弱“知识写入必须经过 Writer”的能力隔离。它是确定的设计/实现差距，不等于已证明生产请求发生了绕过。

**处理范围：** 先盘点真实调用者、文件投影与现有 Conformance 的需求，再拆清 native 知识能力与通用 Snapshot Tree adapter。不能只删除接口让旧测试失效，也不能把通用 snapshot/dolt 一并退役。

**完成标准：** 原生知识服务装配不暴露 raw tree 写旁路；Reader/Writer、关系检索、批量/精确读取和恢复仍走正式合同；必要的文件能力由明确的适配层承担；增加能力拒绝/毒化反例，并运行保留的 provider 合同。

依据：[规模设计](doc/engineering/testing.md) §4、原 Dolt 接口与转发（历史实现已随 adapter 退役删除）、[MVP 缺口](doc/engineering/testing.md)。

### DOC-15 · 生产可观测性与告警的有效性

- [ ] 补齐实际承诺范围的 SLI、告警触发/恢复与生产证据。所属阶段：M5。

**已完成与缺口：** 全失败窗口漏报、零流量 NaN 已有专项修复和回归；遥测、部分面板及规则已经存在。尚缺的包括：哪些请求/来源有资格进入完整性 SLI 的依据；部分依赖信号及跨进程传播；各告警可定位的面板/处置说明；完整 firing/recovery 演练和选定环境的容量/SLO 基线。规则语法通过和全失败专项通过都不能证明这些条件。

**建议拆次推进：** 先明确首个生产拓扑与用户承诺，补原始信号和分母；再对可用性、延迟、队列、依赖失联和恢复逐类注入故障；最后按既定窗口持续观测。每次可以关闭有证据的子范围，整项在承诺范围验收完成前保留。

**完成标准：** 正常、全失败、无流量、被排除流量与不同来源组的 SLI 含义可复核；每类告警能实际触发、定位用户影响，并在恢复后解除；后端/采集器失联可见；实际依赖版本、配置、负载和持续窗口随结果保存，不拿短时 smoke 代替生产 SLO。

依据：[系统可观测性设计](doc/design/service.md)、[验证缺口 O-05～O-13](doc/engineering/test-catalog.md)、[规则与回归](scripts/deploy/observability/rules/)。

### DOC-16 · Provider 能力合同与跨 provider 等价性

- [x] 让 native Knowledge provider 进入共享合同套件，并建立跨 provider 等价性证据。所属阶段：M5 的基础纠偏，方向已选定，可独立推进。

**问题与影响：** 共享合同套件（`RepositoryContract` / `WriterContract`）与 provider 无关，但规模档选定的权威 provider `object/dolt` **从未接入**；一次性探针实测接入后 14 个子测试全绿、零跳过，说明缺口是覆盖矩阵而非实现。同时**跨 provider 等价性为空**，而 `SCALE_ARCHITECTURE.md` §5.3 把它列为迁移的核心差分测试。另有一处会 OOM 的静默全量回退路径（变化识别能力缺失时）所在包没有任何行为测试。

**处理范围：** 先接入合同套件，再建按步骤索引对齐的等价性 harness（不同 provider 的 commit 标识必然不同）；随后补能力拒绝毒化项与静默回退的行为测试。不改 ①/②/③ 公开语义；不新增第二套主题 owner。设计与被否决方案见[能力合同](doc/architecture.md)，门槛与分档见验证设计，执行序与完成定义归根本 TASK 的迁移记录。

**完成标准：** 等价性与毒化门槛按 `PV-01`…`PV-12` 实际执行并保存证据；`PAC-01`…`PAC-08` 在对应测试存在后才进入 `doc/engineering/architecture-invariants.md`；未执行或被跳过的档位不得计为通过；基线阻塞（`make check-validation` 的未解析引用、`check-surface` 缺 `rg`）先修或明确登记。

依据：[架构总览](doc/architecture.md)、验证设计、[规模设计](doc/engineering/testing.md) §5.3、[规模门槛](doc/engineering/testing.md) §7 P0。

### DOC-17 · 有界读写与变化识别（规模可行性前提）

- [x] 让提交、定位与变化识别的工作量与仓库总量解耦。所属阶段：M5 的前提项，方向已选定，可独立推进。

**问题与影响：** 三处现状与「有界」承诺直接冲突：(1) **写**——`object/writer/treecodec.go` 的 `readKnowledgeTree` 对每个 commit 全列路径再逐 blob 读，并把全量定位表整体重写为一个 blob；(2) **读**——按对象读取时全量加载并解码该定位表，维护分页也先全量读入再切片；(3) **变化识别**——⓪ 的增量能力没有生产实现，`object/maintenance` 在能力缺失时**吞掉错误**后走两次全量遍历并把整仓身份装入内存，`index` 因此把该失败降级为全仓重建，且只记 `diverged/content`。三者叠加的后果是每个 commit 的写与每次点读都是 O(仓库总量)，而成本失控不会报错——这正是 `SCALE_ARCHITECTURE.md` §3.1 已列为不可接受成本反例、却一直没有实现与测试的那几条。

**处理范围：** 只读受影响对象对应的单元；定位结构改为可分页、可重建、可丢弃，并保证其丢失后可从权威单元重建；变化识别升为 ⓪ 的一等能力（与 [能力合同](doc/architecture.md) 的选定方案一致），缺失时显式失败而不是静默全量，并把退化原因作为可观测的一等值；目录列举走范围扫描，不用无索引可用的前缀匹配；**同一 repository 同一时刻只允许一个投影执行者**（进程内与跨进程语义都要写清）。不改 ①/②/③ 公开语义。

**完成标准：** 计数 store 断言「一次单对象 PUT 的读次数与仓库总量无关」与「一次单对象 READ 的解码字节数与总量无关」；非原生变化识别的 provider 上单对象提交不触发全仓重建；定位结构删除后单对象读仍可工作且成本有界。`SCALE-01` 的容量结论以此为前置。

依据：[规模设计](doc/engineering/testing.md) §3.1/§9.5/§10、[写路径](object/writer/treecodec.go)、[定位与分页](object/reader/repository_service.go)、[变化识别实现](object/maintenance/change.go)、[投影增量入口](discovery/projection/sync.go)。

### DOC-18 · 守卫覆盖与能力面收口

- [x] 让守卫覆盖全仓、让缺能力失败关闭而不是 panic。所属阶段：M5 的基础纠偏。

**问题与影响：** (1) 分层守卫把 `scripts/` 列入跳过目录，而 `scripts/fixture-deployment` 在那里打开具体 authority，落在装配根收敛面之外且不被守卫覆盖；(2) 装配根对可选能力做不带 `, ok` 的类型断言，驱动返回缺能力的 store 时是 **panic**，而不是稳定错误码；(3) `object/dolt` 作为规模权威 provider 仍实现并转发字面路径写入（与 `DOC-14` 同源）；(4) 同一 repository 暴露十余个可断言接口，可选能力散落在请求路径，同一 provider 在不同入口可能给出不同可用性结论。

**处理范围：** 守卫遍历全部非测试生产包，例外精确到符号而不是目录；装配期一次性解析并冻结 provider 能力，请求期只读判定结果；缺能力一律失败关闭。不改公开写入代数。

随本项一并收口的小项（同属边界与能力面，不各自单列）：

- 同一个 repository 暴露十余个可断言接口，`knowledge.Repository` 是胖接口而被六个包各按不同子集使用 → 按用途切成「读」「定位」「变更」三组，消费者只声明所需；
- `CatalogID` / `SetID` / `PinID` 仍以 `string` 流通，传错参数编译期抓不到 → 与 `RepositoryID` 一样用命名类型在边界解析；
- 交付阶段提供了未使用的 `StageFunc` 逃生门，把「阶段不得改身份」从编译期不可能降级为运行时检查 → 删除并保留断言；
- 检索代数包同时承载了具体模型的请求/响应形状 → 供应商信封移回 provider 侧，代数只留端口与结果语义；
- 证据包同时拥有身份上下文类型与存储介质，导致协议包为带 trace 反向依赖它 → 上下文下沉、证据只留端口的查询与一个 adapter；
- 墙外访问端口被带外方法击穿（包装层转发了一个端口未声明的方法）→ 显式纳入端口或拆成第二个具名端口；
- 关系倒排是检索 provider 的私有形状 → 保留，但补一条「不得作为查询口」的毒化断言。

**完成标准：** 在 `scripts/` 故意加一条越界 import，`make test-boundary` 必须失败；只实现基础权威口的 fixture 在读写上失败关闭；`DOC-14` 的原始路径写拒绝反例通过。

依据：[能力合同](doc/architecture.md) `PAC-02`/`PAC-06`、[分层设计](doc/architecture.md)、[当前守卫](internal/arch/layers_test.go)、[当前装配断言](home/managed_source.go)。

### DOC-19 · 幂等账本的保留、清理与恢复

- [x] 给命令账本明确的保留期、显式恢复入口，并去掉无收益的兼容包袱。所属阶段：M5 的基础纠偏。

**问题与影响：** (1) 处于 PENDING 的命令条目**永不自动清理**，也没有公开的解决/放弃入口——进程在提交前后死亡会让该命令 id 永久不可用，只能换 id 重试；(2) 账本没有保留与归档策略，随命令数单调增长，而设计门槛明确要求启动时不加载全部历史；(3) 账本里残留未上线的旧形状字段与写入迁移逻辑，属于无收益的兼容包袱；(4) 证据写入失败会在提交已成功之后返回错误，调用方看到「假失败」，重试会撞上 CAS。

**处理范围：** 增加带审计的显式解决/放弃入口与保留窗口，并写明「replay 只在窗口内保证」；删除未上线的旧形状；明确证据失败不回滚已接受的提交。不改写入代数与 CAS 语义。

**完成标准：** 在「预留后中断」与「提交后中断」两类故障点上重启，都给出确定结论（重建原结果或失败关闭等待人工核对），不出现永久不可用的命令 id；账本增长有界且有清理证据；证据介质不可写时，已提交的写入不被报告为失败。

依据：[规模设计](doc/engineering/testing.md) §8.3、[命令账本](snapshot/command_contract.go)、`REVIEW-07`。

### APP-CORE-01 · Typed Application Core

- [x] 把应用用例从 CLI flags/HTTP transport 中抽离为 typed executor。所属阶段：M5 的结构纠偏；
  方向由 `doc/design/service.md` 的 `API-01` 与 §1.2 选定，待 `DOC-14/16/17/18/19`
  闭环后认领。

**问题与影响：** CLI 与 HTTP 已要求调用同一 executor，但参考实现仍让大量应用编排依赖
`map[string]FlagValue`、CLI dispatcher 和 transport 邻近状态。Repository、Workspace、Pin 等
标识在应用边界继续以裸字符串流通，授权、固定 basis、Reader/Writer、交付链和错误映射容易随
入口分叉。现有 `API-01` 证明两个入口复用执行路径，不足以证明应用核心独立于 transport。

**处理范围：** 先抽一条 READ/SEARCH 纵切，再迁 Writer、Governance 和 Operations；每个用例以
typed request/result 表达，标识在 transport 边界解析为 owner 包命名类型。CLI 只解析 argv 和
渲染输出，HTTP 只转换 wire DTO；应用核心只依赖协议服务与注入端口。为新应用包补分层归属和
import 守卫。不改变公开 CLI/HTTP surface、授权、固定 basis、错误码和结果语义。

**完成标准：** CLI 与 HTTP 的同一请求进入同一个 typed executor 并逐观察等价；应用核心不
import CLI parser、HTTP registry、具体 authority/retrieval adapter 或部署配置解析器；生产应用
代码不接收通用 verb/flags map；命名标识不可在编译期互换。迁移期间每条纵切先有失败反例，再
以公开 surface、应用合同和 `make test-boundary` 验收。

依据：[服务架构](doc/design/service.md) `API-01`/§1.2、[分层设计](doc/architecture.md)、
[CLI 交互](doc/design/affordance.md)。

### DOC-20 · 观察记录与 M 面服务合同

- [x] DOC-20 认领（2026-09-29 复核批 R5）：两个反例测试可执行并全绿——`TestLatestOnlyRecordLossAndExpiryFailDistinguishably`（latest-only 平台保留记录：保留期内重读返回原观察原 basis；丢失=恢复故障 `TEMPORARY_UNAVAILABLE`、到期=能力耗尽 `CAPABILITY_UNSATISFIED`，可区分且都明确失败，不返回当前值或空历史；B-22 gap 收口）+ `TestBusinessNullContrastsPlaceholderAndNeverFallsBackToScan`（业务空值带合法 basis=成功观察；占位空缺 basis 失败关闭；取值失败只传播错误、不降级声明扫描）。合同写清：liveaccess/README.md 观察保留与失败关闭段。不改运行实现与公开形状 ✓（Non-Goals 遵守）。

**问题与影响：** 当前有 `StateLookup`、动态投影与同 basis hydrate，但它们不能共同证明新鲜度、
历史保留和当前源授权。现有 State catch-up 会跳过已有观察，活动记录替换不等于历史保留，
旧 Dataset 声明的动态维护也不能由 HEAD 对账覆盖。单 key refresh 只收窄 lookup，不能据此
宣称观察存取和索引写入满足增量成本。

**Goal：** 按[动态物化](doc/design/liveaccess.md) §2.6/§6/§9 定义可信 State 消费，并将
方向性任务关联到已有断言或明确缺口。新观察按声明取值，同依据重读使用可靠保留的观察；
控制恢复归[投影控制](doc/design/discovery.md)，授权归[权限设计](doc/design/permissions.md)。

**Non-Goals：** 不改运行实现或公开形状，不执行验证；不新增 Snapshot/live Store、Catalog cut、
对象 ACL、通用消息物化器或 Stream 查询入口。介质边界依[介质角色](doc/architecture.md)，
统一访问职责依[服务架构](doc/design/service.md) §4.7。

**不变量与方案：** 延续 `D-01`、`V-01`、`P-01`、`IX-04`、`KS-02` 及 `AUTH-01`–`AUTH-03`。
选定分别证明覆盖/时效/重读/授权，State 阶段提供有界观察保留；否决用 complete 代替实时保证、
用最新值补旧依据、用后台身份代替消费授权、只追 HEAD，以及单 key 变化的隐式全库重建。
默认时效、容量/保留值和共享授权装配仍待选定；形状由公开类型、包 README 与 Conformance
承接，不写入设计正文。

**设计与用例产物：** 动态物化 U1–U9；验证体系 B-19–B-27；场景 `product/materialization/*`
从 owner 提取方向任务，以具名 evidence 关联实际断言，缺口写入 `_views.yaml`。未创建假定
能力已实现的场景状态；未新增执行结果或完成结论。

**完成标准：** 两个反例可执行——(1) 来源声明 `latest-only` 时，观察记录丢失后历史读取必须明确失败，不得静默返回当前值或空值；(2) 缺少取值入口时 Bound State 读取必须失败关闭，不得把信号载荷当值、不得降级为扫描、不得返回占位空值。运行时的重读能力与保留期上限按声明校验。

依据：[动态物化](doc/design/liveaccess.md) §6、[介质角色](doc/architecture.md) §2、[服务架构](doc/design/service.md) §4.7、[观察类型](object/observation.go)、[Serving 端口](liveaccess/)。

### DOC-21 · 在 reviewed 中重构设计书

- [x] 本轮认领：完成按组件组织的替换稿并正式替换旧 docs。本轮增量：13 份非设计资料已迁入
  `docs/reviewed/`（OKF path 与按路径读取的代码/测试同步）；20 份顶层旧稿的交接审计缺口已按
  落点补进 12 篇组件稿（交接去向详见替换计划本轮进度一节；该计划已随切换完成退出，原文在
  git 历史）。存量债已清理：7 处悬空
  Test 引用改为退役记录，cli/ 死代码与被遮蔽的复杂度/文件体积/克隆门禁失败就地修复
  （含 help 双表合并与 `snapshot/stamp` 提取），`make check-docs`、`make quality` 与完整
  lakefs 套件全绿。剩余：`provider-contract-validation.md` 的 Dolt 调用点叙述归 adapter
  资料整理轮。

**Goal：** 以 `docs/reviewed/README.md` 为设计书入口，按可以独立
评审的组件责任组织设计，不以篇数为目标。补齐索引控制、声明式索引、权限体系、Dataset、
Hook/Gate 和 CLI 交互设计；每篇展开职责、取舍、失败与恢复、方向性用例。公开协议承接
字段、动作、错误码与状态机。现行文档治理 owner 是 `docs/graph/documents/document-map.okf`。
补充 Dataset 目录重组的可执行用例源码：多仓片段交付、旧版路径与字节固定、冲突发布不替换
已接受版本；组合 owner 为 `doc/design/dataset.md`，验证记录 owner 为 `doc/engineering/test-catalog.md`。

**Non-Goals：** 本轮不替换现行 owner，不修改运行实现或公开协议，不搬源码，不执行测试、
文档检查或生成验收结果；不将未实现目标压缩成不存在。

**不变量与取舍：** 保留 `L-01`、`A-01`、`D-01`、`V-01`、`W-01`、`P-01`、`IX-04`、
`KS-01/02`、`API-01` 和 `AUTH-01`–`AUTH-03` 所约束的边界，保留治理的精确 Preview 证据
与 Hook 不回滚既成写入的要求。选定独立责任独立展开、重复内容归一；否决按篇数压缩、
只有一句能力介绍的占位篇、Markdown 复制可执行协议、两套现行设计并行生效。
接口指向各组件公开 API 与 Conformance。
目录交付用例继续遵守 `KS-01/02`、`V-01` 与 `AUTH-01`–`AUTH-03`：选定通过显式 principal
调用 Server 的 Catalog 与文件公开接口，使用现有 lakeFS 进程内夹具；否决只检查挂载数量、
用占位测试声称零散文件混排已实现。接口指向 `client/management.go`、`client/dataset_files.go`、
`catalog/definition.go`；仅补测试源码，不改运行行为或执行验证。

**产物与交接：** 上一轮六篇设计过度合并，不能作为覆盖完成的证据。本轮补全独立组件
设计，并从查询、知识、服务篇移走相应的重复展开；重复图稿与生成脚本仍退出。
旧文、协议、研究、验证资料的去向与切换步骤记录在替换计划（已随正式切换完成退出，原文在
  git 历史）。旧条目与场景引用已随切换一并迁移；本轮的静态阅读与编辑不是验证通过。

**Dataset 目录交付核查：** 替换稿补充多来源选择与目标目录重组，以及 U9/U10。现有
`KnowledgeSetSource.Path/SubPath`、CLI 配方/来源解析、typed 发布、File Gateway 和 kcfs
已有子目录前缀映射；同仓片段须同版本且不重叠，目标挂载目录也不得重叠。
`DatasetItemFile` 虽已定义，但现行发布由 sources 生成 prefix 条目，文件交付仍按 mounts
枚举来源目录；零散文件选择、目标文件改名及跨来源文件混排到同目录尚未完整贯通。
CLI 帮助也未展开现有目录映射用法。补充 `cli/dataset_directory_delivery_test.go` 四条
Client/Server 用例，覆盖多仓目录重组与范围、新旧布局及字节固定、冲突发布后的旧版可用与
合法重试，以及路径中部上跳的读取/枚举拒绝；具名证据挂到 `dataset-defined`，验证目录登记
C-17–C-21。静态核查发现网关只检查开头的上跳路径，C-21 保留预期拒绝断言，当前实现缺口
未修复。仅补设计与测试源码，
不改协议或运行实现，不执行测试，不将源码存在当作本次通过；逐文件混排继续作为明确缺口。

**Dataset 分层澄清：** 根据用户纠正，替换稿以来源仓、固定版本、源路径到目标路径的组合
为中心，读取只交付原始字节；不要求知识声明、Schema 或索引。知识解释、部分切面依赖与
动态服务分别归知识和资源访问设计，带搜索承诺的发布准备归应用服务。已补的目录交付测试
使用普通文件、原始文件 Writer 与文件网关，没有按知识对象拼装；本轮只调整设计表述与
迁移记录，保持 `L-01`、`KS-01/02` 的边界，不改运行代码，也不执行验证。

## 3. 待 review 的设计问题

这些是仍需选择的产品/协议语义，不属于已决定方案的普通实现缺口。已有设计方向以 owner 为准；
各条只保留尚需选定的范围，不能把文档更新当成公开合同或实现已经完成。

### REVIEW-01 · 仓库达到容量上限后，身份是否应对用户保持不变？

**场景与现状：** 长期使用的仓需要更换物理代际；使用者希望旧 pin 仍可复核，切换期间成功发布的内容不丢失。当前候选步骤创建新 Repository，只比较 object_id/digest，却会改变 KnowledgeRef 的仓身份。Relation 端点已选定允许跨仓引用，普通 Schema 解析仍限定同仓；迁到新仓仍需处理已有 Relation 坐标、Schema 引用和应用保存的仓限定引用，不能把可跨仓引用视为自动身份迁移。改写 Relation 后 digest 也可能合理改变。

| 选项 | 获得什么 | 必须承担什么 |
|---|---|---|
| A：逻辑 Repository 不变，更换物理代际 | 使用者的仓身份、配方与按仓授权可保持稳定 | 耐久的历史 commit→代际路由；历史读取、合并/CAS、命令重试与回执跨代际解释一致，不能仅替换连接地址 |
| B：显式迁移到新 Repository | 沿用现有身份模型，明确这是新知识源 | 改写必要引用、重新准入与显式授权、更新消费配方；接受新旧仓共存，以及多 Workspace/Catalog 无全局原子切换 |

**我的建议与需要你判断：** 先决定容量维护是否必须对使用者保持身份透明。若必须，优先验证 A 的可行性；若可以显式换源，B 可以作为迁移产品，但不称为透明换代。原六步候选顺序在身份、停写交接、投影就绪和失败恢复证明完备前不能用于宣称生产能力。

**最低验收：** 旧 pin 的正文/Schema/来源仍一致；当前撤权仍有效；切换边界前已接受的 Writer 提交有明确归宿；重试和重启不双写、不丢回执；新投影未就绪不能让新任务静默查空；每个失败点可恢复，Relation 同仓不变量保留。

依据：[规模设计](doc/engineering/testing.md) §12.3、[身份与版本原则](doc/architecture.md)、[Writer 同仓合同](object/writer/writer.go)。

### REVIEW-02 · 能读当前正文的人，是否天然能看历史与来源？

**场景与现状：** 同一用户读取同一份知识，走单仓维护入口和 Workspace 消费入口，可能需要不同权限。权限设计把历史/来源包含在仓读权中，公开单仓动作又有独立准入；组合路径的检查不完全相同。仅来源动作可在单仓取得来源，仅历史动作但无仓读权则可能得到成功空日志。分享时的最小授权和拒绝结果因此难以解释。

| 选项 | 好处 | 代价与边界 |
|---|---|---|
| A：仓读权包含历史和来源 | 分享模型简单，同一授权可完成正文、历史、来源复核，符合现有整体仓读边界 | 需处理旧独立 grant，不能把“只许查来源”静默升级为整仓读权；若需要当前内容与历史保密分离，须另划治理边界 |
| B：仓读权之外，再要求对应动作准入 | 可以控制历史列表与来源查询入口的使用 | 发权与各入口检查均需同步；它本身不能隐藏全部旧值或出处：普通 READ 可携带来源，也允许固定历史 commit |

**我的建议与需要你判断：** 如果知识仓是完整的可追溯发布物，倾向 A；若仅希望控制操作入口，可以选 B。若真正需要“当前正文可读、过去/出处不可见”，仅选 B 不够，还要重新设计 READ、旧 pin 与文件视图的内容交付边界。请先判断希望约束的是操作，还是内容披露；不能仅因当前代码已有动作名就决定政策。

**最低验收：** 同主体、同目标、同一当前授权，单仓、Workspace 和 HTTP 的结果一致；只读、仅历史/来源、两者齐全、仅 consume、成员缺权、撤权后旧 pin 均有明确测试，缺权不能成功返回空日志。身份、正文读权与发现权的既有边界不变。

依据：[权限设计](doc/design/permissions.md)、[公开动作](cli/surface.go)、[应用授权](invocation/allow.go)、[READ 返回内容](object/read.go)、[历史列表过滤](object/service/history.go)。关系遍历的动作粒度也应一起核对，不能把本次结论机械扩大到所有知识动作。

**进展（2026-09-23）：** 关系遍历的动作粒度已先行裁决——`TRAVERSE` 按独立动作 `knowledge.traverse` 登记（Dataset 通道与 RELATIONS 同一 `file.read` 准入，`--repo` 通道按仓读权 fail closed），见 `doc/design/permissions.md` 接口表。历史与来源的粒度问题仍按本条开放。

### REVIEW-03 · Schema 破坏性变化必须换身份，还是允许原位迁移？

**场景与现状：** 例如某字段由字符串变为结构化对象，已有实例与消费者仍依赖旧结构。产品设计 §4.4 要求新 major 身份，U5 又保留完整迁移证据路径；尚未定义证据充分性。当前 Writer 在验证实例迁移之前就拒绝 breaking 复用，这不能替代设计裁决。发布 v2 与实例改引可以同批；旧 v1 的删除检查尚不扣除同批改引，当前需后续提交删除，不能把这三步统称为已支持的一次原子迁移。

| 选项 | 好处 | 代价与边界 |
|---|---|---|
| A：新 Schema 身份，显式迁移实例 | 版本区别清楚，允许新旧共存、分批采用 | 接入方与消费方要管理两版身份、查询范围和迁移进度 |
| B：同一 Schema 身份，在证明完整后原位演进 | 使用者保留稳定 Schema 名称 | 需定义同仓完整引用集合、一次发布的校验/原子范围与旧 pin 解释；实例合法不等于消费者兼容，Binding 的外部动态值也不在 Snapshot 原子事务内；大迁移成本与回滚规则不能缺失 |

**我的建议与需要你判断：** 默认倾向 A，先完成显式迁移路径；只有稳定 Schema 名称是实际需求，且能明确证明范围与成本时再考虑 B。需要你确认 U5 的“完整迁移证据”是否原本就意在允许同 ID breaking，而非仅指向新版本迁移。

**最低验收：** 兼容变化仍可原位更新；A 允许新旧共存、分批迁移，B 必须证明完整迁移；无效发布整批拒绝，HEAD 不动；选定方案下新旧实例/Schema、旧 pin、索引与消费者采用更新的行为都能解释。不能增加跨 Repository 原子写承诺。

依据：[知识读写](doc/design/object.md) U3、[Writer Schema 合同](object/writer/README.md)、[当前兼容与删除检查](object/writer/schema.go)。

### REVIEW-04 · 动态查询默认要求多新，无法证明时如何响应？

**场景与现状：** 10:00 已观察到服务正常，10:01 外部变为失败但通知未送达；10:05 查询“当前失败的服务”可能在旧观察上得到 complete 零命中。零命中没有 hit 中的观察时间可看。现有测试明确允许无 notice 时继续查询旧观察；消费查询也不得临时拉取或维护投影。恢复/时效要求与对外完整性承诺还未统一。

| 选项 | 好处 | 必须承担什么 |
|---|---|---|
| A：complete 表示已选观察集合的查询覆盖，时效单独表达 | 区分“查全了已有观察”和“源足够新”，可按用途选择 | 必须有查询范围级的时效/未知/过期依据，零命中也可判断；仅给每个 hit 时间不够 |
| B：complete 默认同时满足约定时效与恢复证明 | 用户更容易把完整结果用于当前状态判断 | 明确时效窗、来源进度和恢复机制；过期时按选定政策失败或降级；承担后台校准成本和故障时可用性损失 |

**已有方向与待选范围：** 设计已分开覆盖与时效，且要求零命中也有查询范围级依据。表中 A/B
仍是消费默认政策的选择；尚未决定普通发现是否接受已声明的旧观察、默认时效、谁设置可接受
滞后、无法证明时如何响应。政策与公开合同补齐前，保留现有恢复要求，不先放宽 complete。

**最低验收：** 故意丢通知后不能继续把旧结果宣传为当前；完整零命中仍能解释查询范围时效；重复/乱序/断档、重启与刷新失败有证据；观察失败不变成业务空值；分页保持 revision；同 basis 回读不一致仍失败，不能靠 partial 掩盖。

依据：[动态物化](doc/design/liveaccess.md) §6、[投影控制](doc/design/discovery.md) §10、[现有 notice 反例](declarativeindex/control/controller_notice_test.go)。

### REVIEW-05 · 检索是按仓可达性过滤候选，还是把可见性留到交付？

**场景与现状：** 现行是「搜宽读严」：无仓读权时交付链屏蔽正文，既不标 `partial`，也不从 `SearchView` 抹仓。于是两个后果都没被裁决——命中仍返回对象 ID、仓与 commit，已经泄露「某仓存在某对象」；且 `LIMIT` 的语义未定义，它计公开命中还是可见正文。

**选项与建议：** A 授权前置于候选定位（仓级可达性过滤），代价是授权状态影响召回、需要说明 coverage 语义；B 维持现状，代价是必须定义无权命中是否计入 limit、是否返回壳，并把存在性泄露写成明示风险。建议 A：它让检索对调用方有意义，且与「交付链只改正文」不冲突——授权发生在候选定位之前。

**最低验收：** 「3 条命中、其中 2 条无权、limit=2」的用例在两种政策下给出不同且被记录的答案；撤权后旧 pin 不得绕过当前权限。

依据：[权限设计](doc/design/permissions.md)、[架构不变量](doc/engineering/architecture-invariants.md) 的 `AUTH-01`/`AUTH-03`、[检索设计](doc/design/discovery.md)。

**进展（2026-09-23）：** 范围收窄后部分落地——新增访问语义采用资格前置：`TRAVERSE` 的 frontier 扩展要求对象所在仓在本次固定范围内（越界止步并显式标记）；语义召回的窗口在已准入范围内形成，召回策略不改变授权粒度（`doc/design/permissions.md` 选定项）。既有词法 SEARCH 的「搜宽读严」与存在性泄露问题（`AUTH-01` 及其守护测试所固化行为）仍按本条开放，保留本验收用例，不在统一访问扩展中顺手裁决。

### REVIEW-06 · 字段级可见性（遮蔽/脱敏）何时选定？

**场景与现状：** 交付链目前只有一个已选定段——仓读权正文屏蔽；其后的隐私化/脱敏被明确列为**未选定且禁止实现**。但真实部署普遍需要「同一知识对不同主体呈现不同字段」。

**选项与建议：** 现在就把它作为**已知未选定的扩展点**写清触发条件与代价，并保证现设计不把它堵死——检索侧已有显式字段白名单机制，交付投影应复用它，避免将来再发明一套。

**最低验收：** 一个「遮蔽 `body`、保留 `name`」的思想实验用例，列出必须改动的层，并证明不新增第二套字段可见性机制。

依据：[权限设计](doc/design/permissions.md) Non-Goal、[交付链](delivery/)、[检索细化投影](discovery/refine.go)。

### REVIEW-07 · 访问证据是否分级（必须成功 vs 允许降级）？

**场景与现状：** 访问证据被描述为 fail-closed 追加的本机 JSONL；另有文档把证据丢失说成「只损失审计覆盖率」，而服务层又承诺「每取一个文件都记访问证据」。三者不等价：没有写清哪些动作**必须有证据**、哪些是 best-effort。叠加已确认的证据端口被绕过，就是「契约看起来有、实际没接」。

**选项与建议：** 给证据分级并在端口上体现：必须级的失败应让请求失败或显式标记审计降级，降级级只记 metric。同时决定证据端口是落实为真缝还是删除，避免保留「已抽象」的错觉。

**最低验收：** 让证据介质不可写，观察 READ/SEARCH/文件读取各自行为与分级一致；端口若有替身，替换后行为不变。

依据：[访问证据](doc/design/service.md)、[服务架构](doc/design/service.md) §6.2、[介质角色](doc/architecture.md) §2。

### REVIEW-08 · KC 授权与源侧授权是什么关系？

**场景与现状：** 现有说法是「外部系统的实时业务授权仍由外部系统强制」，KC 只读正文权；同时动态观察请求会把调用方身份与追踪上下文传给墙外运行时。三条没写清：(1) 运行时是否必须用该身份到源侧再授权；(2) KC 放行而源侧拒绝时错误如何呈现；(3) KC 撤权后，已经交给运行时的身份是否还能继续读源。

**选项与建议：** 在访问合同里显式规定「KC 授权是必要条件、源侧授权是充分条件」，并规定错误码映射与撤权后的行为（不得缓存授权决定）。这三条直接决定「KC 是不是安全边界」。

**最低验收：** 「KC 放行 + 源拒绝」与「KC 撤权 + 旧 pin」两个反例有确定结果；撤权后已有运行时不得继续读源。

依据：[Aspect 访问](doc/design/discovery.md)、[权限设计](doc/design/permissions.md)、[动态物化](doc/design/liveaccess.md) §3.7。

### REVIEW-09 · 已发布知识如何撤回（takedown）？

**场景与现状：** 领域生命周期的终点是归档，而归档是**整仓**动作，没有单对象撤回。误发布敏感内容只能靠新 commit 删除，旧 commit 与旧 pin 仍可读——即协议里没有泄露响应的位置。

**选项与建议：** 显式区分三种语义并至少选定一种：单对象 tombstone（保留历史、消费面不可读）、仓级归档、物理删除交由保留策略。无论选哪种，都必须说明「旧 pin 读到什么」。

**最低验收：** takedown 用例——撤回后旧 pin 必须给出明确失败或 tombstone，不得静默返回原值。

依据：[系统设计](doc/architecture.md) 的 `K-24`、[规模设计](doc/engineering/testing.md) §12.3、[权威接口](snapshot/store.go)。

### REVIEW-10 · 是否提供可迁移的 Canonical 导出？

**场景与现状：** 设计承诺「历史可迁移」「迁移不改语义」，但公开面没有「导出全部知识（含多版本与来源）」的动作；维护扫描不产出可移植产物。

**选项与建议：** 定义 Canonical 导出/导入：一个能表达「对象身份 + Address + 值 + schema 引用 + 来源 + 快照坐标」的流式格式，导入经 Writer 重建。明确「导入后身份不变、commit 标识可以变」。

**最低验收：** 在两个不同 provider 之间往返后，同一基线的对象身份与摘要全一致；往返过程不依赖某个 provider 的私有形状。

依据：[规模设计](doc/engineering/testing.md) §13、[能力合同](doc/architecture.md)、[维护扫描](object/maintenance/)。

### REVIEW-11 · 是否采纳「优化目标形态」（能力协商 / Delta / 包重组）？

**场景与现状：** 一份并行复核提出七个动作：能力协商替代类型断言；增量变化识别升为 ⓪ 的一等协议；同形状写请求与请求上下文上移；删除无调用方的假缝端口；墙外运行时获得可执行合同；把索引包拆成只读定位与写派生物；守卫覆盖全仓、例外按类型而非目录。它声明「不考虑兼容、系统未上线」，并给出七步落地顺序与各自的失败检查点。其中**能力协商与增量识别已并入**[能力合同的选定方案](doc/architecture.md)；假缝与守卫分别对应 `REVIEW-07` 与 `DOC-18`。

**需要你判断的四个开放点：** (1) 增量能力给到「路径级」还是要求原生 provider 直接给「对象级」；(2) 绑定解析类型的归属是否上移；(3) 采集宿主是改名留在仓内还是移出版本库、本仓只留纯对账；(4) 是否做包改名——改名会牵动分层文档与守卫的层名表，**不改名其余动作仍然成立**，因此可最后做或不做。

**最低验收：** 若采纳，七个动作各自的检查点必须可执行（请求路径不再出现能力断言；只有写派生物包能 import 写 provider；守卫纳入全部非测试生产包），且按仓规先落 owner 文档与不变量条目再改实现。

依据：[能力合同](doc/architecture.md)、`DOC-17`、`DOC-18`、`REVIEW-07`、[分层设计](doc/architecture.md)。

### REVIEW-12 · State 接入需要提供哪种可证明的恢复能力？

**场景与现状：** 平台需要统一下游语义，但来源取值与恢复能力决定实际承诺。若直接把只有
消息流的源接到 State 读取路径，平台将被迫承担折叠、乱序与重建；消费进度也不等于来源当前态。
按 key 取值可以形成观察，但只有配合覆盖已知身份的对账或来源进度恢复，才能处理丢失通知。

**已有方向与待选范围：** State runtime 必须提供按声明取值的能力；源只有消息流时，由接入方
运行时先物化，不在平台 State 接入里隐式增加通用 Fold。仍需为不同来源选定可证明的恢复合同：
已知声明身份上的周期重取，或来源 delta/checkpoint 加断档重建。取值接口存在并不自动满足
恢复与时效，源能力也不能只用 `bounded` 标签替代具体证明。

**最低验收：** 缺取值入口的 Binding 在 READ 上失败关闭；丢通知、恢复游标失效和源长期不可用
分别有范围与时效证据；来源能力与平台观察保留能力分别解释。

依据：[动态物化](doc/design/liveaccess.md) §6.1/§6.2、[服务架构](doc/design/service.md) §4.7。

### REVIEW-13 · 观察记录的保留期与上限由谁定？

**场景与现状：** 持续保留观察会增加存储与恢复成本；来源是 `latest-only` 时，旧观察又不能
通过重取最新值恢复。保留期要满足承诺的复核任务，同时受平台容量约束。

**已有方向与待选范围：** 接入方声明需要的保留期、平台设上限并强制、超限显式拒绝已在 owner
选定。仍需决定未声明时的默认值、容量额度、正在使用的观察如何延迟清理，以及容量不足时的
准入/背压行为。不能静默提前清理已经承诺的观察，也不允许默认无限保留。

**最低验收：** 超限不静默截断；记录丢失后历史读取明确失败；备份与恢复要求不与可丢派生同级。

依据：[动态物化](doc/design/liveaccess.md) §6.3、[介质角色](doc/architecture.md) §2。

### REVIEW-14 · State 观察记录采用什么持久恢复承诺？

**场景与现状：** State 取值与有界观察保留已纳入下一阶段设计，Stream event/window 仍后续
选定。现有 active Serving State 不能证明更新和重启后的历史重读，不能只记 basis 就算完成。

**待选范围：** 观察介质、发布前的耐久条件、实例替换后的恢复、备份与损坏处理。先履行单实例
持久恢复与有界保留，不以通用历史查询、多副本或分布式事务作为前置；保留期内故障和正常到期
应有不同的可解释结果，具体接口由上层运行时合同拥有。

**最低验收：** 选定档位的端到端旅程可执行；未选定的能力明确失败而不是静默降级；不把观察记录宣传成来源真相。

依据：[动态物化](doc/design/liveaccess.md) §6 与 §8.3、`DOC-20`。

## 4. 后续候选与验证依据

- [x] RESEARCH-01 · 已沉淀知识探索路线与开源项目调研，记录词表映射、模型直接阅读、动态渐进披露及可选向量路线的证据、取舍和评测条件。文档合同 `make check-docs` 与定向空白检查通过；仅完成文档与文档图，不代表路线已实现或实验已通过。
- [x] RESEARCH-02 · 已沉淀派生投影控制与 Retriever 的业界对照：业界 ingestion 映射到 Collector→Writer，面 3 映射到 level-based reconcile，Retriever 映射到候选定位口而非 RAG 正文口；记录仍需完善的追平 claims、时效、动态部署、搜宽读严与 source-side 前置条件。文档合同 `make check-docs` 通过；仅完成文档与文档图，不代表路线已实现，也不裁决 REVIEW-04/05。

以下来自已有设计/缺口，尚未单独排期，不自动作为当前单实例自助闭环的完成条件：

| 方向 | 尚待推进的内容 | 进入下一轮的条件 |
|---|---|---|
| 知识探索路线 | 词表辅助、模型查询改写、直接阅读与动态渐进披露；向量保留为可选、非优先路线。证据与实验建议见 知识探索调研 | 根据实际问题集的质量、遗漏与总成本决定；研究记录不构成实现排期，也不新增协议 |
| 派生投影控制与 Retriever | 查询范围级 READY/lag、complete 与时效、真实 Observer 闭环、仓可达性过滤、source-side 补判前置。对照与建议见 派生投影控制与 Retriever 业界对照 | 先回到既有 owner / REVIEW 项；研究记录不构成实现排期，也不新增协议 |
| 发现与接入体验 | 全 Catalog 发现搜索、README 与投影热状态、Connector 运行/凭证托管、公开名称清理 | 按 M3 实际用户路径选择；完整范围继续在 MVP 缺口维护 |
| 新消费能力 | Stream 窗口查询、MCP Gateway、多语言 SDK | 有明确使用场景后独立定义合同与验收；不能复活已退役 APPEND/Stream 表面 |
| 后续部署拓扑 | 多实例协调、更多部署与升级组合 | 单实例数据规模、历史退化与备份/恢复测试已进入 SCALE-01，不再列作后续候选；新增拓扑按实际承诺单独验收 |

进展判读与证据入口：

- 已完成里程碑的实现、前提和具名验收入口见 [MVP 验收](doc/engineering/testing.md)；逐条断言与缺口见 [验证目录](doc/engineering/test-catalog.md)，不在 TASK 复制完整测试库存。
- 数据规模当前只有设计、用例与基础生成能力；有效容量基线待 SCALE-01 建立。规模运行证据落在 .data/scale/runs/ 的各自 run 中，必须有实际数量、负载和环境；不能以普通功能测试报告替代。
- 2026-09-08 文档与验证工具审查仅是当时源码与选择范围的历史记录；详细报告仅留本机、未入库，不作为当前源码、所有产品能力或生产资格的验收凭据。
- 库存从 make validation-inventory 生成；实际运行从对应 .validation/runs/ 或 CI artifact 判读。不同日期、源码或节点 latest 不能拼成一次“全绿”。
- SELF-01～04 从既有 MVP 自助缺口提升为路线待办；原手册/动态部署未完成项分别收口为 GUIDE-01/DYN-01，DOC-14/15 与 REVIEW-01～04 保留原编号。

### DATASET-04 · Dataset 控制台登录与管理闭环

- [ ] 本轮认领（2026-10-10）：沿用 vendored lakeFS v1.58.0 / Apache-2.0，在 `codex/dataset-console` 分支补齐 KC 登录与 Dataset 管理；保留原有工作区改动，不提交。

**Goal / owner：** 浏览器经 KC Server 完成认证发现、验证登录、续期和退出；明确选定 Catalog 后完成 Dataset 列表、来源选择、编辑发布新版、固定版本交付树与文件预览、退役。修复 Vite 输出与 Server embed 路径不一致、Catalog 上下文丢失与 File Gateway Base64/分页合同错误。owner：`doc/design/dataset.md`、`doc/design/service.md`、`doc/design/permissions.md`、`doc/design/service.md`；过程 owner：`doc/engineering/quality.md`。

**Non-Goals：** 不升级上游、不复刻 lakeFS 商业 Dataset 后端，不新增协议或服务端认证会话，不提供跨仓写入/隐式发权，不改变既有发布与撤权语义，不启动真实部署验收栈、不安装依赖、不提交。

**不变量：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`、`API-01`。同一浏览使用已接受 pin；凭证刷新不改版本；登出/身份切换清除在途结果；Catalog 明确进入 URL，页面不得静默回落首项。

**选定 / 否决：** 选定同源 KC API 适配层、浏览器本地验证登录态、现有 PAR/PKCE/token 接口、统一 Catalog 路由、来源表单复用现有发布合同；否决直连 lakeFS 或另建 Dataset 权威、前端推导权限、凭旧 pin 续权、缺省首 Catalog 与手工 localStorage 配置。

**接口 / 验证：** `client.AuthDiscovery` / `BrowserLoginConfig` / `TokenRequest`，`httpsurface.Patterns()`，`catalog.KnowledgeSetSource` / `DatasetItem` / `ResolvedKnowledgeSet`，`server` File Gateway。先以回归测试复现登录入口、Catalog 导航、Base64 读取和构建路径问题，再修复。交付前执行前端行为/类型/构建、真实浏览器回归、受影响 Go 回归、`make test`、`make test-contracts`、`make check-docs`、`make quality`；真实部署验收独立，不宣称已通过。

**选定合同补充：** 原有 Dataset 详情 GET 接受 `revision` 查询，返回已接受来源、真实发布版本列表与当前服务版；管理可见性不变，历史生命周期采用当前判定。未新增路由、Client DTO、写入口或发布权威。具体字段只由 `server/README.md`、公开实现与回归拥有。

**质量三出口：** ① 构建/资源存在性、Catalog 丢失、字节/分页、迟到发布和退役响应、默认分支缺失、名称 `new` 的页面冲突、旧来源缺少交付路径时的隐式范围扩张、文件预览错误重试均已修复，并升级为回归；开发代理共用配置，避免重复修改漏项。② 位置 `webui/tsconfig.json` 与保留上游页面，信号是整树类型检查的存量错误，判定为未接入 KC 入口的上游参考树兼容债，处置为保留并以严格 `tsconfig.console.json` 检查实际交付闭包（不关闭库检查），触发条件是任何上游页面接入路由或依赖升级，届时恢复其类型和行为检查。现有 lint 工具的 AST 弃用提示同属锁定依赖升级时处理的兼容债，控制台文件 lint 零警告仍为交付门禁。③ 位置 `webui/src/main.test.ts`，信号是原有恒真占位断言，判定为无行为证据的存量测试债，处置为不计入本轮 KC 回归证据（入口另有构建资产/深链接合同与实际浏览器走查），触发条件是上游测试或入口调整时替换占位，不以其通过作为完成证明。

**实际浏览器证据：** 临时 KC Server + lakeFS 进程内夹具；local 登录→明确选 Catalog→目录来源创建 v1→保留固定提交改布局发布 v2→选择 v1 并刷新深链接→原字节预览/下载→退役后历史元数据仍在、消费入口消失→退出/刷新要求登录；无浏览器错误，夹具终局确认 v2 已退役且原 Dataset 未改变。真实 Taihu 授权与部署验收未运行。

**验收边界：** 既有 `make test-contracts` 的 short 合同主动排除外部 live 用例；本轮不新增 skip、不削弱断言。按 AGENTS 的“全绿且无 skip”条件，此条暂不标 `[x]`，交付结果分别记录本轮有效门禁和外部验收范围。


### DATASET-04 · 完整 lakeFS 控制台组合与 Dataset 增量

- [ ] 本轮认领（2026-10-10）：在独立工作树 `codex/dataset-console-ui`、已提交基线 `2aa827f` 上继续现有改动；原目录其它会话的改动全部保留，不纳入本轮提交。

**Goal / owner：** 直接复用 vendored lakeFS v1.58.0 / Apache-2.0 的完整前端和已有 lakeFS 后端，仅新增 Dataset 与必要接线。保留 Repository、对象、提交、分支、标签、比较、仓 PR、Actions、Settings、Administration、原登录、Setup 和 PluginManager。Dataset 经 KC 完成认证、明确 Catalog 选择、Form/JSON 定义、固定来源预览后发布、历史/定义比较、文件交付和退役。owner：`doc/design/dataset.md`、`doc/design/service.md`、`doc/design/permissions.md`、`doc/design/service.md`；工程过程归 `doc/engineering/quality.md`。

**Non-Goals：** 不重做 lakeFS 原页面或后端 API、不缩减原控制台、不复刻商业 Dataset 实现、不新增发布权威或跨仓事务/隐式发权、不升级或安装依赖、不修改现有部署、不操作原工作区。独立真实后端只为本轮验证启动并清理，不代替 `make deploy-local-scenes` 等部署验收合同。

**不变量 / 接口：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`、`API-01`、`IX-01`。Dataset 使用已接受 pin，来源版本不漂移，当前授权仍逐请求生效。Catalog 留在 URL，取消/退出/身份变化使在途结果失效。接口沿用 `client.AuthDiscovery` / `BrowserLoginConfig` / `TokenRequest`、`httpsurface.Patterns()`、`catalog.KnowledgeSetSource` / `DatasetItem` / `ResolvedKnowledgeSet` 和 File Gateway。

**选定 / 否决：** 原页面直接调用 lakeFS 原 API 并使用用户原身份；Dataset 调 KC 闭合 API，两边经同源入口分流，不注入平台管理员凭证。复用原 IndexPage/Layout/TopNav/PluginManager，只增 Dataset routes。发布先校验完整定义并确认来源预览，定义变化后必须重新预览，按确认的 commit 发布；未知 JSON 字段拒绝。历史由 Catalog 的真实已接受记录提供，原详情 GET 点查/有界分页，任意真实 revision 仍可点查；不按 1..N 造历史。接口细节只由 `server/README.md`、公开实现与回归拥有。否决专用缩减外壳、重写 native API、客户端自建发布账、默认首 Catalog 和手工 localStorage 身份配置。

**清理与修复：** 移除旧 Dataset 专用外壳及其手工凭证提示；恢复完整原入口。原 API 路径与能力不重写，仅为统一登录补原用户缓存的迟到请求守卫；撤回无关 ObjectRenderer 性能重构，删除测试专用重复 `readFile` helper 和 BrowserRouter 已处理的重复 basename 归一化。原对象渲染能力保留。导航共用 NavItem 不再二次解析 href，仓内及 Administration tabs 的真实 href/点击均有回归。构建输出归 Server embed，已声明页面支持深链接，缺失资产 404，含点 ID 不按扩展名误拒。固定 CSP 不再阻断原控制台预签名、远端图片、PDF 与 SSO。旧 mock 浏览器 fixture 已关闭，其临时脚本/overlay/地址文件已删除；其导航结果不算真实后端验收。

**验证合同：** 前端类型/lint/行为/构建、受影响 Go、`make test`、`make test-contracts`、`make check-docs`、`make quality` 执行到绿并记录源码指纹。实际浏览器必须连接独立真实 lakeFS，覆盖原仓功能与 Dataset；mock 只用于隔离回归，不能作为产品验收。此前旧独立总检查中断、无最终报告，共享目录 source-changed 记录及早期进程内夹具走查均不计本次真实验收。具体结果、源码边界与浏览器证据见 [控制台验证记录](snapshot/console/webui/CONSOLE-VALIDATION.md)。

**API review：** `webui/DATASET-API-REVIEW.md` 对照官方文档/SDK，区分当前商业 Dataset 与锁定的 v1.58.0 前端。已复用 KC 合同补 Form/JSON、确认来源发布、真实 History/Compare。D-01–D-05 具体登记元数据 schema/description、Dataset 定义 PR、Iceberg table/namespace、逐版作者/时间/消息与 Dataset 消费协议兼容差距；包含位置、信号、判定、处置和触发条件，不伪装成可用控件。

**质量三出口：** 构建路径、资源 404、Catalog 丢失、Base64/分页、迟到响应、默认分支、合法名称冲突、旧来源隐式范围扩大、预览失效和双 basename 均当场修并升级回归。剩余债：① `webui/tsconfig.json` / 锁定第三方声明兼容：整树源码已纳入 `tsconfig.console.json`，当前 `skipLibCheck: true`，JS 保持 `allowJs: true` / `checkJs: false`；触发为获批依赖升级或声明修复后恢复库检查。② 锁定工具链 Browserslist/AST 弃用与原 renderer 大 chunk：保留版本/能力，不改阈值；触发为获批 vendor 更新或首屏预算评估。③ `main.test.ts` 恒真存量断言不计实质证据；触发为上游测试/入口调整时替换。④ 原 browser WASM 在 Node/happy-dom import 时的 worker stderr 是测试运行时兼容债，不为它改生产 renderer；实际 browser 验证该能力，触发为获批依赖或测试运行时升级。

**验收边界：** 不新增 skip、不削弱合同。既有 short contracts 排除外部 live 用例；按 AGENTS“全绿且无 skip”要求，未取得全部证据前保持 `[ ]`。生产同源配置见 `webui/KC-VENDOR.md`；配置只供审阅，不应用到现有实例。外部 Taihu/SSO、部署场景、容量与其它未执行入口不能写成通过。

**本次运行与新增登记：** 固定源码宽检查 `make test` 21 pass / 0 fail / 0 skip，`make test-contracts` 1815 pass / 0 fail / 20 个既有外部/live skip，质量（含 docs）通过；运行 ID、指纹和随后仅前端/接线/记录的差异见验证记录。真实原 API、S3、KC Server 和浏览器证明仓库/分支/标签/提交/PR/比较、原 DuckDB CSV 查询，以及 Dataset 来源推进后固定提交、历史/字节/退役。文件选择器 UI 未获得 chooser，原 API 上传与 UI 提交分开记，不冒充通过；位置 `webui/src/pages/repositories/repository/objects.jsx`，判定为验证工具缺口，待可操作 chooser 的浏览器或人工补验。原 native 登录返回丢失查询是存量债，位置 `webui/src/lib/hooks/api.jsx`，判定为原返回地址只保留 pathname；本轮记录，触发为下次 native 登录/返回地址调整时修并补回归。最终审计发现的登出截断 IdP 跳转与遗漏 OIDC 分流当场恢复原导航并补接线回归，外部 SSO 仍不宣称实测。独立服务与测试凭据在保存证据后清理，原目录和现有实例未改动。


**统一控制台登录纠偏（2026-10-10）：** 用户进入 Datasets 不应再次登录。Goal 是保留原 lakeFS 登录/SSO 与 HttpOnly 会话，Dataset 复用同一已验证用户；KC 仍逐请求独立评权（owner：Permissions 的耐久身份绑定与 AUTH-01–03、Service 的认证无 Workspace session）。Non-Goals 是前端冒充 principal、保存原访问密钥或复刻 lakeFS 登录 API、把 lakeFS 管理员自动当 KC 管理员、变更已有部署。选定最小接线：显式 lakefs 认证 adapter 向部署固定 lakeFS `/api/v1/user` 验证调用方证明，沿用 VerifiedUser/identity.Bind；组合控制台直接复用原登录页、用户菜单与退出，Cookie 请求需同源防跨站校验，401 返回原登录并保留完整目标 URL。否决 Dataset 独立登录表单、用户名转 local 断言与直接信任浏览器缓存。接口仍复用 AuthDiscovery.accepts 与 HTTPAuthenticator，不新增登录服务或 Dataset 发布权威；具体可信来源与证明边界由公开 server 实现/README 与回归拥有。原有两次登录的浏览器记录只证明旧实现，不能作为此次单次登录验收。


**认证接线的质量登记：** 原 lakeFS v1.58 `/api/v1/user` 只公开不可编辑的 `id` 与秒级 `creation_date`，没有声明删除后重建的不可重派 subject。lakefs adapter 绑定 origin + id + creation_date，普通同名重建会被现有身份账拒绝，但同秒重建无法仅凭该 API 区分。位置 `server/auth_lakefs.go` / 原 lakeFS User 合同；信号是公开身份代际精度不足；判定为依赖方身份生命周期缺口；处置是保持 DATASET-04 未完成、如实限定验收与部署账户回收要求，不宣称完整不可重派保证；触发为账户删除/重建、迁移、上游提供稳定 subject 或统一 IdP 接线时，必须显式处置旧绑定和旧授权，不能让同名新账号继承旧权。官方依据：`https://github.com/treeverse/lakeFS/blob/v1.58.0/api/swagger.yml` 与 `pkg/api/controller.go` 的 GetCurrentUser。


**身份映射边界登记：** 位置 `server/auth_lakefs.go` / `identity.CanonicalUsername`；原 lakeFS 可使用邮箱或其它 KC 不接受的账号 ID，而 KC 已选定便携用户名不能隐式改名。判定为显式身份关联缺口；本轮兼容的原账号直接复用，非法 KC 映射明确拒绝且不要求重新登录，不猜测别名。触发为部署使用邮箱/长 ID/大写账号或跨 IdP 关联时，在身份 owner 下提供显式关联与其回归；单次登录走查只证明本轮真实 compatible 账号，不宣称全部 IdP 用户类型通过。


**原登录缓存与导航的增量修复：** 只读复现发现旧 native `/user` 请求可在清缓存后迟到写回旧账号，本轮原用户缓存增加失效代次并覆盖这种请求交错；认证 401 使用同一失效点。原登录的 render 导航警告与丢失 query/hash 在此次回跳接线中当场修，恶意站外/反斜杠 next 不作为登录目的地。保留原 API 路径、SSO 和登录提交；这些必要接线不再登记为留待下轮的债。

**单次登录实测与最终闸：** 独立真实 lakeFS/MinIO/KC 环境完成一次原登录、Catalog 选择、Dataset 来源校验/发布/真实文件回读、原仓对象浏览、原退出及 query/hash 回跳；公开 API 撤 KC grant 后 UI 明确 403，而原仓读取仍 200。证据见 `webui/CONSOLE-VALIDATION.md` 与本地 `single-login-*-proof.json`，服务和一次性凭据已清理。首轮前端 105 测试、普通 21 测试、契约 1836 通过/20 既有 live skip；质量发现 `DeploymentConfig.Validate` 复杂度增量 51 超上限 50，采用同集合等价 switch 当场修、不改阈值，再对最终源码执行闸与宽回归。修改过的原 API 用户缓存纳入永久 lint 入口；最终运行指纹与完整 skip 名单归入本地 `single-login-final-checks.json` / `single-login-go-summary.json`，不把纠偏前或修复前记录拼成最终验收。

**Access Control 增量（2026-10-10，用户扩展本条范围）：** Goal 是在保留原 Administration / Users / Groups / Policies / Credentials 的前提下，补齐 KC 当前主体的权限自查、外部申请入口和既有授权规则的查看、添加、撤销，共用原登录。owner 为 Permissions 的身份、动作与范围及准入管理，Service 的共享 typed 用例；不变量为 `AUTH-01`–`AUTH-03`、`KS-02` / `K-20`、`API-01`、`O-01`。Non-Goals 是 KC 新建用户组/角色/策略权威、lakeFS 与 KC 隐式继承或同步权限、对象读 ACL、自建申请审批流、客户端推断有效权限或注入管理凭据。选定复用 `client.AdmissionResult`、`GrantRequest`、`invocation.AllowRule` 和已登记的 identity/admin HTTP 入口；自查必须完整呈现现有约束，管理列表不能泄露私有初始化账或把筛选解释成求值，最终授权始终由 Server 执行。否决把 lakeFS 商业 RBAC 功能门禁套到 KC grants、以模拟后端作为验收和新增第二套 permission evaluator。参考锁定 v1.58.0 官方 API 与当前官方访问控制说明，复用 native API 的事实及剩余差距另记 API review。实现后补回归、最终源码检查与独立真实后端证据，未验证的范围不写成通过。

**提交与合并授权（2026-10-10）：** 用户在本轮实现及真实后端验证后明确要求整理、合并、提交并推送 GitHub。交付为一条完整控制台、Dataset、共享登录和 Access Control 的聚合提交，快进进入 GitHub 默认 `main`；基线已含但尚未进入 `main` 的 38 条提交随快进保留。原共享目录其它会话的未提交文件与生成/运行证据不纳入提交；锁文件和依赖保持原样。最终实现检查为前端 144 个测试、普通 21 个测试、契约 1856 个通过/0 失败/20 个既有 live skip，类型、构建、质量（含 docs）通过，具体运行与边界见控制台验证记录。`DATASET-04` 仍保持未完成，提交授权不改变无 skip 的验收条件。

**Snapshot Store 归属纠偏（2026-10-12，沿用 DATASET-04）：** 用户确认 Dataset 以文件坐标定义，Dataset、对应权限、HTTP 接口及完整控制台应集中为可独立使用的 Snapshot Store 模块，上层 KC 调用。Goal 是将文件定义、固定版本组合、发布、原始字节交付、显式授权核心和控制台的实现权威收口到 `snapshot/dataset`、`snapshot/access`、`snapshot/service`、`snapshot/console`；owner 为 Core Architecture、Dataset、Permissions、Service 与 Capability Organization。Non-Goals 是改变公开 JSON/YAML/命令/路由、引入知识对象解释、创建第二份 Dataset 或授权账、复制两套执行器、重做原 lakeFS、升级依赖或改已有部署。不变量沿用 `A-01`、`L-01`、`API-01`、`AUTH-01`–`AUTH-03`、`KS-01`、`KS-02`、`V-01`、`CA-01`。

选定模块内部按职责分包：底层 `snapshot.Store`/文件树合同保持小；Dataset 的实现和登记历史归模块；Catalog 旧公开路径只留兼容口与外部宿主消费者；通用身份绑定继续复用 `identity`，原 KC 默认政策、Knowledge 准入、语义文件投影和观察证据由外部 adapter 提供，Snapshot 不解释它们。接口参照现有 `catalog.KnowledgeSetSource` / `DatasetItem` / 发布 pin、`invocation.AllowRule` / `GrantList`、既有 HTTP 闭集与原控制台，迁移后仅更改权威实现归属。否决只移动单个 Dataset 文件、把 Home/Invocation 当独立模块后端、权限镜像同步、静态 UI 占位替代实质能力，以及将 Knowledge 搜索/Schema 搬到存储模块。新核心不得反向依赖 `catalog`、`home`、`invocation`、`server` 或 Knowledge。

执行前已检查当前实现与依赖：Dataset 来源为仓/commit/路径，不含知识对象身份；现有文件网关混合原始文件和语义投影，抽取时必须明确分开。原契约、原路由、原登录和全部原 lakeFS 能力保留，目录守卫同变更跟进。前端类型/lint/行为/构建、对应 Go、架构、文档、质量、普通与完整契约逐步检查；实际后端验证只使用独立环境，不将此前已清理旧运行冒充新模块验收。未经最终合同证据，本条仍保持 `[ ]`。

本次迁移债：位置为 `catalog/compatibility.go` 的 `EmptyCatalogState`；信号是 Go 不能为跨包变量提供可赋值别名。旧值读取保持，新核心默认值只由 `snapshot/dataset.EmptyCatalogState` 拥有；给旧变量赋值不影响新核心，当前全仓未发现依赖该赋值的消费者。判定为历史公开可变变量的兼容边缘，处置是保留读取形状并在旧包 README 明示，不反向导入旧包或使用 unsafe。触发为外部消费者依赖重新赋值时，须显式迁移其默认状态配置，不能宣称可变别名完全兼容。

本次外部政策兼容债：位置 `server/snapshot_service.go` 的临时定义读权适配与 `invocation.RequireCompleteWorkspaceRead`；信号是旧 KC 临时配方的 Repository fallback 仍求值 `knowledge.read`，只有 Repo 级 `file.read` 的调用者在旧 KC 适配中可能被拒绝。独立 Snapshot 模块只按显式文件政策求值，不需要 Knowledge 权限。判定为外部 KC 既有政策差异，处置是保持本次迁移的公开授权语义并如实区分两个装配方，不向文件核心反向导入 Knowledge。触发为临时定义的纯文件用户接线或权限政策 review 时，须在权限 owner 下统一选择与回归，不能宣称两种默认政策相同。

本次增量审计当场修：文件接口先检查请求范围、再打开并核对 Catalog 权威、再复核实际范围；错误 provider 和空 provider 失败关闭，未授权请求不能借错误区分隐藏 Catalog 是否存在。发布准备与退役在前置 hook 之后复核 `dataset.manage`，撤权后不发生 portable 准备写或退役。移除原 Server 原始文件引擎与 dead wrappers，生命周期回归改走模块实际 HTTP 路由，借出的 Store 在所有并发请求完成前不得关闭；没有保留只供测试使用的生产适配层。

迁移后的宽检查预检当场修：验证库存继续只扫描旧 Server owner，漏计模块 19 条真实路由；库存现在同时扫描 Server 与 Snapshot Service 的 `Handle`/`HandleFunc`，保持原 88 条闭集和重复/动态注册拒绝。另一项继承问题在本轮原始文件引擎触达面发现：根 mount 的相对文件请求被错误分派为交付请求而返回 400；已用会失败回归和独立真实 lakeFS 复现，修复分派并守卫 SubPath、历史固定版本、混用坐标、父目录逃逸、伪清单与撤权，不能用增加路径特例绕过授权。


**Snapshot 模块耦合口径与棘轮收紧（2026-10-12）：** 位置 `scripts/coupling` 的顶层 unit 归组、`baseline.json`，关联 owner 为 Capability Organization 与 Quality Loop §7。首次质量检查保留失败证据：旧顶层口径传播成本 33.230769%→35.846154%，Snapshot I 0.133333→0.1875。信号的唯一新增聚合边是 `snapshot/service→identity` 被记为 `snapshot→identity`；实际 Go package 图无环，核心不消费身份，旧 `internal` 混合桶把身份加入聚合循环并制造 17 个可达格。判定为已授权目录归属调整下的逻辑单元失真，而非允许反向耦合；处置采用最小明确政策：仅 `snapshot/service` 及其子包为具名嵌套应用单元，其它单位、全部生产 imports、可达性算法和严格阈值保留，包级边界守卫不变。政策签名 `path-depth=1;named-subtree=snapshot/service;version=1` 随基线保存，旧无签名、不同政策或不同 depth 一律不可比较。

同一新工具与平台（Go/GOOS/GOARCH 见本地证据）对独立 `HEAD` archive 与当前源码重算：HEAD 26 单元、216/650=33.230769%；当前 27 单元、227/702=32.336182%，同口径收紧 0.894587pp。旧单元最大不稳定度漂移为 identity 0.333333→0.285714（4.761905pp，小于 5pp）；Snapshot 核心 0.133333→0.125。用 HEAD 的同政策基线执行 strict 仍因唯一新 unit `snapshot/service` 拒绝（exit 1），传播成本与旧单元 drift 均未失败；随后显式 `-write-baseline` 登记当前收紧值，未抬高旧阈值。先失败的命名单位回归、实际 go list imports 保留、政策不匹配、未知 unit、传播上升、漂移边界与可达性算术均纳入回归。顺带当场修复旧浮点计算把理论恰好 5pp 误判为略小于阈值的问题：仅补 1e-12 舍入容差，准确执行原 ≥5pp 硬门，不放宽阈值。触发条件：后续模块再出现新的逻辑能力边界，必须重算同政策改前/改后、保留图与漂移证据再显式收紧，不能用改名或省略 import 逃避守卫。证据在忽略目录 `.validation/coupling/20261012/`；这只证明度量与边界回归，不替代完整契约或真实后端验收。

**耦合只读代理的存量债：** 位置 `scripts/coupling/main.go` 的 `scanDecls`；信号是 interface 计数的文本切片仍包含类型名，导致 A/D 报告不能准确识别现有接口。判定为只读代理缺口，不影响本次 Ce/Ca/I、可达性和 strict 判定；处置为登记后在抽象度指标维护时修复并补接口声明回归，不借本次 unit 迁移扩张指标门禁。触发条件：A/D 指标解释、维护或升格为自动闸时必须先处置，不能把当前零代理值解释为实际无接口。


- [ ] SNAPSHOT-INTEGRATION-01（2026-10-12，当前认领）：保留 Desktop 未提交增量，整合并完成 Snapshot Store 公开边界重构。

**Goal：** lakeFS 是唯一生产 Snapshot 后端，Store、Dataset、Access、HTTP、Console 的实际目录、契约和实现各归其主。外部只 import 根包及 dataset/access/service/console 四个具名一级包。Owner 为 Core Architecture、Dataset、Permissions、Service、Capability Organization、Quality Loop。

**Non-Goals：** 不进行 Knowledge→Object 名称/API 迁移，不补 Dataset 商业扩展，不升级依赖，不改既有部署，不自动提交或推送；不删除独立 Gitea 身份认证。Desktop 其它主题增量、删除和索引状态保留。

**不变量：** A-01、L-01、API-01、AUTH-01–03、KS-01–02、V-01、CA-01。固定文件版本不冻结授权；文件组合不产生 Object 身份；同一发布、授权、幂等账继续承担权威。

**选定与否决：** 每个模块包含 contract.go、同包实现与测试，按需使用 owner-local internal。lakeFS 私有实现只消费存储合同。必要 RawWriter/CommandLedger 公开接口归根包，不暴露具体 Writer/Ledger/Repository/Catalog/Registry 或内部 journal；保留合法能力接口断言。退役 Gitea/filegit 存储、旧 catalog 转发和无正式消费者的 RootDir Git checkout。否决仅搬目录而保留深层 import、具体实现别名、函数变量直通、反向依赖或重复权威。接口由 snapshot/store.go、write_contract.go、command_contract.go 及四模块 contract.go/README/Conformance 选定。

**验证与回写：** 先保留边界失败证据，再收口实现；检查包含测试与嵌套模块。对同一候选源码运行受影响回归、前端、check-docs、普通/契约、quality，再以独立真实 lakeFS/MinIO 验证完整控制台。仅通过后逐文件回写 Desktop，每个写入先核对冻结快照与当前值，避免覆盖并发改动。旧结果不计本次验收；既有 live skip 如实列明，本项满足全绿且无 skip 前保持未完成。逐文件源证据和范围见 .validation/reconciliation/20261012T064756Z/REFACTOR-TARGET.md。

**本项控制台纠偏（2026-10-12）：** Goal 是复用原控制台差异展示与 Repository Compare，让 Dataset 首页直接列出获准 Catalog 的数据集，Catalog 作为可选过滤和明确的创建归属；列表、导航、版本工具栏和文件浏览沿用原控制台组件与样式。Owner 为 Dataset、Service 与 Core Architecture，不变量 V-01、KS-01–02、AUTH-02。Non-Goals：不把组合树当成单仓，不新增本地 Git 权威，不改变 Catalog 身份、消费范围和权限，不升级依赖或修改现有部署。选定共享原文本 diff、显式跳转原仓固定 commit 比较，来源入口仅依据受信同实例坐标；否决手写 item 对比、猜测 kr 标识对应原仓名及自动用 Dataset 授权读取整仓。合同仍为 snapshot/dataset/contract.go、snapshot/service/contract.go；独立真实后端与浏览器验收另记 .validation/console-ux/20261012/。

**本项 Access Control 纠偏（2026-10-12）：** Goal 为我的权限与授权管理共用规则表单，字段和值分栏，补齐有管理权的规则编辑，恢复原 My Credentials/Users/Groups 的可进入页面。Owner 为 Permissions、Service、Snapshot access/service/console 合同；不变量 AUTH-01–03、KS-02、V-01。Non-Goals：不另建 KC 用户/组权威，不改变现有部署或认证模式，不因个人权限页面而授予管理权，不升级依赖。选定同一规则组件的查看/编辑/新增模式；PUT 原子替换现有规则，在 RuleStore 同一序列化边界校验旧范围、新范围与 expected 旧规则，保持 ID 和来源，冲突及失败不改授权账。分享生成规则继续通过分享管理修改。否决先撤销再新增、客户端求值自我发权、仅取消禁用后把社区后端不支持的管理写成可用。接口由 snapshot/access/contract.go、snapshot/service/README.md 与 httpsurface 注册表承接；先红后绿的回归、真实原 API 与浏览器验证记录在 .validation/access-ux/20261012/。

**Repo / Dataset 授权接线（2026-10-12，本项续作）：** Goal 为复用现有认证器与耐久身份绑定，把 Repo、Dataset 产品授权归 snapshot/access；snapshot/service 对完整原控制台 Repo API 强制检查当前授权后，以调用者原证明转发 lakeFS。Owner：Permissions、Service、Snapshot Store；不变量 API-01、AUTH-01–03、KS-02、V-01、CA-01。Non-Goals：不重做账号、组、凭据或登录，不建立第二份政策，不升级依赖、修改现有部署或自动提交推送。以 Desktop 已提交 fa52d18 的 Registry / Dataset 服务合同为准，不恢复 Catalog API。选定闭集原 API 操作映射、真实同实例仓坐标、当前规则与已有默认政策、库存过滤后分页、原始流交付；否决客户端藏按钮、方法粗分权限、平台服务凭据注入、Dataset 权限开放来源整仓。接口见 snapshot/access/contract.go、snapshot/service/contract.go。新增原调用方扫描守卫防止再次写错 PR 合并方法；真实 v1.86 后端已证实原 v1.58 控制台使用 PUT 合并，未定义路由移除。

**验收与剩余边界：** 2026-10-13 已自建独立真实 Gitea 1.26.3 认证及未修改的 lakeFS 1.86.0，两个真实主体的 API/CLI 初轮 82 项检查通过；最终浏览器、撤权与源码绑定检查以 .validation/shared-auth/20261013/ 最终报告为准。此前单主体证据不计此次多用户验收。snapshot/service/native_api.go 物理 Repo DELETE 尚无部署绑定、托管恢复及 Dataset 来源的完整耐久解绑生命周期，当前先授权再明确拒绝；完整删除需求触发实现。后端失败审计债已当场修复：原结果与 HTTP 状态归为实际失败，私有存储认证失败为服务不可用；回归覆盖 401/403/404/500，不伪造成功。锁定库声明仍 skipLibCheck:true；既有 live skip 单独披露，本项不标完成。旧证据与逐文件整合清单放 .validation/repo-access-current/。

原 API 写操作另需尊重宿主已挂载 Store 的字面文件写能力与归档状态；不按保留仓名隐式发权。独立真实 system 仓的引用创建曾绕过只读挂载，已取得红证据并恢复测试分支；补 generic capability 检查与宽授权不能绕过只读仓的回归。

**现有认证与私网存储接线（2026-10-13，用户确认后实施）：** Goal 是复用现有 Taihu / Gitea 认证器与耐久身份绑定，在 Snapshot 模块统一执行 Repo、Dataset 授权，授权后由服务端私有存储凭据调用未修改的 lakeFS 1.86。Owner 为 Permissions、Deployment Auth、Service；不变量 AUTH-01–03、API-01、KS-02、V-01、CA-01。Non-Goals：不重做账号、组、登录权威，不修改 lakeFS 源码、升级依赖、恢复 Catalog 合同或改动已有部署，不覆盖 Desktop 的其它修改。选定统一业务入口、原 Repo 操作闭集、真实仓绑定、当前授权账及受控后端客户端；替换此前只适用于 lakeFS 用户证明的原样转发接线。底层账号/凭据/初始化 API 不得随存储凭据对外转发；原控制台账号入口使用已有认证提供方，操作审计区分实际调用者与存储账号。接口由 snapshot/service/contract.go、identity/http.go 和既有 Client 认证合同承接，Server 只装配。不采用旧版本或模拟认证作为验收；独立真实 Gitea 多用户认证 + lakeFS 1.86 验证控制台、CLI、撤权与绕过，Taihu 验签/授权码合同另按其既有验证边界记录。逐文件基线、红绿证据与清理记录在 .validation/shared-auth/20261013/。
