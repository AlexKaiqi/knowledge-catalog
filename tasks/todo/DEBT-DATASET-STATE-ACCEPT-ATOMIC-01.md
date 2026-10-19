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
