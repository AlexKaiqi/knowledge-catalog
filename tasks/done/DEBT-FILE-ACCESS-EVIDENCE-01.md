# DEBT-FILE-ACCESS-EVIDENCE-01 · 接通文件读取访问证据

## 债

- 位置：`server/dataset_file_service.go` 的 `snapshotRecordFileRead` 与 `invocation/observability_access.go` 的 `objectAccessCommand`。
- 信号：文件服务调用 `RecordRead` 后，宿主传入 `file-read`，但访问账入口的命令白名单不包含它，记录函数直接成功返回。
- 判定：既有 Dataset 文件读取的证据接线缺口，同一路径也影响新增 Repo 文件读取；不影响实际字节读取与逐请求授权。
- 处置：单独接通文件访问记录，明确成功、失败和授权拒绝的记录边界，并经真实 Server 读取访问账验证；不靠回调被调用证明落账。
- 触发条件：声明文件下载具有完整访问审计能力，或下次修改文件访问账/证据管线前。

## 目标

文件访问的实际来源、固定版本、调用身份与结果进入既有访问账；Repo 读取不伪造 Dataset pin。

## 依据

- [服务](../../doc/design/service.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 真实 Server 的 Repo 与 Dataset 文件读取后，查询访问账可核对身份、路径、来源版本及结果。
- 授权失败、源文件错误与块读取按约定记录，不泄露凭据，不重复写入。
- 受影响回归与 `make check-docs`、`make quality` 通过。

## 前置

无。

## 不做

不改变文件读取权限、对象声明或存储接口，不建立第二份访问账。

## 交付

- 接通 `file-read` 访问账，文件网关在响应前统一持久化一次最终结果。前缀与单文件映射保留真实仓路径、repository、commit；Dataset 只保留服务端接受的 pin。分块每请求一条，源错误与拒绝沿用原错误，留证失败不能交付成功字节。
- 语义合成文件保留已确定的来源快照，不虚构物理仓路径；`OpenView` 使用请求内记录回调。未获准或伪造 pin 不被解析成可信来源；原仓固定 commit 授权的命中规则也保存到审计。
- 先红：`.validation/runs/20261015T094818Z-b9f3ce22f0fa/report.json` 中正式 HTTP 读取后三种文件地址均无审计，证据存储故障仍返回 200。补充固定 commit 授权红例在 `/tmp/kc-file-access-rule-ref-red-20261015.jsonl`，读取成功但 RuleID 丢失。
- 修复后：`server/file_access_evidence_test.go`、`server/file_access_failure_evidence_test.go` 经正式 HTTP 读取再查询实际访问账，覆盖 trace/request 过滤、实际路径、分块去重、拒绝、缺失版本/文件、投影快照与证据故障。含既有文件网关、多仓对象、外部值回归的 race 组全部通过：`.validation/runs/20261015T100054Z-afa2ca9f1cc4/report.json`，38 个通过事件，0 fail/skip，运行期间源码未变化。
- 扩展合同：文件服务/SDK/CLI 与 Server 相关用例 307 个通过事件，记录 `.validation/runs/20261015T095615Z-708dea5012e0/report.json`；invocation、observability、object、object/reader、object/service 五包全量 388 个通过事件，记录 `.validation/runs/20261015T100126Z-83b796ea0b49/report.json`。上述两组均无 fail/skip，源码稳定。
- `make test` 在独立临时 OpenSearch 上通过：`.validation/runs/20261015T095924Z-6b1cc50455d7/report.json`，20 个通过事件，源码稳定；这是 lakeFS 进程内场景夹具，不是真实部署验收。首次复用共享搜索实例的容量失败登记为 `DEBT-SHARED-SEARCH-CLEANUP-01`，未删除共享数据。
- 外部重构不在本条实现范围。保留 `liveaccess/plugin/server_audit_correlation_test.go`：同一 trace 两次不同值与另一 trace，源离线且 Server 重启后按原 request/trace 查回各自值和观察、不重新取源；当前 race 回归通过，重构完成后的整体验收由用户确认。
- 质量自检：沿既有访问账与授权管线，无新 HTTP 字段或第二份账；同步记账避免 panic 时伪记成功，拒绝路径不为审计额外探查来源。未提交。
- 最终 `make quality`（含 `make check-docs`）通过，记录 `.validation/runs/20261015T100249Z-6c6b69d0af08/report.json`，源码稳定；未降低任何门禁阈值。

## 验证边界

- 完整架构组仍有试跑材料的术语守卫失败：`.validation/runs/20261015T095615Z-edcb6963017a/report.json`，由 `DEBT-TERMINOLOGY-TRIAL-01` 承接，未删除或放宽断言；该组运行期间还有外部改动，不能记为源码固定的全仓验收。
- 本条专项已修复并回归通过，因完整架构未全绿暂留 doing；真实 lakeFS 部署与外部重构整体验收未执行。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；Repo／Dataset及失败路径的文件访问账专项保留；完整入口仍按来源分列，继续 doing。

## 交付

2026-10-16 按本条自身验收收口：文件网关已在成功字节交付前可靠保存一次最终访问结果，Repo 保留实际仓、路径和固定 commit，Dataset 仅采用服务端接受的 pin。分块按请求留证，授权拒绝与来源错误保留原结果，证据写入失败阻止成功交付；语义视图保留真实来源快照，不虚构物理文件。

已验证：正式 HTTP 文件访问账及相关 race 回归（`.validation/runs/20261015T100054Z-afa2ca9f1cc4/report.json`） 38 个通过事件、0 fail / 0 skip、源码稳定；文件服务、SDK、CLI 与 Server 扩展合同（`.validation/runs/20261015T095615Z-708dea5012e0/report.json`） 307 个通过事件、0 fail / 0 skip。较后的 374 来源合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`） 中本条六项文件访问账测试及子项共 15 个事件全部通过；直接相关的五个实现及回归文件与该来源字节一致。b16 quality（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`） 包含 check-docs，稳定通过。各组有重叠，不累计。

“真实 Server 读取后查账”的验收由正式 HTTP 路由与实际持久化账查询满足，没有以回调被调用替代落账。该要求不等于真实 lakeFS 部署验收；后者仍单独报告。其它 live skip 不属于本条剩余条件，也不将不同来源拼成当前全仓全绿。未提交、未安装依赖；目录由统一收口处理。
