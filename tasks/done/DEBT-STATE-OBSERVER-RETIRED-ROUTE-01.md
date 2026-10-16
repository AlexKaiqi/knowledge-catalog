# DEBT-STATE-OBSERVER-RETIRED-ROUTE-01 · 外部观察夹具仍调用已退役通知入口

## 债

- 位置：`.data/scenes/service-initialized/source-repositories-configured/repository-attached/_materials/accessor/observer.py:46` 及对应使用说明。
- 信号：`send()` 仍 POST `/operations/v1/projections:notice`；Dataset 范围搜索已删除该仓级公开入口。
- 判定：真实自动观察夹具仍会失败；默认跳过的 live 测试不能证明它可用。本轮静态 MVP 不使用该夹具。
- 处置：按同批索引维护入口规则并入 `INDEX-SEARCH-SPACE-01`，由主任务保留剩余验收。
- 触发条件：本任务在进行中的 Dataset 观察刷新与旧入口退役改动。

## 目标

观察刷新通过设计允许的 Dataset 维护入口执行，回执绑定当前修订，旧修订保持原观察。

## 依据

[发现](../../doc/design/discovery.md)、[LiveAccess](../../doc/design/liveaccess.md)。

## 验收

真实 runtime 观察更新当前 Dataset 修订，保留来源版本且不移动 HEAD；旧通知入口继续被拒绝，完整契约与质量门禁通过。

## 前置

`INDEX-SEARCH-SPACE-01`。

## 不做

不恢复按仓通知入口，不在静态 MVP 验收中冒称自动观察完成。

## 关闭

未完成即归并：剩余实现与验收已并入 `INDEX-SEARCH-SPACE-01`，不另起与同批文件重叠的并行任务。
