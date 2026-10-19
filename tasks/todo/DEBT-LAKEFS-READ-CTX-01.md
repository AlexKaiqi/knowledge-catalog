# DEBT-LAKEFS-READ-CTX-01 · lakeFS 单次对象读取不接收取消

## 债

- 位置：`snapshot/internal/lakefsimpl/client.go` 的 `getObject`（`http.NewRequest` 无 ctx，302 后直连读取用
  `c.direct.Get`）；上游 `object/reader` 与 `snapshot.Store` 读取接口不带 `context.Context`。
- 信号：DEBT-MAINTENANCE-CANCEL-01 收口时，投影维护已在对象粒度检查取消，但正在进行的那一次 lakeFS 读取
  仍要自然结束；读取挂起时退出等待上限由 HTTP 客户端超时决定，而不是关闭信号。
- 判定：设计要求维护收到取消后尽快停下；对象粒度检查已让常规退出有界（等一次读取），单次读取不可取消是剩余缺口。
  给读取链加 ctx 要改 `snapshot.Store` / Reader 公开读取形状及全部实现，超出 DEBT-MAINTENANCE-CANCEL-01 范围。
- 处置：读取链接收 ctx（或 Store 持有可由关闭取消的生命周期 ctx），lakeFS 请求改用 `http.NewRequestWithContext`；
  不以缩短超时代替。
- 触发条件：下一次修改 `snapshot.Store` / Reader 读取形状时；或真实部署中观察到退出被单次读取卡住超过十秒。

## 目标

关闭信号能中断正在进行的 lakeFS 对象读取，退出等待不再依赖客户端超时。

## 依据

- [发现与检索](../../doc/design/discovery.md)：生命周期与退出顺序。
- [工程质量闭环](../../doc/engineering/quality.md)：质量问题三个出口。

## 验收

- 回归：lakeFS 读取挂起（夹具 HTTP 服务不返回）时取消维护，读取在有界时间内返回 `context.Canceled`。
- 受影响完整回归、`make check-docs`、`make quality`、`make test` 全绿且无 skip。

## 前置

无。

## 不做

不改变退出顺序，不以超时放弃维护。
