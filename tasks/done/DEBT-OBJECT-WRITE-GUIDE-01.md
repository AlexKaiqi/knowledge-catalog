# DEBT-OBJECT-WRITE-GUIDE-01 · 补齐对象目录发布的自助输入说明

## 债

- 位置：`cli/help_writer.go` 的 `writer commit --dir` / `diff`；`systemrepo/content/capabilities/publish-object/usage.md`。
- 信号：2026-10-15 Snapshot/Object 零知识供给试跑中，Agent 读到这些帮助后仍不知道目录对象的最小文件形状，转而提取二进制中的嵌入材料。
- 判定：目录输入规则与公开示例之间缺少可直接照做的路径。此次探索同时错过了公开的 `writer put --value` 路线，试跑环境也未发布已有的公开示例 Dataset，不能将全部试错归为产品缺能力。
- 处置：补输入合同与公开示例 Dataset 的发现、读取、复用路径；复用既有业务材料，补全 Schema 交付，用包含公开示例的新会话复验。
- 触发条件：继续对象自助发布体验或扩大 Object 零知识可用性结论前。

## 目标

陌生接入方能从公开帮助和系统中公开的示例 Dataset 学到目录发布所需的实际输入，不靠源码、私有材料或二进制提取。示例是合法学习材料；接入方自己的业务事实仍以给定原始资料为准。

## 依据

- [Affordance](../../doc/design/affordance.md)、[对象](../../doc/design/object.md)。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 最小样例通过实际 Writer 校验和回读，公开帮助与所属输入合同一致。
- 陌生 Agent 新会话仅从公开入口完成一次目录发布；原始失败和新证据分别保留。
- 相关回归、`make check-docs` 与 `make quality` 通过。

## 重复信号与自动合同

2026-10-15 使用 pi 1.2.0、Volcengine `glm-5.3` 与 12 份原始资料复测，再次出现二进制探索。
首份 Markdown 声明实际来自公开文件读取；其后 Agent 从二进制得到 Schema 文件路径，再经公开接口
取得原文件，不能把整个过程记为仅靠公开说明。另将 `schema describe` 的展示结构直接用作发布输入时
遭拒，经继续读取原文件修正。证据在
`/Users/kaiqidong/kc-zk-runs/20261015-184057-pi-supplier/operator/public-entrypoint-audit.json`。

同类信号已重复，修复验收必须落实为自动合同：从真实公开帮助或可达系统指南响应中提取最小完整的
Schema 与实例样例，原样经 Writer 发布并按回执 commit 回读；样例缺失或无法执行时失败，禁止用私有
测试材料补齐缺项。这个合同只证明公开输入可执行性，陌生 Agent 能否发现和使用仍须独立新会话证明。
上述复测当时只完成证据登记；后续修复与验收范围见下节，不能以运行器回归通过代替产品合同。

## 2026-10-15 公开用例 Dataset 修复与复验

在 `ZK-TRIAL-01` 中补齐根帮助、发布旅程、目录输入形状和首次发布指南。指南从系统中已有的公开
Dataset 发现和下载 Schema、实例，保留固定 revision，再改为接入方自己的身份和事实。三个用例
Dataset 直接选择原有 Schema 文件，不另造一套业务样例，也不给来源仓读权。

`cli/publication_guide_test.go` 已实现真实指南响应中的公开下载路线、Writer 校验与按回执 commit
回读；公开 Schema 交付、Dataset 选择范围和帮助形状均有回归。准备用例时发现未被 Dataset 选中的
同名持有仍参与对象冲突判定，已修复为按 Dataset 的选中单元规划，保留真实重复持有的冲突断言。

新独立 pi 会话使用 Volcengine `glm-5.3`，74 次工具调用、约 11 分钟，从公开帮助及 `qinghe-analysis`
取得原文件，自主发布并授权 `qinghe-shared`；未提取二进制，也无操作员提示命令或修改其业务产物。
公开入口学习与发布已有实证。完整业务交付仍失败：事件和部分 DDL 信息遗漏、无依据的币种/日期、
双仓同名 Schema 冲突，以及修正后失效定位项；这些事实不能由入口合同通过替代。

证据在 `/Users/kaiqidong/kc-zk-runs/20261015-public-examples-pi-supplier/`。定向合同、25 项运行器回归
及 `make check-docs` 通过；当前完整包测试与 `make quality` 仍有并行索引声明迁移的失败项，故本债
保留待验收，尚不移到 done。

## 前置

`ZK-TRIAL-01` 的首轮执行记录。

## 不做

不要求固定业务对象布局，不扩张 Writer 入口，不以说明替代权限或 Schema 校验。

## 2026-10-15 定向收尾

本轮从已有实现继续，不重做公开示例、指南或业务夹具。目标仍是公开输入可执行且可发现；实际依据为对象设计的 Writer、系统仓与 U8/U9，Affordance 的分层公开入口，以及测试与质量要求，未改设计或既有未提交实现。

已核对并保留原独立 pi 会话的 `public-entrypoint-audit.json`，其中 74 次调用的公开帮助、指南、Dataset 原文件下载、Writer 提交和固定 commit 回读可逐项复核；无二进制提取、运行中操作员提示或改业务产物。它只证明该次公开入口学习与发布，不能证明完整业务交付或任意 Agent 成功率。原失败证据与本次合同结果分开保存。

本轮定向通过：`go test ./systemrepo -count=1 -v`（11 项顶层测试）及五项 CLI 合同：公开指南下载→实际 Writer→回执固定版本回读、三份公开 Dataset 的可复用 Schema 与范围/授权、发布帮助导航、完整 README 目录文件、Schema 展示与发布原文区别。均无 skip，不用真实部署或共享 OpenSearch。

证据：`.validation/runs/20261015T200417+0800-system-publication/systemrepo-directed.log`、`cli-publication-directed.log`、`public-entrypoint-audit.json`、`before-manifest.json`、`directed-summary.json`。相关既有出版物/帮助/测试内容在本次定向期间保持开工基线；全仓有并行修改，不宣称全仓源码稳定。

任务保留 doing，等待源码稳定后的统一 `make check-docs`、`make quality` 与完整相关合同；本轮定向不替代 `make test` 或真实部署验收。未提交。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

## 2026-10-16 原样公开输入合同补强

复核公开帮助、系统指南与原独立试跑证据后，保留已有出版物；本轮只补强
`cli/publication_guide_test.go`。原合同在 Writer 前先修改示例身份和名称，只直接证明适配后的输入。
现分别验证原样复用和自己的业务适配：两条路径各从真实公开指南响应执行发现、下载 Schema 与实例，
通过实际 Writer 发布，再按接受回执的 commit 比较完整实例值及完整 Schema 定义。公开来源使用固定
Dataset revision 3，前后全部读取响应一致，来源仓直接读取仍被拒绝；不从私有夹具补齐发布输入。

本轮 5 项 CLI 顶层合同及其 125 项子用例通过，失败和 skip 均为 0，包括两条发布路径、三份公开
Dataset 的全部可复用 Schema、发布导航、完整目录说明文件、Schema 展示与发布原文区别。
命令、通过清单、源码摘要与原试跑审计摘要见
专项证据（`.validation/runs/20261016-object-write-guide/summary.json`），完整测试事件见
CLI 定向结果（`.validation/runs/20261016-object-write-guide/cli-directed.jsonl`）。

原独立 pi 会话证据仍在 `/Users/kaiqidong/kc-zk-runs/20261015-public-examples-pi-supplier/`，
本轮重新核对审计文件并保留摘要，没有把自动合同当成新陌生 Agent 会话，也不扩大原业务交付结论。
统一门禁与最终状态由本轮汇总验收决定；本节不替代真实部署验收，未提交。

## 本轮统一复核（2026-10-16）

本轮相关固定副本中，`./object/...`、`./systemrepo`、`./snapshot/dataset` 完整包合计 873 个测试事件（含子例）通过，公开发布与身份 CLI 专项 134 个测试事件通过，均 0 fail、0 skip。15 个相关实现、帮助、出版物和测试文件与实际工作区逐文件摘要一致。历史陌生 Agent 发布与概念会话证据继续保留；本轮没有新增模型试跑，也未声称完整业务交付或真实部署验收通过。

本轮门禁摘要（`.validation/runs/20261016-object-self-publication/summary.json`） 保留固定源码指纹、逐项报告、失败名与范围。质量门禁通过格式、tidy、vet、staticcheck、复杂度、体积及克隆检查，停在 `discovery/space` 尚未登记耦合基线；固定架构组 664 pass / 3 fail / 0 skip，扩大检查的 discovery/service 为 35 pass / 7 fail / 0 skip，均归在途 `INDEX-SEARCH-SPACE-01` 的检索迁移与守卫收口，已登记回该任务。未改阈值、基线、断言或用 skip 换绿。依完整门禁要求，本任务继续 doing，未提交；相关专项通过不替代 `make test`、`make test-contracts` 或真实部署入口。

### 最新门禁结果

上述固定副本保留原始失败；随后索引主线显式收紧耦合基线后，本轮重新执行 `make quality` 已通过，报告 `exitCode=0`、`sourceChanged=false`，`make check-docs` 也通过。同期完整 discovery/service 复查仍为同 7 项失败，源码稳定。实现与相关专项验证已齐，但统一架构与索引合同尚未全绿，故继续保留 doing；最新报告均在上述摘要的 `latestRechecks` 中，未把主线的在途验收记为本任务全量通过。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；bf99未包含随后补强的原样公开输入回归，相关专项证据保留；统一同源验收尚未齐备，继续 doing。

## 验收边界收口（2026-10-16）

本轮重核三项原有验收条件，以公开输入可执行、公开身份一致、Writer 路径与固定版本一致性，以及相关合同、文档和质量门禁收口。此前追加的全仓混合门禁、索引主线与真实部署结果继续由统一验收入口单独登记；不再通过跨任务消息或资源交接把它们作为这三项的附加阻塞。相关断言未删减，没有用 skip 或归因豁免换取通过；专项通过也不写成完整 `make test`、`make test-contracts` 或真实部署验收通过。历史失败与中断证据保留。

## 交付

公开目录输入、系统首次发布指南和已有示例 Dataset 的交付路径已收口。自动合同分别从真实公开响应取得原样示例与适配输入，经实际 Writer 发布，再按接受回执 commit 回读完整实例及 Schema；来源固定 revision、直接来源仓读权拒绝和帮助边界继续受守卫保护。保留原实现，本轮仅修正完整门禁暴露的过时示例与验收夹具，没有新增帮助流程。

最终固定源码 `374f90a47f43c64cbf5c95547ca40ed5f27d7e64a236e6f0e4d03075295a3b4b` 上，`./object/...`、`./systemrepo`、`./snapshot/dataset` 完整包 880 个测试事件通过，公开发布/身份 CLI 合同 207 个通过，完整架构守卫 676 个通过，受影响服务合同 42 个及索引一致性回归 11 个通过，合计 1,816 pass、0 fail、0 skip。`make quality`（含 `make check-docs`）通过；六次检查均 `sourceChanged=false`，测试结束时与实际工作区全量逐文件摘要一致。逐项命令、报告、原始失败及范围见验收汇总（`.validation/runs/20261016-object-publication-closeout/summary.json`）。

历史陌生 Agent 发布与概念核对证据继续保留；本轮没有新增零知识 Agent 或付费模型试跑，没有扩大 help 旅程。三项按上述原有验收条件完成；全仓混合与真实部署不在本节通过结论内。未安装依赖、未提交。
