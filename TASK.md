# TASK

执行接力棒，不是设计权威。一次交互只认领一条 `[ ]`。`[x]` 仅当对应契约全绿且无 skip/删断言。

权威映射见 `docs/README.md`（documentation-governance）。设计类文档合同标题由 `make check-docs` 强制。

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
