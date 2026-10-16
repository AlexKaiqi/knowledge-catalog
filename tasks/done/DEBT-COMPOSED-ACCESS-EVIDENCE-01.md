# DEBT-COMPOSED-ACCESS-EVIDENCE-01 · 多仓静态对象访问账保留每个来源版本

## 债

- 位置：`object/holding.go` 的 `ComposeHoldingValues`、`invocation/observability_access.go` 的 `ObjectAccesses`、`invocation/verbs_read.go` 的多仓读取。
- 信号：2026-10-15 审计核对中，以公开组合函数生成两个仓持有不同方面的对象；响应的 sources 正确包含两个 repository/commit，传入公开访问证据提取函数后得到空数组。相同单仓对象可正常提取一条固定坐标。
- 判定：多仓组合结果用来源版本向量表达依据，而访问账仍依赖顶层单一 repository/commit；缺少与组合形状对应的提取，无法据该账还原成功静态读取采用了哪些来源版本。
- 处置：沿既有访问账保存每个实际采用来源的固定坐标与适用位置，增加正式 Server 读取后查询访问账的回归；不从业务正文推断证据，不建立第二份审计账。
- 触发条件：承诺多仓对象的访问可复核，或下次修改对象组合/访问证据提取前。

## 目标

审计能够回答一次多仓静态对象读取实际采用的各来源仓、commit、对象和所选范围，未采用或无权来源不会冒充成功访问。

## 依据

- [对象](../../doc/design/object.md)、[服务](../../doc/design/service.md)、[权限](../../doc/design/permissions.md)。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 复现与边界

- 两个 `ObjectValue` 使用同一对象 ID：仓 a 的 commit-a 提供 definition，仓 b 的 commit-b 提供 health 静态值。
- `ComposeHoldingValues` 的 sources 有两条正确坐标；`invocation.ObjectAccesses(composed)` 返回空数组；`ObjectAccesses(a)` 返回仓 a、commit-a 与对象 ID。
- 公开函数复现程序保存在本机 `/tmp/kc-audit-multisource-20261015.go`；上述输入与输出为可重建的最小证据，不依赖该临时文件保留。
- 初次审计证明到组合结果与访问账提取边界；修复轮新增正式 Server 旅程反例，红绿证据见下方交付。
- 不把它写成外部成功结果静默丢失：当前 `composeObserved` 遇到跨仓外部观察或失败会拒绝根读取，要求按定位符读取；单仓 LiveAccess 原结果审计已有独立测试。
- 原始文件下载漏账另由 `DEBT-FILE-ACCESS-EVIDENCE-01` 承接。

## 验收

- 正式 Server 经同对象的两仓不同方面读取后，按 request-id 查询审计，精确核对两个实际来源 commit 与对象范围。
- 来源随后更新，旧访问账仍指向原固定版本；按旧依据回读得到原内容。
- 定位符只选择一仓方面时不记录未访问的另一仓；无权来源不计为成功采用。
- 业务正文中的伪 repository/commit/objectId 不形成证据；原有单仓、来源冲突与权限断言保留。
- 受影响测试、`make check-docs`、`make quality` 通过；定向结果与真实部署验收分别报告。

## 前置

无。

## 不做

不改变对象身份、组合规则、跨仓动态读取边界和审计权限；不处理原始文件下载的访问证据。

## 交付

- 实现：`ObjectAccesses` 将组合结果的来源版本向量转换为既有访问目标，每个采用的来源方面保留仓、commit、对象、原请求引用/定位符和方面范围。沿既有去重路径追加，不把未授权来源、正文或 provenance 当作成功访问依据；没有增加公开字段。
- 正式回归：`server/TestHTTPComposedReadAuditKeepsEachAdoptedSource` 经 Object SDK、正式 HTTP 路由和认证/追踪路径读取，再经审计 SDK 分别按 request-id 与 trace-id 查询；验证两个采用来源、第三仓 withheld、来源推进后账不变且按账中旧版本回读、单仓深定位符、撤权后不记未采用来源，以及正文伪坐标隔离。仓为进程内内存夹具，不是实际 lakeFS 部署验收。
- 提取回归：`invocation/TestComposedAccessExtractionUsesSourceVector` 验证公开组合结果、重复信封去重及成员集合按采用的方面范围记账，不因成员数量膨胀。
- 先红：上述正式回归在修复前失败，`composed audit must name both adopted sources, got []observability.ObjectAccess(nil)`；日志 `/tmp/kc-composed-access-red-20261015.log`，退出 1。
- 定向绿：新增两条与既有正文隔离、Search 回读、Interface 观察证据测试通过；日志 `/tmp/kc-composed-access-green-20261015.log`，退出 0。完整门禁由本轮根任务统一执行，本任务暂留 doing；未提交。
- 根任务复核：五包全量合同 `.validation/runs/20261015T100126Z-83b796ea0b49/report.json`（388 个通过事件）及文件/对象/外部审计 race 回归 `.validation/runs/20261015T100054Z-afa2ca9f1cc4/report.json`（38 个通过事件）均通过、无 fail/skip、源码稳定。`make test` 的 lakeFS 夹具场景通过，记录 `.validation/runs/20261015T095924Z-6b1cc50455d7/report.json`。
- 完整架构组另有试跑材料术语失败，证据 `.validation/runs/20261015T095615Z-edcb6963017a/report.json`，由 `DEBT-TERMINOLOGY-TRIAL-01` 承接；不放宽守卫，也不将专项修复写成全仓验收。本条因此暂留 doing。
- 最终 `make quality`（含 `make check-docs`）通过，记录 `.validation/runs/20261015T100249Z-6c6b69d0af08/report.json`，源码稳定；未降低任何门禁阈值。
- 已验证范围：`go test ./invocation ./observability ./object ./object/reader ./object/service -count=1` 五包全量通过（`/tmp/kc-composed-access-contracts-20261015.log`）；新增 Server 旅程与 `TestHTTPCachedReadRechecksRevocationAndKeepsOldBasis` 在 `-race -count=1` 下通过（`/tmp/kc-composed-access-race-20261015.log`）；`make check-docs` 通过。以上均无 skip；未运行真实部署验收。
- 自检：只消费已有来源元数据、复用既有访问目标与去重规则；单仓/定位符仍走原路径，不新增公开形状或绕过授权，不改变外部观察读取边界。本轮增量未发现需要另登记的问题。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；各实际来源版本入账的专项证据保留；v3完整合同仍有主线红、e2e未完成，继续 doing。

## 交付

2026-10-16 按本条自身验收收口：组合对象的实际来源版本向量已进入既有访问账，每个采用来源保留仓、commit、对象与方面范围。未采用或无权来源、正文伪坐标不形成成功访问证据；单仓及定位符路径沿用原合同，没有新账或公开字段。

已验证：五包合同（`.validation/runs/20261015T100126Z-83b796ea0b49/report.json`） 388 个通过事件，以及 文件、组合对象和外部访问 race 回归（`.validation/runs/20261015T100054Z-afa2ca9f1cc4/report.json`） 38 个通过事件，均 0 fail / 0 skip、源码稳定；范围有重叠，不累计。374 来源合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`） 中提取回归和正式 HTTP 后查账旅程均通过，覆盖各来源旧版本回读、撤权、单仓定位和正文隔离。复核直接相关的五个实现及回归文件与该来源字节一致。b16 quality（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`） 包含 check-docs，稳定通过。

自身合同已由相关字节及实际测试证明，宽报告中其它 live skip 不计作本条通过或阻碍，也不据多个来源宣称新候选全量绿。真实 lakeFS 部署与跨仓动态根读取不在本条交付范围，部署结果仍单列。未提交、未安装依赖；目录由统一收口处理。
