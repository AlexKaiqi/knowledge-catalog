# SYSTEM-GUIDE-EXECUTABLE-01 · 系统仓短指南从"指路"改为可执行

## 目标

系统仓里的使用指南承担任务语义，读者读完不必再回查 `kc help` 才敢动手。当前九篇指南中，`first-publication`（4.2 KB）与 `model-from-materials`（4.1 KB）达到这个标准，其余五篇操作指南在 812-908 字节之间，正文是五条左右各二十字的要点，例如 `recover-a-write`、`revise-an-aspect`、`review-a-change`、`share-a-dataset`、`connect-source`。

这批短指南在系统仓里各占一个对象身份，却只做到"提示去查 help"，投入产出不成比例，也不足以支撑陌生 agent 只读系统仓完成任务。

## 依据

- [affordance](../../doc/design/affordance.md)：逐步披露；系统仓持有任务语义，完整命令合同由 help 提供。
- 系统仓导读 [readme.md](../../systemrepo/content/readme.md)：指南承担"做具体工作"的入口定位。
- [工程质量闭环](../../doc/engineering/quality.md)：质量问题三个出口——当场修、登记为债、升级为自动检查。

## 验收

逐篇给出该指南是否达到"可执行"的判定与结论，未达标的补齐，达标的不改写。每篇补齐后的正文至少包含：触发场景与前提条件、按顺序的步骤与每步产物、本步失败时的处置、完成判据与常见误判。

- 受影响指南的对象身份、Schema 引用、导航关系保持不变，`go test ./systemrepo/...` 全绿。
- `make check-docs` 通过；导读中的描述与各篇实际内容一致。
- 抽查验证：让不了解本项目的人只读某一篇指南，按文中步骤与产物描述能说出下一步动作和失败处置，不需要先读 `kc help` 才能判断顺序。

## 前置

无。与 `SYSTEM-MODEL-GUIDE-02` 改同一批文件，按 `## 前置` 依次执行，不并行。

## 不做

不复制完整 CLI 参数表或命令输出到系统仓（help 仍是参数权威）；不为凑字数扩写；不改指南之外的 `capabilities/` 用法说明。

## 本轮审阅与实现（2026-10-20）

逐篇按前提、步骤产物、失败处置、完成判据与误判核对九篇指南：

| 指南 | 判定 | 处置 |
|---|---|---|
| first-publication | 已有下载、基点、提交、回执与读回路径 | 保留正文 |
| model-from-materials | 已有判断顺序、固定样例、产物与误判处置 | 保留正文 |
| connect-source | 已有供给/连接、登记、解除、失败与重试路径 | 保留正文 |
| recover-a-write | 原正文未区分回执状态及查不到的含义 | 补原请求保存、APPLIED/PENDING/ABANDONED 分流及固定版本复核 |
| revise-an-aspect | 原正文未说明完整替换输入和条件发布 | 补固定基点、纯值文件、条件 PUT、冲突与其余方面复核 |
| review-a-change | 原正文缺可执行的候选与目标固定路径 | 补分支提交、固定 commit 评审、条件合并及拒绝后的下一步 |
| share-a-dataset | 原正文缺范围选择、实际命令和消费侧验证 | 补目录映射、接受记录、独立授权及消费者同修订读回 |
| first-read | 原正文的失败类别没有对应处置 | 补同修订搜索/读回、未就绪/无权/缺失处理及资源文件读取 |
| use-dynamic-values | 原正文未交代观察失败和保存流程 | 补声明核对、逐值判断、身份隔离及显式持久化路径 |

六篇改动保留 frontmatter、对象身份、Schema 引用与原有能力导航；未修改关系对象或能力说明。
依据为 Affordance、对象、Dataset 与治理设计；未修改设计篇。
当前 Dataset CLI 的显式 revision 与设计目标的服务端分配不同，指南按当前公开 help 说明该差距，
不把未落地的自动分配写成现行用法。

## 待验收

系统仓与 CLI 回归、文档及质量门禁运行后补记结果。陌生读者的独立抽查尚未完成，
本轮作者按每步产物和失败分支复读不冒充该项验收；满足任务全部验收后再移 done。

第二轮 `make quality`（含文档检查）与独立 `make check-docs` 已通过；
证据 `.validation/runs/20261020-task-closeout/`。完整契约发现的提案示例问题已由
`OBJECT-VERSIONING-01` 记录并修正文档表达，未修改任何命令守卫。

系统仓指南的 shell 示例已接入既有 `TestProductDocumentationUsesCurrentCommands`，
未增加独立命令解析或放宽参数检查；九篇共 66 个示例通过当前产品命令边界检查。
新示例中的数值版本采用具体演示值，并在正文要求替换为实际固定修订。

## 已验证范围（2026-10-20）

- 最终 `go test ./object/... ./systemrepo/...`、两组完整契约失败项的定向复验
  `TestProductDocumentationUsesCurrentCommands` 与 `TestSceneViews` 均通过，无 skip。
- 场景 `--check-product`、`--check-views`、`--check-states` 及 `make quality` 通过。
- 冻结改动后重跑 `make test` 通过（CLI 场景 44.392s，退出 0）；先前运行因源码变化无效，不作同候选通过证据。
- 首轮完整 `make test-contracts` 为 5111 pass / 4 fail / 14 skip（计具名子测试，含失败父级）；
  两组失败分别为提案命令示例、Dataset U16/U17 漏登记缺口，均已修复并定向复验。
  该完整运行期间源码变化，未把定向结果与它拼成同候选全绿；14 条外部 live 未验收。
- 独立的全包 CLI 重复运行因与完整契约范围重复而主动终止，以完整契约及最终定向、产品场景入口为证据。
- 日志汇总位于 `.validation/runs/20261020-task-closeout/`；完整契约原始记录为
  `.validation/runs/20261020T081735Z-6ba069d1fab4/`。

独立陌生读者抽查未完成，任务保持 doing；未提交。

## 交付（2026-10-21）

实现范围：九篇指南逐篇判定，六篇补齐（`recover-a-write`、`revise-an-aspect`、`review-a-change`、
`share-a-dataset`、`first-read`、`use-dynamic-values`），三篇达标保留原文。补齐后正文均含触发场景与
前提、按序步骤与每步产物、失败处置、完成判据与常见误判；frontmatter、对象身份、Schema 引用与
能力导航未变。

已验证范围：`go test ./object/... ./systemrepo/...` 全绿（1 skip 为需真实 lakeFS 的上传吞吐）；
九篇 66 个 shell 示例经既有 `TestProductDocumentationUsesCurrentCommands` 检查通过；
场景 `--check-product`、`--check-views`、`--check-states` 与 `make quality`（含 `make check-docs`）通过；
`make test` 0 FAIL / 0 SKIP；当前源码 `make test-contracts` 0 FAIL / 18 SKIP（均为既有环境分级项）。

判定依据为逐篇正文复核：指南体量从 812-908 字节提升到 2385-3762 字节（`model-from-materials`
维持 13676 字节的既有完整正文）。独立陌生读者抽查未执行，属外部验证入口；
本轮作者按每步产物与失败分支复读，不作为该项证据。
