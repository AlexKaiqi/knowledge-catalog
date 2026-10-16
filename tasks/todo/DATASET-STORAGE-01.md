# DATASET-STORAGE-01 · Dataset 的来源提交保留、配额与紧急清除

## 目标

未退役修订持有其冻结的来源提交，lakeFS 回收不删除它们；退役后保留释放。回收规则会越过保留时部署拒绝
启动，不潜伏到读取时才报错。按 Dataset 设配额，超出时自动发布与手动发布都不接受新修订，当前修订照常服务。
紧急清除在有界时间内重写共用索引，证据中的观察值替换为“已清除”，摘要与访问记录保留，清除状态给出完成
时间。

## 依据

- [Dataset](../../doc/design/dataset.md)：U13 来源提交不被回收、U14 配额、U15 紧急清除。
- [LiveAccess](../../doc/design/liveaccess.md)：观察证据的清除。
- [Snapshot](../../doc/design/snapshot.md)。

## 验收

- [accept-12-dataset-revisions](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-12-dataset-revisions.yaml)
  中以下规格全绿：`accept-revision-source-commit-held`、`accept-revision-gc-rule-check`、`accept-revision-quota`、
  `accept-revision-emergency-purge`。
- `make check-docs`、`make quality`、`make test`、`make deploy-local-scenes` 通过（回收与提交保留需要真实 lakeFS）。

## 前置

- `DATASET-RETENTION-01`：提交保留与紧急清除都以修订退役为释放点。
- `DATASET-REVISION-01`：`accept-revision-quota` 含自动发布路径。

## 不做

不做跨仓事务；不清除来源仓历史；不按使用情况自动退役。

## 来源

2026-10-16 从 `DATASET-REVISION-01` 拆出，原因见该任务“拆分”一节。
