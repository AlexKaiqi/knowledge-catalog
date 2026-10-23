# CONCURRENCY-LOCK-CASES-01 · 端侧边界场景卡住用例集

## 目标

从端（CLI、Server HTTP、FUSE 挂载、Home 本地写）出发，补齐"边界场景会不会卡住"的用例：
每个入口在锁被长期持有、远端慢、进程被中断时，是否在有界时间内以确定错误返回，
而不是无期限地等待。用例按端入口组织，并逐条落到 Dataset 写路径的三层锁上
（`fence(setID)` → `service.writeMu` → `registry.writeMu`）。

判定的不是"锁是否存在"，而是"从端看这个等待是否可终止、可观测、可重试"。

"请求被主动取消时服务端能否放弃等待"不在本任务：那要求 `Service` 公开接口带 `context.Context`，
属能力缺口而非用例缺失，由 `DEBT-SERVICE-CANCEL-CONTEXT-01` 承接。本任务只证明：在没有取消通道的
当前实现下，各端入口的等待仍有界、可观测，且不把未完成的写报告为成功。

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

其中"无法取消"这一半由 `DEBT-SERVICE-CANCEL-CONTEXT-01` 承接：要修它必须给 `Service` 公开接口
补 `context.Context`，牵动公开 API 形状。本任务只覆盖另一半：在上述实现不变的前提下，CLI 中断、
读侧、FUSE、Home 本地写、30s/60s 超时错配这些入口的等待是否仍有界可观测。

## 待补用例

按端入口分组，每条都要有明确的完成判据，不能只写"命令能跑完"。

1. CLI 写被中断：`kc` 侧写操作进行中发送 SIGINT，断言进程在有限时间内退出，且不把未完成的写报告为成功；
   服务端残留的写允许继续，但客户端必须有明确终态。CLI 侧补 `signal.NotifyContext` 即可证明，
   不依赖服务端取消能力。
2. 读侧不被卡住：三层锁全部被持有时，逐个读入口（`Set`、`DatasetVersion`、`Log`、`DumpState`、
   `ResolveDataset`、`ReadVirtualFile`、只读与请求视图）都要在有界时间内返回已接受的旧版本，不排队。
   已有一条"读不因被持有的提交而等待"，需扩到 FUSE 与 Server 路径。
3. FUSE 边界：挂载内读被持锁的写阻塞时，`Read`/`Lookup` 必须在有界时间内返回 errno，
   不能让内核挂起；文件句柄要能在取消后释放。
4. Home 本地写：`connectionLock` 与 `jsonfile.Locked` 是进程内 `Mutex` 无超时，
   断言同一路径的读-改-写在进程被中断后不留下半写文件，且第二个进程不以阻塞方式永久等待。
5. 超时对齐：端侧 30s 硬超时与服务端锁内多个 60s 远端段的不一致要有用例暴露
   （客户端已超时、服务端仍在写的情况），断言此时端侧得到的是可重试的确定错误，
   而不是"看起来失败、实际已提交"。
6. 卡住可观测：锁等待期间要有可断言的观测点（等待开始/结束、耗时、被谁持有），
   使上述用例除时间上界外还能断言"确实在等锁"而非"卡在别处"。

原第 2、3 条（Server 请求取消传播、锁等待可取消）已移出本任务：它们的根因是 `Service` 公开接口不带
`context.Context`，属于能力缺口而非用例缺失，验收要求也牵动公开 API 形状。现在由
`DEBT-SERVICE-CANCEL-CONTEXT-01` 承接，本任务不再要求为它们先写红用例。

## 验收

1. 上述每条用例在普通与 `-race` 下都能稳定复现与判定，不用 sleep 作为唯一同步手段。
2. 新增用例在 [doc/engineering/test-catalog.md](../../doc/engineering/test-catalog.md) 登记，
   写明端入口、被持锁、期望上界与失败码；用例名可在源码检索到。
3. 用例能区分三种结果：正常返回、确定错误（含错误码）、无界等待。第三种即失败。
4. 本任务内全部用例须为绿，不含"先红后改"的条目。取消传播缺口已由
   `DEBT-SERVICE-CANCEL-CONTEXT-01` 单独承接，不在本任务内写红用例。
5. `make check-docs`、`make quality`、`make test` 全绿且无 skip。

## 前置

- 与三处在途迁移按文件集串行，本任务后做：`ARCH-HOME-REPOSITORIES-01` 迁移 `home/connections.go`
  的 `connectionLock`（第 4 条）；`INDEX-PUBLISH-DECOUPLE-01` 取消
  `snapshot/dataset/publication_coordination.go` 的票据语义（第 5 条的超时观察点）；
  `ARCH-INVOCATION-01` 重划 `server/service_invoke.go` 的职责（第 2 条的 Server 路径）。
  开工前先确认这三批已交付，否则用例测的是即将消失的接缝。

## 不做

- 不改 `Service` 公开接口签名、不把 `writeMu` 分片、不为测试加生产后门。取消传播缺口归
  `DEBT-SERVICE-CANCEL-CONTEXT-01`。
- 不把"跨 Dataset 写是否应并行"当结论；本任务只保证端侧等待有界可终止。
- 不重复 `.data/scenes` 的业务旅程；只做端到端的等待边界用例。
- 不删除、跳过或弱化既有并发断言。
- 不处理"发布请求本身耗时超过端侧超时"这一类：那是发布路径把索引构建放在请求内造成的，
  归 `INDEX-PUBLISH-DECOUPLE-01`，本条只在第 5 条保留超时值不一致的证据，不把它当锁问题修。
