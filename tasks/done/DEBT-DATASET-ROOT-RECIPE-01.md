# DEBT-DATASET-ROOT-RECIPE-01 · 根映射发布的配方写回避免空指针

## 债

- 位置：`server/snapshot_service.go` 构造配方写回 Invocation；`invocation/verbs_dataset.go` 的 `AuthorizeRoutedWrite`。
- 信号：2026-10-15 Snapshot/Object 零知识试跑中，合法 `dataset define` 根映射两次返回 HTTP 502；operator 在独立世界重复请求得到 EOF，服务日志均为 `cx.Observation.Authorization` 空指针。
- 判定：根映射触发向来源仓保存配方时，适配层没有提供下游依赖的 Observation。与 `_schemas` 目录无关，包含它的非根映射发布已成功。
- 处置：修正同一写回应用路径的上下文组合，补正式 CLI/HTTP 根映射发布和授权回归，不绕过 Writer 或授权检查。
- 触发条件：扩大 Snapshot/Dataset 零知识发布通过范围或下次修改配方写回前。

## 目标

合法根映射的配方写回和 Dataset 发布能得到真实结果，缺权与暂时故障分别返回合同错误，不产生 panic。

## 依据

- [Dataset](../../doc/design/dataset.md)、[服务](../../doc/design/service.md)、[权限](../../doc/design/permissions.md)。
- [测试与验证](../../doc/engineering/testing.md)。

## 验收

- 按正式 Server 路线发布 `--source 'sales-semantic=refs/heads/main@'`，获得合法回执并能回读冻结内容。
- 覆盖配方写回授权、响应丢失及失败不公开半成品，原有权限守卫不削弱。
- 相关回归、`make check-docs` 与 `make quality` 通过。

## 前置

`ZK-TRIAL-01` 已保存 Agent 原始失败、operator 重现和服务栈证据。

## 不做

不涉及索引、LiveAccess，不改变 Dataset 来源或对象身份合同，不以改布局规避原请求。

## 进展（2026-10-15）

- 本轮目标：正式 Server 的根映射配方写回继承当前请求的观察依赖；保留 Dataset 管理、来源发布许可与来源写入三种独立授权，来源写入失败时不接受候选发布。
- 实际依据：Dataset 的“发布”与 U2/U5/U7、服务的“一次请求”“跨模块任务”与 U1/U4、权限的“三种资源”“发布不发权”；不改设计。
- 基线证据：`.validation/runs/20261015T120352Z-dataset-root/baseline/` 保存开工状态、既有 Server diff 与原文件。`server/snapshot_service.go` 已有他人在途修改，先不改它；新增 `cli/dataset_root_recipe_test.go` 通过真实 Server 与进程内 lakeFS 夹具复现根映射。

## 定向验证与等待

- 失败证据：`baseline/root-regression.log` 记录正式 CLI 根映射、HTTP 缺来源写权两条路径在 `AuthorizeRoutedWrite` 解引用空 Observation 后 panic/EOF；缺 Dataset 管理或来源发布许可仍按合同拒绝。`baseline/root-server-regression.log` 记录 Server 故障回归同样在授权处先 panic。
- 拟议生产修复仅一处：`server/snapshot_service.go` 的 `BeforePublish` 为写回 Invocation 注入当前 context/runtime/operation 的 `invoke.NewObservationTelemetry`；精确补丁在 `observation-wiring.patch`。不以空观察替身掩盖接线缺失，不改授权规则。
- 新增 `cli/dataset_root_recipe_test.go`：正式 `dataset define --source ...=refs/heads/main@` 的回执、冻结配方回读、来源推进后旧内容不漂移；Dataset 管理、来源发布许可、可选的来源写权三者分别检查。
- 新增 `server/dataset_root_recipe_test.go`：来源写入拒绝、写入完成但应答丢失、后续准备故障分别返回 `TEMPORARY_UNAVAILABLE`；旧发布与登记 HEAD 保持不变，已完成的独立配方写入不回滚，重试复用同一源 commit；独立断言写回授权进入当前请求的真实遥测，防止退化为空 Observation。
- **补丁预演通过，尚非工作区验收**：`go test -overlay .validation/runs/20261015T120352Z-dataset-root/proposed/overlay.json ./server ./cli -run '^TestDatasetRootRecipe' -count=1`，证据为 `proposed/root-regression.log`。overlay 只替换证据目录中的 Server 副本，受保护的工作区生产文件未修改。
- 当前等待归属：`server/snapshot_service.go` 同时属于 `AUTH-LAYOUT-01` 的在途范围，须等该任务进入 done 或按停滞接管规则完成后再应用补丁。之后还需实际工作区定向验证与统一 `make check-docs`、`make quality`；目前不声称验收通过，不移入 done。未使用真实测试部署或共享 OpenSearch，未提交、未安装依赖。

## 实现与定向验证（2026-10-15 21:34）

- 解除等待依据：`tasks/done/AUTH-LAYOUT-01.md` 与 `tasks/done/GITEA-LOGIN-RETIRE-01.md` 均已存在，认证交付解除 `server/snapshot_service.go` 的修改归属限制。
- **实现完成**：对当前服务源码再次核对并保存原文/diff后，原 `observation-wiring.patch` 通过 `git apply --check`；只在根映射配方写回 Invocation 增加真实 `invoke.NewObservationTelemetry`，保留当前 context、runtime、operation 和 provider。其它在途修改原样保留，未改 INDEX 源码或任务。
- **工作区定向验证通过**：`go test -json ./server ./cli -run '^TestDatasetRootRecipe' -count=1`，10 个通过、0 失败、0 skip。覆盖正式 CLI 根映射回执及冻结读取、独立管理/来源发布/来源写入权限、写入失败/响应丢失/准备失败不公开半成品、重试与真实请求授权遥测。
- **相关合同通过**：对 Server、CLI、Snapshot HTTP、Invocation 的 Dataset 发布与管理钩子、来源发布授权、临时配方消费/重放、非部署配方拒绝执行选定组，80 个通过、0 失败、0 skip；包含上述 10 项，不能相加为独立覆盖。精确选择式在 `related-contracts-report.json`。
- 新证据目录：`.validation/runs/20261015T133154Z-dataset-root-workspace/`；`root-regression.jsonl`、`root-regression-report.json`、`related-contracts.jsonl`、`related-contracts-report.json` 保存实际工作区结果；`workspace-observation-only.diff` 证明本次相对解除保护时源码只有必要一行。原红证据与补丁预演保留在先前目录，预演不充作本次验收。
- 两次测试期间 `server/snapshot_service.go` 及两份根映射回归源码前后 SHA256 一致；其它并行线仍有改动，未声称全仓源码稳定。未观察到本生产文件出现非本轮新增修改。
- **验收尚未完成，保留 doing**：尚需稳定窗口统一执行 `make check-docs`、`make quality` 及整体测试；此前全量门禁受已暂停的 `INDEX-DECLARATION-01` 在途状态阻挡，不由本任务改动它来换绿。本次未跑完整架构组、`make test` 或真实部署验收，未使用本机测试部署/共享 OpenSearch，未提交或安装依赖。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；根映射写回及拒绝路径专项保留；Dataset迁移失败按主线报告归属，仍待同源完整门禁，继续 doing。

## 交付

2026-10-16 按本条自身验收收口：根映射配方写回 Invocation 已注入当前请求的真实 Observation，保留 Dataset 管理、来源发布许可与来源写入许可的独立检查。合法根映射得到回执并可按冻结版本回读；写入失败、应答丢失或准备失败不公开半成品，不回滚已成立的独立来源提交。

已验证：实际工作区根映射回归（`.validation/runs/20261015T133154Z-dataset-root-workspace/root-regression-report.json`） 10 pass / 0 fail / 0 skip；相关合同（`.validation/runs/20261015T133154Z-dataset-root-workspace/related-contracts-report.json`） 80 pass / 0 fail / 0 skip，范围有重叠，不累计。374 来源合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`） 再次记录根映射 10 个通过事件，未 skip。后续共享 Server 文件仅追加拒空身份守卫，原 Observation 接线和两份根映射回归不变；新增入口风险由 b16 拒空身份与正常授权回归（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-a5016df50e0b/report.json`） 覆盖。b16 quality（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`） 含 check-docs 稳定通过。

本条自身要求正式 Server 路线和相关合同，未把真实 lakeFS 部署列为完成条件；正式 HTTP 与进程内 lakeFS 夹具结果不写成真实部署验收。不同来源的旧合同与新增守卫证据分别保留，不宣称当前候选完整入口全绿，不等待无关 live 用例。未提交、未安装依赖；目录由统一收口处理。
