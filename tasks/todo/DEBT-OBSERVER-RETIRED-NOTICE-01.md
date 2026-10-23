# DEBT-OBSERVER-RETIRED-NOTICE-01 · 外部观察夹具仍调用已退役通知入口

## 债

- 位置：`.data/scenes/service-initialized/source-repositories-configured/repository-attached/_materials/accessor/observer.py:46`，以及同节点 `domain-schema-published/_materials/references/observation-refreshed.md` 的操作说明。
- 信号：`send()` 仍 POST `/operations/v1/projections:notice`，该仓级公开入口已随 Dataset 范围检索退役；说明文档仍把它列为公开入口。
- 判定：这是自动观察夹具与说明的漂移，不属于 Dataset 内对象检索能力。核对 `projections:notice` 路由与对应探针可确认剩余范围。
- 处置：按 Dataset 修订刷新改写该夹具，或改用设计允许的调度入口；同步 `observation-refreshed.md` 的操作说明。不得以改写 live 测试代替该入口可用。
- 触发条件：需要运行外部自动观察用例，或恢复该夹具所在场景子树时。

## 目标

外部观察夹具与操作说明都只使用当前存在的公开入口；旧仓级通知入口不再被调用或描述为可用。

## 依据

[发现](../../doc/design/discovery.md)的观察与刷新、[LiveAccess](../../doc/design/liveaccess.md)的逐值依据。

## 验收

- `observer.py` 不再向已退役入口发送通知，改为维护者显式配置的 Dataset 修订刷新或设计允许的调度入口。
- 该夹具所在场景可执行，并验证旧修订不刷新、当前修订观察更新与来源 HEAD 不变。
- `observation-refreshed.md` 的操作说明与实际公开入口一致。
- 受影响场景检查与 `make check-docs` 通过。

## 前置

无。与 `INDEX-SEARCH-SPACE-01` 不重叠：后者只负责 Dataset 内检索能力，不修改观察夹具。

## 不做

不实现自动跟随发布（归 `DATASET-REVISION-01`），不改检索侧代码。

## 由来

原登记为 `DEBT-STATE-OBSERVER-RETIRED-ROUTE-01`，登记后按同批文件规则并入 `INDEX-SEARCH-SPACE-01`。2026-10-22 核实该信号与 Dataset 内对象检索无关，从该任务剥离，另立本条。
