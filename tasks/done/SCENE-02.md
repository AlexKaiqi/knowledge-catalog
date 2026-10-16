# SCENE-02 · 可复用状态、独立用例与关注点视图

- [ ] 本轮认领：按真实复用整理全树状态与验证用例，隔离同节点探针环境，并从就近元数据生成可重叠子图和主视图并集检查。owner 为 [`doc/engineering/test-catalog.md`](../../doc/engineering/test-catalog.md) 与 [`.data/scenes/README.md`](../../.data/scenes/README.md)。

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

**保留的场景失败（2026-10-15 更新）：** 原缺口是关系端点带仓前缀、`index.RelationsAtContext` 限定端点与被查仓相同。REFACTOR-OBJECT-MODEL-01 阶段 3 改为端点只按对象 ID 引用、关系在请求范围内跨仓查询（`--object metric/gmv`），场景步骤已同步；在最终验收的真实部署走查（`make deploy-local-scenes`）中复核，未复核前不视为关闭。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
