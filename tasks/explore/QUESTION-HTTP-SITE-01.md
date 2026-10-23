# QUESTION-HTTP-SITE-01 · 产品 site 的 HTTP 说明与远程部署实际行为有哪些不一致

## 要回答的问题

对预览服务 `http://9.134.13.45:7385`（`--auth local`）逐条实测产品 site
`.data/site/http.html` 描述的全部发现与访问接口后，哪些"说明写的行为"与"服务实际行为"不一致；
每一项应当改说明、改实现，还是两者都改。

不预设每条都该改代码：`409` vs `400`、`/metrics` 免认证、`X-Tai-User` 拒绝这几项，
说明可能只是没跟上实现，此时正确处置是改 `.data/site/http.html`。

## 已知证据

2026-10-22 在远程预览服务上按 site「一条完整的读取流程」逐步实测，数据集
`autumn-launch-kit`（rev 3 / rev 6）、`qqbrowser`（rev 2）、`qinghe-analysis`、`store-oncall`。

已确认一致：认证握手与 `whoami`、`admission`、`inventory`、`datasets`、`dataset show`、
`resolve`、`mounts:list`、`tree:list`、`file:read`（默认 512 KiB、上限 4 MiB、`eof`/`totalBytes`、
不带 pin 409）、`objects:read` 含 `#/` 定位符、`objects:resolve`、`provenance:describe`、
`bindings:resolve`、`log:query` 与 `resources:access` 的仓范围拒绝、插件 help/capabilities、
`schemas:list` 分页与游标归属校验、`search` `limit` 上限 1000、401/403/404 形态。

不一致的六项，逐项证据见下方「逐项分析」，其中三项是要修的实现缺陷
（访问声明挂起、`check` 空 body 判 PASSED、检索未就绪错误码缺登记），
三项需要先定"改说明还是改实现"（`fields[].access` 降级、无授权可读、`/metrics` 与 `X-Tai-User`）。

## 判定标准

- 每项给出：site 原文、实测行为、实现位置、判定（改说明 / 改实现 / 两者）、合同依据。
- 判定"改实现"须指出既有设计篇或包 README 里与之冲突的已选定形状；
  只与 site 文案冲突、与设计一致的，判定"改说明"。
- 判定"改说明"须确认该行为是设计意图，而不是尚未被发现的口径漂移。
- 排除项写进结论，避免后续重复开工。

## 前提

无。确认要改实现的项另开任务承接，不复用本探索的 ID。

## 不涉及

不修改 `.data/site/` 之外的站点脚本；不借本次核对扩张接口面；
不处理 site 明确声明不在范围内的写入、授权管理、运维观测与 `/api/v1/` 透传。

## 结论

2026-10-22 核对完成，逐项判定与处置如下。

判定改说明（已改 `.data/site/http.html`）：

- 检索未就绪的状态码是 `400 CAPABILITY_UNSATISFIED`，不是 site 写的 `409 PRECONDITION_FAILED`。
  `invoke/http_write.go` 把 `ErrPreconditionFailed` 映射到 409，而它的语义是"对象、摘要、游标或工作树与
  HEAD 不符"，与"索引未就绪"不同类；实现语义正确，site 写错。已在错误清单补
  `CAPABILITY_UNSATISFIED`，并把 409 那条里的"检索索引需要重新准备"移除。
- `/metrics` 在 `--auth local` 下免认证（`server/serve_facade.go` 先判 `Authenticated()` 再判管理员），
  site 原称免认证只有探活与 `auth`。已补 `/metrics` 与探活接口入表，并注明 taihu 部署需管理员。
- `X-Tai-User` / `X-Tai-Identity` 只在 taihu 认证器里读取，local 认证器不读，因此"拒绝"在 local 下
  不可能发生。已把该条限定到 taihu 部署，并说明 local 模式不校验身份来源。
- `fields[].access` 来自检索声明编译：未就绪时 `annotateReport` 直接跳过，字段为空属设计降级，
  site 却把它当作唯一判断依据。已补"未就绪时为空，看 `search.state` 与 `search.reason`"。
- `check` 不带 pin 时按当前修订检查并可能返回 `PASSED`。`cli/deployment_read_concurrency_test.go`
  依赖这一形态返回 200，构成事实合同；实现不改，site 说明该用法只能确认"现在能不能读"，
  不能验证已丢失的 pin。

判定改实现（已落地并带回归测试）：

- `access-specs:describe` 在大数据集上不可取消。`AccessDescribeExecutor.Execute` 原先丢弃
  `context.Context`，`PlanAccess` 也不接受 ctx，客户端 240 秒超时后服务端仍在继续。已新增
  `PlanAccessContext` 并在执行器内传入，`accessplan_cancel_test.go` 断言取消后不再打开任何成员。

仍未定性（未改，需远程部署声明）：

- 无授权身份可读全部库存、对象与文件。`invocation/allow.go` 的 `InventoryReadAllowed` 对非空身份放行，
  `withDatasetDefaults` 注入部署声明的 authenticated 默认读。本地 `.kc-home/layout.yaml` 无
  `dataset-access` 声明，无法判断远程是显式声明还是判定缺口。

未收敛（需独立任务，超出改说明范围）：

- `qqbrowser` 与 `tianqiong-qqbrowser` 的 `access-specs:describe` 在 240 秒内仍无响应，rev 1/rev 2
  均如此，而同数据集 `schemas:list`、`dataset show` 均低于 0.1 秒，`autumn-launch-kit`、
  `qinghe-analysis`、`store-oncall` 的同一接口 2 秒内返回。取消能力只保证请求可被放弃，
  不解决成本本身；成本结构（每成员重复全量绑定扫描，O(仓数²)）需要单独定位与改造。
