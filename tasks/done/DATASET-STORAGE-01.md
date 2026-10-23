# DATASET-STORAGE-01 · 来源提交保留与配额

## 目标

未退役修订持有其冻结的来源提交，lakeFS 回收不删除它们；退役后保留释放。回收规则会越过保留时部署拒绝启动，
不潜伏到读取时才报错。按 Dataset 设配额，超出时自动发布与手动发布都不接受新修订，当前修订照常服务。

## 依据

- [Dataset](../../doc/design/dataset.md)：U13 来源提交不被回收、U14 配额。
- [Snapshot](../../doc/design/snapshot.md)。

## 验收

- [accept-12-dataset-revisions](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-12-dataset-revisions.yaml)
  中以下规格全绿：`accept-revision-source-commit-held`、`accept-revision-gc-rule-check`、`accept-revision-quota`。
- `make deploy-local-scenes` 通过（回收与提交保留需要真实 lakeFS）。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- `DATASET-RETENTION-01`：提交保留以修订退役为释放点。
- `DATASET-REVISION-01`：`accept-revision-quota` 含自动发布路径。

## 不做

不做跨仓事务；不清除来源仓历史；不按使用情况自动退役；不做紧急清除（`DATASET-PURGE-01`）。

## 拆分（2026-10-22）

原任务同时装 U13 提交保留、U14 配额、U15 紧急清除。前两项共用"修订退役即释放"的同一实现，
紧急清除则是独立能力：它要重写共用索引、把证据观察值替换为“已清除”、保留摘要与访问记录并给出完成时间，
触及 `liveaccess` 证据合同。范围与实现都不重叠，拆为 `DATASET-PURGE-01`。

## 关闭

2026-10-23 并入 `DATASET-LIFECYCLE-01`（原 `DATASET-RETENTION-01`），未按本文件单独验收。

并入理由：U13 提交保留与 U14 配额都以 U12 引入的“修订已退役”为唯一判定输入，本文件自己的“前置”就写着
提交保留以修订退役为释放点、“前两项共用修订退役即释放的同一实现”；三者还改同一批文件——修订状态机、
发布准入与回收触发点。分成两条时本文件必须先等保留任务把状态机定稿，同一处代码改两遍。

承接情况：

- 三条规格（`accept-revision-source-commit-held`、`accept-revision-gc-rule-check`、`accept-revision-quota`）
  原样保留在 `DATASET-LIFECYCLE-01` 的验收列表里。
- “回收规则会越过保留时部署拒绝启动，不潜伏到读取时才报错”保留在该任务的目标与验收里。
- “不做跨仓事务、不清除来源仓历史、不按使用情况自动退役、不做紧急清除”逐条保留在该任务的“不做”里。

本文件要求的 `make deploy-local-scenes` 保留在承接任务，未降级为夹具验收。
