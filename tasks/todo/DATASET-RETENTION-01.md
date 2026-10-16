# DATASET-RETENTION-01 · Dataset 修订的检索保留与退役

## 目标

检索保留按“至少保留多长时间且至少保留几个修订”执行；超出保留的修订可读不可搜，搜索明确报检索已不可用，
不返回零命中；被替换修订的续页只在宽限内可用，过后明确要求重新开始。退役修订读取与搜索都拒绝，错误与
“检索已不可用”可区分，发布历史与来源提交仍可追溯；当前修订不能退役。无人访问和任务固定都不改变保留。
测试部署提供可控时钟；旧代际固定保留 3 分钟即删除的逻辑改为按保留策略回收。

## 依据

- [Dataset](../../doc/design/dataset.md)：U12 修订的检索保留与退役。
- [发现](../../doc/design/discovery.md)：索引维护、续页。

## 验收

- [accept-12-dataset-revisions](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-12-dataset-revisions.yaml)
  中以下规格全绿：`accept-revision-search-retention`、`accept-revision-continuation-grace`、
  `accept-revision-retire`、`accept-revision-retire-current-rejected`、`accept-revision-usage-no-auto-retire`、
  `accept-revision-pinned-task-out-of-retention`。
- 旧代际固定保留 3 分钟即删除的逻辑改为按保留策略回收，有回归测试证明未满保留的旧修订检索不被提前回收。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- `INDEX-SEARCH-SPACE-01`：保留与回收的对象是按 Dataset 修订的检索空间。
- `INDEX-USAGE-01`：仅 `accept-revision-usage-no-auto-retire` 中“使用数据逐修订列出访问时间与空间”的断言依赖它；
  其余规格不依赖，可先做，该条最后补绿。

## 不做

不按使用情况自动保留或退役；不做来源跟随（`DATASET-REVISION-01`）；不做来源提交保留、配额与紧急清除
（`DATASET-STORAGE-01`）；不清除来源仓历史。

## 来源

2026-10-16 从 `DATASET-REVISION-01` 拆出，原因见该任务“拆分”一节。
