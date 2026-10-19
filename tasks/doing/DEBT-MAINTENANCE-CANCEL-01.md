# DEBT-MAINTENANCE-CANCEL-01 · 投影维护不响应取消，SIGTERM 后服务长时间不退出

## 债

- 位置：`declarativeindex/control/snapshot.go` 的 `applyPending` 调 `Index.Ensure(repo, commit)`；
  `declarativeindex/projection/sync.go` 的 `Ensure` / `rebuild` / `apply` / `applyChangedObjects`；
  `discovery/adoption.go` 的 `AccessSpecForRepositoryChecked`；端口 `declarativeindex/control/controller.go`
  与 `discovery/plugin/ports.go` 的 `Ensure(objectmodel.Repository, kernel.CommitID)`；
  `snapshot/internal/lakefsimpl/client.go` 的 `getObject` 用不带 context 的 `http.Client.Get`。
- 信号：2026-10-19 预览服务（lakeFS + 在线 ES + 向量）收到 SIGTERM 后，`BeginShutdown` 已把 handler 置为 closed，
  但 `StopMaintenance` → `Controller.Close` 的 `wg.Wait()` 要等一整轮 catch-up 自然跑完（单个 Dataset 修订
  重建数分钟），`server.Shutdown` 迟迟不执行：端口不释放、所有请求返回 `409 HTTP handler has closed`，
  只能 `SIGKILL`。goroutine dump 显示 worker 停在 `Ensure → specAtCommit → AccessSpecForRepositoryChecked →
  ResolveAddress → lakefsimpl.getObject`，整条链没有 ctx。
- 判定：[发现与检索](../../doc/design/discovery.md) 要求“退出时先停新请求，等在途请求与维护结束”，等待维护结束
  是设计；缺陷在于维护收到取消后不能在对象粒度停下，`kc serve` 帮助里“最多十秒的 HTTP 关闭等待”因此不成立。
  修复要给公开端口 `Ensure` 加 ctx，涉及 4 个实现与 30 余处调用，超出发现时任务（HTTP 接口页跑通）范围。
- 处置：`Ensure` 及其构建循环接收 ctx，在逐对象读取、编译、写入批次之间检查取消；取消时中止 rebuild 会话
  （`RebuildSession.Abort`），不留下半成品 meta；lakeFS 读取改为带 ctx 的请求。不以超时丢弃维护来绕过。
- 触发条件：下一次修改 `Ensure` 端口或 `projection/sync.go` 构建循环时；或对外演示前。

## 目标

服务收到 SIGTERM 后，后台投影维护在对象粒度内停下，HTTP 在十秒内完成关闭、进程退出；中断的重建下次启动
按冷构建或 schema 变化重来，不留可被查询误用的半成品。

## 依据

- [发现与检索](../../doc/design/discovery.md)：生命周期与退出顺序。
- [工程质量闭环](../../doc/engineering/quality.md)：质量问题三个出口。

## 验收

- 回归：后台 rebuild 正在逐对象读取时触发 `BeginShutdown`，worker 在有界时间内返回 `context.Canceled`，
  `server.Shutdown` 随后完成，引擎 meta 未前移；原 `server/plugin_shutdown_http_test.go` 两条断言不削弱。
- 预览服务实测：sync 进行中 `systemctl --user stop kc-dev-server` 在 15 秒内结束，无需 SIGKILL。
- 受影响完整回归、`make check-docs`、`make quality`、`make test` 全绿且无 skip。

## 前置

无。

## 不做

不改变退出顺序（先停新请求、再等在途与维护、资源只释放一次），不以超时放弃维护。

## 进展（2026-10-19）

实现：

- 端口 `declarativeindex/control.MaintenanceIndex` 与 `discovery/plugin.ProjectionSynchronizer` 的 `Ensure` / `EnsureAt`
  增加 `context.Context`；`projection.Index.Ensure/EnsureAt` 把 ctx 传给 `rebuild`、`apply`、`applyHydrated`、
  `applyChangedObjects`，在进入、取 spec 后、逐对象、每个 500 对象批次前、写引擎前检查取消；流式重建取消时调用
  `RebuildSession.Abort`，引擎 meta 不前移。`IndexPlugins.EnsureAt` 的 Dataset 路径改为把调用方 ctx 传给
  `PrepareDataset`（原为 `context.Background()`）；`PrepareDatasetScope`、`ProjectionSyncExecutor`、`applyPending`、
  `catchUpFixed` 都传入调用方 ctx。前台 `Apply` / `Rebuild` 与 Snapshot 钩子 `AfterSnapshot` 形状不变。
- `applyPending`：取消导致的失败不写 `FAILED` 记录，目标保持 `PENDING`，下次启动续做。

回归（先在去掉取消检查的代码上确认失败，再恢复到绿）：

- `server/plugin_shutdown_rebuild_test.go` `TestHTTPBeginShutdownStopsInFlightRebuildWithoutAdvancingMeta`：后台重建读对象途中
  `BeginShutdown`，在读完当前页后有界返回；目标保持 `PENDING`、无错误记录；`Close` 完成；引擎 meta 与文档未前移；
  只读了一页。去掉取消检查时该用例失败（目标变成 `READY`）。
- `declarativeindex/projection/maintenance_cancel_test.go`：重建与增量两种路径取消后返回 `context.Canceled`，
  重建会话被 `Abort`，READY meta 不前移。
- `declarativeindex/control/controller_cancel_test.go`：`Close` 取消在途 `Ensure`，有界返回，目标保持 `PENDING`。
- `server/plugin_shutdown_http_test.go` 原两条断言未改，仍通过。

已验证：主工作区最终源码上 `make check-docs` PASS、`make quality` 0、`make test` 0；临时 OpenSearch 下受影响包
（`declarativeindex/...`、`discovery/...`、`invocation/...`、`server`、`home`）`go test -short` 21 个包 ok、无 SKIP；
`home`、`server`、`declarativeindex/{control,projection,opensearch/...}` 的 `-race` 通过。日志：`.validation/remaining-20261019/`。

剩余：

- 正在进行的那一次 lakeFS 对象读取仍不接收取消（读取链没有 ctx），退出等待上限是一次读取；
  登记为 `tasks/todo/DEBT-LAKEFS-READ-CTX-01.md`。
- 验收里的预览服务实测（`systemctl --user stop kc-dev-server` 15 秒内退出）未做：本机为 macOS，没有该预览服务与
  systemd；需在预览服务所在主机上执行后才能移到 `done/`。
