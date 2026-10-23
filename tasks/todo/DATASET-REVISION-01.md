# DATASET-REVISION-01 · Dataset 来源跟随：分支前进自动发修订

## 目标

来源分支快进后，系统以服务身份自动发修订并准备索引，连续前进只追最新提交；
接受与检索就绪是两个状态：索引准备失败不回滚已接受的修订，该修订停在检索未就绪、当前检索就绪修订继续
服务，并报出原因、落后来源与目标提交。

手动来源不跟随。非快进、删分支或撤销发布许可时停止跟随并报出原因，维护者手动发布或恢复许可后重新跟随。

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

各规格对应的分支：

| 规格 | 覆盖的分支 |
|---|---|
| `follow-default` | 快进即发修订 |
| `follow-coalesce` | 连续前进只追最新提交 |
| `follow-min-interval` | 有最小间隔 |
| `follow-prepare-failure` | 准备失败继续服务并报原因 |
| `cross-repo-combination` | 组合校验不过不接受，改动到齐后自动通过 |
| `follow-declaration-change` | 索引声明的分支改动随新修订生效 |
| `manual-source` | 手动来源不跟随 |
| `follow-non-fast-forward` | 非快进停止跟随 |
| `follow-permission-revoked` | 撤销许可停止跟随，恢复后重新跟随 |

## 前置

- `INDEX-SEARCH-SPACE-01`：自动发修订要按 Dataset 修订准备检索空间。
- `DATASET-PUBLISH-SAFETY-01`（已归档）：自动发布依赖发布重试幂等与发布前对象冲突检查。
- `DATASET-LIFECYCLE-01`：跟随会持续累积修订，先有检索保留回收才不会无界增长；测试部署的可控时钟也在该
  任务落地，`accept-revision-follow-min-interval` 依赖它。

## 并入

`DEBT-DATASET-VERSION-CONTRACT-01`（2026-10-23 并入，原文件在 `done/` 且不改写）。

- 位置：`doc/design/dataset.md` 的 U16、U17，`.data/scenes/_views.yaml` 第 18-19 行为这两条登记的 gap。
- 信号：产品视图校验报 U16、U17 既无验证关联也无明确 gap；补登记后仍缺实现与语义回归。
  U16 是服务端分配版本号、单调不可复用、同定义重放不增号、请求内固定 `latest`；U17 是两个已发布版本的
  指针与元数据差异，且不读来源字节。当前 CLI 仍要求调用方显式传 `revision`。
- 并入理由：本条触发条件是“下一次修改 Dataset 版本分配、发布幂等或历史对比入口时处理”，而本任务要做的
  自动发修订必然改这三处：自动发修订要由服务端取下一个号、要按定义判等决定是否增号、要按固定版本服务。
  原债自己也写着“与 `DATASET-REVISION-01` 的范围重叠审查后并入，避免同文件双线实施”。
- 优先级修正：原债把自己的性质定为“新功能，按 bug 先做新功能先不做”。这不成立：
  [Dataset](../../doc/design/dataset.md) 的进度表把“版本号改为服务端分配：当前实现要求调用方传 `revision`，
  与‘系统生成、不接受指定’不一致”列在近期，U16 是版本语义本身而非可选扩展。本条按核心语义推进，
  不再以“新功能”为由搁置。
- 剩余验收并入本任务：

  - U16 的服务端分配、单调不可复用、同定义重放不增号及请求内固定版本各有可证伪回归。
  - U17 比较两份已接受清单的增删改与元数据，不读来源字节；来源不可用仍返回指针差异及明确状态。
  - 具体验证关联到产品条目，已证明部分再收窄 `.data/scenes/_views.yaml` 的 gap，
    不以解析或命令成功代替语义断言。
  - 改动触及发布幂等与版本分配时，`make test-contracts` 全绿且无 skip。

## 不做

不按使用情况自动保留或退役；不做跨仓事务；检索保留、退役、配额与紧急清除都在 `DATASET-LIFECYCLE-01`。

## 沿革

2026-10-16 原任务一条承接跟随、检索保留、退役、来源提交保留、配额、紧急清除六项能力，并以整份 accept-12
为验收。按 Dataset 的 U11 至 U15 拆成多条：本任务 U11，`DATASET-RETENTION-01` U12，
`DATASET-STORAGE-01` U13/U14，`DATASET-PURGE-01` U15。2026-10-22 再拆出紧急清除。
accept-12 的 19 条规格按上列任务分配，无遗漏无重复。验收执行器 `.data/scenes/accept.py` 的 `owners()`
仍按文件前缀把整份 accept-12 归到本任务；实际归属以上列任务的验收列表为准。
原并入的发布幂等与冲突检查已拆为 `DATASET-PUBLISH-SAFETY-01` 并归档。

2026-10-23 `DATASET-RETENTION-01` 与 `DATASET-STORAGE-01` 合并为 `DATASET-LIFECYCLE-01`，
本文件的依赖引用随之更新，验收范围不变。同日并入 `DEBT-DATASET-VERSION-CONTRACT-01`，见`## 并入`。
同日 `DATASET-PURGE-01` 也并入 `DATASET-LIFECYCLE-01`（原因见该任务 `## 关闭`），三条 U12–U15 现在
只有两个承接任务；原 19 条规格改为 9 条归本任务、10 条归生命周期任务。

`accept.py` 的按规格归属仍待落地。`ACCEPT-HARNESS-01` 正在改该文件，按任务规则先不并发修改；
下次修改 `accept.py` 时把 `owners()` 的 accept-12 分支按下面三组切分，不再整份归本条：

| 承接任务 | 规格 |
|---|---|
| `DATASET-REVISION-01` | `follow-default`、`follow-coalesce`、`follow-prepare-failure`、`follow-min-interval`、`follow-declaration-change`、`follow-non-fast-forward`、`follow-permission-revoked`、`manual-source`、`cross-repo-combination`（9 条） |
| `DATASET-LIFECYCLE-01` | `search-retention`、`continuation-grace`、`retire`、`retire-current-rejected`、`usage-no-auto-retire`、`pinned-task-out-of-retention`、`source-commit-held`、`gc-rule-check`、`quota`、`emergency-purge`（10 条） |

合计 19 条，与 `accept-12-dataset-revisions.yaml` 的规格数一致。切分后 `requiresDebtRegistration` 仍按
实际承接任务查 `tasks/done/`，不会因合并后的任务 ID 变化而误判。
