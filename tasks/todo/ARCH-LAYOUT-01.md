# ARCH-LAYOUT-01 · 目录重组与无生产者错误码清理

## 目标

一个能力一个顶层目录，依赖单向；会话与登录收进 `client/`；`cli/` 里的合同测试回到所属包；`home/` 拆小；
没有生产者的错误码删除。

## 依据

- [架构](../../doc/architecture.md)、[路线](../../doc/roadmap.md) 的目录重组表。
- [架构不变量](../../doc/engineering/architecture-invariants.md)。

## 验收

- `internal/testkit` 的 `DatasetPin` 与 `declarativeindex/plugin` 测试里的 `pluginDatasetPin` 和 `home.DatasetPin` 合并（原阶段 3 登记的债，拆 `home/` 后才能不成环）。
- 交付时给出顶层目录与依赖图、每个目录一句职责，并与 `doc/architecture.md` 逐条核对；这份报告是 `FINAL-ACCEPTANCE-01` 的一部分。
- `make check-docs`、`make quality`（含耦合度基线）、`make test`、`make test-contracts` 通过。

## 前置

- `INDEX-SEARCH-SPACE-01`、`LIVEACCESS-READ-01`、`DATASET-REVISION-01`、`DATASET-RETENTION-01`、`DATASET-STORAGE-01`：
  先完成插件与 Dataset 修订重构，避免目录刚搬完又随插件改写。
- `ROUTE-MANIFEST-01`：先把路由清单收成一份，搬目录时不再对账三处清单。

## 不做

不改公开行为；不给 `invoke/` 改名以外的包做语义调整；不做路由清单合一（`ROUTE-MANIFEST-01`）。

## 拆分（2026-10-16）

单一路由清单与原并入本任务的 `DEBT-UI-REDIRECT-STATUS-01`（UI 补斜杠跳转状态码随 Go 版本变化）改的是同一份
路由，不依赖目录搬迁和 Dataset 修订重构，拆为 `ROUTE-MANIFEST-01`，原验收条目与并入内容整体转入该任务。
`done/` 中原债的关闭记录写的是并入本任务，按历史保持原样。
