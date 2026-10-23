# DEBT-SERVICE-CANCEL-CONTEXT-01 · Dataset 写路径的取消传播缺口

## 债

- 位置：`snapshot/dataset/contract.go:15-48` 的 `Service` 公开接口；`definition.go:140`、`service.go:173`、
  `revision_fence.go:51` 的 `context.Background()` 回落；`snapshot/dataset/publication_coordination.go` 的
  `lockForContext`；`server/service_invoke.go:67` 的 `r.Context()`。
- 信号：`Service` 的全部公开写方法不带 `context.Context`，因此 `lockForContext` 拿到的 `ctx.Done() == nil`，
  直接走阻塞式 `mutex.Lock()`。Server 侧确实持有 `r.Context()`，但进入 `Service` 后丢弃，请求级取消传不到
  锁等待。客户端断开后，服务端继续在锁内跑完远端 IO，后续写请求排队等它。
- 判定：这是能力缺口，不是测试缺失。当前实现对"锁被长期持有时，等待方可否取消"没有答案，因为等待方
  根本没有取消通道。
- 处置：给 `Service` 写路径补 `context.Context`，把取消传到 `lockForContext` 与远端调用，并为
  `ctx.Done() == nil` 分支定义明确语义（不再是无界阻塞）。改的是公开 Go API 形状，须先过设计篇。
- 触发条件：实现端侧可取消写，或下一次修改 `snapshot/dataset/contract.go` 的公开接口时。

## 目标

从端断开或主动取消写请求时，服务端在有限时间内放弃锁等待并返回 `context.Canceled` 语义，而不是继续排队到
远端 IO 结束；不遗留未释放的锁，后续请求仍能取到锁。

## 依据

- [Dataset](../../doc/design/dataset.md)：发布与写路径。
- [服务](../../doc/design/service.md)：统一执行器、请求载体与请求级授权。
- [测试与验证](../../doc/engineering/testing.md)。

## 验收

- 锁等待可取消：对 `fence(setID)` 与 `service.writeMu` 两个位置分别注入取消，断言有界返回、不遗留未释放锁、
  后续请求仍能取到锁（当前 `lockForContext` 的 `ctx.Done() == nil` 分支不成立，改后须覆盖该分支）。
- Server 请求取消传播：客户端主动断开后，断言服务端在有限时间内放弃锁等待并返回取消语义。
- 现有写路径的行为、错误码与幂等结论不变；受影响包定向回归与 `-race` 通过。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- 需先修订 [Dataset](../../doc/design/dataset.md) 与 [服务](../../doc/design/service.md) 中写路径的接口形状，
  公开 API 变更不能直接落代码。
- 与 `INDEX-PUBLISH-DECOUPLE-01` 在 `publication_coordination.go` 上冲突：该任务取消 `PublicationProtection`
  票据语义，本债改同一文件的锁等待。按文件集串行，本债后做。

## 不做

不把 `writeMu` 分片；不为测试加生产后门；不改变无取消场景下写路径的等待语义；不顺手改 `Service` 的读方法。

## 由来

2026-10-23 从 `CONCURRENCY-LOCK-CASES-01` 拆出。原任务第 2、3 条要求用例覆盖这个缺口，同时自己写明
“修复会牵动公开形状，另开任务”，但那条任务并不存在，用例因此永久停在红。拆出后两边各归其位：
本债负责让取消可达，原任务只负责其余端侧等待边界。

同日 `INDEX-PUBLISH-DECOUPLE-01` 与 `ARCH-INVOCATION-01` 正在改本债涉及的三处接缝中的两处
（`publication_coordination.go`、`server/service_invoke.go`），本债的发起时间应排在它们之后。
