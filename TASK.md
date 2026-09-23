# 项目路线与当前任务

目标：把 Knowledge Catalog 通用知识底座交付成接入方和消费方可以独立使用的产品。部署方一次准备服务、存储供给和授权策略；日常接入、发布、修改、分享、检索和采用更新由使用者自行完成。

本文件记录路线、里程碑进展、下一步和待裁决事项。设计权威与关系见 [文档地图](docs/README.md)；完整能力边界见 [MVP 验收](docs/reviewed/mvp-acceptance.md)，验证方法和库存见 [验证体系](docs/reviewed/test-catalog.md)。已完成的逐次修改、重复勾选和临时调试记录已清理，长期结论留在对应 owner、代码和验证产物中。

一次只认领一项待办；完成需有对应范围的验收证据，不以跳过或删除断言换绿。下列路线是当前推进建议，未约定完成日期；review 条目分别注明已有设计方向与仍待选定的部分。

## 1. 项目路线与进展

**现在的位置：基础协议与可恢复的服务骨架已经建立；本轮普通用户自助修复已通过功能验收。** 统一用户名、持久登录、同名托管供给、管理地址、自有 Gitea 连接、首次准入与受限分享已实现，完整验收由 WALK-01 记录；动态 State 部署与规模资格仍是独立待办。

| 阶段 | 已具备的能力 | 当前状态与下一道门槛 |
|---|---|---|
| M1 基础协议与固定版本消费 | 稳定身份、Writer 单仓发布、CAS/幂等、Schema、来源、Workspace/pin、临时选源、检索后同版本回读、权限交付链、typed Client | **已交付参考实现**；继续由共享合同和正式 CLI/HTTP 旅程回归保护 |
| M2 可恢复单实例与仓分配机制 | 独立 Catalog Snapshot 权威、耐久状态与可丢缓存、已有源只读 attach、服务凭证创建托管仓、创建者策略授权、实例替换后续用 | **已补齐同名用户供给**；Gitea/LakeFS 托管供给与管理地址进入耐久恢复路径，正式验收见 WALK-01 |
| M3 普通用户全程自助 | 获权后可发布、修改、评审、移除、复核和采用更新；单文件手册已有完整内容 | **本轮修复已验收**；SELF-01～04 的本轮实现与验证由 WALK-01 汇总，真实外部身份服务与更广部署资格单独记录；手册剩 GUIDE-01 |
| M4 动态 State 完整部署 | State 精确 READ、独立动态投影、声明/观察双 basis、notice-and-pull、同版本 hydrate 已有 | **局部能力已有，可信消费闭环待补齐**；DOC-20 收口设计与用例，DYN-01 承接真实 Observer/runtime/lakeFS/KC/OpenSearch 与重启恢复；默认时效待 REVIEW-04 |
| M5 数据规模与生产资格 | 规模档位、压测场景树用例和基础输入生成器已有；原生点读、增量机制与部分遥测已有 | **尚无有效容量结论**；SCALE-01 补执行器、小规模基线、逐档扩量、历史与恢复证据，DOC-14/15 补能力隔离和观测；代际方案待 REVIEW-01 |

验证贯穿每个阶段，不集中到最后。M1/M2 的“已交付”保留既有实现与历史验收结论；并不表示本次整理重新跑过完整产品套件。

当前先完善 DOC-20 的 State 消费设计与方向性用例，再按缺口落公开合同和实现：范围级覆盖/时效、
后台恢复、观察保留、旧 Dataset 声明维护与源授权。DYN-01 承接 lakeFS 上的部署证据；单 key
增量成本纳入 SCALE-01，不能以 lookup 次数代替整个维护路径成本。Stream 窗口与持续订阅后续
分别推进；自助产品已有结果由 WALK-01 保留，手册与其它规模任务仍按原条目跟踪。

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

下列条目各自独立可认领。它们分别落在重构目标形态的哪一面、彼此的依赖顺序，以及「目标」与「现状」的逐条差距，见
[重构目标形态与执行序](docs/REFACTOR_TOPOLOGY.md)。该文只导航并标注三态（已固化 / 已选定未闭环 / 待裁决），不裁决任何 `REVIEW-*`，也不新增 owner。

### QUALITY-01 · 工程质量闭环：债登记（DTO 信封重复、staticcheck 存量与观察项）

- [x] 已收口（2026-09-23）：U1000 八处已按上一段处置并删除；剩余 13 处 cli 存量项与 DTO 信封重复均已处置——staticcheck/gocyclo/dupl 存量由并行收口轮清零，DTO 七字段信封按登记触发条件（下次触碰服务路由时）收敛为共享信封 embed 类型（`cli/service_routes.go` `knowledgeScope`/`knowledgeDatasetIdentity`，`client/knowledge.go` `KnowledgeScope`/`KnowledgeDatasetIdentity`/`KnowledgeRepoPin`；JSON wire 契约不变，序列化守卫测试通过），`make quality` 全绿。原条目：本轮认领 U1000 处置；债务登记，条目形状（位置 / 信号 / 判定 / 处置 / 触发条件）见 [`docs/reviewed/quality-loop.md`](docs/reviewed/quality-loop.md) §3。位置：`cli/service_routes.go`（约 10 个 request struct，:131–:269）与 `client/knowledge.go:19,107`。信号：`Catalog/Dataset/Pin/Definition/Repository/Commit/Ref` 七字段信封组成组重复（2026-09-23 试跑基线：全仓 7 行重复块 605 组，本条为最大非机械组）。判定：真问题（低危；行为由 httpsurface 闭集路由守卫与 client parity 守卫兜底）。处置：收敛为共享信封 embed 类型。触发条件：下次新增或修改服务路由触碰同文件时一并收敛，不单独排期。另登记 staticcheck 存量（2026-09-23 首次跑 `make quality` 报出 8 处 U1000；同日复核完整闸层输出为 21 处，含 `cli/allow.go` `knowledgeActionVerbAllowed`、`cli/dataset_consume.go` `openServing`/`searchVisiblePin`、`cli/stores_observe.go` SA4006×2 及 `cli/remote_*`、`cli/scene_feature_test.go` 等并行工作线文件；统一访问 TRAVERSE 增量文件经定向 staticcheck/gocyclo/dupl 复核为零新增）：`catalog.datasetRestrictsRepository`、`home.absStoreDir`、`home.looksLikeLocalPath`、`index.(*Index).searchEngine`、`index.(*Index).searchEngineAt`、`knowledge.(*SystemRepository).operation`、`retrieval/opensearch.(*openSearchEngine).closePIT`、`scripts/docs-serve.piece`；判定：疑似死代码但需逐条复核（接口实现、反射或并行工作线待接线均可能为误报），处置=按 owner 逐条复核后删除或接线，不机械删除；触发条件=并行工作线收口、`make quality` 重跑后处置。同轮观察项（判定设计使然，不处置只随热点复查跟踪）：`home/deployment_runtime.go` `InitializeDeployment`（176 行/51 分支，fail-closed 恢复合同要求）、`index/state.go` `refreshState`（161 行，内聚单算法）。2026-09-23 增补：耦合度量基线已建立（`scripts/coupling/baseline.json`，23 单元、传播成本 38.74%，`make quality` 附带只读耦合报告）；方法与读法约定见 [`docs/reviewed/quality-loop.md`](docs/reviewed/quality-loop.md) §7。U1000 处置结果（2026-09-23，本轮）：上述 8 处经逐条复核全部判定真问题并删除——`catalog.datasetRestrictsRepository` 为 knowledge/reader 正本之外的陈旧副本；`home.absStoreDir` / `looksLikeLocalPath`、`index.searchEngine` / `searchEngineAt`、`retrieval closePIT` 为死包装或死代码；`knowledge.(*SystemRepository).operation`、`docs-serve.piece` 为死代码；连带清理 `home_system.go` 注释引用与 `home_mount.go` 失效 strings import。验证：go build / go vet / gofmt 全净，`make test` 绿（ok kc/cli 106s）、`make check-docs` 绿。剩余 13 处 cli 项归统一访问 TRAVERSE 工作线处置；并行线收口、`make quality` 全绿后勾选本条。

### CLI-REFACTOR · 产品 CLI 形状重构

- [x] 已落地：应然设计见 [`docs/CLI.md`](docs/CLI.md)；按 [`cli/REFACTOR.md`](cli/REFACTOR.md) 落地产品闭集：四条真人路径（进房、读知识、写仓、挂源给权）；主路径约 20 条动词；resolve/治理/运维/catalog audit 进阶，根 help 不出现。验收锚点 `TestProductCLIRefactorDefinesTheExactPublicSurface`（60 条）+ `TestRemovedCommandsAreRejected`；落地顺序见 REFACTOR §14，协议缺口见 §15。`catalog use` Client 持久化阻塞后续；create/attach 分离、`detach` 新语义、`grant` 去 `admin` 前缀；知识/写拒绝 `--catalog`/`--dataset`/`--source`。不做旧 argv 兼容层。验收：`make check-docs` 与 `make test` 全绿，场景树与 E2E 已全部改用新 argv，无 skip。
- [x] 已落地：同一轮收口读得懂的输出（同文 §1.1、§4.1、§7）：Client 入口与登录解耦，login/logout 回执不含 `server`；`admission show` 换成「本人 grants + 申请入口」并删 `admission request`（部署只留 `admission.requestURL`）；System 仓发布 `schema/core/source-profile/v1` 并自带 `kr://kc/system` 源说明；`schema list` 删 `coverage` / `exhausted`、每条 schema 不再各带 `repository` / `commit`，`log` / `audit` 同形对齐；`help` 改根/分组/叶子三层渐进式披露。验收：`make check-docs` 与 `make test` 全绿，场景分页断言改用 `continuation`，无 skip。

### CATALOG-01 · Catalog 登记表不能落在易失盘

- [x] 已落地：生产 Catalog 登记表落在独立 Snapshot 权威（`dolt` / `gitea` / `lakefs`，与知识仓同类介质、不同 identity），不走本机 Git、`stateDir` SQL、知识仓或 Writer。`CatalogBinding` 只接受 `driver`+`dir`/`dsn`；未知 YAML `remote:` 与 Git `file://`、实例盘路径、缺 driver 在 `deployment init` / `serve` 失败关闭。丢失的 Catalog 权威不得重建空登记表。本地 `kc init` 夹具仍用 gitdir `NewRegistry`，不是生产路径。验收：`make check-docs` 与 `make test` 全绿，无 skip。锚点 `TestDeploymentRejectsLegacyCatalogGitRemoteField` / `TestDeploymentRejectsInstanceBoundAuthorities` / `TestDeploymentDoesNotRecreateLostCatalogBranch` / `TestSnapshotRegistryPersistsMembershipWithoutKnowledgeSemantics` / `TestCloseReleasesDurableDirectory`。

### CATALOG-02 · Catalog 对所有认证用户可见

- [x] 已落地：已认证用户可发现公开 Catalog（`show` / `catalog list` / `catalog use`），无需 `catalog.read` grant；`private: true` 与未知 Catalog id 仍 fail-closed 要 grant。发现不等于 `knowledge.read`、也不等于 grant/admin。无 `Deployment` 的本地 Home 仍要 grant。验收：`make check-docs` 与 `make test` 全绿，无 skip。锚点 `TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant` / `TestPrivateCatalogStillRequiresCatalogReadGrant` / `TestManagedRepositoryProviderCreatesPublishesAndResumes`。

### AUTH-REPO-01 · 仓的已认证默认可读

- [x] 已落地：仓可声明已认证默认可读（与 Catalog 公开发现同一模式）；未声明 fail closed。不设仓级 public/private，不按 `kr://kc/system` 授权短路。System Repository 选用该声明；运行时 Writer 仍拒绝写平台仓。验收：`make check-docs` 与相关 `go test` 全绿，无 skip。锚点 `TestUndeclaredSystemRepositoryStillRequiresGrant` / `TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant` / `TestDeclaredSystemRepositoryUsesAuthenticatedDefault` / `TestRuntimeWriterRefusesSystemRepository`。

### README-01 · README 是 Markdown 知识对象

- [ ] 本轮认领：README 是 frontmatter 知识对象；约定路径可被 ② 解释；默认 SEARCH 编该对象 `body`；已接入仓直推 published HEAD 后 ③ 能编进投影。`catalog show` 不派生 title/summary。owner 为 [`docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md`](docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md)、[`docs/reviewed/terminology.md`](docs/reviewed/terminology.md)、[`docs/COMPOSITION.md`](docs/COMPOSITION.md)、[`docs/PROJECTION_CONTROLLER.md`](docs/PROJECTION_CONTROLLER.md)。

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

**接口指向：** `knowledge.CoreReadmeSchemaV1`、`knowledge/readme.go`、`internal/repofile` codec、`index.compileValue`。不新增 CLI。

**如何验证：** `make check-docs`；下列测试无 skip，再 `make test`。

- `TestSystemRepositoryPublishesReadmeSchemaAndInstance` / `TestAssertReadmeBinding`
- `TestParseMarkdownReadmeFrontmatter` / `TestIngestMarkdownReadmeFrontmatter` / `TestReadmeAspectIsMarkdownBody`
- `TestReadmeGitCommitWithoutWriterLocators` / `TestPlainMarkdownReadmeIsNotKnowledge`
- `TestCatalogShowRepositoriesStayIdentityAndReadmeIsKnowledge`
- `TestProjectionCompilerIndexesReadmeBodyAsText` / `TestGitPushedReadmeCompilesIntoAccessSpecText`
- `TestProductScenes/catalog-initialized`（库存身份）/ `TestProductScenes/system-schema-published`（`kc/system#readme`）

### WRITE-VALIDATE · 进 published 的知识发布过同一套 Writer 校验

- [ ] 未认领：人可以不经过 `kc writer` CLI 改自有仓；成为已发布知识的那次提交必须过与 COMMIT 相同的 Schema / CAS 基点 / 同仓 `schema_ref`（保护分支或 CI 代发 / 只验不写）。owner 为 [`docs/COMPOSITION.md`](docs/COMPOSITION.md) §3.5、[`docs/GATES.md`](docs/GATES.md)、[`knowledge/writer/README.md`](knowledge/writer/README.md)。

**Goal：** Writer 代数可在不推进 ref 时复用；CI 有公开检查口。

**Non-Goals：** 不把校验塞进 ingestion control；不把外部 CI 做成每次 COMMIT 的 Gate；不抢走业务 git；不改 Connector 必须走 Writer。

**不变量：** `K-21`；Gate 只拦 proposal → published。

**选定与否决：** 选定目录写入走 `writer commit --dir`（对照当前版本求差是 commit 内部动作）。选定可选 `kc diff` 看同一对照。否决产品 CLI 另设 pack / status / plan；否决把 ChangeSet 文件当日常 CLI 操作数；否决最短旅程必须先 diff 再提交；否决面 3 当写面。

### DATASET-01 · 文件层 Dataset 与消费授权（取代 KSET-01）

KSET-01（Workspace→知识集、不改组合语义）不再认领。消费组合改为文件清单 Dataset；授权与组合分成两份整理稿，**验收通过后才升格进文档图并改 owner**。

- [x] 当时两份 Dataset 整理稿已完成；后续文档重构由 DOC-21 接手，当前替换稿见 [`Dataset`](docs/reviewed/dataset.md) 与 [`权限体系`](docs/reviewed/permissions.md)。本条历史完成记录不表示替换稿已完成交接或验收。
- [x] 实现与 owner 升格已落地：发布冻 commit（无 store / 解不出 selector 失败关闭）、Items 为消费真相（空清单不放行整仓）、`file.read`×Dataset、Serving/SEARCH 按清单 path 过滤（hydrate 前）。公开动词是 `dataset define|retire|overlay`；消费是 `file.read`；管理/解析是 `dataset.manage` / `dataset.resolve`。产品面只有 `--dataset`、`KC_DATASET`、HTTP `/datasets`、JSON `datasets`；不保留 `kset` argv、`--kset`、`KC_KSET`、`kset.consume` 或 HTTP `/ksets`。

**Goal：** 跨仓只读切片是命名、版本化的文件清单；检索只跟 latest；消费者 `file.read` 挂在 Dataset 上，维护仍挂 Repository。

**Non-Goals：** 不接 lakeFS Datasets API；不把 Dataset 做成 Snapshot 仓；不为每个 `vN` 建索引；不把 path/Aspect 做成 allow 资源。

**不变量（目标，升格后写入不变量表）：** 清单 item 发布时钉 commit；消费 pin 冻文件清单；`file.read`×Dataset 与仓 `knowledge.read` 互不蕴含；pin 不锁权限；SEARCH 不超出服务版 path。

**选定与否决：** 选定文件层 Dataset 收掉仓名单 kset；消费 action 是 Dataset 上的 `file.read`。否决双轨 kset+Dataset、消费再要整仓 `knowledge.read`、每文件一行 grant。

### DATASET-02 · 发布权威、消费范围与服务生命周期收口

- [ ] 本轮认领：修复架构 review 复现的 pin 扩权、跨 Aspect 越界、语义 VFS 越界、服务版索引随 HEAD 丢失与版本可覆盖问题；消费与发布编排收敛到可独立验证的公共边界。

**交接状态（2026-09-20；后经用户纠偏）：** 本轮实现已落地。当时中止了运行中的全量回归并保留待验收状态；「用户要求只做实现、由用户在其他环境测试」的表述并非用户约定，已废弃。现行标准：代码修改必须通过测试，待验收状态由全量测试与契约门重新确认。

**默认测试入口纠偏（2026-09-21，本次不执行）：** Goal：`make test` 和 testsuite 默认入口使用原有 lakeFS 场景夹具，不要求准备整套部署；owner 为 `docs/reviewed/test-catalog.md`，部署验收边界见 `docs/SERVICE_ARCHITECTURE.md` §11.1。Non-Goals：不改生产 provider 默认值，不迁移全部历史夹具，不删除或削弱合同，不运行测试或启动容器。不变量为 `A-01`、`V-01`；选定运行 `TestProductScenes` 与 `TestMetricPermissionScenes`，复用显式 `KC_TEST_OPENSEARCH_URL`，未配置时沿用一次性 OpenSearch。真实部署场景保留 `make deploy-local-scenes`，也保留在显式 `test-all` 中；原 component/boundary/应用合同组合保留 `test-contracts`。否决先前将默认测试绑定 `deploy-local` 的做法，以及将场景通过宣称为完整组件、HTTP/VFS 或真实部署验收。接口沿用现有 testsuite 分组和 Go 场景入口。

**Goal：** Dataset 的不可变发布记录是消费坐标和文件范围的服务端权威；当前服务版仅在必要投影准备成功后切换，源 HEAD 前进不破坏已发布版本。owner：`docs/COMPOSITION.md`、`docs/PERMISSIONS.md`、`docs/PROJECTION_CONTROLLER.md`、`docs/SERVICE_ARCHITECTURE.md`，产品目标见 `docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md`。

**Non-Goals：** 不把知识或索引放进 Catalog，不把外部 live 数据写成 Snapshot，不按 Dataset revision 复制索引，不改变 VFS 只读且受本机容量限制的边界，不扩展跨仓写事务，不降低架构守卫与既有场景断言。

**不变量：** `V-01`、`KS-01`、`KS-02`、`AUTH-01` / `AUTH-02` / `AUTH-03`、`PC-01`。pin 冻结内容不冻结权限；任何读取、候选回读、Schema/动态绑定和文件投影均不能超出同一服务版范围；发布失败保留上一服务版。

**选定与否决：** 选定 Catalog 内不可变版本记录与单独当前指针、按发布记录重建消费 pin、知识读取共享完整对象范围校验、按固定 Snapshot basis 准备和恢复派生索引、typed 应用编排。否决信任客户端 Items / 任意 commit、任一 Aspect 命中就授权整个对象、语义 VFS 整仓共享投影、消费请求临时重建索引、用 HEAD 索引冒充服务版索引。切片中不完整的知识对象不伪装成完整对象；文件访问仍保留清单内文件。

**接口与验收：** `catalog.KnowledgeSet` / `ResolvedKnowledgeSet`、`knowledge/reader.Serving`、`knowledgeapp`、`index.Controller` 与既有 File Gateway。先把 review 的真实失败固化成回归，再实施；覆盖伪造 Items/commit、历史 pin、当前撤权、跨 Aspect、语义 VFS、源 HEAD 前进与发布失败/重启恢复，最后运行 `make check-docs`、`make test`，全绿且无 skip 才勾选。

**关系边界增补（用户已确认）：** 允许关系保存在独立图谱仓，端点引用其他仓的对象；Writer 仍只提交关系所在仓，不解析或写入端点仓，不引入跨仓事务或隐式读权。Goal 是让 `REL-01`、系统设计 §7、Provider 合同与 Dataset 跨仓一跳检索一致；Non-Goals 为跨仓写、递归图查询、自动把引用对象加入 Dataset。不变量为 `REL-01`、`W-01`、`V-01`、`AUTH-02`、`R-01`；选定结构化 KnowledgeRef 与固定 Dataset 成员投影扇出，否决端点仓等于关系存储仓的校验和 authority 扫描。接口沿用 `CanonicalRelation` / `RelationRetrieveRequest` / `DatasetRelationsExecutor`。先记录 Writer/检索拒绝跨仓的失败，再修正合法双端点 Relation 场景；保留形状、权限、同版本回读和单仓写入断言，整体验收完成前不勾选。

**Provider 差分补充：** 原生 Dolt 实测复现了更新未携带路径时被 Writer 强填默认路径的问题，破坏文件切片稳定性。沿用 `docs/reviewed/provider-contract-validation.md` 的路径提示等价合同与 `KS-01`，目标是未指定新路径的更新保留原路径；不改变显式迁移语义。选定直接传递原始 ChangeSet，由 provider 在合并现有单元后为新单元补默认路径；否决写入前无条件补路径或在测试中补显式路径掩盖差异。回归为 `TestNativeKnowledgeDoltMatchesTreeProviderByOperationStep`，包含原有路径迁移、历史、正文及新增跨仓关系步骤。

**Dolt 验证补充：** 真实 adapter 回归另捕获预期失败查询后把已存在 commit 报成不存在。按 `docs/STORE_ADAPTERS.md` 的 Snapshot 版本合同与 `V-01`，修复 Docker 会话在远端分离 stdout/stderr 后破坏语句确认顺序的问题：在容器内合并输出，再经 Docker 传输；不以重试或忽略错误掩盖版本读取失败。接口不变，保留 ref/merge 合同，并重复验证真实引擎错误后的正常查询。

### DATASET-03 · 逐文件交付贯通、dataset clone 与 Dataset 管理前端

- [x] 本轮完成：U10 逐文件交付贯通——`KnowledgeSetSource` 增 `File`+`Target`（交付寻址时 `Path` 必空）、发布校验目标唯一与祖先互斥（文件对文件、文件对前缀）、overlay 文件按目标替换并继承冻结坐标；File Gateway 交付寻址枚举/读取（`Path` 为根寻址判据、pin 重放保留已接受交付树、Length 0 默认 512 KiB）；C-21 收口（`datasetFSRepositoryPath` 归一化后必须仍落在所选子树内）；`kc dataset clone` 物化当前服务版交付树（非空目录拒绝、只随服务版演进、argv 旅程 `TestDatasetCloneJourney`）；`/ui/` Dataset 控制台进 httpsurface 闭集（87 条）并带页面证据测试；webui vendor（lakeFS v1.58.0 pinned，Apache-2.0，保留 LICENSE/NOTICE）构建产物嵌入 `cli/webapp`。文档：TEST_CATALOG C-17–C-21 转「已定位」并登记证据、reviewed/dataset.md U9/U10 与结尾约束、CLI_EVALUATION 增 clone 行并逐字核对 60 条 help 引用、client/README 补 File Gateway 节；「测试由用户在其他环境执行」表述经用户纠偏废弃（AGENTS.md/TASK.md）。验证：`go test ./cli/`（无环境与 `KC_TEST_OPENSEARCH_URL` 两遍口径）、`./catalog/ ./client/ ./home/ ./internal/... ./index/`（带 OpenSearch）全绿，`make test` 96.9s PASS、`make test-contracts` 640.8s PASS（公开命令覆盖 59/59）、`make check-docs` PASS。

**Goal：** 发布、目录枚举、文件读取与授权对逐文件条目完整成立：发布产出 `DatasetItemFile` 条目并校验目标冲突，文件网关按交付目录树枚举/读取文件条目（支持改名与多来源混排进同一交付目录），相对路径任何位置的父目录跳转都被拒绝；`kc dataset clone` 把已发布 Dataset 的当前服务版交付目录树物化成本地普通目录（产品 argv 不收 `--pin`，见 `docs/CLI.md`），不依赖 kcfs。owner：[`docs/reviewed/dataset.md`](docs/reviewed/dataset.md)（U6/U9/U10、固定范围与生命周期）、[`docs/CLI.md`](docs/CLI.md)（产品 argv）、[`httpsurface`](httpsurface/README.md)（闭合路由表）。

**Non-Goals：** 不把 Dataset 做成可写目标，不复制字节进 Catalog，不给逐文件条目建独立宿主挂载点，不扩展跨仓事务，不在 clone 时绕过 Dataset 消费授权或写入清单外路径；本轮不改 kcfs 的 FUSE 能力边界（逐文件条目在 FUSE 计划中显式拒绝并明确报错，混合 eager 文件与 lazy 目录挂载的 datasetfs 扩展留作后续），不新增 HTTP 路由（clone 与逐文件交付复用既有闭合面，请求形状做向后兼容扩展）。

**不变量：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`。清单 item 发布时钉 commit；空清单不放行整仓；同一目标文件只有一个明确来源，冲突显式拒绝，不按挂载或来源顺序覆盖；逐文件条目与所属仓的其它条目共享同一冻结 commit；路径规范化后仍须处于选定子树内。

**选定与否决：** 选定逐文件条目搭乘现有 `KnowledgeSetSource` 形状（`File`+`Target` 字段，`Path` 必须为空），recipe `.kc-dataset.yaml` 与 overlay 增加 `files:` 段，发布时静态校验目标唯一性，内容级同名冲突在交付时显式报错（发布不解析来源树内容，不做发布期目录遍历）；网关请求在 `MountPath+Directory/File` 之外增加可选 `Path`（交付路径）以寻址挂载子树外的文件条目，文件条目只在首页合并进枚举结果。否决发布期遍历来源树做冲突检查（发布必须不依赖来源树读取）、否决按顺序静默覆盖、否决为 clone 新开 HTTP 路由、否决 clone 生成 manifest 文件（消费坐标由服务端发布记录承担，回执只打印版本与来源 commit）。

**接口与验收：** `catalog.KnowledgeSetSource` / `DatasetItem` / `HashResolved` / recipe / overlay；`cli` 文件网关（`dataset_file_service.go`、`datasetfs.go`）与 `client` typed 请求；`kc dataset clone` 进 `cli/surface.go` 公开闭集（动作 `file.read`×Dataset 消费）；webui vendor 保留上游 LICENSE/NOTICE 并记录 pinned commit，API 层只接 httpsurface 闭合路由。回归：U10 发布/枚举/读取/授权端到端、目标冲突发布拒绝、C-21 断言转绿、clone 物化与拒绝非空目录、clone 只随当前服务版演进。代码修改必须通过测试；契约全绿且无 skip 才勾选。

### SCENE-01 · 走查只用协议场景树

- [x] 本轮完成：走查与 `TestProductScenes` 都只用 `.data/scenes`；`Given existing repository` 经 lakeFS adapter 打开既有 authority。父 home 副本分叉独立物理仓（拷目录不够）。Gherkin 仍 store-agnostic。不接 Datasets API，不出现 `warehouse-agent`。

**Goal：** `Given existing repository` 经 `snapshot/lakefs` 打开既有 authority；`TestProductScenes` 父 home 副本必须隔离物理仓（拷目录不够）。owner 为 `STORE_ADAPTERS.md`（`snapshot-authority-and-derived-media`）与 `.data/scenes/README.md`。

**Non-Goals：** 不改 Gherkin 写 lakefs；不把 Catalog 组件夹具（`NewRegistry` gitdir）改成生产 OpenSnapshotRegistry；不在 `cli` 里 import adapter；不把 docker Graveler 编进协议包；不接 lakeFS Datasets API。

**不变量：** `A-01`、`V-01`；更换 Snapshot driver 不得改变 Reader/Writer/Catalog 命令语义。

**选定与否决：** 选定进程内协议忠实 lakeFS HTTP 假服务作为 hermetic `make test` 夹具，经 `home` 的 lakefs `OpenExisting` 打开；父副本 `ForkStamps` 改写 `remote.yaml` DSN。否决场景执行器继续默认 Dolt；否决把 docker Graveler 编进协议包；否决 Gherkin 写介质名。

**接口与验收：** 夹具在 `internal/testkit`，装配仍走 `cli.AddRepository(..., "lakefs", dsn)`。锚点 `TestSceneExistingRepositoryFixtureUsesLakeFS`（stamp `driver=lakefs` + 两份副本 DSN 互异）与 `TestProductScenes`（全绿、无 skip）。Catalog 仍是 gitdir `NewRegistry`。

### SCENE-02 · 可复用状态、独立用例与关注点视图

- [ ] 本轮认领：按真实复用整理全树状态与验证用例，隔离同节点探针环境，并从就近元数据生成可重叠子图和主视图并集检查。owner 为 [`docs/reviewed/test-catalog.md`](docs/reviewed/test-catalog.md) 与 [`.data/scenes/README.md`](.data/scenes/README.md)。

**Goal：** 状态表达可重建、供后续引用的前置条件；用例表达从该前态出发的操作与断言，临时后态不被其它用例继承。视图可以从非根开始，主视图并集覆盖声明节点、构建边和验证证据。

**Non-Goals：** 不改协议、授权语义或新增产品 CLI 形状；不把视图当作唯一终态或另写依赖树；不把所有有写入的过程升格为状态；只按实际前态和消费者调整全树目录；不把 Go 独立旅程伪装成复用 scene fixture。遵循上述 owner 的证据分母与同次运行规则。

**不变量：** `AUTH-01` / `AUTH-02` / `AUTH-03`、`V-01` 与场景结构、公开命令覆盖、父 construct 复用合同保持；探针互不继承临时写入、授权和运行时状态；显示范围不改变真实构建前置。

**选定与否决：** 选定目录作为构建依赖唯一来源，节点及 probe/Go evidence 就近标注视图，视图定义只存标题/说明和可选显示边界；构建后冻结，每条 probe 在独立副本或重放环境中执行。测试专用端点为每个环境隔离 OpenSearch 物理索引与控制记录，使有投影与无投影前态都不被其它用例污染；不改产品配置或替换检索引擎。否决手写节点/边清单、隐式跨 probe 顺序依赖与只按权威数据是否写入判断状态。

**接口与验收：** `.data/scenes/tree.py` 的文本/JSON 投影与检查入口、`cli/scene_feature_test.go` 执行器及现有场景元数据；先保留隔离与投影缺失的失败证据，再定向验收，最后 `make check-docs`、`make check-validation`、`make test`，无 skip 才完成。

**产品关联增补 Goal：** 按产品 owner `docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md` §8 的稳定 U 条目生成产品视图，建立“产品承诺 → 具体用例断言 → 构建前态”的可追溯关系。文档身份与路径解析以 `docs/graph/` 为准；用例就近声明 `verifies`，工程视图仍保留。`docs/reviewed/test-catalog.md` 拥有证据判读。

**产品关联 Non-Goals / 不变量：** 不在派生 `product.html` 保存独有定义，不手抄产品条目与节点清单，不因映射存在就宣称执行或完整验收通过，不改产品行为或修复上一轮既有协议失败。产品新增条目、引用失效、无依据映射必须可检查；未决或未覆盖范围显式记录 gap。既有 `AUTH-*` / `V-01` 等产品合同不变。

**产品关联选定与验收：** 从指定 owner 章节提取条目 ID 和标题；probe、具名 Go evidence 与 construct 中的明确断言分别引用条目，依赖状态自动补全。否决仅在整个 view 挂文档链接、节点标签自动继承与重复维护成员归属。先新增反例测试，再验证产品引用、缺口检查、原工程视图兼容及 `make check-docs`；不把历史执行结果作为本次通过证明。

**产品关联定向结果（全树迁移前）：** U1–U10 自动生成产品视图，17 条具体验证关联与 9 条剩余范围说明就近可追溯；U5、U9 暂无场景关联，保留显式 gap。32 条视图反例/回归测试与 10 项 Go 场景结构合同通过，无 skip；工程并集覆盖 51 个声明节点、50 条边、59 条 probe 和 125 个 Go 引用。`make check-docs` 通过（35 文档、121 关系）；`make check-validation` 仍只报告下述既有 4 处悬空测试引用。该结果只验证关联与结构，不宣称产品 U 条目完整验收。

**全树整理增补 Goal（用户纠正后）：** 以真实前态复用审计全树，取消仅为测试分组、读操作或单次验证后态建立的目录；`schema-browsed` 等独立 Go 证据就近归到实际前态，临时授权/撤权/归档等步骤归入独立 probe。节点名描述实际建立并被消费的条件，用例名描述操作和预期。owner 仍为场景 README 与 `docs/reviewed/test-catalog.md`。

**全树整理边界与方案：** 本次扩展原样板范围到全树；不改变产品合同、断言或 Go 业务测试，不把 Go 自带 setup 算成场景前态复用，不用空目录、只读 construct 或人为拆分探针凑复用。保留真实构建前置、多个独立用例的共享条件及已有部署走查入口；一次性连续步骤合并为一个 probe，沿途所有断言保留。`AUTH-*` / `V-01`、产品 claim 关联与工程并集保持；先留下伪节点失败证据，再迁移并核对命令/断言与 Go 引用未丢失，更新结构守卫的宿主定位而不降低其语义要求。

**全树整理落地：** 51 个原目录收敛为 21 个共享前态、20 条真实构建边和 65 条独立 probe；每个保留状态声明实际 `fixture`，结构门禁追踪至少两个直接或下游 scene 消费者，独立 Go 不计入复用。11 个 Go-only 目录全部取消，`schema-browsed` 回到初始化前态；临时授权、撤权、归档、退役与一次性校验归入 probe。`source-repositories-configured`、`semantic-knowledge-published`、`dataset-query-principals-granted`、`proposal-preview-created`、`qinghe-knowledge-published` 改名为实际交付条件。清河两种终态用显式 `goto --probe` 正向走查，避免重新伪造状态。

**迁移保全与验证范围：** 117 个去重 `(source, Go Test)`、17 条产品关联、122 份已快照材料完整保留；原 746 项命令关联断言中 742 项仍在可执行 feature，原 Go-only observation 的 4 项参考断言仍保存在说明材料中，其具名 Go Oracle 不变。第一轮实际回放中 `TestProductScenes` 的全部 16 个节点通过；旧 Agent 解析守卫依赖文件顺序的问题已改为核对完整且无重复的角色/brief 集合，Agent 脚本与插件测试的场景引用已同步。新增 10 条 Python 反例验证前态复用、走查 probe 选择与执行前变量检查；最后结果以 `.validation/runs/` 中同次选择范围报告为准，不把声明完整性当产品通过。

**本次定向回放结果：** `.validation/runs/20260920T050906Z-f3af36a0ef1b` 在同一源码指纹下实际运行 44 项顶层测试，42 项通过、2 项失败、0 跳过；`TestProductScenes` 及 16 个子节点均通过。两处失败分别是 `TestMetricPermissionScenes` 和 `TestSceneMutatingGrantProbesAreIsolated` 的同一个 Dataset 跨仓关系断言：预期 `rel/defines/gmv`，实际 `hits=[]`，仍保留原断言。之后仅补正运行时材料路径、走查变量预检及说明，42 项 Python 合同通过；不将定向回放表述为全仓全绿。

**仍属既有的 Agent 执行缺口：** 本次只修正 companion 的场景定位、brief 主体与静态入口；脚本 live/bootstrap 中旧 workspace/admin 命令形状仍需独立更新，不声称已执行付费 Agent 验收。

**验收中发现的前态缺口：** 语义实例旅程曾引用关系仓尚未发布的 Relation Schema；按同仓解析合同在 Domain Schema 构建阶段经 Writer 发布并回读。业务夹具另为 `relationType` 声明文本访问，并验证图仓能搜到 `defines`、不会额外命中 `merchandise`；按 `docs/RETRIEVAL.md` 与 `index/README.md` 保留跨仓 SEARCH 对每个成员能力的检查，不跳过关系仓。同步补正两业务仓的逐仓隔离断言与 `includes nonempty` 的匹配器，使现有断言按已声明 DSL 生效，不改协议语义。

**验收中发现的路由缺口：** 显式 `--dataset` 的 Server 检索被误判为无选择的 Catalog discovery，导致原有 Dataset 用例要求无关的默认发现配置。按 `docs/CLI.md`、`docs/COMPOSITION.md` 与 `cli/README.md` 的显式操作数合同（`API-01` / `V-01` / `KS-01` / `KS-02`）仅修正选择器分类并补回归，保留 Dataset 原有授权与回读断言；不通过配置默认 Workspace 绕过问题。

**全量验收阻塞：** 既有覆盖文档引用的 `TestHTTPWorkspaceSearchKeepsDatasetFileReadBody`、`TestResolveDescriptorBindingAtPinnedCommit`、`TestProductTemporaryPinConsumesFrozenKnowledgeAndCurrentPermissions` 尚无同名测试；架构守卫另报告 `home/managed_named.go`、`home/repository_id.go` 的 adapter import 与 `ManagedRepositoryConfig` fixture seam。它们不由本次场景重组引入，不削弱守卫或改写证据声明来通过；完整验收未绿前保持未勾选。

**保留的场景失败：** `dataset-query-principals-granted/dataset-cli-discovers-searches-reads.feature` 已按现有关系端点合同传入完整 `kc://scene/knowledge/metric/gmv`，仍要求返回 Dataset 内关系仓的 `rel/defines/gmv`。当前 Workspace 关系路径只查端点所属知识仓，返回空 `hits`；`index.RelationsAtContext` 又限定端点与被查仓相同。该跨仓关系执行缺口超出状态/用例组织任务，保留原预期与失败，不改成只查单仓或删除关系步骤。

### SYSTEM-META · 接入方与 Agent 如何理解这套知识模型

- [ ] `kr://kc/system` 目前不足以自描述：接入方不知道该怎么定义知识，Agent 也无法从系统本身理解「什么是 Entity / Relation / Aspect」，以及所有实体共有的元属性（如 lastModified）。仓根 README 知识对象（见 `cli/REFACTOR.md` §7.1）只解决「这个仓是什么」，这一条解决「这套模型是什么」。

### DOLT-01 · 原生 Dolt 每条查询付一次引擎冷启动

> 状态：已随 Dolt adapter 退役关闭；以下保留为历史选型与实测记录。

- [x] 本轮完成：让同一 Repository 在一条命令内复用常驻 Dolt 查询会话，取消每条 SQL 一个 `dolt` 进程的执行方式。

**问题与实测：** `snapshot/dolt/command.go` 的 `query` 每次都执行一个新的 `dolt sql -r json -q`。本机无宿主 `dolt` 时走 Docker fallback，单条查询实测约 750 ms（`docker run --rm`）或约 135 ms（常驻容器 `docker exec`）；开销是 `dolt` 进程启动，与数据量无关。一条只写 2 个对象的 CLI 用例发出 113 次 `dolt` 进程调用，其中 39 次 `DOLT_HASHOF`、13 次 `dolt_branches`、12 次 `SHOW TABLES LIKE` 共 64 次（57%）是每次 `home.Open` 重复的开仓元数据探测。结果是 `go test ./cli` 单条用例耗时 150～220 秒并触发整包超时。这是执行通路缺陷，不是数据规模结论。

**Goal：** 把 `query` 改为向该 rootDir 的常驻 `dolt sql -r json --continue` 会话逐条发送语句并读回结果，会话由既有 `Home.Close()` → `snapshot.Registry.Close()` 释放，即 CLI 是一条命令、Server 是被服务的 Home；变更类命令先释放会话，Dolt 仍只有一个活动写者。owner 为 `STORE_ADAPTERS.md`（`snapshot-authority-and-derived-media`）与 `snapshot/dolt/README.md`。

**Non-Goals：** 不引入新依赖（含 `database/sql` 与任何 MySQL wire driver）；不改 `snapshot.Store` / `TreeStore` / `HistoryStore` 公开形状；不改 ref、CAS、archive、幂等与错误码语义；不改 `kc_files` / `kc_units` / `kc_objects` 表形状；不把 `dolt sql-server` 作为运行时依赖；不改 Gitea adapter；不把测试夹具的容器编排搬进协议代码（`LAYERS.md`、`SCALE_ARCHITECTURE.md`、`PROVIDER_ABSTRACTION_CONTRACT.md`）。

**不变量：** `A-01`、`W-02`、`V-01`；provider 执行通路改变不得改变 Reader/Writer/Catalog 与命令语义，CAS 与命令幂等保持，AS OF 固定 basis 回读结果逐字节不变。

**选定与否决：** 选定按 rootDir 复用的常驻 `dolt sql --continue` 会话（纯 `os/exec`，实测 0.4 ms/语句），结果走 stdout、错误走 stderr 逐条归属，写语句无结果集故以哨兵语句框定响应；任何变更类命令（`init`/`add`/`commit`/`checkout`/`reset`/`branch`/`sql -q` DDL 与 `runSQLScript`）先关闭该 rootDir 的会话再执行，因为实测会话持有独占写锁会使外部进程写入变为 `database is read only`。否决 `dolt sql-server` + `go-sql-driver/mysql`（未批准依赖，且 server 通路把 `COUNT(*)` 等整数结果变成字符串，改变既有 JSON 解码）；否决仅启动 `sql-server` 而客户端仍是 `dolt sql -q` 进程（实测约 205 ms/查询，比现状更慢）；否决把 Gitea 换成 CLI 测试默认 store（会替换掉 native Dolt 默认路径覆盖，与 DOC-14 / provider 合同缺口冲突）；否决只在测试夹具复用容器（仅省约四成，不解决调用次数）。

**接口与验收：** 会话实现私有于 `snapshot/dolt`，不出现在包外公开 API；先跑会失败的进程计数证据（同一流程的 `dolt` 进程启动数上界），再实现，再以 Dolt `RepositoryContract` / `WriterContract`、`snapshot/dolt`、`knowledge/dolt` 与 `make check-docs` 验收，不以 skip 或删除断言换绿。

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

### CACHE-01 · 同版本正文缓存与独立后台预热

- [x] 已完成：服务内同版本正文缓存、可注册的独立后台消费者、有界热点与可选冷启动预热；2026-09-08。38 项本任务合同均实际执行通过、零跳过，缓存及接缝定向 race 通过。最终 `make test` 零失败，真实 adapter 套件零失败、零跳过；短套件的既有条件跳过不计为通过。实现、错误语义补修和范围边界见 [缓存实施与验证记录](.validation/reviews/cache-2026-09-08.md)。

**Goal：** 在固定 basis 的 SEARCH / RELATIONS / 精确 READ 回读处复用完整 Snapshot 正文，批量回源仅处理未命中项；预热独立于索引，并能在发布、丢通知和进程重启后通过 HEAD 对账恢复。owner 为 `SERVICE_ARCHITECTURE.md` §4.8、`STORE_ADAPTERS.md`、`PROJECTION_CONTROLLER.md`。

**Non-Goals：** 不让 Snapshot/Reader 持有语义缓存实现，不修改 Writer/Catalog 协议、不改变公开 SEARCH 候选与正文语义、不缓存动态 State observation、不新增消息中间件或分布式缓存；不把有界预热解释成全仓覆盖或生产容量资格（`LAYERS.md`、`RETRIEVAL.md`、`LIVE_MATERIALIZATION.md`、`SCALE_ARCHITECTURE.md`）。

**不变量：** 保持 V-01、C-01、CA-01/CA-02/CA-03、PC-01、P-01、R-01/R-02、AUTH-01/AUTH-03、KS-02；旧 pin 不读新版本，命中按当前权限交付，缓存副本不受 selector/State hydrate/交付修改污染，消费请求不构建投影。

**选定与否决：** 选定仓 + commit + 对象/完整 Address 的有界进程缓存、批量 miss 回源、返回副本、后台热点与有限冷启动预热；派生消费者分别跟踪进度与故障，用来源版本变化和对账驱动。否决 ID-only/latest 回退、用索引文档更新替代正文变化、让缓存实现 Retriever、Writer 同步预热以及按仓共用所有派生状态。

**接口与验收：** 窄读取端口由 `knowledge.Hydrator` 拥有，上层实现位于 `retrieval/cache`，查询执行器与 Reader/Serving 仅依赖端口；后台扩展由 `index.SnapshotConsumer` / `Controller.RegisterConsumer` 拥有。先运行会失败的缓存/接缝/独立消费者证据，再实现；以版本/权限/副本隔离、批量回源次数、预热恢复及正式 `make check-docs` / `make test` 验收，不以 skip 换绿。

### RETRIEVAL-01 · 声明式索引与有界检索执行可靠性

- [x] 已完成：按知识探索调研 §6 实施声明/类型贯通、精确标量与分页、增量可见性、预算/取消、Workspace 有界归并及批量读取；2026-09-08。本任务必需合同均实际执行通过，未用 skip 消除失败。最终 `make test` 成功，公开命令成功与边界覆盖均为 70/70；真实适配器及定向 race 零失败、零跳过。全项目短套件的既有条件跳过与本轮范围分别列在 [实施与验证记录](.validation/reviews/index-mvp-2026-09-08.md)，不计为通过；模型质量及生产容量实验仍未宣称验收。

**Goal：** 贯通真实 Schema 发布、按访问声明编译、类型化查询、固定版本分页与增量发布；补齐查询预算、取消和批量读取，以失败反例和正式合同验证改进。

**Non-Goals：** 沿用 [Aspect 访问](docs/ASPECT_ACCESS.md)、[检索](docs/RETRIEVAL.md)、[知识 Schema](docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md) 与 [投影控制](docs/PROJECTION_CONTROLLER.md) 的归属；不新增访问关键字、向量引擎、任意图路径、成本优化器或索引内 LLM，不改变发现/正文授权及 Writer 边界。

**不变量：** `S-01`、`I-01`、`C-01`、`R-01`、`P-01`、`IX-03`、`IX-04`、`KS-02`；知识身份、固定 basis 回读、物理派生可重建及增量成本边界保持成立。

**选定方案：** 保留三类 access 与现有分层；声明类型统一校验；只编必要索引槽位；无损类型化比较与游标；后端部分失败准确传播；全部增量可见后发布；补判必须可证明，查询执行有预算并支持取消；同版本批次复用定位，未变化投影不重写。时间精确物理表示属于 adapter 并升级物理版本。词表映射和直接/渐进阅读通过消费侧既有能力验证。

**否决方案：** 静默截断长值、浮点替代整数、毫秒实现收窄时间合同、末批刷新冒充全批可见、任意字符串补判冒充全文精确、输出 LIMIT 冒充总预算、按 Workspace 复制投影。

**接口与验证：** 具体形状落在 [knowledge](knowledge/schema.go)、[retrieval](retrieval/README.md)、[index](index/README.md) 和提供方公开合同；先记录红例，再修复并运行 `make test`、真实 OpenSearch 与必要并发检查。未通过的检查与未执行的模型/规模实验分别记录，不提前勾选。

### WALK-01 · 2026-09-08 产品走查修复

- [x] 本轮已完成：修复身份、登录会话、托管供给与管理地址、接入权限和消费入口问题。`make test-all` 命令通过，公开命令成功及风险边界覆盖均为 70/70；没有用 skip 或删除断言消除失败。运行期间并发变更仅涉及研究文档，原记录保留 `source-changed`，精确指纹重建证明被测代码未改。修复、逐项证据和补充文档验证见 [走查交付报告](.validation/reviews/product-walkthrough-2026-09-08-fixed.md)。SELF-01～04 的更广部署边界仍单独保留，真实组织 SSO、远程自有 Dolt 和生产源长期运行未宣称已验收。

### SELF-01 · KC 产品身份与客户端会话贯通

- [ ] 让已有认证入口产生统一的 KC 用户名，并让交付客户端持续使用已配置的 Server 与登录会话。所属阶段：M3。

**本轮实现：** 可信人类身份统一为 KC 用户名；不可变 IdP subject 与用户名耐久绑定，旧身份仅经显式迁移。客户端保存默认 Server、按 Server 隔离会话并复用刷新；Taihu 的应用秘密留在 Server。定位反例已由红转绿，正式验证由 WALK-01 汇总。真实组织身份服务的生产登录仍需部署环境验收。

**要完成什么：** 各认证器把可信身份映射为统一的 KC 用户名；登录后，产品身份和公开 identifier 使用规范用户名，例如 `kaiqidong`。认证提供方的 subject、数字 ID、token 和前缀只用于内部认证关联及审计，不进入普通命令、仓名或分享输入。用户名必须由可信身份源验证，并明确唯一性、改名和回收规则，不能由请求自报。将服务入口封装在交付客户端，登录后跨命令复用连接并处理凭证刷新或失效；配方与 pin 不保存凭证。

**需要判断：** 用户名是否全局唯一、能否改名或被回收；若存在变化，KC 必须保存内部稳定绑定并定义迁移与防冒领规则，但不因此把内部 subject 暴露给用户。

**完成标准：** `kaiqidong` 在空用户配置下使用已交付的客户端登录，`whoami` 和后续产品界面显示 `kaiqidong`；授权、审计、托管仓所有权与重启恢复始终归于同一 KC 用户。普通输出不出现 `gitea:4` 之类 Store subject。凭证失效可恢复，后续每次命令按当前授权重新求值；全过程不修改部署配置，也不手工指定 Server 地址。固定 pin 不因登录刷新而变化。

依据：[认证设计](docs/reviewed/deploy-auth.md) §4、[服务设计](docs/SERVICE_ARCHITECTURE.md) §5、[当前登录实现](cli/README.md)、[产品缺口](docs/reviewed/mvp-acceptance.md)。

### SELF-02 · Gitea 与 Dolt 托管 Snapshot Store 自动供给

- [ ] 用户登录后只需创建 Repository，后台自动完成 Store 账号/租户和仓的分配、绑定与恢复。所属阶段：M3；这是现有底层 managed create 之上的产品闭环。

**目标流程：** `kaiqidong` 登录 KC，只选择创建 Repository、用户可理解的仓名和允许的托管 Store（Gitea 或 Dolt）；KC 后台从当前用户与产品上下文生成完整 Repository identity 和幂等命令身份，确保该 Store 中存在 identifier 为 `kaiqidong` 的账号或隔离租户，创建仓、保存内部连接、登记 Catalog，并按策略授予创建者维护能力。用户不填写完整 Catalog/Repository 协议坐标，不接触 command-id、Store 地址、服务账号、token、数据库目录或内部 allocation id。

**本轮实现：** 用户按名称和允许的 Store 建仓，Gitea ensure 同名账号，Dolt 分配用户名隔离空间；原始分配账、绑定与创建者授权保持幂等。普通返回包含管理地址，自己的仓列表可重新查询。Gitea 原生 SSO 需要已配置登录源；Dolt 选定 KC 管理页，不把本地目录伪装为 DoltLab。正式验证由 WALK-01 汇总。

**设计边界：** KC 用户名是跨 Store 的产品 identifier；Gitea/Dolt 的内部 subject、账号 ID、仓 ID、DSN、凭证和目录只保存在 Store Directory/供给账。Repository identity 不随选择 Gitea 或 Dolt 而改变，也不能从物理仓名反推协议身份。Gitea 和 Dolt 可以有不同的物理隔离机制，但对用户呈现相同的创建、状态、失败和恢复语义。

**需要补齐：** 托管 provider 的用户/租户 ensure、仓 allocate、权限安装、binding 持久化和幂等恢复状态；用户名冲突、已有同名账号归属、部分成功、重试、停用/改名、Store 不可用、配额及删除/归档规则。服务凭证只能代表平台执行供给，不能成为知识创建者身份。

**完成标准：** 全新的 `kaiqidong` 只登录 KC，输入仓名并选择 Gitea 或 Dolt，即得到属于 `kaiqidong` 隔离空间的 Repository；随后直接发布和回读。创建返回用户名称、仓名、逻辑 Repository identity 和状态，两个 Store 的普通返回均不暴露物理账号 ID、allocation、路径、command-id 或服务凭证；重复点击和实例替换返回同一逻辑结果；另一个用户不能占用、打开或恢复 `kaiqidong` 的 Store 空间。任一分阶段失败都有可恢复后态，不产生无人认领账号或仓。

依据：[服务与托管仓设计](docs/SERVICE_ARCHITECTURE.md)、[Store 边界](docs/STORE_ADAPTERS.md)、[当前供给实现](home/managed.go)、[Gitea managed adapter](snapshot/gitea/managed.go)。

### SELF-03 · 接入方自行连接和维护自己的 Snapshot 仓

- [ ] 提供自有仓连接、校验、凭证更新和恢复入口。所属阶段：M3；平台托管 create 已完成，不重做。

**本轮实现：** 独立 connect / connection show、check、rotate 入口支持允许来源上的自有 Gitea 仓，逐仓凭证保存在 Server 私有耐久账。连接与轮换先只读验证，失败保留原连接，重启后仍可轮换过期凭证。自有远程 Dolt 与 Gitee 没有声明为已支持。正式验证由 WALK-01 汇总。

**要完成什么：** 在服务管理面保存可恢复的连接及凭证引用，验证来源存在、身份一致、权限足够，再按既有合同进行 Catalog 准入；失败不得创建、收养或改写外部仓。连接、秘密与 Catalog 成员关系分别由各自权威管理。

**需要判断：** 首批自有仓覆盖的后端、凭证归属与维护权限；若目标确为 Gitee，需要单独核对 adapter 能力。连接暂停/移除如何影响已有消费者，也要有明确产品语义，不能顺带删除远端知识。

**完成标准：** 两位接入方独立连接两个既有仓、轮换各自凭证并持续维护；彼此不能使用对方连接；实例替换后仍可恢复；身份不符、凭证失效、重复请求和准入失败均有明确后态，外部仓不被初始化或污染。

依据：[介质与接入边界](docs/STORE_ADAPTERS.md)、[部署与恢复合同](home/README.md)、[当前产品缺口](docs/reviewed/mvp-acceptance.md)。

### SELF-04 · 首次准入与仓维护者分享

- [ ] 让普通用户按明确策略开通基础能力，让仓维护者在获授范围内分享与撤销。所属阶段：M3。

**本轮实现：** 首次准入按显式资格和窄动作策略请求，仓维护者在当前获权与平台转授上限的交集内分享和撤销。首次授予有耐久回执，重试或重启不恢复撤销规则；消费方旧 pin 仍按当前权限检查。正式验证由 WALK-01 汇总。

**要完成什么：** 明确准入资格、可授予的基础能力、仓维护者的转授上限及撤销规则，并提供普通用户入口。用显式可追溯策略承接，不能以“已登录”“已登记”代替授权。

**需要判断：** 首次准入基于组织资格、邀请还是审批；维护者可分享哪些动作、能否转授以及撤销如何影响下游授权。来源/历史读取的最小权限先依 REVIEW-02 统一，不能在不同入口得到不同结果。

**完成标准：** 新接入方获得准入、创建仓并发布；本人通过普通入口授权消费方，消费方自行发现、选源、读取并采用更新；撤销后旧 pin 也不能继续越权读取。全过程不授予全局管理员权限、不由部署方临时补权，重启或重放不补回已撤销权限。

依据：[权限设计](docs/PERMISSIONS.md)、[产品旅程](docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md)、[托管仓现有验收](docs/reviewed/mvp-acceptance.md)。

### DYN-01 · 动态 State 的真实部署与重启恢复

- [ ] 完成投影控制设计 §11.3 的整条部署验收。所属阶段：M4；承接原动态索引未完成项。

**现状与影响：** State READ、动态投影、变化通知及同 basis 回读均有实现和局部验证。缺的是独立 Observer、runtime、lakeFS、KC 与 OpenSearch 串联后的完整证据，以及 DOC-20 所列消费闭环缺口；已有双容器/adapter 测试不能证明 KC 重启后整条链仍能继续运行。

**要完成什么：** 用真实源变化触发 Observer，服务经已授权 Binding 拉取，推进动态投影；稳定知识仍由 Collector 走 Writer。KC 不直接读源夹具，runtime 不与 KC 共享内存 fake，Collector 不直写索引，观察值不写进 Snapshot。

**完成标准：** 源值改变后无需消费者触发维护即可检索到新观察；Snapshot commit 和文件视图不变；KC 替换/重启后能够恢复并继续追赶；重复、乱序、失败通知有后态证据，分页不混版本，回读失败不伪装为零命中。通知未送达时的时效/失联行为按 REVIEW-04 裁决验收，不能顺带声称实时完整。

依据：[投影控制](docs/PROJECTION_CONTROLLER.md) §11.3/§12、[动态物化](docs/LIVE_MATERIALIZATION.md)、[现有索引合同](index/README.md)。这里验收 State，不把 Stream 窗口或多实例当作已经完成。

### GUIDE-01 · 单文件手册的阅读与打印验收

**知识模型中的索引声明（2026-09-23）：** 架构图②列出字段 `text/filter/sort` 及其用途，并将 `origin` 外部访问原点与字段索引声明分开；产品 owner 同步。文档图、手册服务测试、格式检查与四档宽度离线检查通过。

**统一访问原语（2026-09-23）：** 全景图按发现、定位读取、关联溯源、外部访问四组列出公开原语，并在产品 owner 中强调 BROWSE 的有界范围与 ACCESS/INVOKE 分责。文档图、手册服务测试、格式检查及四档宽度离线检查通过。

**外部状态接入图分组（2026-09-23）：** 在整体架构图中用一个“外部状态源”区域包含 Resource Access 与 Observer，说明同一来源的当前值访问和变化通知是两项接入职责；两条连线仍分别指向统一访问/索引控制层和索引控制层。产品 owner 同步说明。

**Snapshot 提交路径澄清（2026-09-23）：** 架构图补出业务原生提交与 KC 服务端提交两条入口，在⓪中明确 commit 与发布分支推进，并将索引控制的输入改为已发布 commit 推进。接入章节说明自有仓等价校验、文件提交与知识发布分责、托管入口的代理式产品方向及现有 lakeFS 写权威约束；不宣称标准 Git 协议兼容已交付。产品 owner 同步。文档图、手册服务测试、格式检查与四档宽度离线检查通过，已查看更新后的架构图；仅调整文档。

**整体架构图（2026-09-23）：** 按本轮 ASCII 对齐结果，将接入方、⓪–③、统一访问和 VFS 合并进同一张内嵌矢量图；③内显式展示索引控制层，连接 Observer、Resource Access 和 Snapshot 版本推进，区分取值与变化通知连线。保留版本、权限纵栏及四层跳转，产品 owner 同步说明图中职责。`make check-docs`、手册服务测试、格式检查通过；四档宽度离线检查无整页溢出，窄屏图内滚动，已检查桌面和打印样式，样本在 `/tmp/kc-integrated-architecture-qa/`。未重做整册分页打印验收。

**章节拆分（2026-09-23）：** 将瞬时状态独立为第 09 节“解决方案：瞬时状态接入”，第 10 节“只读 VFS”仅讲 Dataset 本地文件消费；同步目录、后续编号并保留旧跳转锚点。文档图、手册服务测试、格式检查通过；离线核验 13 节目录与编号一致，四档宽度无整页横向溢出。仅调整手册组织，未改变产品行为。

- [ ] 本轮认领：重写产品价值与分层采用路径，对齐 Dataset 发布、独立授权及默认最新消费，明确 VFS 仅适用于本机放得下的 Dataset 且目前只读；核对现行 CLI，并补齐浏览器视觉、窄屏、复制和打印验收。所属阶段：M3；承接原产品手册未完成项。

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

**尚不能勾选：** `make test` 在生成验证库存时被现有场景目录 `.data/scenes/catalog-initialized/grants-bootstrapped` 缺少 `_meta.yaml` 阻断，尚未执行产品套件；没有删除断言或用 skip 绕过。待场景树恢复后补跑完整契约。验证记录：`.validation/runs/20260920T024414Z-71bd11adcb42`。

依据：[产品手册](docs/product.html)、[产品能力边界](docs/reviewed/mvp-acceptance.md)。手册仍需随实际交付状态更新，不能提前承诺尚未验收的能力。

### SCALE-01 · 数据量、历史量与并发增长下的容量验证

- [ ] 建立可重复的规模测试与结果台账，得到实际支持范围、瓶颈和容量上限。所属阶段：M5；执行器和 S0/S1 基线从当前阶段并行推进。

**要回答的问题：** 知识持续增加、持续修改、同时被多人检索时，系统能承载多少数据，延迟和成本如何增长，索引多久追平，重启/恢复要多久，以及最先限制容量的是哪条通路。功能用例通过、生成了大量输入或单次命令成功，都不能回答这些问题。

**当前进展：** 已有规模设计、按性能视角场景树组织的压测用例（旧 KC-PERF-01～12 已映射进 `.data/scale/scenes/` 状态树与探针）及基础流式生成器；runner 仍待实现，各探针 `requires` 列出所需 runner 能力。生成器只输出 table family/事件描述，尚未形成经公开 Writer 实际提交、发压、采样、判定的闭环；历史参数只记录目标提交数，不生成对应数量的真实提交。已有 S0 数据生成 smoke 的历史记录，没有可据以宣称 S0 端到端压测或更大档位通过的完整容量报告。最高通过数据档、历史上限 Hmax、支持并发和恢复时间均待测。

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
| A 工具与环境就绪 | 基础生成器已有，完整 runner/证据链待补 | 固定 seed、真实模型/历史、断点续跑；单用例选择与速率/并发控制；经公开 Server/Writer 发压；环境、原始样本与结果可复算。先核实现有代码和部署是否满足档位前置，不照搬历史审计的环境数值 |
| B S0/S1 基线 | 待建立 | 先证明发压端、采样和判定有效，再同环境重复测量；分别量化文件型 Gitea 与 native Dolt 的适用通路和成本增长，保留失败与瓶颈。小规模结果用于改进，不外推为百万表资格 |
| C 数据/历史逐档扩量 | 待 B 通过及专用环境就绪 | S2→S3，再按模型进入 S4/S5，R1 与 H0～H4 分开控制变量；每档通过后再升级，有正确性或资源越界先停止并保留证据；未测档标明未测 |
| D 持续运行与容量结论 | 待选定档位及恢复方案具备 | 完成稳态持续窗口、突发、重建与故障恢复；报告最大通过数据量/历史档、支持负载、瓶颈、单位存储成本及五年容量区间。代际相关结论必须有 REVIEW-01 选定方案的实际恢复证据 |

**需要判断：** 首次产品交付承诺的数据量、历史量、并发/时效和部署资源档位。现有 S3 target、S5/H4 qualification 是规模规划，不能默认成已兑现的首发承诺；在确认专用环境和资源投入前，先完成 S0/S1 的测量闭环。具体阈值引用已有规模 gate，不通过临时降低数据量、持久性或副本配置来过线。

**完成标准与结果记录：** 按已确认的承诺范围逐项给出实测值、门槛、通过/失败/证据无效及未测项；保存源码指纹、实际版本/硬件/配置、seed、真实数量与字节、采样窗口、原始指标/trace/扫描计数和正确性对账。小规模基线完成后更新本项阶段状态，目标范围未验收前不勾整项。后续只在此保留最新有效基线链接、已通过范围、下一档阻断与瓶颈，详细运行历史留在报告中。S0/S1 验证工具链不意味着大档通过，普通 make test/test-all 不承担容量资格。

依据：[规模模型与门槛](docs/reviewed/scale-benchmark.md)、[压测场景树与用例入口](.data/scale/CASES.md)（用例本体在 [.data/scale/scenes/](.data/scale/scenes/README.md)，按性能视角的场景树组织）、[环境配置合同与就绪性](.data/scale/ENVIRONMENT.md)、[当前生成器](.data/scale/generator/generate.py)。新增动态/多仓规模用例需补到这些关联文件，TASK 不另维护一套执行规范。

### DOC-14 · 原生 Dolt 与 Tree 能力隔离

- [x] 收口 native Knowledge adapter 的公开能力边界。所属阶段：M5 的基础纠偏，方向已选定，可独立推进。

**问题与影响：** 规模设计要求 native Knowledge adapter 不暴露 TreeStore；当前 knowledge/dolt 仍实现 TreeStore、DirectoryReader 并转发 ApplyTreeCommit。调用方仍可能获得原始路径写入口，削弱“知识写入必须经过 Writer”的能力隔离。它是确定的设计/实现差距，不等于已证明生产请求发生了绕过。

**处理范围：** 先盘点真实调用者、文件投影与现有 Conformance 的需求，再拆清 native 知识能力与通用 Snapshot Tree adapter。不能只删除接口让旧测试失效，也不能把通用 snapshot/dolt 一并退役。

**完成标准：** 原生知识服务装配不暴露 raw tree 写旁路；Reader/Writer、关系检索、批量/精确读取和恢复仍走正式合同；必要的文件能力由明确的适配层承担；增加能力拒绝/毒化反例，并运行保留的 provider 合同。

依据：[规模设计](docs/reviewed/scale-architecture.md) §4、[当前接口与转发](knowledge/dolt/repository.go)、[MVP 缺口](docs/reviewed/mvp-acceptance.md)。

### DOC-15 · 生产可观测性与告警的有效性

- [ ] 补齐实际承诺范围的 SLI、告警触发/恢复与生产证据。所属阶段：M5。

**已完成与缺口：** 全失败窗口漏报、零流量 NaN 已有专项修复和回归；遥测、部分面板及规则已经存在。尚缺的包括：哪些请求/来源有资格进入完整性 SLI 的依据；部分依赖信号及跨进程传播；各告警可定位的面板/处置说明；完整 firing/recovery 演练和选定环境的容量/SLO 基线。规则语法通过和全失败专项通过都不能证明这些条件。

**建议拆次推进：** 先明确首个生产拓扑与用户承诺，补原始信号和分母；再对可用性、延迟、队列、依赖失联和恢复逐类注入故障；最后按既定窗口持续观测。每次可以关闭有证据的子范围，整项在承诺范围验收完成前保留。

**完成标准：** 正常、全失败、无流量、被排除流量与不同来源组的 SLI 含义可复核；每类告警能实际触发、定位用户影响，并在恢复后解除；后端/采集器失联可见；实际依赖版本、配置、负载和持续窗口随结果保存，不拿短时 smoke 代替生产 SLO。

依据：[系统可观测性设计](docs/SYSTEM_OBSERVABILITY.md)、[验证缺口 O-05～O-13](docs/reviewed/test-catalog.md)、[规则与回归](docs/observability/)。

### DOC-16 · Provider 能力合同与跨 provider 等价性

- [x] 让 native Knowledge provider 进入共享合同套件，并建立跨 provider 等价性证据。所属阶段：M5 的基础纠偏，方向已选定，可独立推进。

**问题与影响：** 共享合同套件（`RepositoryContract` / `WriterContract`）与 provider 无关，但规模档选定的权威 provider `knowledge/dolt` **从未接入**；一次性探针实测接入后 14 个子测试全绿、零跳过，说明缺口是覆盖矩阵而非实现。同时**跨 provider 等价性为空**，而 `SCALE_ARCHITECTURE.md` §5.3 把它列为迁移的核心差分测试。另有一处会 OOM 的静默全量回退路径（变化识别能力缺失时）所在包没有任何行为测试。

**处理范围：** 先接入合同套件，再建按步骤索引对齐的等价性 harness（不同 provider 的 commit 标识必然不同）；随后补能力拒绝毒化项与静默回退的行为测试。不改 ①/②/③ 公开语义；不新增第二套主题 owner。设计与被否决方案见[能力合同](docs/PROVIDER_ABSTRACTION_CONTRACT.md)，门槛与分档见[验证设计](docs/reviewed/provider-contract-validation.md)，顺序与完成定义见[执行指南](docs/PROVIDER_REFACTOR_GUIDE.md)。

**完成标准：** 等价性与毒化门槛按 `PV-01`…`PV-12` 实际执行并保存证据；`PAC-01`…`PAC-08` 在对应测试存在后才进入 `ARCHITECTURE_INVARIANTS.md`；未执行或被跳过的档位不得计为通过；基线阻塞（`make check-validation` 的未解析引用、`check-surface` 缺 `rg`）先修或明确登记。

依据：[执行指南](docs/PROVIDER_REFACTOR_GUIDE.md)、[能力合同](docs/PROVIDER_ABSTRACTION_CONTRACT.md)、[验证设计](docs/reviewed/provider-contract-validation.md)、[规模设计](docs/reviewed/scale-architecture.md) §5.3、[规模门槛](docs/reviewed/scale-benchmark.md) §7 P0。

### DOC-17 · 有界读写与变化识别（规模可行性前提）

- [x] 让提交、定位与变化识别的工作量与仓库总量解耦。所属阶段：M5 的前提项，方向已选定，可独立推进。

**问题与影响：** 三处现状与「有界」承诺直接冲突：(1) **写**——`knowledge/writer/treecodec.go` 的 `readKnowledgeTree` 对每个 commit 全列路径再逐 blob 读，并把全量定位表整体重写为一个 blob；(2) **读**——按对象读取时全量加载并解码该定位表，维护分页也先全量读入再切片；(3) **变化识别**——⓪ 的增量能力没有生产实现，`knowledge/maintenance` 在能力缺失时**吞掉错误**后走两次全量遍历并把整仓身份装入内存，`index` 因此把该失败降级为全仓重建，且只记 `diverged/content`。三者叠加的后果是每个 commit 的写与每次点读都是 O(仓库总量)，而成本失控不会报错——这正是 `SCALE_ARCHITECTURE.md` §3.1 已列为不可接受成本反例、却一直没有实现与测试的那几条。

**处理范围：** 只读受影响对象对应的单元；定位结构改为可分页、可重建、可丢弃，并保证其丢失后可从权威单元重建；变化识别升为 ⓪ 的一等能力（与 [能力合同](docs/PROVIDER_ABSTRACTION_CONTRACT.md) 的选定方案一致），缺失时显式失败而不是静默全量，并把退化原因作为可观测的一等值；目录列举走范围扫描，不用无索引可用的前缀匹配；**同一 repository 同一时刻只允许一个投影执行者**（进程内与跨进程语义都要写清）。不改 ①/②/③ 公开语义。

**完成标准：** 计数 store 断言「一次单对象 PUT 的读次数与仓库总量无关」与「一次单对象 READ 的解码字节数与总量无关」；非原生变化识别的 provider 上单对象提交不触发全仓重建；定位结构删除后单对象读仍可工作且成本有界。`SCALE-01` 的容量结论以此为前置。

依据：[规模设计](docs/reviewed/scale-architecture.md) §3.1/§9.5/§10、[写路径](knowledge/writer/treecodec.go)、[定位与分页](knowledge/reader/repository_service.go)、[退化路径](knowledge/maintenance/change.go)、[投影增量入口](index/sync.go)。

### DOC-18 · 守卫覆盖与能力面收口

- [x] 让守卫覆盖全仓、让缺能力失败关闭而不是 panic。所属阶段：M5 的基础纠偏。

**问题与影响：** (1) 分层守卫把 `scripts/` 列入跳过目录，而 `scripts/fixture-deployment` 在那里打开具体 authority，落在装配根收敛面之外且不被守卫覆盖；(2) 装配根对可选能力做不带 `, ok` 的类型断言，驱动返回缺能力的 store 时是 **panic**，而不是稳定错误码；(3) `knowledge/dolt` 作为规模权威 provider 仍实现并转发字面路径写入（与 `DOC-14` 同源）；(4) 同一 repository 暴露十余个可断言接口，可选能力散落在请求路径，同一 provider 在不同入口可能给出不同可用性结论。

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

依据：[能力合同](docs/PROVIDER_ABSTRACTION_CONTRACT.md) `PAC-02`/`PAC-06`、[分层设计](docs/LAYERS.md)、[当前守卫](internal/arch/layers_test.go)、[当前装配断言](home/managed_source.go)。

### DOC-19 · 幂等账本的保留、清理与恢复

- [x] 给命令账本明确的保留期、显式恢复入口，并去掉无收益的兼容包袱。所属阶段：M5 的基础纠偏。

**问题与影响：** (1) 处于 PENDING 的命令条目**永不自动清理**，也没有公开的解决/放弃入口——进程在提交前后死亡会让该命令 id 永久不可用，只能换 id 重试；(2) 账本没有保留与归档策略，随命令数单调增长，而设计门槛明确要求启动时不加载全部历史；(3) 账本里残留未上线的旧形状字段与写入迁移逻辑，属于无收益的兼容包袱；(4) 证据写入失败会在提交已成功之后返回错误，调用方看到「假失败」，重试会撞上 CAS。

**处理范围：** 增加带审计的显式解决/放弃入口与保留窗口，并写明「replay 只在窗口内保证」；删除未上线的旧形状；明确证据失败不回滚已接受的提交。不改写入代数与 CAS 语义。

**完成标准：** 在「预留后中断」与「提交后中断」两类故障点上重启，都给出确定结论（重建原结果或失败关闭等待人工核对），不出现永久不可用的命令 id；账本增长有界且有清理证据；证据介质不可写时，已提交的写入不被报告为失败。

依据：[规模设计](docs/reviewed/scale-architecture.md) §8.3、[命令账本](snapshot/commandlog/)、`REVIEW-07`。

### APP-CORE-01 · Typed Application Core

- [x] 把应用用例从 CLI flags/HTTP transport 中抽离为 typed executor。所属阶段：M5 的结构纠偏；
  方向由 `SERVICE_ARCHITECTURE.md` 的 `API-01` 与 §1.2 选定，待 `DOC-14/16/17/18/19`
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

依据：[服务架构](docs/SERVICE_ARCHITECTURE.md) `API-01`/§1.2、[分层设计](docs/LAYERS.md)、
[产品 CLI](docs/CLI.md)。

### DOC-20 · 观察记录与 M 面服务合同

- [ ] 把 M 面取值路径与观察记录的合同写清，并补两个反例测试。所属阶段：M4/M5 交界；设计与用例部分已整理，未执行验证；当前由 DOC-21 继续整合文档，本项不勾选。

**问题与影响：** 当前有 `StateLookup`、动态投影与同 basis hydrate，但它们不能共同证明新鲜度、
历史保留和当前源授权。现有 State catch-up 会跳过已有观察，活动记录替换不等于历史保留，
旧 Dataset 声明的动态维护也不能由 HEAD 对账覆盖。单 key refresh 只收窄 lookup，不能据此
宣称观察存取和索引写入满足增量成本。

**Goal：** 按[动态物化](docs/LIVE_MATERIALIZATION.md) §2.6/§6/§9 定义可信 State 消费，并将
方向性任务关联到已有断言或明确缺口。新观察按声明取值，同依据重读使用可靠保留的观察；
控制恢复归[投影控制](docs/PROJECTION_CONTROLLER.md)，授权归[权限设计](docs/PERMISSIONS.md)。

**Non-Goals：** 不改运行实现或公开形状，不执行验证；不新增 Snapshot/live Store、Catalog cut、
对象 ACL、通用消息物化器或 Stream 查询入口。介质边界依[介质角色](docs/STORE_ADAPTERS.md)，
统一访问职责依[服务架构](docs/SERVICE_ARCHITECTURE.md) §4.7。

**不变量与方案：** 延续 `D-01`、`V-01`、`P-01`、`IX-04`、`KS-02` 及 `AUTH-01`–`AUTH-03`。
选定分别证明覆盖/时效/重读/授权，State 阶段提供有界观察保留；否决用 complete 代替实时保证、
用最新值补旧依据、用后台身份代替消费授权、只追 HEAD，以及单 key 变化的隐式全库重建。
默认时效、容量/保留值和共享授权装配仍待选定；形状由公开类型、包 README 与 Conformance
承接，不写入设计正文。

**设计与用例产物：** 动态物化 U1–U9；验证体系 B-19–B-27；场景 `product/materialization/*`
从 owner 提取方向任务，以具名 evidence 关联实际断言，缺口写入 `_views.yaml`。未创建假定
能力已实现的场景状态；未新增执行结果或完成结论。

**完成标准：** 两个反例可执行——(1) 来源声明 `latest-only` 时，观察记录丢失后历史读取必须明确失败，不得静默返回当前值或空值；(2) 缺少取值入口时 Bound State 读取必须失败关闭，不得把信号载荷当值、不得降级为扫描、不得返回占位空值。运行时的重读能力与保留期上限按声明校验。

依据：[动态物化](docs/LIVE_MATERIALIZATION.md) §6、[介质角色](docs/STORE_ADAPTERS.md) §2、[服务架构](docs/SERVICE_ARCHITECTURE.md) §4.7、[观察类型](knowledge/observation.go)、[Serving 端口](knowledge/serving/)。

### DOC-21 · 在 reviewed 中重构设计书

- [ ] 本轮认领：完成按组件组织的替换稿；正式替换旧 docs、协议交接与验证仍未完成，不勾选。
  本轮增量：13 份非设计资料已迁入 `docs/reviewed/`（OKF path 与按路径读取的代码/测试同步）；
  20 份顶层旧稿的交接审计缺口已按落点补进 12 篇组件稿（详见
  [`REVIEW_PLAN.md`](docs/graph/REVIEW_PLAN.md) 本轮进度一节）。存量债已清理：7 处悬空
  Test 引用改为退役记录，cli/ 死代码与被遮蔽的复杂度/文件体积/克隆门禁失败就地修复
  （含 help 双表合并与 `snapshot/stamp` 提取），`make check-docs`、`make quality` 与完整
  lakefs 套件全绿。剩余：`provider-contract-validation.md` 的 Dolt 调用点叙述归 adapter
  资料整理轮。

**Goal：** 以 [`docs/reviewed/README.md`](docs/reviewed/README.md) 为下一版入口，按可以独立
评审的组件责任组织设计，不以篇数为目标。补齐索引控制、声明式索引、权限体系、Dataset、
Hook/Gate 和 CLI 交互设计；每篇展开职责、取舍、失败与恢复、方向性用例。公开协议承接
字段、动作、错误码与状态机。现行文档治理 owner 是 `docs/graph/documents/document-map.okf`。
补充 Dataset 目录重组的可执行用例源码：多仓片段交付、旧版路径与字节固定、冲突发布不替换
已接受版本；组合 owner 为 `docs/COMPOSITION.md`，验证记录 owner 为 `docs/reviewed/test-catalog.md`。

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
旧文、协议、研究、验证资料的去向和切换步骤记录在
[`docs/graph/REVIEW_PLAN.md`](docs/graph/REVIEW_PLAN.md)。旧条目与场景引用在正式切换时一并
迁移；本轮的静态阅读与编辑不是验证通过。

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

依据：[规模设计](docs/reviewed/scale-architecture.md) §12.3、[身份与版本原则](docs/KNOWLEDGE_CATALOG_DESIGN.md)、[Writer 同仓合同](knowledge/writer/writer.go)。

### REVIEW-02 · 能读当前正文的人，是否天然能看历史与来源？

**场景与现状：** 同一用户读取同一份知识，走单仓维护入口和 Workspace 消费入口，可能需要不同权限。权限设计把历史/来源包含在仓读权中，公开单仓动作又有独立准入；组合路径的检查不完全相同。仅来源动作可在单仓取得来源，仅历史动作但无仓读权则可能得到成功空日志。分享时的最小授权和拒绝结果因此难以解释。

| 选项 | 好处 | 代价与边界 |
|---|---|---|
| A：仓读权包含历史和来源 | 分享模型简单，同一授权可完成正文、历史、来源复核，符合现有整体仓读边界 | 需处理旧独立 grant，不能把“只许查来源”静默升级为整仓读权；若需要当前内容与历史保密分离，须另划治理边界 |
| B：仓读权之外，再要求对应动作准入 | 可以控制历史列表与来源查询入口的使用 | 发权与各入口检查均需同步；它本身不能隐藏全部旧值或出处：普通 READ 可携带来源，也允许固定历史 commit |

**我的建议与需要你判断：** 如果知识仓是完整的可追溯发布物，倾向 A；若仅希望控制操作入口，可以选 B。若真正需要“当前正文可读、过去/出处不可见”，仅选 B 不够，还要重新设计 READ、旧 pin 与文件视图的内容交付边界。请先判断希望约束的是操作，还是内容披露；不能仅因当前代码已有动作名就决定政策。

**最低验收：** 同主体、同目标、同一当前授权，单仓、Workspace 和 HTTP 的结果一致；只读、仅历史/来源、两者齐全、仅 consume、成员缺权、撤权后旧 pin 均有明确测试，缺权不能成功返回空日志。身份、正文读权与发现权的既有边界不变。

依据：[权限设计](docs/PERMISSIONS.md)、[公开动作](cli/surface.go)、[应用授权](cli/allow.go)、[READ 返回内容](knowledge/read.go)、[历史列表过滤](cli/object_log.go)。关系遍历的动作粒度也应一起核对，不能把本次结论机械扩大到所有知识动作。

**进展（2026-09-23）：** 关系遍历的动作粒度已先行裁决——`TRAVERSE` 按独立动作 `knowledge.traverse` 登记（Dataset 通道与 RELATIONS 同一 `file.read` 准入，`--repo` 通道按仓读权 fail closed），见 `PERMISSIONS.md` 接口表。历史与来源的粒度问题仍按本条开放。

### REVIEW-03 · Schema 破坏性变化必须换身份，还是允许原位迁移？

**场景与现状：** 例如某字段由字符串变为结构化对象，已有实例与消费者仍依赖旧结构。产品设计 §4.4 要求新 major 身份，U5 又保留完整迁移证据路径；尚未定义证据充分性。当前 Writer 在验证实例迁移之前就拒绝 breaking 复用，这不能替代设计裁决。发布 v2 与实例改引可以同批；旧 v1 的删除检查尚不扣除同批改引，当前需后续提交删除，不能把这三步统称为已支持的一次原子迁移。

| 选项 | 好处 | 代价与边界 |
|---|---|---|
| A：新 Schema 身份，显式迁移实例 | 版本区别清楚，允许新旧共存、分批采用 | 接入方与消费方要管理两版身份、查询范围和迁移进度 |
| B：同一 Schema 身份，在证明完整后原位演进 | 使用者保留稳定 Schema 名称 | 需定义同仓完整引用集合、一次发布的校验/原子范围与旧 pin 解释；实例合法不等于消费者兼容，Binding 的外部动态值也不在 Snapshot 原子事务内；大迁移成本与回滚规则不能缺失 |

**我的建议与需要你判断：** 默认倾向 A，先完成显式迁移路径；只有稳定 Schema 名称是实际需求，且能明确证明范围与成本时再考虑 B。需要你确认 U5 的“完整迁移证据”是否原本就意在允许同 ID breaking，而非仅指向新版本迁移。

**最低验收：** 兼容变化仍可原位更新；A 允许新旧共存、分批迁移，B 必须证明完整迁移；无效发布整批拒绝，HEAD 不动；选定方案下新旧实例/Schema、旧 pin、索引与消费者采用更新的行为都能解释。不能增加跨 Repository 原子写承诺。

依据：[产品与 Schema 生命周期](docs/KNOWLEDGE_PRODUCT_AND_SCHEMA.md) §4.4/U5、[Writer Schema 合同](knowledge/writer/README.md)、[当前兼容与删除检查](knowledge/writer/schema.go)。

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

依据：[动态物化](docs/LIVE_MATERIALIZATION.md) §6、[投影控制](docs/PROJECTION_CONTROLLER.md) §10、[现有 notice 反例](index/controller_notice_test.go)。

### REVIEW-05 · 检索是按仓可达性过滤候选，还是把可见性留到交付？

**场景与现状：** 现行是「搜宽读严」：无仓读权时交付链屏蔽正文，既不标 `partial`，也不从 `SearchView` 抹仓。于是两个后果都没被裁决——命中仍返回对象 ID、仓与 commit，已经泄露「某仓存在某对象」；且 `LIMIT` 的语义未定义，它计公开命中还是可见正文。

**选项与建议：** A 授权前置于候选定位（仓级可达性过滤），代价是授权状态影响召回、需要说明 coverage 语义；B 维持现状，代价是必须定义无权命中是否计入 limit、是否返回壳，并把存在性泄露写成明示风险。建议 A：它让检索对调用方有意义，且与「交付链只改正文」不冲突——授权发生在候选定位之前。

**最低验收：** 「3 条命中、其中 2 条无权、limit=2」的用例在两种政策下给出不同且被记录的答案；撤权后旧 pin 不得绕过当前权限。

依据：[权限设计](docs/PERMISSIONS.md)、[架构不变量](docs/reviewed/architecture-invariants.md) 的 `AUTH-01`/`AUTH-03`、[检索设计](docs/RETRIEVAL.md)。

**进展（2026-09-23）：** 范围收窄后部分落地——新增访问语义采用资格前置：`TRAVERSE` 的 frontier 扩展要求对象所在仓在本次固定范围内（越界止步并显式标记）；语义召回的窗口在已准入范围内形成，召回策略不改变授权粒度（`PERMISSIONS.md` 选定项）。既有词法 SEARCH 的「搜宽读严」与存在性泄露问题（`AUTH-01` 及其守护测试所固化行为）仍按本条开放，保留本验收用例，不在统一访问扩展中顺手裁决。

### REVIEW-06 · 字段级可见性（遮蔽/脱敏）何时选定？

**场景与现状：** 交付链目前只有一个已选定段——仓读权正文屏蔽；其后的隐私化/脱敏被明确列为**未选定且禁止实现**。但真实部署普遍需要「同一知识对不同主体呈现不同字段」。

**选项与建议：** 现在就把它作为**已知未选定的扩展点**写清触发条件与代价，并保证现设计不把它堵死——检索侧已有显式字段白名单机制，交付投影应复用它，避免将来再发明一套。

**最低验收：** 一个「遮蔽 `body`、保留 `name`」的思想实验用例，列出必须改动的层，并证明不新增第二套字段可见性机制。

依据：[权限设计](docs/PERMISSIONS.md) Non-Goal、[交付链](delivery/)、[检索细化投影](retrieval/refine.go)。

### REVIEW-07 · 访问证据是否分级（必须成功 vs 允许降级）？

**场景与现状：** 访问证据被描述为 fail-closed 追加的本机 JSONL；另有文档把证据丢失说成「只损失审计覆盖率」，而服务层又承诺「每取一个文件都记访问证据」。三者不等价：没有写清哪些动作**必须有证据**、哪些是 best-effort。叠加已确认的证据端口被绕过，就是「契约看起来有、实际没接」。

**选项与建议：** 给证据分级并在端口上体现：必须级的失败应让请求失败或显式标记审计降级，降级级只记 metric。同时决定证据端口是落实为真缝还是删除，避免保留「已抽象」的错觉。

**最低验收：** 让证据介质不可写，观察 READ/SEARCH/文件读取各自行为与分级一致；端口若有替身，替换后行为不变。

依据：[访问证据](docs/OBSERVABILITY.md)、[服务架构](docs/SERVICE_ARCHITECTURE.md) §6.2、[介质角色](docs/STORE_ADAPTERS.md) §2。

### REVIEW-08 · KC 授权与源侧授权是什么关系？

**场景与现状：** 现有说法是「外部系统的实时业务授权仍由外部系统强制」，KC 只读正文权；同时动态观察请求会把调用方身份与追踪上下文传给墙外运行时。三条没写清：(1) 运行时是否必须用该身份到源侧再授权；(2) KC 放行而源侧拒绝时错误如何呈现；(3) KC 撤权后，已经交给运行时的身份是否还能继续读源。

**选项与建议：** 在访问合同里显式规定「KC 授权是必要条件、源侧授权是充分条件」，并规定错误码映射与撤权后的行为（不得缓存授权决定）。这三条直接决定「KC 是不是安全边界」。

**最低验收：** 「KC 放行 + 源拒绝」与「KC 撤权 + 旧 pin」两个反例有确定结果；撤权后已有运行时不得继续读源。

依据：[Aspect 访问](docs/ASPECT_ACCESS.md)、[权限设计](docs/PERMISSIONS.md)、[动态物化](docs/LIVE_MATERIALIZATION.md) §3.7。

### REVIEW-09 · 已发布知识如何撤回（takedown）？

**场景与现状：** 领域生命周期的终点是归档，而归档是**整仓**动作，没有单对象撤回。误发布敏感内容只能靠新 commit 删除，旧 commit 与旧 pin 仍可读——即协议里没有泄露响应的位置。

**选项与建议：** 显式区分三种语义并至少选定一种：单对象 tombstone（保留历史、消费面不可读）、仓级归档、物理删除交由保留策略。无论选哪种，都必须说明「旧 pin 读到什么」。

**最低验收：** takedown 用例——撤回后旧 pin 必须给出明确失败或 tombstone，不得静默返回原值。

依据：[系统设计](docs/KNOWLEDGE_CATALOG_DESIGN.md) 的 `K-24`、[规模设计](docs/reviewed/scale-architecture.md) §12.3、[权威接口](snapshot/store.go)。

### REVIEW-10 · 是否提供可迁移的 Canonical 导出？

**场景与现状：** 设计承诺「历史可迁移」「迁移不改语义」，但公开面没有「导出全部知识（含多版本与来源）」的动作；维护扫描不产出可移植产物。

**选项与建议：** 定义 Canonical 导出/导入：一个能表达「对象身份 + Address + 值 + schema 引用 + 来源 + 快照坐标」的流式格式，导入经 Writer 重建。明确「导入后身份不变、commit 标识可以变」。

**最低验收：** 在两个不同 provider 之间往返后，同一基线的对象身份与摘要全一致；往返过程不依赖某个 provider 的私有形状。

依据：[规模设计](docs/reviewed/scale-architecture.md) §13、[能力合同](docs/PROVIDER_ABSTRACTION_CONTRACT.md)、[维护扫描](knowledge/maintenance/)。

### REVIEW-11 · 是否采纳「优化目标形态」（能力协商 / Delta / 包重组）？

**场景与现状：** 一份并行复核提出七个动作：能力协商替代类型断言；增量变化识别升为 ⓪ 的一等协议；同形状写请求与请求上下文上移；删除无调用方的假缝端口；墙外运行时获得可执行合同；把索引包拆成只读定位与写派生物；守卫覆盖全仓、例外按类型而非目录。它声明「不考虑兼容、系统未上线」，并给出七步落地顺序与各自的失败检查点。其中**能力协商与增量识别已并入**[能力合同的选定方案](docs/PROVIDER_ABSTRACTION_CONTRACT.md)；假缝与守卫分别对应 `REVIEW-07` 与 `DOC-18`。

**需要你判断的四个开放点：** (1) 增量能力给到「路径级」还是要求原生 provider 直接给「对象级」；(2) 绑定解析类型的归属是否上移；(3) 采集宿主是改名留在仓内还是移出版本库、本仓只留纯对账；(4) 是否做包改名——改名会牵动分层文档与守卫的层名表，**不改名其余动作仍然成立**，因此可最后做或不做。

**最低验收：** 若采纳，七个动作各自的检查点必须可执行（请求路径不再出现能力断言；只有写派生物包能 import 写 provider；守卫纳入全部非测试生产包），且按仓规先落 owner 文档与不变量条目再改实现。

依据：[能力合同](docs/PROVIDER_ABSTRACTION_CONTRACT.md)、`DOC-17`、`DOC-18`、`REVIEW-07`、[分层设计](docs/LAYERS.md)。

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

依据：[动态物化](docs/LIVE_MATERIALIZATION.md) §6.1/§6.2、[服务架构](docs/SERVICE_ARCHITECTURE.md) §4.7。

### REVIEW-13 · 观察记录的保留期与上限由谁定？

**场景与现状：** 持续保留观察会增加存储与恢复成本；来源是 `latest-only` 时，旧观察又不能
通过重取最新值恢复。保留期要满足承诺的复核任务，同时受平台容量约束。

**已有方向与待选范围：** 接入方声明需要的保留期、平台设上限并强制、超限显式拒绝已在 owner
选定。仍需决定未声明时的默认值、容量额度、正在使用的观察如何延迟清理，以及容量不足时的
准入/背压行为。不能静默提前清理已经承诺的观察，也不允许默认无限保留。

**最低验收：** 超限不静默截断；记录丢失后历史读取明确失败；备份与恢复要求不与可丢派生同级。

依据：[动态物化](docs/LIVE_MATERIALIZATION.md) §6.3、[介质角色](docs/STORE_ADAPTERS.md) §2。

### REVIEW-14 · State 观察记录采用什么持久恢复承诺？

**场景与现状：** State 取值与有界观察保留已纳入下一阶段设计，Stream event/window 仍后续
选定。现有 active Serving State 不能证明更新和重启后的历史重读，不能只记 basis 就算完成。

**待选范围：** 观察介质、发布前的耐久条件、实例替换后的恢复、备份与损坏处理。先履行单实例
持久恢复与有界保留，不以通用历史查询、多副本或分布式事务作为前置；保留期内故障和正常到期
应有不同的可解释结果，具体接口由上层运行时合同拥有。

**最低验收：** 选定档位的端到端旅程可执行；未选定的能力明确失败而不是静默降级；不把观察记录宣传成来源真相。

依据：[动态物化](docs/LIVE_MATERIALIZATION.md) §6 与 §8.3、`DOC-20`。

## 4. 后续候选与验证依据

- [x] RESEARCH-01 · 已沉淀知识探索路线与开源项目调研，记录词表映射、模型直接阅读、动态渐进披露及可选向量路线的证据、取舍和评测条件。文档合同 `make check-docs` 与定向空白检查通过；仅完成文档与文档图，不代表路线已实现或实验已通过。
- [x] RESEARCH-02 · 已沉淀派生投影控制与 Retriever 的业界对照：业界 ingestion 映射到 Collector→Writer，面 3 映射到 level-based reconcile，Retriever 映射到候选定位口而非 RAG 正文口；记录仍需完善的追平 claims、时效、动态部署、搜宽读严与 source-side 前置条件。文档合同 `make check-docs` 通过；仅完成文档与文档图，不代表路线已实现，也不裁决 REVIEW-04/05。

以下来自已有设计/缺口，尚未单独排期，不自动作为当前单实例自助闭环的完成条件：

| 方向 | 尚待推进的内容 | 进入下一轮的条件 |
|---|---|---|
| 知识探索路线 | 词表辅助、模型查询改写、直接阅读与动态渐进披露；向量保留为可选、非优先路线。证据与实验建议见 [知识探索调研](docs/reviewed/knowledge-exploration-research.md) | 根据实际问题集的质量、遗漏与总成本决定；研究记录不构成实现排期，也不新增协议 |
| 派生投影控制与 Retriever | 查询范围级 READY/lag、complete 与时效、真实 Observer 闭环、仓可达性过滤、source-side 补判前置。对照与建议见 [派生投影控制与 Retriever 业界对照](docs/reviewed/ingestion-retrieval-research.md) | 先回到既有 owner / REVIEW 项；研究记录不构成实现排期，也不新增协议 |
| 发现与接入体验 | 全 Catalog 发现搜索、README 与投影热状态、Connector 运行/凭证托管、公开名称清理 | 按 M3 实际用户路径选择；完整范围继续在 MVP 缺口维护 |
| 新消费能力 | Stream 窗口查询、MCP Gateway、多语言 SDK | 有明确使用场景后独立定义合同与验收；不能复活已退役 APPEND/Stream 表面 |
| 后续部署拓扑 | 多实例协调、更多部署与升级组合 | 单实例数据规模、历史退化与备份/恢复测试已进入 SCALE-01，不再列作后续候选；新增拓扑按实际承诺单独验收 |

进展判读与证据入口：

- 已完成里程碑的实现、前提和具名验收入口见 [MVP 验收](docs/reviewed/mvp-acceptance.md)；逐条断言与缺口见 [验证目录](docs/reviewed/test-catalog.md)，不在 TASK 复制完整测试库存。
- 数据规模当前只有设计、用例与基础生成能力；有效容量基线待 SCALE-01 建立。规模运行证据落在 .data/scale/runs/ 的各自 run 中，必须有实际数量、负载和环境；不能以普通功能测试报告替代。
- [2026-09-08 文档与验证工具审查结果](.validation/reviews/docs-audit-2026-09-08.md) 对应其记录的源码指纹和选择范围，包含本轮 TASK 整理之前的定向检查；它不证明当前所有产品能力或生产资格。
- 库存从 make validation-inventory 生成；实际运行从对应 .validation/runs/ 或 CI artifact 判读。不同日期、源码或节点 latest 不能拼成一次“全绿”。
- SELF-01～04 从既有 MVP 自助缺口提升为路线待办；原手册/动态部署未完成项分别收口为 GUIDE-01/DYN-01，DOC-14/15 与 REVIEW-01～04 保留原编号。
