# CONCURRENCY-LOCK-CASES-01 · 端侧边界场景卡住用例集

## 目标

从端（CLI、Server HTTP、FUSE 挂载、Home 本地写）出发，补齐"边界场景会不会卡住"的用例：
每个入口在锁被长期持有、远端慢、进程被中断、请求被取消时，是否在有界时间内以确定错误返回，
而不是无期限地等待。用例按端入口组织，并逐条落到 Dataset 写路径的三层锁上
（`fence(setID)` → `service.writeMu` → `registry.writeMu`）。

判定的不是"锁是否存在"，而是"从端看这个等待是否可终止、可观测、可重试"。

## 依据

- [Dataset 设计与发布语义](../../doc/design/dataset.md)
- [Snapshot 设计与注册表权威](../../doc/design/snapshot.md)
- [测试与验证](../../doc/engineering/testing.md)、[测试目录](../../doc/engineering/test-catalog.md)
- 实现接缝：[service.go](../../snapshot/dataset/service.go) 的 `commitReceiptContext`、
  [revision_fence.go](../../snapshot/dataset/revision_fence.go)、
  [publication_coordination.go](../../snapshot/dataset/publication_coordination.go) 的 `lockForContext`、
  [registry.go](../../snapshot/dataset/registry.go)、[revision_reference.go](../../snapshot/dataset/revision_reference.go)
- 端侧入口：[cli/run.go](../../cli/run.go)、[server/service_invoke.go](../../server/service_invoke.go)、
  [datasetfs/mount_linux.go](../../datasetfs/mount_linux.go)、[home/connections.go](../../home/connections.go)

## 现状（已核实，2026-10-22）

端侧等待能力的分层事实：

- `Service` 公开接口（[contract.go:15-48](../../snapshot/dataset/contract.go#L15-L48)）全部不带 `context.Context`。
  `PublishDataset`、`DefineDataset`、`RetireDataset`、`RegisterRepository` 等写方法都回落到
  `context.Background()`（`definition.go:140`、`service.go:173`、`revision_fence.go:51`），
  因此 `lockForContext` 的 `ctx.Done() == nil` 分支直接走阻塞式 `mutex.Lock()`，无法被取消。
- Server 侧确实拿到 `r.Context()`（[service_invoke.go:67](../../server/service_invoke.go#L67)），
  但调用链进入 `Service` 后丢弃，请求级取消传不到锁等待。
- CLI 侧以 `context.Background()` 发起（[run.go:53](../../cli/run.go#L53)、[run.go:56](../../cli/run.go#L56)），
  没有 `signal.NotifyContext`，Ctrl-C 不会取消正在等待的写。
- CLI 到 Server 的 HTTP 客户端有 30s 超时（`client/session/session.go:249`），
  但那是整请求硬超时：超过即中断连接，服务端仍在锁内继续跑，客户端与权威状态会不一致。
- lakeFS 控制面 HTTP 超时为 60s（`snapshot/internal/lakefsimpl/client.go:115`），
  而 `registry.writeMu` 内包含 `loadAt`、`snapshotYAML`、`ApplyTreeCommit`，
  单次写可能串行多个 60s 段，端侧 30s 早已断开。
- 已有的并发用例都是"锁语义"用例（读不等待、单一赢家、取消不等另一个提交），
  没有一个是"从端发起、边界场景会不会卡住"的用例。

因此"卡住"在当前实现里是可复现的：CLI 断连后服务端继续持有三层锁，
后续任何写请求都排队到该次远端 IO 结束；期间请求方既收不到进度也无法取消。

## 待补用例

按端入口分组，每条都要有明确的完成判据，不能只写"命令能跑完"。

1. CLI 写被中断：`kc` 侧写操作进行中发送 SIGINT，断言进程在有限时间内退出，
   且不把未完成的写报告为成功；服务端残留的写允许继续，但客户端必须有明确终态。
   现实现预期失败（`context.Background()`），该用例先红后改。
2. Server 请求取消传播：客户端主动断开（取消 `r.Context()`）后，断言服务端在有限时间内
   放弃锁等待并返回 `context.Canceled` 语义，而不是继续排队到远端 IO 结束。
3. 锁等待可取消：对 `fence(setID)` 与 `service.writeMu` 两个位置分别注入取消，
   断言有界返回、不遗留未释放锁、后续请求仍能取到锁（当前 `lockForContext` 的
   `ctx.Done() == nil` 分支使这条不成立，用例要覆盖该分支）。
4. 读侧不被卡住：三层锁全部被持有时，逐个读入口（`Set`、`DatasetVersion`、`Log`、
   `DumpState`、`ResolveDataset`、`ReadVirtualFile`、只读与请求视图）都要在有界时间内返回
   已接受的旧版本，不排队。已有一条"读不因被持有的提交而等待"，需扩到 FUSE 与 Server 路径。
5. FUSE 边界：挂载内读被持锁的写阻塞时，`Read`/`Lookup` 必须在有界时间内返回 errno，
   不能让内核挂起；文件句柄要能在取消后释放。
6. Home 本地写：`connectionLock` 与 `jsonfile.Locked` 是进程内 `Mutex` 无超时，
   断言同一路径的读-改-写在进程被中断后不留下半写文件，且第二个进程不以阻塞方式永久等待。
7. 超时对齐：端侧 30s 硬超时与服务端锁内多个 60s 远端段的不一致要有用例暴露
   （客户端已超时、服务端仍在写的情况），断言此时端侧得到的是可重试的确定错误，
   而不是"看起来失败、实际已提交"。
8. 卡住可观测：锁等待期间要有可断言的观测点（等待开始/结束、耗时、被谁持有），
   使上述用例除时间上界外还能断言"确实在等锁"而非"卡在别处"。

## 验收

1. 上述每条用例在普通与 `-race` 下都能稳定复现与判定，不用 sleep 作为唯一同步手段。
2. 新增用例在 [doc/engineering/test-catalog.md](../../doc/engineering/test-catalog.md) 登记，
   写明端入口、被持锁、期望上界与失败码；用例名可在源码检索到。
3. 用例能区分三种结果：正常返回、确定错误（含错误码）、无界等待。第三种即失败。
4. 端侧入口的取消能力缺口（`Service` 接口无 ctx）在本任务内只登记与用例化，不改公开接口形状；
   若要改接口，另开任务并先过设计篇。
5. `make check-docs`、`make quality` 全绿且无 skip。

## 不做

- 不改 `Service` 公开接口签名、不把 `writeMu` 分片、不为测试加生产后门。
- 不把"跨 Dataset 写是否应并行"当结论；本任务只保证端侧等待有界可终止。
- 不重复 `.data/scenes` 的业务旅程；只做端到端的等待边界用例。
- 不删除、跳过或弱化既有并发断言。

## 前置与风险

- 第 2、3 条的根因是 `Service` 接口不带 ctx，修复会牵动公开形状；用例先按"当前应为红"落地，
  修复方案另开任务。
- 第 7 条涉及端与服务端超时策略，若结论需要调整超时值，属实施任务，本任务只出用例与证据。
