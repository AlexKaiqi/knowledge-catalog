# DEBT-EMPTY-PRINCIPAL-01 · Server 求值入口缺空身份拒绝守卫

## 债

- 位置：`invocation/allow.go` 的 `PrincipalAllowed`（`as == ""` 分支）与 `invocation/dataset_access.go` 的消费求值。
- 信号：身份为空时直接放行。这是本地直连无 `--as` 的 owner 路径，部署 Server 依赖上游认证保证身份非空，求值函数本身没有防线。
- 判定：守卫缺口，不是行为 bug；若将来 Server 入口漏带身份，会静默变成全放行，和“不存在 owner bypass”只隔一层。
- 处置：Server 求值路径上身份为空即拒绝（本地直连除外），必要时在装配层强制；补反例测试。
- 触发条件：触碰认证装配或新增 Server 入口时（`GITEA-LOGIN-RETIRE-01` 会触碰）。原条目 `QUALITY-03`。

## 目标

经 Server 的请求不可能以空身份通过授权求值。

## 依据

- [权限](../../doc/design/permissions.md)。

## 验收

- 反例：构造一个漏带身份的 Server 请求，得到明确拒绝而不是放行。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

无。

## 不做

不改本地直连的语义。

## 进展

AUTH-LAYOUT-01 的交付明确未处理本债。本轮依据权限篇核对 Server 主体与本地直连边界：
只在中央 Server 执行及 Snapshot 资源取得入口拒绝漏带主体，不按 local 动作名字增加放行特例，
不改变本地直接调用 `ExecuteApplicationOperation` 的 owner 语义。

初始归属阻塞：`invocation/allow.go`、`invocation/dataset_access.go` 已有未释放的 Dataset 默认消费改动；
`cli/fixture_home_test.go` 已有未释放的 INDEX/既有测试装配改动；`server/snapshot_service.go` 仍由
在途 `DEBT-DATASET-ROOT-RECIPE-01` 接线。后开始本任务不修改这些重叠文件，也不应用只改中央
typed 入口、却使本地夹具变红的半套生产修复。

已存在防线：独立 Snapshot `Resources.authorize` 在调用应用授权回调前拒绝空 caller。
尚未覆盖：typed Server 执行入口的 flags 漏掉 `as` 会经 `PrincipalOf` 转为 owner；Snapshot
`acquireSnapshotResources` 在上述资源授权防线前，仍可先按空 caller 构造 owner journal 并执行库存求值。
红例与可审补丁仅保存于 `.validation/`，待重叠文件归属释放后再落地；全局门禁由统一入口执行，
本任务保持 doing，不宣称目标或验收已完成。

反例和待归属释放的完整提案见 本轮记录（`.validation/runs/20261015T133603Z-empty-principal-debt/findings.md`）：
修改前红例证明 typed 缺主体执行操作体、Snapshot 库存 acquire 空 caller 返回成功；独立 Snapshot
资源授权的已有拒空守卫保持通过。补丁只在 `.validation`，同时覆盖两个中央入口及本地夹具迁移，
不添加按 `local.*` 名字逃生的放行分支。

overlay 首轮 21 个通过事件、0 fail/skip 且源码指纹稳定；最终增加本地夹具遥测/审计关联检查后
22 个通过事件、0 fail/skip，但检测到运行期间工作区内容变化，报告为 `source-changed`，不算工作区验收。
源码改动未落地，任务仍因上述重叠文件归属阻塞；未运行全局门禁或部署验收，未提交。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 2026-10-16 核对

- 修复仍未落地：`invocation/allow.go` 的 `PrincipalAllowed` 仍在 `as == ""` 时直接返回 true；红例与补丁只在 `.validation/`。
- 原归属阻塞的变化：`CONSUME-BASELINE-01` 已在 `done/`，`DEBT-DATASET-ROOT-RECIPE-01` 的实现已完成、只等统一门禁。
  `invocation/allow.go`、`invocation/dataset_access.go`、`cli/fixture_home_test.go`、`server/snapshot_service.go` 在工作区
  仍有未提交改动；`INDEX-SEARCH-SPACE-01` 声明改动 `invocation/`，落地前先确认它是否仍在改这几个文件。

## MVP 收尾实现（2026-10-16）

- 本轮按用户“优先完成阻碍 MVP 的改动，改完统一验证”的约定接管此前仅保存在 `.validation/` 的完整提案；
  逐份核对当前源码后追加修改，保留现有 Dataset、插件与认证改动，不再受上文的历史归属等待阻挡。
- `InvokeApplicationWithTelemetryAtHome` 在身份归一、读取 Home、授权与留证之前拒绝缺失、空字符串及空白主体，
  返回 `UNAUTHENTICATED`。`acquireSnapshotResources` 在取得 Home、设置 journal 或库存授权之前同样拒绝空白 caller。
- 本地组件夹具改用既有 `ExecuteApplicationOperation`；从原 `_test.go` helper 提取共享的遥测与 EvidenceChain，
  保留本地 owner、request/trace/span 及审计关联。没有修改 `PrincipalAllowed` / `OwnerBypass` 的本地语义，
  没有新增生产入口或按 `local.*` 名字放行。
- 四包增加 `empty_principal_contract_test.go`：typed 库存/对象/文件/写入不执行且不触碰 Home；实名仍需授权；
  Snapshot acquire 不打开 Home、不设 owner journal；独立 Snapshot 原守卫不接受 query 冒充 caller；
  本地直接执行与夹具遥测/审计关联保持。
- 实际触碰设计为[权限](../../doc/design/permissions.md)的 Server 认证及当前授权边界；设计未改。
  `invocation/README.md` 同步说明中央 Server 与本地组件执行入口的身份边界。
- **实现已落地，验证待统一执行**：本轮尚未执行测试、`make check-docs`、`make quality` 或真实部署验收，
  旧 overlay 通过不能替代这次工作区验收。任务保留 doing，未提交、未安装依赖。

## 交付（2026-10-16，doing 统一补验）

- 最终冻结源 `547254bbd50b3062682da5489aa394167f734a9c5b766c6f5bf5bf0b4287ffa5`：中央 typed Server、Snapshot acquire、资源守卫及本地直连/遥测共 7 个顶层回归通过，无 skip；漏带或空白身份在授权和 Home 副作用前拒绝，本地 owner 语义保持。逐项证据见身份守卫记录（`.validation/runs/20261016-doing-validation/empty-principal-proof.json`）。
- 同候选 `make test` 为 18 pass / 0 fail / 0 skip，`make quality`（含 `make check-docs`）通过；完整 `make test-contracts` 三段全部执行，4818 pass / 0 fail / 11 个显式外部 live skip。各入口 `sourceChanged=false`；这些 skip 的实际独立补验与未验范围在本轮汇总（`.validation/runs/20261016-doing-validation/summary.json`）分别列明，不记为本条反例通过。
- 本条目标与指定验收达成，移入 `done/`；未提交、未安装依赖。
