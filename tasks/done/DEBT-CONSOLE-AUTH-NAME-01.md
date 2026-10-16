# DEBT-CONSOLE-AUTH-NAME-01 · 控制台按认证方式名字判断共享登录

## 债

- 位置：`snapshot/console/webui/src/lib/api/session.ts` 的 `restore`（`discovery.mode === 'taihu'`）与 `src/lib/api/sessionProof.ts`（`mode === 'taihu'` / `mode !== 'taihu'`）。
- 信号：2026-10-15 AUTH-LAYOUT-01 把服务端的按名字分支收进 `identity/`，并删除了控制台的 lakefs Cookie 分支；控制台仍只认 `taihu` 这个名字，换一种同样接受 `Authorization` 的认证方式时前端要改。
- 判定：可替换性在前端的缺口，不是行为 bug；当前只有 Taihu 一种可供浏览器使用的认证方式。
- 处置：控制台按配对发现（`localAssertion`、`accepts`、`browserLogin`）判断能否登录与怎样换 token，不按 `mode` 名字；补“非 taihu 名字的验证型配对可登录”的回归用例。
- 触发条件：新增第二种供浏览器使用的认证方式，或改动控制台登录流程时。

## 目标

控制台的共享登录只依赖 `/identity/v1/auth` 的配对形状，不依赖认证方式名字。

## 依据

- [服务](../../doc/design/service.md)：认证可替换，替换认证器不改服务与授权代码。

## 验收

- `session.ts`、`sessionProof.ts` 不再出现认证方式名字比较。
- 现有控制台测试全绿，新增非 taihu 名字的配对用例。
- `make check-docs` 通过。

## 前置

无。

## 不做

不改服务端配对返回，不新增登录方式。

## 本轮实现与验证

目标与实际设计：依服务设计“部署与认证”的可替换要求，控制台按当前 `/identity/v1/auth` 的 `localAssertion`、`accepts`、`browserLogin` 判定是否可登录/续期，不再按认证方式名字分支；不改服务端认证配对或新增登录方式。AUTH-LAYOUT-01 与 GITEA-LOGIN-RETIRE-01 的 done 文件存在后，从其交付的当前前端代码继续，保留原改动。

已实现共用 `sessionPairing.ts`。`mode` 只作为不透明配对坐标防错配；配对能力仅留在内存，持久化 v2 证明不能自行声明当前服务能力。退出清除配对/证明，迟到续期不覆盖新主体或清除新任务。旧测试按 lakeFS 名字拒绝、却同时声明接受 Authorization 的反例，与认证可替换的设计冲突，已改为缺 Authorization 能力的等价拒绝反例；原 no-outbound、localAssertion、同主体、退役证明及迟到响应风险全部保留，无 skip。

质量问题当场修复并各有先红后绿：空续期 token 被错误接受；续期 promise 跨 epoch 复用/旧失败清除新证明；独立复核发现 token 交换 await 后、whoami Authorization 发送前晚查 epoch。最后一项新增两种配对变化回归，真实工作区先 2 红再修复到 13 项定向全绿，断言仅有变更前已发出的 token 请求，绝不发送旧证明到 whoami，当前主体与证明保留。统一检查在交换前、交换返回后发证明前及 whoami 返回后落盘前核对 epoch 与当前配对。

最终前端验证：完整 206 项测试（32 suites）通过，0 fail/pending/todo/skip；typecheck 与既有 lint:console 通过。三次前端全目录 tracked/non-ignored 文件内容指纹前后一致；仅证明前端范围稳定，不代称全仓稳定。原非 Taihu 配对及两个续期缺陷证据在 `.validation/runs/20261015T133648Z-console-auth-pairing/`；独立复核在 `.validation/runs/20261015T135000Z-console-pairing-review/`；最后两条工作区红绿和完整回归、命令与前后指纹在 `.validation/runs/20261015T135317Z-console-pairing-fix/`。

实现完成，整体验收尚未完成：最新统一 make quality（含 check-docs）通过，但完整架构及默认场景仍有其它在途任务失败，依本轮全量门禁规则保留 doing，详见下方统一结果。未运行 build、部署或真实认证服务；未安装依赖、未提交。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本轮未重跑前端专项；固定快照全量门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；本轮未重跑前端专项，既有206项前端通过不替代统一门禁，继续 doing。

## 交付

2026-10-16 按本条自身验收收口：控制台共享登录和续期按当前配对的 localAssertion、accepts 与 browserLogin 判断能力，不按认证器名字分支。mode 仅作为不透明配对坐标防错配；配对变化、退出和迟到响应不发送旧证明或覆盖新主体。未修改服务端发现协议，也未新增登录方式。

已验证：完整控制台结果（`.validation/runs/20261015T135317Z-console-pairing-fix/test-summary.json`） 32 suites、206 项全部通过，0 fail / pending / todo / skip；测试、typecheck 与 lint 汇总（`.validation/runs/20261015T135317Z-console-pairing-fix/checks-summary.json`） 三项 exit 0、前端源码稳定。包含非 taihu 名字的验证型配对与续期竞态反例；本次复核 完整测试报告（`.validation/runs/20261015T135317Z-console-pairing-fix/webui-tests-report.json`） 记录的全部 204 个前端源文件 hash，与当前逐一一致。session.ts、sessionProof.ts 也再次核对，不存在认证方式名字比较。b16 quality（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`） 包含 check-docs，稳定通过。

本条自身验收不要求前端 build、真实认证服务或部署，未执行的这些入口不计为通过；其它任务的 live skip 与后端场景不构成本条剩余条件。前端原固定来源和后续文档质量证据分别保留，不拼成新候选全仓全绿。未提交、未安装依赖；目录由统一收口处理。
