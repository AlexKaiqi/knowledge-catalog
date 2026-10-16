# DEBT-ACCEPT-ORIGIN-HTTP-CLOSE-01 · 外部取值源自测显式关闭 HTTP 错误响应

## 债

- 位置：`.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_materials/acceptance/live-origin/test_origin.py` 的 HTTP 请求辅助函数。
- 信号：2026-10-15 使用仓库 `.venv` 跑 9 条源服务回归时，HTTP 400/401/404/503 的 `HTTPError` 在读取正文后没有关闭，Python 3.14 发出 `ResourceWarning`；证据 `.validation/runs/20261015T120400Z-accept-harness/origin-test.log`。
- 判定：辅助函数成功路径使用上下文管理，错误路径遗漏响应关闭；不是产品协议失败。该文件在开工前已有未提交内容，当前不直接改写。
- 处置：错误响应也使用上下文管理，保留状态码、正文与原断言；具体补丁见同证据目录的 `origin-response-close.patch`。获准合并既有在途文件后应用，再以 `ResourceWarning` 为错误运行原有全部回归。
- 触发条件：`ACCEPT-HARNESS-01` 集成真实源服务部署夹具与离线自测前。

## 目标

源服务自测的拒绝和故障路径不依靠垃圾回收关闭连接。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 原有 9 条回归全部通过，`ResourceWarning` 作为错误运行时没有遗漏关闭。
- `make check-docs`、`make quality` 全绿，无 skip。

## 不做

不修改源服务行为，不抑制资源警告，不削弱拒绝路径断言。

## 交付

### 实现

2026-10-16 在保留原修改的基础上，将 `LiveOriginTest.call` 的 HTTPError 错误响应也放入上下文管理，保留状态码、正文与所有既有断言。新增独立回归使用实际 HTTPError/BytesIO 验证返回前关闭；先红后绿，见 `entry-origin-red.log` 与 `entry-origin-green.log`。原“等待追加授权”的阻塞已解除。

### 已验证范围

原源服务 9 条回归在 `-W error::ResourceWarning` 下通过（`origin-nine-green.log`）；新增入口/close 回归 3 条通过。只证明离线夹具，不宣称真实部署验收。证据目录 `.validation/runs/20261016T012005Z-accept-harness`；源码写入完成后交由主线在稳定窗口统一运行全量门禁，本线没有运行完整架构组、`make quality` 或 `make test`，没有移 done。

## 关闭

2026-10-16 并入 `ACCEPT-HARNESS-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
