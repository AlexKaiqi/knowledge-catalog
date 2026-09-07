# TASK

执行接力棒，不是设计权威。一次交互只认领一条 `[ ]`。`[x]` 仅当对应契约全绿且无 skip/删断言。

权威映射见 `docs/README.md`（documentation-governance）。设计类文档合同标题由 `make check-docs` 强制。

## 接入方自助创建托管 Repository

- [x] 打通已授权接入方通过正式 Client 申请托管仓、发布知识、固定版本回读与替换实例后继续维护；同步文档和协议旅程。

Goal：按 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`、`SERVICE_ARCHITECTURE.md`、`PERMISSIONS.md` 与 `STORE_ADAPTERS.md`，部署方一次配置存储供给与明确授权策略后，接入方能自行创建托管 Repository 并立即使用 Writer/Reader，无需逐仓改配置或补权。
Non-Goals：遵守上述 owner；不改变 Snapshot/Knowledge 身份和 Writer 单仓边界，不让 attach 创建或迁移源，不增加客户端任意路径/DSN/凭证连接，不将供给过程伪装为跨介质原子事务，不把 Catalog Git 成员关系搬回本机配置。
不变量：`A-01`、`API-01`、`V-01`、`W-01`、`W-02`、`WS-02`、`AUTH-01`、`AUTH-02`、`P-01`；供给与恢复证据归入现有 owner。
选定：独立 create 应用入口；部署声明托管供给与创建者初始动作策略；建仓前耐久预留随机分配身份；按持久阶段恢复供给、Catalog 登记、可撤销初始授权；成功后才能报告完成；重启只读恢复，失败保留显式重试证据。否决：复用会收养已有源的 Open、修改逐仓部署配置、创建者永久越权、重放补回已撤销权限、Catalog 登记失败清除供给记录。
接口依据：`home/` 的部署/供给公开类型与 adapter 能力、`cli/surface.go`、正式 typed HTTP 与 `client/`；场景遵循 `.data/scenes/README.md`。
变更前证据：只读审计确认公开 Client 无托管建仓入口，OpenDeployment 只恢复静态 repositories；底层 Open 会初始化或收养已有源，Writer ledger 的失败清理不能保存多阶段供给。先补并运行失败证据，再实现；完整契约全绿前不勾选。

实现证据：`catalog repo create` 经独立 typed Client/HTTP 和 `catalog.repositories.create` 准入；只接受 Catalog、仓身份及 command-id。部署一次声明 Dolt/Gitea 供给及 creatorActions，服务把分配、绑定、阶段及原结果写入 `managed.db`，不修改逐仓配置。Catalog 成员仍以 Git 接受为准，初始规则和一次策略回执同文件耐久保存，撤权后重放不补权。托管仓可按原只读 attach 合同加入其它 Catalog。

用户旅程证据：新增并列 `managed-repository-created` Go Oracle；同一普通主体仅持创建准入，从不存在的目标仓完成 create、PUT、pack/commit、固定版本 READ/PROVENANCE、清除缓存并替换实例、双重幂等重放、CAS 更新及撤权。标准 Dolt e2e 定点 13.918s 全绿，真实 Gitea 供给/发布/恢复路径全绿。没有通过修改静态绑定或临时补权替用户补步骤。

边界证据：保留身份和竞争请求无供给/发权副作用；缺状态卷、managed 账及初始化回执拒绝重建。独立审查先复现丢失账本初始化回执会遗漏历史绑定、Gitea 探测错误误触发初始化、已打开句柄跟随被替换源三个失败，修复后对应回归及 race 全绿。动态 Registry 与原生 Dolt 查询亦有并发/归属验证。首次 Dolt 目录创建与归属标记持久化之间中断仍保守要求恢复或核对，不收养无归属来源。

最终验收：真实 Gitea 分组 exit 0（Snapshot 23.886s、CLI 7.249s），文档图、独立 HTML/锚点、场景及公开面守卫、go vet、gofmt、定点 race 均通过。首次完整运行在覆盖门禁发现 create 正式 Client 边界只有 1/2；新增重复申请及同命令改目标的真实失败断言，没有调低门禁。最终完整 `make test` exit 0（CLI 1419.753s、Catalog 25.306s），日志 `/tmp/kc-managed-final-make-test-v2.log`；覆盖报告 `/tmp/kc-managed-final-coverage.json` 确认 60/60 个业务命令成功路径及失败边界达标，create 有 3 条独立边界（要求 2 条）。未删除或削弱断言、未用 skip 消除失败，未提交；独立产品手册条目保持其原状态。

## 可恢复部署与 Snapshot 接入重构

- [x] 将 Catalog 改为独立 Git 权威、分离声明配置/耐久数据/实例缓存，彻底移除 `kc local`，以只读预检及一次 Catalog 提交完成已有 Snapshot 接入；同步公开合同、文档、场景和重部署验收。

Goal：按 `COMPOSITION.md`、`SERVICE_ARCHITECTURE.md`、`STORE_ADAPTERS.md` 与用户确认的恢复目标，使实例工作盘可丢弃，既有 Catalog、治理策略、过程账和 Snapshot 可恢复；用户一次 attach 即完成 Catalog 准入。
Non-Goals：遵守上述 owner 的层级边界；不改变知识身份/Writer 单仓写边界，不创建跨 Repository 事务，不把秘密、正文或不可重建原始证据当 Catalog 缓存，不在 attach 中创建或迁移源。
不变量：`A-01`、`L-01`、`V-01`、`W-01`、`W-02`、`WS-01`、`WS-02`、`AUTH-01`、`AUTH-02`、`API-01`、`P-01`、`O-01`；新增恢复/只读接入证据归入现有 owner 与架构证据体系。
选定：声明式部署配置、远端 Git ref 确认为 Catalog 保存边界、独立耐久状态目录、可丢实例缓存；只读 OpenExisting 后一次 Catalog Git 准入；公开 `catalog repo attach`，初始化/恢复分开，`local` 无兼容别名。否决：本机 commit 即保存、扫描本机目录当成员权威、attach 自动建仓、两份持久 attached/register 状态、缺失策略静默视为空。
接口依据：`catalog/` Registry/CatalogState、`snapshot.Store`/adapter Conformance、`home/` 部署配置、`cli/surface.go` 与正式 typed HTTP；场景依据 `.data/scenes/README.md`，主题关系由 `docs/graph/` 管理。
变更前证据：审计确认 Catalog Save 只写实例 Git；现有 Gitea/Dolt attach 会初始化源；grants/gates/command ledger/control state 仅落 Home。各实现子项先添加并运行失败证据，再迁移到新合同；完整检查前保持未完成。

实现证据：公开 `local` 与独立 register 已删除；配置入口为 `deployment init|status|system publish --config` 与 `serve --config`。Catalog 写入以独立 Git remote/ref 的 CAS 接受为准，含重复生命周期操作的权威检查；Snapshot 接入只读，任务 pin 保存临时定义并直接供知识命令重放。配置、来源、耐久策略/账本/证据和可丢缓存的责任见 `home/README.md`。

恢复与并发证据：正式 Run/HTTP 回归验证实例替换、两间 Catalog 隔离、Writer 回执重放、提案继续合并、撤权与 Gateway 固定 pin；缺失状态卷/既有 Catalog 分支拒绝重建。`deployment_read_concurrency_test.go` 与 Catalog ReadView 的 race 回归先捕获共享请求身份竞态，再验证并行请求的独立审计身份；Git 双句柄测试验证失败提交和重复操作不能凭过期内存报成功。

用例证据：移除单独 `repository-registered` 层后，迁移文件无遗漏；当前 42 个状态、33 个 construct、44 个 probe。bundle 声明既有入口，保持导览与实际执行分母的区别；同主体正式消费旅程及正式恢复旅程分别验收，未把所有 bundle 宣称为独立 HTTP 旅程。

专项验收：真实 Gitea / Dolt / OpenSearch、独立 State runtime、服务角色与 Linux/FUSE 分组最终全部通过；FUSE 修复后完整重跑无 SKIP。插件 typecheck/test/build/pack 检查通过（22 tests）；数仓静态测试 25 项及 DW-CLI-02 的 24 步通过，未运行完整 live MySQL 数仓旅程。文档图、公开 surface、gofmt、go vet 与定点 race 通过。

最终验收：全部实现和断言修订后，完整 `make test` exit 0；component / boundary / CLI-HTTP / Catalog 全绿（CLI 1398.740s、Catalog 23.902s），日志 `/tmp/kc-deployment-final-make-test.log`。未删除或削弱断言、未用 skip 消除失败，未提交。独立产品手册任务仍由其原条目记录。

## 可独立分享的产品使用手册

- [ ] 将 `docs/product.html` 整理为面向接入方与消费方的单文件离线手册；核对公开用法，验证离线链接、移动布局、打印与文档契约。

Goal：按 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` 的接入、消费与项目旅程，让读者在一个 HTML 内理解价值、概念和上手步骤。交付客户端封装唯一服务入口，接入方与消费方应能自行完成日常操作，不逐次依赖部署方。
本轮续修：重新核验首次登录、平台仓申请/自有仓连接、权限开通、首次发布、修改再发布及消费方采用更新。修正此前把部署方逐仓配置当成正常用户旅程的描述；以 `MVP_ACCEPTANCE.md` 如实记录自助入口尚未实现的部分。
Non-Goals：遵守该 owner 与 `docs/README.md` 的派生展示边界；本次不实现新的认证、供给、连接或授权接口，不改变协议或设计所有权，不扩展为托管网站。
不变量：`I-01`、`V-01`、`W-01`、`W-02`、`WS-01`、`WS-02`、`AUTH-01`、`AUTH-02`、`D-01`、`S-01`。
选定：内嵌样式、图示与渐进增强交互，角色分流和少量公开 CLI 示例；否决：依赖相邻 Markdown 的阅读链、外部 CDN、把设计目标当成已交付保证。
接口依据：`cli/help.go`、`cli/SURFACE.md`、Reader/Writer 公开合同与 Conformance；主 owner 为 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`，术语与直接依赖按 `docs/graph/` 核对。
变更前证据：原 HTML 存在 25 个指向文件外的阅读链接，单文件分发检查失败；`make check-docs` 基线通过。
前次证据：独立 HTML 无外部资源依赖，含窄屏/打印样式与键盘可聚焦代码区；文档服务测试、JS 语法与 `make check-docs` 通过。实际 Server/Client 曾验证发布、重放、固定版本回读、来源、Schema 浏览和命名知识集 pin。临时 pin 已由后续实现补齐定义与 Catalog，现按该公开合同更新消费路径。
本轮变更前证据：原手册多处要求部署方逐仓配置，缺少带内容前置条件的修改发布实例；`make check-docs` 基线通过。只读核验确认仓源绑定、首次授权和普通用户登录仍有产品缺口，已有权限下的 Writer 维护与临时 pin 消费可由用户独立操作。
本轮交付：手册改以已封装连接的客户端为入口，临时选源为消费主路径；补首次创建、带 CAS 的修改发布、旧/新 pin、同请求重试、提案评审、移除和历史恢复。产品、服务与组合 owner 同步自助目标，MVP 保留真实缺口。文档服务测试、离线锚点与命令/JS 语法、文档图契约通过；只读公开合同复核及纯内存 Writer/Reader、CLI mock/授权定向测试通过。
未完成验收：浏览器工具拒绝本地文件 URL，移动/打印仅完成静态审查，未做视觉验收；本轮不重跑全量集成。临时 pin 端到端测试在底层 Dolt 夹具首次 Schema PUT 超时，未形成完整旅程全绿证据。本条保留未勾选。

## CLI / HTTP 公开面语义审查

- [x] 按 [`cli/REFACTOR.md`](cli/REFACTOR.md) 与 [`cli/HTTP_REFACTOR.md`](cli/HTTP_REFACTOR.md) 落地公开 CLI argv、help 分组与 typed HTTP 路径（65 条）；旧 path 不进分母。审查底表仍是 [`cli/SURFACE.md`](cli/SURFACE.md)。不把命令或路由穷尽清单写进 `docs/*.md`。

## 用 ai-native-project-maintenance 梳理文档

- [x] 给 `class` 为 foundation / decision / runtime / evolution 的设计 Markdown 补齐 Goal / Non-Goals / 硬性约束 / 选定与否决 / 接口契约 五段（从现有正文抽出，不另造宪法），并用 `make check-docs` 强制这些标题。
- [x] 设计文档写应然：不得用当前实现收窄 Non-Goals；实现偏差点名到 owner 或 `MVP_ACCEPTANCE.md`。见 `docs/README.md` 应然/实然分层。
- [x] 对照设计应然，审计剩余文档问题与实现偏差；把仍缺的缺口写入 `MVP_ACCEPTANCE.md`，不在本回合改协议代码。
- [x] 按篇删除设计 Markdown 里可复制的协议 Schema、错误码表和命令穷尽清单，改为指向包 README、公开 Go API 与 Conformance；先从 `ASPECT_ACCESS.md`、`OBSERVABILITY.md`、`SERVICE_ARCHITECTURE.md` 开始。
- [x] 在不搬迁目录树的前提下，把 `KNOWLEDGE_CATALOG_DESIGN.md` §9.2 / §9.4 的 ADR 与明确拒绝收成可引用的决策块，专题文档只 `refines`、不再复述另一套否决表。
- [x] 把仍按「问题 / 第一性原理 / 决策」展开的专题正文接到文首五段之下：文首是合同，后文只保留调研证据和推导，删除与文首重复的原则编号。

## 消费面 BROWSE 与源说明

- [x] 冻 BROWSE 为 Catalog/知识集/源说明 + Schema 分页（否决对象 LIST 与 git README）；发布源说明 System Schema 与每仓保留实例身份；Catalog 库存拼装记入缺口。

- [x] `kc catalog show` 与 `catalog repo list` 的 `repositories` 从纯 id 改为带源说明的对象（应用层 READ 保留对象）；`catalog list` 与知识集成员名单仍是 id。

## 权限：发现与读分层

- [x] 把 Catalog 发现与仓级 READ 分层写入 `PERMISSIONS.md` 及直接相关篇：固定元信息供过滤、一份索引、命中后最外层屏蔽；实现缺口记入 `MVP_ACCEPTANCE.md`。不改协议代码。

- [x] 把 hydrate 之后、返回之前的交付组装写成可挂接的链（非 Hook、非新协议层）；首段是仓读权屏蔽，后续可挂隐私化等规则。

- [x] 整理 `PERMISSIONS.md` §7.2：发现 / 固定元信息 / 交付链分小节；卫星文档只引用不复述。

- [x] 收敛 `PERMISSIONS.md` 交付合同：去掉悬空槽位、分清已固化/未固化约束、C-01 与交付链分责、补图边；卫星只引用首段读权屏蔽。不改协议代码、不新增 Oracle。

- [x] 写清权限动作×阶段：`catalog.read` 与 `workspace.consume` / `knowledge.search` 分责、无读权不是 `partial`、屏蔽信封不另造 DTO。不改协议代码、不新增 Oracle。

- [x] SEARCH 搜宽读严：候选不按 `knowledge.read` 裁仓；hydrate 后交付链屏蔽正文；`workspace.consume` 不放行 `knowledge.*`。

- [x] 把 hydrate 后的交付链做成独立 `delivery/` 包：输入知识 ID，输出可见内容；首段仓读权；补单元验证并接到 SEARCH。

- [x] 用已声明的 metric AccessHints（text / filter / 无 access）验证：定位只走声明面，无 `knowledge.read` 时 SEARCH 命中屏蔽正文、READ fail closed。

- [x] 三种登录身份（taihu / agent / service）× grant 矩阵：local 跳过 IdP，启动 HTTP 服务验证 SEARCH/READ 结果。

- [x] 把 metric 权限用例改成可读场景过程：声明 → 三种登录 → 配权 → SEARCH/READ，用人能顺着读的步骤验收。

- [x] metric 权限场景改成可解析执行的 feature：人读同一份 Given/When/Then，Go 跑 Oracle；任务块给人看也可给 Agent，不把 Agent 当协议绿。

- [x] 把 metric feature 的 `"""` 拆成可对 Agent 说的阶段任务，并登记 KC-AGENT-01 companion；协议 Oracle 仍是 Go Then。

- [x] 整理权限场景哪些适合独立验证，并设计 scenes 平铺节点目录（视图为树，维护用 depends_on）。

- [x] 按「同一变化来源」重画 scenes 节点目录（不限于权限；独立≠无依赖）。

- [x] 按分层 ⓪–③ 与接入方/消费方/项目使用者把 scenes 收成一棵树（GMV 挂在脊上，不另起根）。

- [x] 状态用目录名；目录内放构建该状态的过程；去掉 GMV 式命名，拆细 semantic knowledge。

- [x] 把句柄 Binding、墙外 Connector/采集、观察通知与物理权威补进 scenes 状态树。

- [x] 按公开产品入口（help 三主题 + kcfs/VFS）把缺的能力状态补进 scenes 树。

- [x] 按 PERMISSIONS 接口表把授权动作世界补进 scenes 树。

- [x] 按可提供的功能点梳理 scenes，除真实认证外都落成可跑用例。

- [x] 把声明式索引与动态索引收成独立功能点并挂上可跑用例。

- [x] 按可提供的功能点完整梳理 scenes（宿主到冻结），除真实认证外都有用例。

- [x] 把各环节所需材料挂进 scenes 树，construct 从夹具加载。

- [x] 材料经节点步骤写入权威，不挂 Catalog；维护夹具与自动化过程。

- [x] 按状态树嵌套目录把协议用例放到 `.data/scenes/`。

- [x] 场景树材料自包含，不依赖 data-warehouse。

- [x] 节点目录只表示分叉；构建逻辑与材料放进特殊标识目录。

- [x] 场景执行器自己读目录树，把可 construct 的路径跑完。

- [x] 每个场景节点挂 gitignore 的 `_results/`，保存该节点验证结果。

- [x] 场景 feature 先观测再断言后态；禁止只写 command succeeds。

- [x] 树脊先观测 System Schema 夹具，再打开知识仓；接入方按系统 → 空仓 → Domain Schema 构建。

- [x] 写 `.data/scenes/README.md`：组织、维护、执行、用例规范与场景不变量；`AGENTS.md` 指向它。

- [x] 树脊在 attach 之后走公开写入：pack 预览 → commit Domain Schema → put 实例；禁止 Given material 代替 `kc writer`。

## 场景树覆盖公开 CLI 子路径

Owner：`.data/scenes/README.md`；覆盖格子仍 `docs/TEST_CATALOG.md`。公开 argv 权威 `cli/surface.go`。

- [x] 公开 CLI 用户可操作子路径全部出现在场景 `When I run`（capability 挂载不等于跑过）；拆开 attach/register 等合并步骤；help consume/write/compose 最短路径可顺着读。墙外 runtime / FUSE / live 认证的成功态仍 go-test，合同里点名。

## 评审后的可学习性与装配缝

Owner：`cli/SURFACE.md`、`.data/scenes/README.md`、`LAYERS.md` / `SERVICE_ARCHITECTURE.md`、`ARCHITECTURE_INVARIANTS.md`。不改协议动词、不新开第④层、不把缺口改成 Non-Goal。

- [x] CLI 操作数闭集、`knowledge access`/`invoke` 拆义、`workspace pin --out`、help 最小 grant 与仅 HTTP 闭集。scenes 最短 write 仍绿。
- [x] 场景执行器复用父 home；consume / pin / SEARCH / READ 可整段进 feature。
- [x] 装配：client / HTTP registry / Home 包边界；内部动词键对齐公开名。`internal/arch` 仍绿。
- [x] 不变量交叉表；未落地表面移出「当前入口」叙述。`make check-docs`。

## 瞬时观察与动态索引（index.dynamic）

Owner：`LIVE_MATERIALIZATION.md`（Binding / Observation）、`PROJECTION_CONTROLLER.md`（投影控制）。
State 精确 READ 与独立动态投影已有（B-08..B-18、I-15..I-20、I-22）。不新开第三篇「瞬时知识」文档。
Stream window / 多实例仍见 `MVP_ACCEPTANCE.md` 已有缺口，State 控制收口之前不拆成下一批实现。

- [x] 把 State 控制实然缺口写入 `MVP_ACCEPTANCE.md`，并纠正 `TEST_CATALOG.md` I-21：不得把 `index-sync` + lookup 标成 change notice 已兑现。
- [x] 把 SEARCH 代数从 `LIVE_MATERIALIZATION.md` 挪到检索专题（或明确检索 owner）；瞬时篇只拥有 Binding / Observation / Serving State。改 `docs/graph/` 边。`make check-docs`。不改协议代码。
- [x] 冻结 change notice 入站合同：只带 Binding / Address / 仓 / ref 定位与可选 sourceRevision hint，不带正文；与 Writer ChangeSet 分面。选定形状进公开类型与包 README；政策仍 `PROJECTION_CONTROLLER.md` §3.2。
- [x] 投影控制器第二条输入：长寿命 `Controller.Start` 接收 notice、按固定 Binding pull、发布动态投影；不得与 Snapshot HEAD 合成一个 key；消费 SEARCH/READ 仍不得 `RefreshState`。补 I-21 应然 Oracle。
- [x] notice 只刷新受影响 Address；全量枚举仅冷启动或 reconcile。`index-sync` / `projection sync` 只保留 Snapshot EnsureAt、历史 pin、强制重建和排障，不再作为动态 live 的唯一入口。
- [x] `observation-refreshed` 补 construct / probe feature（或让场景执行器能跑动态投影），`index.dynamic` 不再只有 go-test README。
- [ ] `PROJECTION_CONTROLLER.md` §11.3 Docker 首版：真实 observer、独立 runtime、Gitea、KC 重启后动态投影仍可搜；不得把当前双容器 adapter 旅程记成整组 D 通过。
