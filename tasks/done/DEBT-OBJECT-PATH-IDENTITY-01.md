# DEBT-OBJECT-PATH-IDENTITY-01 · 同路径改身份后维护对象定位一致性

## 债

- 位置：`object/writer/treecodec.go` 的对象定位更新、目录发布输入与 `object/reader` 的定位身份校验。
- 信号：2026-10-15 公开示例零知识试跑中，pi 首次遗漏 frontmatter 起始行，形成路径身份；第二次在同路径补正确身份后，169 个定位项中有 87 个旧身份仍指向身份已变化的文件。两个旧身份经同事 Dataset 正式回读均以定位不符拒绝。
- 判定：Reader 没有错误返回新对象，但成功提交后留下失效定位。Agent 已读到显式删除说明却漏做是触发条件；Writer 按新身份加载和更新定位、按路径覆盖旧文件，未保护未选中的旧对象，是独立一致性缺陷，不能归为缺少基础删除帮助。
- 处置：先以同路径两次发布复现，按“未选中的单元与其他对象保持不变”合同拒绝未显式 REMOVE 的路径占用写入，并验证同批 REMOVE 与 PUT 的一致性；补新旧固定提交及 Dataset 回读回归，不静默删除旧身份、不扩大扫描、不放宽 Reader 校验。
- 触发条件：下一次修改目录发布、对象路径占用或定位维护前。

## 目标

目录发布的路径与身份发生变化时，成功提交不得留下指向另一身份的旧定位项，失败应给出可采取行动的依据。

## 依据

- [对象](../../doc/design/object.md)、[Snapshot](../../doc/design/snapshot.md)、[Dataset](../../doc/design/dataset.md)。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 同路径从隐式身份改为显式身份的最小复现先失败，修复后通过；旧固定提交保持原语义。
- 保留 Reader 定位身份校验，覆盖新身份、旧身份及受限 Dataset 的回读。
- 相关合同、`make check-docs`、`make quality` 全绿。

## 前置

`ZK-TRIAL-01` 的 `20261015-public-examples-pi-supplier/operator/business-review.json` 和
`stale-identity-readback.json`。只对两个旧身份做过真实回读，其他 85 个为逐文件静态定位核对。

## 不做

不改对象身份协议，不把模型资料遗漏算作 Writer 错误，不人工修补试跑交付来改写原验收结论。

## 实现与定向交付

目标是成功发布不留下指向另一身份的旧定位项；实际触碰对象设计的身份、Writer 与 U13/U14/U17，Snapshot 的固定提交与身份随内容原子演化，以及 Dataset 的固定来源版本、文件选择范围。未改设计。

先红证据：`.validation/runs/20261015T201430+0800-object-path-identity/minimal-red.log` 中，真实目录两次 Ingest，先按路径产生 `records/orders`，再在 `records/orders.yaml` 改为显式 `table/orders` 的 properties 方面。旧代码返回 APPLIED 并推进 HEAD，确认独立写边界缺陷。

实现仅改开工前干净且无 INDEX 在途改动的 `object/writer/treecodec.go`，新增 `path_occupancy.go` 与 `path_identity_regression_test.go`。文件型 Writer 在写入前读取基点上的精确目标文件；另一身份占用时要求同批显式 REMOVE，失败指明路径、原身份和恢复方式，不自动删旧对象。批内最终的不同维护单元也不能占同一路径。没有新增扫描或路径权威，没有修改 Reader、Schema、既有测试或守卫。

未声明对象身份的原始 Snapshot 文件也是已占用路径：Object Writer 不能从任意字节推导本次有权替换的对象身份，故同样拒绝静默覆盖并提示选择其他路径。已补原始文件反例与换空路径恢复，验证原字节保留；合法同身份目录更新和路径迁移继续通过。

已验证：最小复现由红转绿；失败不推进 HEAD、无已接受回执；同批 REMOVE/PUT 两种顺序；旧/新固定提交中的旧/新身份、内容与定位；只选单文件并更改交付路径的 Dataset 回读；私有邻居不被补读且未选对象保持；同对象不同方面/不同对象在已有或同批中的同路径冲突；原有单对象读取成本不随仓规模增长。

新增 lakeFS 适配器进程内夹具实际验证了缺路径 `OBJECT_REF_UNRESOLVED`、空路径创建、同身份更新和异身份占用拒绝；与普通内存夹具缺路径合同一致。缺 commit 仍是 `VERSION_UNRESOLVED`，其它读取错误不被当作空路径。未使用真实部署或共享 OpenSearch。

最终受影响包 `go test ./object/writer ./object/format ./object/reader -count=1 -v` 全绿，168 项顶层测试，无 skip；运行前后全仓 Go 源码、go.mod/go.sum 内容 hash 没有变化。证据在同目录 `final-affected-packages.log`、`final-affected-packages-summary.json` 与 `source-before/after-final-packages.json`；更早日志单列每轮回归。回归初稿的 ReadAddress 参数顺序编译错误另存，未当作产品失败证据。

并发核对：原任务前文 20:05:38 的修订早于本条 20:14:30 开工，`task-before.md` 与本次追加前内容完全一致；不属于开工后冲突。`concurrent-task-observation.json` 保存事实，三份源码也未出现外部变更。无需接管，未接管他人任务。

自检：固定路径检查复用现有 TreeReader 和对象地址合同，不扩张公开接口，不重复或放宽对象代数与 Reader 校验；未发现新债。任务保留 doing，等待统一的 `make check-docs`、`make quality` 及完整相关门禁；这些定向结果不替代 `make test` 或真实部署验收。未提交。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

独立增量评审未发现可重现缺陷；额外覆盖整对象与精确成员删除释放、兄弟成员不释放目标、精确删除保留邻居、同身份迁移及stale basis拒绝，8项定向通过、无skip。被评审源码前后相同，证据 `.validation/runs/20261015T201430+0800-object-path-identity/review/`。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

## 本轮收尾（2026-10-16）

复核现有路径占用检查与对象 U13/U14/U17、Snapshot 固定提交、Dataset 文件选择合同，未发现生产实现缺口；保持精确点查与显式 REMOVE 边界，不扩大扫描、公开接口或 Reader 宽容度。

此前独立评审的删除粒度验证只保存在专项证据目录，本轮将其固化为维护中的 `TestPathReuseRequiresRemovalOfItsOwningMember`：根删除释放该对象全部成员路径，精确成员删除只释放自身，删除兄弟成员不得释放目标；每种情况均覆盖 PUT/REMOVE 两种顺序，并复核旧固定提交、邻居内容及失败不推进版本。仅新增回归测试，未改生产代码。

已验证：`go test ./object/writer ./object/format ./object/reader -count=1 -json`，182 项顶层测试通过，0 fail、0 skip，三个包全部通过。证据为 完整日志（`.validation/path-identity-final-20261016.jsonl`）、摘要（`.validation/path-identity-final-20261016-summary.json`） 与 删除粒度回归（`.validation/path-identity-removal-boundaries-20261016.jsonl`）。本轮未启动服务、安装依赖或提交；等待三条对象发布债的统一门禁后再迁移状态。

## 本轮统一复核（2026-10-16）

本轮相关固定副本中，`./object/...`、`./systemrepo`、`./snapshot/dataset` 完整包合计 873 个测试事件（含子例）通过，公开发布与身份 CLI 专项 134 个测试事件通过，均 0 fail、0 skip。15 个相关实现、帮助、出版物和测试文件与实际工作区逐文件摘要一致。历史陌生 Agent 发布与概念会话证据继续保留；本轮没有新增模型试跑，也未声称完整业务交付或真实部署验收通过。

本轮门禁摘要（`.validation/runs/20261016-object-self-publication/summary.json`） 保留固定源码指纹、逐项报告、失败名与范围。质量门禁通过格式、tidy、vet、staticcheck、复杂度、体积及克隆检查，停在 `discovery/space` 尚未登记耦合基线；固定架构组 664 pass / 3 fail / 0 skip，扩大检查的 discovery/service 为 35 pass / 7 fail / 0 skip，均归在途 `INDEX-SEARCH-SPACE-01` 的检索迁移与守卫收口，已登记回该任务。未改阈值、基线、断言或用 skip 换绿。依完整门禁要求，本任务继续 doing，未提交；相关专项通过不替代 `make test`、`make test-contracts` 或真实部署入口。

### 最新门禁结果

上述固定副本保留原始失败；随后索引主线显式收紧耦合基线后，本轮重新执行 `make quality` 已通过，报告 `exitCode=0`、`sourceChanged=false`，`make check-docs` 也通过。同期完整 discovery/service 复查仍为同 7 项失败，源码稳定。实现与相关专项验证已齐，但统一架构与索引合同尚未全绿，故继续保留 doing；最新报告均在上述摘要的 `latestRechecks` 中，未把主线的在途验收记为本任务全量通过。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；bf99未包含随后补强的删除粒度回归，保留其独立固定源码证据；完整同源门禁尚未齐备，继续 doing。

## 验收边界收口（2026-10-16）

本轮重核三项原有验收条件，以公开输入可执行、公开身份一致、Writer 路径与固定版本一致性，以及相关合同、文档和质量门禁收口。此前追加的全仓混合门禁、索引主线与真实部署结果继续由统一验收入口单独登记；不再通过跨任务消息或资源交接把它们作为这三项的附加阻塞。相关断言未删减，没有用 skip 或归因豁免换取通过；专项通过也不写成完整 `make test`、`make test-contracts` 或真实部署验收通过。历史失败与中断证据保留。

## 交付

Writer 在目标路径覆盖前精确核对原持有身份，只有本批显式移除其所属根或精确成员才释放路径；PUT/REMOVE 顺序不改变结果。根删除、成员删除和兄弟删除边界、旧固定 commit、邻居内容、失败不推进版本及 Dataset 文件选择回归均保留并通过。实现仍使用精确点查，不新增全仓扫描、公开接口或宽容读取。

最终固定源码 `374f90a47f43c64cbf5c95547ca40ed5f27d7e64a236e6f0e4d03075295a3b4b` 上，`./object/...`、`./systemrepo`、`./snapshot/dataset` 完整包 880 个测试事件通过，公开发布/身份 CLI 合同 207 个通过，完整架构守卫 676 个通过，受影响服务合同 42 个及索引一致性回归 11 个通过，合计 1,816 pass、0 fail、0 skip。`make quality`（含 `make check-docs`）通过；六次检查均 `sourceChanged=false`，测试结束时与实际工作区全量逐文件摘要一致。逐项命令、报告、原始失败及范围见验收汇总（`.validation/runs/20261016-object-publication-closeout/summary.json`）。

历史陌生 Agent 发布与概念核对证据继续保留；本轮没有新增零知识 Agent 或付费模型试跑，没有扩大 help 旅程。三项按上述原有验收条件完成；全仓混合与真实部署不在本节通过结论内。未安装依赖、未提交。
