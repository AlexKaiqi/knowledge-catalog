# DEBT-SCENE-META-TESTNAMES-01 · 场景元数据引用了不存在的测试名

## 债

- 位置：`.data/scenes/**/_meta.yaml`。
- 信号：仍引用已不存在的测试名，如 `TestCatalogAuditIsGitLog`、`TestRegisterRetireArchive`、`TestRemoteAccessDescribeSendsWorkspaceNotRepository`、`TestHistoricalDatasetPinSearchHasNoIndependentProjection`、`TestDeploymentIdentityMigration…` 和两个 `…OnLiveGitea`。
- 判定：存量漂移，同类信号已出现两次以上。
- 处置：升级为守卫——`_meta` 引用的 Go 测试名必须存在；同时清掉存量。
- 触发条件：`GITEA-LOGIN-RETIRE-01` 删完 Gitea 用例之后、`ARCH-LAYOUT-01` 之前。

## 目标

场景元数据里的每个测试名都能在代码里找到，改名或删除测试时守卫会红。

## 依据

- `.data/scenes/README.md`。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 新守卫在故意写入不存在的测试名时变红；存量全部修正或删除对应引用。
- 场景树三项检查与 `.data/scenes/_tests` 通过。

## 前置

`GITEA-LOGIN-RETIRE-01`。

## 不做

不改测试本身的断言。

## 本轮进展与验证边界

本轮实际依据为场景 README 的风险/Oracle 元数据合同与工程质量的守卫回喂要求；目标是所有引用都真实存在，保留风险归属，不因没有 `verifies` 而绕过检查。

实际扫描 320 个 Go 测试引用，发现 7 个失效引用（5 个不同旧名）及 3 个文件占位引用；已有 `_product_views.py` 仅在条目有 `verifies` 时验证测试名，未覆盖本任务的反例。6 项新增反例对原守卫先红，证据保存于 `.validation/runs/20261015T133726Z-scene-meta-testnames/`。

可审补丁 `scene-meta-guard-and-references.patch` 取消该条件豁免，并按 `risk-mapping.txt` 将 6 份元数据更新到当前等价 Oracle，文件占位改为具体测试，未删除风险或削弱断言。在独立副本中完整 68 项离线测试与三项场景树检查全绿；替代 Oracle 实际定向 16 个通过、无 skip。副本与补丁预演不能记为工作区实现或验收。

现有元数据文件与 `tree.py` 有未释放的他人在途改动；INDEX 接管后原执行方恢复写入，该线已暂停。依后开始任务遇同文件先停的约束，本任务没有修改这些文件，也未只落一半守卫使工作区持续变红。补丁与逐风险映射已完整保存，等待重叠文件归属明确。任务保留 doing。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 恢复推进（2026-10-16）

用户已明确允许在分派范围保留既有未提交改动并追加必要修复。重读场景合同、质量规则，逐项复核
旧 `risk-mapping.txt` 与当前声明/断言后，已将 `_product_views.py` 的具名 Go 检查扩到所有 Go
evidence，不再取决于 `verifies`；六份元数据按当前等价 Oracle 更新，并将文件占位完整展开为
具名测试。旧身份迁移和登记空间归档行为保持退役，标题改为当前真正证明的边界，不虚构历史检索
能力。已有 Gitea 引用删除和 Schema access 退役改动均保留，未借本任务扩修 INDEX 说明。

独立六反例先在当前守卫下全部失败：
`.validation/runs/20261016-scene-meta-testnames/20261016T012146Z-862e043ab154/report.json`。
实际修复后，68 项离线测试全绿、无 skip：`20261016T012206Z-178d66377ca4`；
三项场景树检查全绿：`20261016T012229Z-e7d89690fd83`（views）、
`20261016T012229Z-ea55d20da792`（states）、`20261016T012229Z-85831dd5b9dc`（product）。
当前替代 Oracle 定向共 16 pass / 0 fail / 0 skip：`20261016T012253Z-abafe180bbae`。
以上修复后报告均位于同一证据父目录，源码指纹稳定；Python 执行的用例数见各 `output.log`，
不把 runner 的 Go 事件零计数误报成 Python 未执行。

无需修改 `tree.py`；没有修改 Go 断言。整文件展开仅证明引用有效，未声称展开的所有 Go 用例
或真实 OpenSearch 历史检索已验收。本轮文件已冻结，保留 doing，完整门禁由统一入口执行；
未提交、未安装依赖。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；引用展开及六反例专项证据保留；I-21迁移后的失效引用按主线承接，不将旧源通过当作最新全树通过，继续 doing。

## 交付

2026-10-16 收口：全部 Go evidence 的具名测试引用都经同一守卫检查，不再依赖 `verifies`；失效名称及文件占位
已按等价 Oracle 修正，原风险归属与 Go 断言保留。六项反例覆盖无产品链接、空链接、无产品文档、文件占位、
测试改名和正式检查入口，不以删除引用绕过风险。

已验证范围：68 项离线回归（`.validation/runs/20261016-scene-meta-testnames/20261016T012206Z-178d66377ca4/report.json`）、
views（`.validation/runs/20261016-scene-meta-testnames/20261016T012229Z-e7d89690fd83/report.json`）、
states（`.validation/runs/20261016-scene-meta-testnames/20261016T012229Z-ea55d20da792/report.json`）、
product（`.validation/runs/20261016-scene-meta-testnames/20261016T012229Z-85831dd5b9dc/report.json`）
三项场景检查及16 项替代 Oracle（`.validation/runs/20261016-scene-meta-testnames/20261016T012253Z-abafe180bbae/report.json`）
均通过，源码稳定、无 skip；Python 数量以对应 `output.log` 为准。
后续完整合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`）
的 `TestSceneViews` 再次实际通过三项结构检查。守卫及 18 份场景元数据在来源 `374f90a4…` 与候选 `8667c2f7…`
之间相同；质量门禁（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`）
通过，包含 `make check-docs`。

本任务证明引用真实存在与检查有效，不将列出的全部行为或真实 OpenSearch 历史检索都记作验收通过；其它条件 live
仍按独立入口报告。未修改 Go 断言、操作部署、安装依赖或提交；任务移动及归档后的文档、质量复核统一执行。
