# DATASET-REVISION-01 · Dataset 来源跟随：分支前进自动发修订

## 目标

来源默认跟随：分支快进后系统以服务身份自动发修订并准备索引；连续前进只追最新提交；准备失败时当前修订
继续服务并报出失败原因、落后来源与目标提交；跨仓组合校验不过就不接受，配套改动到齐后自动通过；索引声明
的分支改动随新修订生效；有最小间隔。手动来源不随分支前进；非快进、删分支或撤销来源发布许可时停止跟随
并报出原因，维护者手动发布或恢复许可后重新跟随。

## 依据

- [Dataset](../../doc/design/dataset.md)：U11 来源分支前进，Dataset 自动跟随。
- [发现](../../doc/design/discovery.md)：索引维护。
- [Snapshot](../../doc/design/snapshot.md)。

## 验收

- [accept-12-dataset-revisions](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-12-dataset-revisions.yaml)
  中以下规格全绿：`accept-revision-follow-default`、`accept-revision-follow-coalesce`、
  `accept-revision-follow-prepare-failure`、`accept-revision-manual-source`、
  `accept-revision-follow-declaration-change`、`accept-revision-follow-permission-revoked`、
  `accept-revision-follow-non-fast-forward`、`accept-revision-cross-repo-combination`、
  `accept-revision-follow-min-interval`。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- `INDEX-SEARCH-SPACE-01`：自动发修订要按 Dataset 修订准备检索空间。
- `DATASET-PUBLISH-SAFETY-01`：自动发布依赖发布重试幂等与发布前对象冲突检查。
- `DATASET-RETENTION-01`：跟随会持续累积修订，先有检索保留回收才不会无界增长；测试部署的可控时钟也在该
  任务落地，`accept-revision-follow-min-interval` 依赖它。

## 不做

不按使用情况自动保留或退役；不做跨仓事务；检索保留与退役在 `DATASET-RETENTION-01`；来源提交保留、配额与
紧急清除在 `DATASET-STORAGE-01`。

## 并入（2026-10-16）

原并入本任务的 `DEBT-DATASET-DEFINE-IDEMPOTENCY-01`（发布重试幂等）与 `DEBT-DATASET-CONFLICT-ADMISSION-01`
（发布前对象冲突检查）不依赖检索空间，2026-10-16 拆为前置任务 `DATASET-PUBLISH-SAFETY-01`，位置、信号与
剩余验收在那里继续；本任务的自动跟随依赖它的幂等结果。

## 拆分（2026-10-16）

原任务一条承接跟随、检索保留、退役、来源提交保留、配额、紧急清除六项能力，并以整份 accept-12 为验收，
范围过大。按 Dataset 设计的 U11 至 U15 拆成三条，用 `## 前置` 连接：

- 本任务：U11 跟随。
- `DATASET-RETENTION-01`：U12 检索保留与退役，含测试可控时钟和旧代际固定 3 分钟回收逻辑的替换。
- `DATASET-STORAGE-01`：U13 来源提交保留、U14 配额、U15 紧急清除，含真实 lakeFS 部署验收。

accept-12 的 19 条规格按上述三条任务的验收列表分配，无遗漏无重复。验收执行器 `.data/scenes/accept.py` 的
`owners()` 仍按文件前缀把整份 accept-12 归到本任务；实际归属以三条任务的验收列表为准，执行器按规格 ID
细分归属，随 `ACCEPT-HARNESS-01` 下次修改 `accept.py` 时处理。
