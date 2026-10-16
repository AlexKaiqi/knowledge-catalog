# DEBT-OBJECT-IDENTITY-PUBLICATION-01 · 系统仓对象身份说明与公开行为一致

## 债

- 位置：`systemrepo/content/concepts/object/definition.md`、`concepts/address/definition.md`、`concepts/repository/definition.md`。
- 信号：2026-10-15 零知识概念判断试跑中，Agent 同时读到“身份由 Repository 与 object_id 确定、URN 编码来源仓”与根帮助“不含 Repository、跨仓同 ID 合并方面”的冲突说法。
- 判定：Object 系统出版物的身份说明仍有旧语义，公开说明相互矛盾。Agent 本次按当前调用合同答对，不代表此冲突无影响。
- 处置：按对象设计、公开 URN 与 Reader 合同同步系统出版物，并把此类跨公开入口的矛盾纳入守卫。
- 触发条件：扩大 Object 零知识可用性结论或下次发布系统出版物前。

## 目标

陌生使用者从系统仓与帮助得到一致的对象身份、仓边界和统一引用解释。

## 依据

- [对象](../../doc/design/object.md)、[Affordance](../../doc/design/affordance.md)、[核心概念](../../doc/concepts.md)。
- [测试与验证](../../doc/engineering/testing.md)。

## 验收

- 系统仓身份与 URN 说明符合当前公开合同，相关出版物与身份守卫通过。
- 新会话概念判断不再读到互斥的身份规则，引用可复核。
- `make check-docs` 与 `make quality` 通过。

## 前置

`ZK-TRIAL-01` 的首轮执行记录。

## 2026-10-15 信息充分性复核与修复

用户明确：公开信息足够时，Agent 未采用信息不作为产品改造依据。本轮重复 Schema 时，Agent 已实际
读到根帮助的冲突规则和叶帮助的子目录、单文件选择；不能把该失误归因为缺少规则。
但上述三篇公开概念仍与根帮助、URN 和 holding 合同矛盾，因此在 `ZK-TRIAL-01` 范围内修正：
身份仅含 object_id，来源仓与版本属于读取依据，跨仓不相交方面可组合，同方面重复仍冲突。

`systemrepo/identity_publication_test.go` 对实际出版物增加旧错误叙述守卫，并验证公开示例 URN
换仓后保持不变。先红后绿，`go test ./systemrepo` 全包和 Object 引用/holding 合同通过。
未改身份协议，也没有为模型漏读增加一套强制操作流程。

证据为 `/Users/kaiqidong/kc-zk-runs/20261015-public-examples-pi-supplier/operator/information-*`。
`make check-docs` 通过；本次最终 `make quality` 在并行索引迁移的 `object/reader/schema_test.go`
未使用 hasHints 辅助函数处失败，未调整门禁。新模型概念会话未重跑，故保留待验收。

## 不做

不变更对象身份、URN、授权或存储实现，不涉及索引与 LiveAccess 重构。

## 2026-10-15 定向收尾

开工时三篇系统出版物（对象、对象地址、仓）与 `systemrepo/identity_publication_test.go` 已有修复。本轮保留其原状，依据对象设计“身份：URN 不感知 repo”、冲突、对象引用、读取与系统仓，Affordance 的跨公开入口一致性，以及核心概念，补公开响应合同和证据。

原先五条矛盾的失败证据已核实为 `information-identity-red.log`；它发生在既有修复之前。本轮不回退出版物制造失败。已有守卫继续检查全部出版正文并验证 URN 改仓不变，11 项 systemrepo 顶层测试通过。

新增 `cli/system_identity_publication_test.go`：经进程内认证 Server 读取三篇概念，核对不带仓的统一引用、独立的仓与 commit，以及按该 commit 重现同一正文；根帮助和公开出版物里的具体 URN 改来源仓后保持不变；另外五条原始矛盾作为反例仍被拒绝。不是只匹配当前正文句式。两项定向通过、无 skip。合同初稿曾误用内部 `objectRef` 字段，已按公开 CLI `objectId` 合同修正；该测试开发错误单列日志，不算产品先红证据。

证据：`.validation/runs/20261015T200417+0800-system-publication/systemrepo-directed.log`、`cli-identity-public-response.log`、`cli-identity-contract-shape-red.log`、`information-identity-red.log`。`public-identity-responses.json`、`public-root-help.txt` 和三份 `public-*-response.json` 是实际公开输出，保留完整身份与固定版本，供无历史新会话概念判断复核。尚未把合同通过写成新会话通过。

本轮未修改开工已有的出版物或测试；相关内容校验和保持基线。全仓有并行修改，未跑统一门禁，不宣称全仓稳定。任务保留 doing，等待新会话判断与统一 `make check-docs`、`make quality`；不替代真实部署验收。未提交。

## 新会话概念核对与表述收口

无历史新会话仅阅读本轮保存的根帮助与三篇系统概念响应，结论为对象身份、URN、范围与来源依据一致，未见互斥规则；报告见同证据目录 `blind-concept-review.md`。它指出根帮助“跨仓是同一对象的不同 Aspect”半句过满，虽随后已有同范围同方面跨仓冲突说明，单独阅读仍可能被当作无条件判定。

该质量信号由本任务当场修：实际拥有文件 `object/cli/commands.go` 开工前和修改前均干净，已改为同 ID 跨仓仍是同一对象，不同仓可以维护不相交方面；保留已有同方面冲突句，不在 `cli/help.go` 重复一份。补丁保留在 `root-help-precision.patch`。新增公开合同要求根帮助同时交代冲突码、同方面、多个来源仓和读取范围，并用缺上下文反例及等价改写测试守卫本身，不固定当前句式。

CLI 相关帮助及三项公开身份合同、对象 CLI 本地帮助定向通过，无 skip；日志为 `cli-root-help-precision.log`，修正后的实际公开响应另存 `public-identity-responses-refined.json`，保留原 blind 输入不覆盖。该收口不扩张协议，仍等待统一门禁。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

## 2026-10-16 完成性复核

按对象设计的身份、冲突、对象引用、读取和系统仓，Affordance 的分层帮助与同源说明，以及公开 `Reference`、`SelectHoldings` 合同重新复核：对象、地址、仓三篇出版物与根帮助一致；身份只由带命名空间的 `object_id` 确定，仓、Dataset、固定 commit 和版本向量属于范围或依据，同范围同方面跨仓持有明确冲突。本轮没有发现需要继续修改身份协议或说明正文的缺口，保留已有修复。

重新执行 `systemrepo` 与 `object/cli` 全包、Object 引用与 holding 定向、CLI 三项公开身份合同：分别 51、55、8、3 项通过（含子测试），均无失败或 skip。实际公开响应重新留存：三篇概念均经认证 Server 读取，带独立的仓与 commit，按回执 commit 回读正文相同；根帮助和出版物的 URN 示例通过换仓不改引用检查。证据见本轮测试报告（`.validation/runs/20261016-object-identity-completion/test-report.json`）和公开响应（`.validation/runs/20261016-object-identity-completion/public-identity-responses.json`）。

此前无历史概念会话已完成；本轮公开响应合同继续保护其读取材料。上述结果仅为身份说明及其受影响范围的验证，不替代完整合同或真实部署验收。任务状态由本轮统一门禁结果决定；未提交。

## 本轮统一复核（2026-10-16）

本轮相关固定副本中，`./object/...`、`./systemrepo`、`./snapshot/dataset` 完整包合计 873 个测试事件（含子例）通过，公开发布与身份 CLI 专项 134 个测试事件通过，均 0 fail、0 skip。15 个相关实现、帮助、出版物和测试文件与实际工作区逐文件摘要一致。历史陌生 Agent 发布与概念会话证据继续保留；本轮没有新增模型试跑，也未声称完整业务交付或真实部署验收通过。

本轮门禁摘要（`.validation/runs/20261016-object-self-publication/summary.json`） 保留固定源码指纹、逐项报告、失败名与范围。质量门禁通过格式、tidy、vet、staticcheck、复杂度、体积及克隆检查，停在 `discovery/space` 尚未登记耦合基线；固定架构组 664 pass / 3 fail / 0 skip，扩大检查的 discovery/service 为 35 pass / 7 fail / 0 skip，均归在途 `INDEX-SEARCH-SPACE-01` 的检索迁移与守卫收口，已登记回该任务。未改阈值、基线、断言或用 skip 换绿。依完整门禁要求，本任务继续 doing，未提交；相关专项通过不替代 `make test`、`make test-contracts` 或真实部署入口。

### 最新门禁结果

上述固定副本保留原始失败；随后索引主线显式收紧耦合基线后，本轮重新执行 `make quality` 已通过，报告 `exitCode=0`、`sourceChanged=false`，`make check-docs` 也通过。同期完整 discovery/service 复查仍为同 7 项失败，源码稳定。实现与相关专项验证已齐，但统一架构与索引合同尚未全绿，故继续保留 doing；最新报告均在上述摘要的 `latestRechecks` 中，未把主线的在途验收记为本任务全量通过。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；身份出版物、公开响应及新会话证据保留；专项与候选门禁分源记录，完整合同尚未齐备，继续 doing。

## 验收边界收口（2026-10-16）

本轮重核三项原有验收条件，以公开输入可执行、公开身份一致、Writer 路径与固定版本一致性，以及相关合同、文档和质量门禁收口。此前追加的全仓混合门禁、索引主线与真实部署结果继续由统一验收入口单独登记；不再通过跨任务消息或资源交接把它们作为这三项的附加阻塞。相关断言未删减，没有用 skip 或归因豁免换取通过；专项通过也不写成完整 `make test`、`make test-contracts` 或真实部署验收通过。历史失败与中断证据保留。

## 交付

对象、地址、仓三篇系统出版物与根帮助的身份解释已收口。公开响应合同验证不含仓的 URN、独立来源仓和固定 commit，按回执 commit 重现正文，并保留原始矛盾反例和缺少 holding 上下文的反例。既有新会话概念核对证据已复核；本轮没有改身份、URN 或授权协议。

最终固定源码 `374f90a47f43c64cbf5c95547ca40ed5f27d7e64a236e6f0e4d03075295a3b4b` 上，`./object/...`、`./systemrepo`、`./snapshot/dataset` 完整包 880 个测试事件通过，公开发布/身份 CLI 合同 207 个通过，完整架构守卫 676 个通过，受影响服务合同 42 个及索引一致性回归 11 个通过，合计 1,816 pass、0 fail、0 skip。`make quality`（含 `make check-docs`）通过；六次检查均 `sourceChanged=false`，测试结束时与实际工作区全量逐文件摘要一致。逐项命令、报告、原始失败及范围见验收汇总（`.validation/runs/20261016-object-publication-closeout/summary.json`）。

历史陌生 Agent 发布与概念核对证据继续保留；本轮没有新增零知识 Agent 或付费模型试跑，没有扩大 help 旅程。三项按上述原有验收条件完成；全仓混合与真实部署不在本节通过结论内。未安装依赖、未提交。
