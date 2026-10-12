# dataset-manage-granted

`manager` 持有 `scene-set` 的 `dataset.manage`：可以发布或退役该 Dataset。不放行该 Dataset 的 `file.read`、`dataset.resolve`，也不放行任何仓上的 `knowledge.*` / 发权。对另一份 Dataset 的 manage 不共享。

本节点只冻结授权世界，不退役共享的 `scene-set`。`dataset-defined` / `dataset-retired` 是配方生命周期。

构建与探：`TestKnowledgeSetAuthorizationCoverageIsHonest`、`TestDatasetFileReadDoesNotImplyRepoKnowledge`。
