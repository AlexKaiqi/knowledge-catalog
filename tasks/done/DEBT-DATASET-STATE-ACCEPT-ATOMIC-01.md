# DEBT-DATASET-STATE-ACCEPT-ATOMIC-01 · Dataset 观察最终接受的原子边界

## 债

- 位置：`declarativeindex/projection/dataset_state.go` 的 `RefreshDatasetState`：`idx.stateMu` 写锁内的 `requireCurrentDataset` 复查之后、`session.Commit()` 完成之前。
- 信号：`requireCurrentDataset` 读取的发布权威（`plugin/entry.go` 注入的 `options.Datasets.Set`）在进程外；复查返回“当前”后、引擎提交完成前，权威仍可能发布新修订。既有守卫（`liveaccess/discovery/member_projection_test.go` 的 `TestDatasetRevisionPublishedDuringObservationPreservesOldGeneration`）只覆盖观察期间（`ObserveDatasetValue` 回调内）的发布，锁内复查已拦截该窗口；Commit 执行期间的窗口未被覆盖。引擎 `Commit`（`opensearchimpl/projection.go`）成功后即删除旧代际（`retireGeneration`），事后无法撤回。
- 判定：进程内锁不能闭合这个缝；修复需要引擎公开恢复旧代际控制文档的形状，或把修订条件接入引擎提交的 CAS 条件，属于公开形状决策，不在本轮顺手扩张。
- 处置：评估给 `RebuildSession.Commit` 增加条件（提交时携带 Dataset 修订期望，CAS 失败即 Abort 保留旧代际）；或给引擎增加恢复旧 READY 控制文档的窄接口。任一方案先给设计篇依据再实施。补一条并发反例回归：发布钩子注入 Commit 执行期间，断言旧代际保留服务。
- 触发条件：下一次修改 `RefreshDatasetState` / 引擎提交形状 / Dataset 发布链路时；或 `DATASET-REVISION-01`（自动跟随发布修订）实现时，该窗口从理论风险变为高频路径。

## 目标

观察刷新的最终接受与 Dataset 修订权威原子：Commit 完成时修订仍为刷新目标修订，否则旧代际保留服务，新观察不发布。

## 依据

- [发现](../../doc/design/discovery.md)：检索空间按修订、同版本回读。
- [Dataset](../../doc/design/dataset.md)：发布与修订语义。
- [工程质量闭环](../../doc/engineering/quality.md)：质量问题三出口。

## 验收

- 并发反例回归（发布落在 Commit 执行期间）先红后绿：新观察被拒、旧 READY 代际继续服务、错误为 `ErrPreconditionFailed`。
- 引擎代际恢复或条件提交路径有专项回归；既有 `TestDatasetRevisionPublishedDuringObservationPreservesOldGeneration` 与 `TestDatasetCurrentRevisionObservationRefreshPublishesNewGeneration` 保持绿。
- 受影响合同、`make check-docs`、`make quality` 通过。

## 不做

不把修订权威搬进 Index（保持进程外）；不为撤回引入跨仓事务；不放宽既有观察期守卫断言。

## 交付

方案：修订围栏。不改引擎公开形状，也不搬动修订权威，按 setID 在进程内排序发布与观察的最终接受。
任务原文的两个处置（引擎 CAS 条件、恢复旧控制文档）都要扩张引擎公开形状；围栏在提交前后让发布排序，
达到同样的目标（Commit 完成时修订仍为目标修订，否则不提交、旧代际保留服务），因此选围栏。

实现范围：
- `snapshot/dataset/revision_fence.go`：按 setID 的互斥；`WithCurrentRevision(setID, revision, fn)` 持围栏检查“存在、未退役、修订相等”，不满足返回 `ErrPreconditionFailed` 且不执行 fn。发布（`dataset-define`）与退役（`dataset-retire`）经 `commitDataset` 先取围栏再进入 registry 提交；读与其它 Dataset 不受影响。
- `declarativeindex/projection/dataset_state.go`：`session.Commit()` 与写入 `idx.states` 在围栏内完成；围栏拒绝且未进入提交时 `Abort`，旧代际保留服务。
- `declarativeindex/plugin/entry.go`：注入 `options.Datasets.WithCurrentRevision`，替换原先的修订查询 lookup。

局限：围栏是进程内的。跨进程的并发发布仍由 registry head CAS 兜底，不在本债范围；`DATASET-REVISION-01` 实现时无需再改此处。

已验证：
- `snapshot/dataset/revision_fence_test.go`：同 Dataset 发布被排序在持有围栏之后；围栏拒绝时不执行 fn。
- `declarativeindex/plugin/dataset_revision_fence_test.go`：发布钩子注入 `Commit` 执行期间，发布被排序在接受之后；被取代修订再刷新返回 `ErrPreconditionFailed` 且不再提交。先红后绿，`-race` 通过。
- 既有 `TestDatasetRevisionPublishedDuringObservationPreservesOldGeneration`、`TestDatasetCurrentRevisionObservationRefreshPublishesNewGeneration` 保持绿。
- `make check-docs`、`make quality`、`make test` 通过，`make test` 无 skip；`make test-contracts` 退出码 0，其中的 skip 均为外部 live 环境用例。
- 未验：真实部署入口（`make deploy-local-scenes`）未运行。
