# 走查

定位：整理稿。guide。可执行步骤在 [`.data/scenes/`](../../.data/scenes/README.md)。现稿 [`WALKTHROUGH_v5.1.md`](../WALKTHROUGH_v5.1.md) 仍有效，直到本篇升格。

---

## 1. 定位、边界、规范

**定位。** 走查写「既有前态 → 本次操作 → 可观察结果」。只有实际被后续构建或多个用例复用的前态才是状态节点；一次性发权、撤权、归档、校验与合并可以完整发生在一个用例中。章节是给人走的任务，不是命令课或状态清单。

没有走查，协议旅程只存在于场景树和测试名里，接入方看不到「现在在哪、下一步改什么」。

**边界。**

- 不定义协议、错误码、不变量。那是设计文档和 `ARCHITECTURE_INVARIANTS.md`。
- 不定 argv / HTTP 形状。那是 `cli/surface.go` 与 Conformance。
- 不当覆盖格子或运行证据。那是 `TEST_CATALOG.md`。
- 不教怎么写 feature、怎么嵌目录、执行器怎么跑。那是 `.data/scenes/README.md`。
- 不拥有 U1–U10 应然旅程。那是 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`。
- 不写数仓实体、容量、产品手册。
- 不套设计文档五段合同。guide 只写目标、前提、操作、可观察结果。

**规范。** 走查服从实际构建关系，不另编一棵故事树。

1. 目录嵌套表达真实构建前置；状态的 `fixture` 说明交付什么条件。验证的标题、来源和产品关联就近写在 `_meta.yaml`，任务写在入口的 `_bundles.yaml`。
2. 从已有前态起笔，不重写祖先，也不把部署方 setup 插进消费者任务。
3. 构建步骤在 `_build/construct.feature`；独立验证在 `_probes/`。具名 Go 证据使用自己的 setup，不冒充 scene 构建或 fixture 消费者。观测钉状态字段或错误码，禁止只写命令成功。
4. 树父不是发权。attach / Workspace / pin 都不授 `knowledge.read`。
5. create 与 attach 不互相隐含；`knowledge-published` 与 `domain-schema-published` 是两条写入脊。
6. `_build` / `_materials` / `_probes` / `_results` 不是分叉。probe 可变更本用例环境，但其后态不被其它 probe 或子状态继承。
7. 公开命令闭集由 construct / probe 和具名 Go 证据钉住，不另写覆盖清单。
8. 清河茶铺夹具留在 `named-repositories-created/qinghe-knowledge-published`，不另建数仓黑盒套件。

独立验收看前态是否真的被消费、用例是否保留断言；不能用目录名、独立 Go 测试数或只写了 goto 目标证明复用。

---

## 2. 从生成视图定位

全图与关注点视图由 [`.data/scenes/tree.py`](../../.data/scenes/tree.py) 读取目录和本地元数据生成，本篇不再手抄第二棵树：

```bash
.venv/bin/python .data/scenes/tree.py
.venv/bin/python .data/scenes/tree.py --view access-control
.venv/bin/python .data/scenes/tree.py --view publication
.venv/bin/python .data/scenes/tree.py --family product
.venv/bin/python .data/scenes/tree.py --probes-at catalog-initialized
```

显示入口可以不从根开始；隐藏的祖先仍是执行所需前置。工程视图按关注点筛选；产品视图按文档条目关联到具体断言。两者均不表示用例已运行通过。

`catalog-initialized` 已有 System 信任根，System 读与不可写是这里的独立用例。`source-repositories-configured` 交付的是已配置但尚未登记的业务源仓，供未登记边界与 attach 构建复用，不能把它称为再次发布 System Schema。

Schema 分页浏览、正式平台建仓、部署恢复、动态观察与文件挂载的细合同通过宿主上的具名 Go evidence 定位。这些不是空壳状态，测试自身的 setup 和 Oracle 保持独立。

---

## 3. 任务

任务入口与 walk 来自状态上的 `_bundles.yaml`。construct 进入可复用前态；probe 从该前态验证独立风险。多个 probe 即使列在同一个 bundle，也不靠排序传递授权、会话或报告。精确命令和完整断言在 feature 中。

### bootstrap · `catalog-initialized`

**目标。** 检查初始 Catalog 库存与授权前态。

**前提。** 场景夹具已给出 Catalog、System 信任根和空授权规则；正式部署的 bootstrap 管理规则由另一前态表示。

**操作与结果。** `probe-status.feature` 列出 `kr://scene/catalog` 并观察空规则。根 construct 钉 Dataset 为空、System 仓已登记及 `home` / `namespace` 缺席。`probe-system-schema-readable.feature` 回读 System；两个不可写 probe 分别检查普通和特权主体的 Writer 请求被拒绝。

System 分页浏览的 Go 用例 `TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent` 也挂在这里，验证分页与正文边界；浏览本身不产生新状态。

`access-rejects-invoke-flags.feature`、`read-rejects-pin-flag.feature` 与 `feedback-rejects-unknown-outcome.feature` 也从根前态检查参数拒绝，不要求先发布 Domain Schema 或 Dataset。

### catalog-allow · `catalog-initialized`

**目标。** 给已认证主体 `catalog.read`，并证明它不隐含审计或管理权限。

**前提。** 场景 Catalog 规则为空；本机夹具无正式 Deployment 默认发现声明。

**操作与结果。** 构建 `catalog-read-granted` 后，两个独立 probe 分别拒绝登记表审计，以及发权/归档；原读权规则保持。该前态确实被这两个用例复用。

### catalog-revoke · `catalog-initialized`

**目标。** 撤销本条 Catalog 读权后，下一库存请求被拒绝。

**操作与结果。** `probe-revoke-catalog-read-denied.feature` 在同一用例内给 `revoke-probe` 发权、按返回的 ID 撤权、观察后续 `show` 被拒绝。不依赖 `catalog-read-granted` 中 bot 的读权，也不建立撤权后的空壳节点。Dataset 撤权与仓授权独立性由本宿主的 `TestUserJourneyManageAgentAccess` 另外证明。

### catalog-audit · `catalog-initialized`

**目标。** `catalog.audit.read` 可读登记历史，不放行库存发现。

**操作与结果。** `probe-audit-grant-without-inventory.feature` 从空授权前态出发，发放审计权，回读规则和历史，再观察 `show` / `catalog list` 被拒绝。临时授权只属于本用例。

### catalog-create · `catalog-initialized`

**目标。** 建仓准入不隐含库存读取或管理权限。

**操作与结果。** 构建 `catalog-create-granted`，以 `grant list` 求值证明规则命中；两个独立 probe 验证库存与管理边界。真正创建平台仓、发布回读、替换实例后续用，定位到根节点的 `TestManagedRepositoryProviderCreatesPublishesAndResumes` 等正式配置 Go 证据，不假装由这个 Home 构建。

### product-core · `repository-attached`

**目标。** 从接入完成的既有仓验证权限边界并发布知识。

**前提。** 业务 Snapshot 已配置并 attach；接入完成不代表 create 或发权。

**操作与结果。**

- `probe-inventory-without-body.feature` 临时发放库存读权，验证身份可见而正文仍拒绝。
- `probe-writer-grant-isolates-principals.feature` 验证接入方可以提交、旁观者写入被拒绝。
- `probe-archive-rejects-dataset-definition.feature` 归档 Catalog 后拒绝定义 Dataset；`probe-detach-keeps-head-readable.feature` 验证拿下成员不删除 Snapshot。
- `probe-publish-permissions-keeps-read-denied.feature` 从 `repository-attached` 自行发布权限知识并验证读取仍被拒绝，证明 permissions Aspect 是知识，不是 KC 闸门；不要求先发布普通知识。
- 普通知识由 `knowledge-published` 交付后续 Dataset 和治理用例。
- Schema 走 `domain-schema-published`，其后 `schema-read-granted` 的两个用例验证实体浏览、字段内省与实例读权边界；语义实例由 `semantic-knowledge-published` 交付。

这两条写入脊保持各自的真实前置。Binding、资源访问与动态观察的具名 Go evidence，以及访问 runtime 材料均挂在 `repository-attached`。`access-missing-runtime-unavailable.feature` 自行发布 Binding 后验证 runtime 不可达；`invoke-missing-capability-denied.feature` 验证缺少操作能力时拒绝调用。仓默认可读和 Collector Preview 也在这里挂独立 Go evidence。

### http-local · `http-served`

**目标。** 在同一类已启动 Server 前态上验证身份绑定、客户端会话与准入查询。

**操作与结果。** 三个独立 probe 分别检查匿名请求被拒绝与 reader 绑定、login 后 whoami 的当前身份及 logout 回执、本人授权与申请入口。登录不是树根，也不发权。访问账、trace 与分页细节通过这里的具名 Go 证据定位；只观察 access/hitmap 来源的 probe 挂在根节点。

### absent-surfaces · `catalog-initialized`

**目标。** 退役入口必须失败。

**操作与结果。** `probe-retired-checkout-denied.feature` 在初始库存上调用 checkout 并观察用法错误。对象 LIST、connector-run、APPEND/Stream、MCP、export 等冻结边界保留在 `TestRemovedCommandsAreRejected` / `TestAppendAndStreamSurfacesStayAbsent`；不声称这个单条 feature 运行了全部入口。

### P-22 · `projection-synced`

**目标。** 仓级搜索可定位，正文交付仍独立受读权控制。

**前提。** 投影追上 published HEAD。投影是 SEARCH 前提，不是 READ 前提。

**操作与结果。** 构建 `knowledge-search-granted` 后分别执行声明式搜索、搜索主体读正文被拒绝、临时增加读权后交付完整 Canonical 的用例。无读权时 HTTP 命中保留、正文剥离且不标 partial；`read-grant-returns-canonical.feature` 的授权和回读在同一用例中，不再建立一次性读权后态。

### P-23 · `dataset-query-principals-granted`

**目标。** 同一消费者自己发现、固定版本、检索、读取。

**前提。** 语义脊已发布 `scene-set`，消费者拥有相应 Dataset 查询授权；不把 operator 的 setup 插进消费步骤。

**操作与结果。** `dataset-cli-discovers-searches-reads.feature` 全程由同一主体经 Server 发现、SEARCH、READ 和解析对象，返回固定来源。另有独立用例验证服务版本推进与主体间授权隔离。Dataset 的 `file.read` 只开放该清单，不隐含仓级 `knowledge.*`；pin 不发权。仓级无读权的正文屏蔽由 P-22 验证。

### maintenance · `dataset-defined`

**目标。** 已有知识上创建提案和 Preview，再分别验证结构或完成合并。

**前提。** 普通知识脊已有 Dataset；Gate 只绑 merge，COMMIT 不走 Gate。

**操作与结果。** 构建 `proposal-opened` 时 candidate 已产生、main 仍旧值；构建 `proposal-preview-created` 时把 candidate 叠到祖先已发布的 scene-notes v1，取得固定 Preview basis。Preview 构建不另发布 Dataset；发布 v2 由 `dataset-defined` 上的 `probe-publish-next-dataset-revision.feature` 独立验证。

从该 Preview 独立执行两个用例：

- `probe-structure-validation-keeps-main.feature` 执行协议结构检查，得到 PASSED 报告，main 仍为原值；它不运行外部业务套件。
- `probe-record-passed-validation-merges-candidate.feature` 记录外部套件已经给出的 PASSED，先读 main 仍旧值，再用本用例产生的报告合并并读回 proposed。只推进目标仓 Ref；既有 Dataset pin 不随之移动，下次重新解析相应新版本才见新值。

第二个用例不消费第一个用例的报告；校验、报告和合并后态不各建一个目录。

### 清河茶铺 · `qinghe-knowledge-published`

**目标。** 从已发布的双仓知识分别验证检索与 Dataset 范围裁剪。

**前提。** `named-repositories-created` 已显式创建并 attach `table-meta`、`sales-semantic`；本节点向两仓写入清河夹具，准备墙外资源访问，并回读表、列、作业、关系、模型和指标及实时计数。

两个独立用例使用同一前态：`probe-sync-projections-and-search.feature` 同步两仓投影，验证表、列、模型与指标可检索；`probe-publish-dataset-with-scoped-members.feature` 只组合 tables、semantic-models、metrics 路径，回读订单表、销售模型和 GMV，作业不在清单。后者的 Define/READ 不依赖前者投影。

正向现场走查明确选择用例：

```bash
.venv/bin/python .data/scenes/goto.py qinghe-knowledge-published --probe probe-sync-projections-and-search.feature
.venv/bin/python .data/scenes/goto.py qinghe-knowledge-published --probe probe-publish-dataset-with-scoped-members.feature
```

每条命令准备所需前态后执行选定用例。只 goto 状态不会顺带构建投影或发布 qinghe-sales；验证拒绝与错误码的用例交给测试执行器。
