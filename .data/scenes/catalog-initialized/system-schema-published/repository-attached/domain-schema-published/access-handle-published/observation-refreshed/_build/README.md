# observation-refreshed

接入方 Observer 发出 source changed（notice 可丢、可乱序）。容器在 `access-handle-published/_materials/accessor/observer.py`。平台按**固定 Binding** 向 Resource Access lookup，刷新可丢**动态索引**。Repository HEAD 不变。未成功观察的 Binding 不能进索引当 MISSING。

这不是声明式 Snapshot 投影（见 `projection-synced`）。两条车道共用 Schema AccessHints 编译出的 AccessSpec。

Hook 是出站，不能冒充这条入站通知。

构建与探：`TestProjectionControllerNoticePullsStateWithoutChangingSnapshot`、`TestChangeNoticeRejectsBody`、`TestProjectionNotifyPullsBoundStateWithoutChangingHEAD`。公开入口是 `operations projection notice` / `POST /operations/v1/projections:notice`。
