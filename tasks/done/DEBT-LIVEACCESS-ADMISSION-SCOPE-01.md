# DEBT-LIVEACCESS-ADMISSION-SCOPE-01 · 已接纳的新仓无需重装 LiveAccess

## 债

- 位置：`liveaccess/plugin/entry.go` 的固定 allowed map、`home/plugins.go` 的访问插件装配。
- 信号：真实部署启动之后经合法 create/attach 的仓，`kc access` 返回 `Repository is outside this plugin instance`；同一快照下源操作也被构造时的仓列表阻断。
- 判定：部署的来源集合可以变化，插件却保存启动快照；重启才能取得能力不符合插件按部署实例解释当前范围内声明的合同。
- 处置：登记后立即并入共用真实验收环境的 `ACCEPT-HARNESS-01`，增加宿主提供的当前成员判定端口和准入/移除/并发回归。
- 触发条件：`ACCEPT-HARNESS-01` 已在执行真实部署规格，当前失败直接触发修复与重新验收。

## 目标

运行中的 LiveAccess 实例采用当前部署已接纳的来源，新仓不需重装插件；未接纳或已移出来源仍拒绝取值及源操作。

## 依据

- [插件](../../doc/design/plugins.md)
- [服务](../../doc/design/service.md)

## 验收

真实 create/attach 后的 lookup 与 invoke 通过，未接纳及移出来源不外呼；默认与自定义 Entry 均取得宿主范围，运行时只打开一次；并发/race、受影响合同及统一文档/质量门禁全绿。

## 前置

并入 `ACCEPT-HARNESS-01`，使用其真实部署和统一门禁。

## 不做

不刷新请求运行时，不改变 HTTP 形状，不修改索引插件的作用域。

## 关闭

登记后立即并入 `ACCEPT-HARNESS-01`。实现与红绿证据在 `.validation/runs/20261016-liveaccess-current-scope/`；真实部署与统一门禁由主任务继续完成，未以定向绿代替完整验收。
