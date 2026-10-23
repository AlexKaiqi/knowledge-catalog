# DATASET-PURGE-01 · 紧急清除 Dataset 修订的证据观察值

## 目标

紧急清除在有界时间内重写该修订自己的物化语料：证据中的观察值替换为“已清除”，摘要与访问记录保留，
清除状态给出完成时间。清除不删除使用记录本身，不改修订的可读性。

语料边界按 `INDEX-PUBLISH-DECOUPLE-01` 的结论：每个 Dataset 修订一份独立语料，不存在跨修订共用的查询
结构。因此清除范围是“这个修订的这份语料”，不是“一份被多个修订共用的索引”,也不存在清除时影响其他
修订检索的可能。

## 依据

- [Dataset](../../doc/design/dataset.md)：U15 紧急清除。
- [LiveAccess](../../doc/design/liveaccess.md)：观察证据的清除。

## 验收

- [accept-12-dataset-revisions](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-12-dataset-revisions.yaml)
  中 `accept-revision-emergency-purge` 全绿。
- 清除后仍能读到修订与对象；证据的摘要与访问记录存在，观察值位置读到的已是“已清除”。
- 清除只重写该修订自己的语料，同一对象的其他修订语料不受影响，也不需要重写。
- 清除有明确完成时间，重复清除不报错也不重复改写。
- `make deploy-local-scenes` 通过（语料重写需要真实 lakeFS 与搜索容量）。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- `DATASET-LIFECYCLE-01`：清除以修订退役为触发与释放点。
- `INDEX-SEARCH-SPACE-01`：清除重写的对象是按 Dataset 修订的检索空间。

## 不做

不做跨仓清除，不清除来源仓历史，不删除使用记录与访问账，
不把清除做成异步后台任务（须在有界时间内完成）。

## 来源

2026-10-22 从 `DATASET-STORAGE-01` 拆出：紧急清除触及 `liveaccess` 证据合同与共用索引重写，
与"来源提交保留、配额"没有共同实现。

## 关闭

2026-10-23 并入 `DATASET-LIFECYCLE-01`（U15 紧急清除成为该任务的第四节验收）。并入理由：本文件“来源”
一节写明的拆出前提是“紧急清除触及共用索引重写”，该前提已被 `INDEX-PUBLISH-DECOUPLE-01` 的设计决定 9
推翻为“每个 Dataset 修订一份独立语料、不共享查询结构”，本文件自己的目标一节也已按新结论写过。共用索引
不存在之后，清除与保留落在同一份语料上：退役释放它、清除重写它，都读同一个修订状态，由同一个回收接缝
执行，属于同一件事。原文与验收条款完整保留在 `DATASET-LIFECYCLE-01`，本文件在 git 历史里可查。
