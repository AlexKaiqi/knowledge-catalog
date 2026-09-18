# dataset-resolve-granted

`resolver` 持有 `dataset.resolve`：HTTP 可解析本次 ResolvedKnowledgeSet。不发权、不读正文。Dataset 上的 `file.read` 才读清单内文件；仓 `knowledge.read` 只服务 `--repo`。主体不用 `bot`，避免和其它发权分叉叠权。

构建与探：`TestOpenedKnowledgeSetPinDoesNotMoveWithLaterCommit`、`TestKnowledgeSetAuthorizationCoverageIsHonest`、`TestDatasetFileReadDoesNotImplyRepoKnowledge`。
