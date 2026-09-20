# dataset-resolve-granted：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# dataset-resolve-granted

`resolver` 持有 `dataset.resolve`：HTTP 可解析本次 ResolvedKnowledgeSet。不发权、不读正文。Dataset 上的 `file.read` 才读清单内文件；仓 `knowledge.read` 只服务 `--repo`。主体不用 `bot`，避免和其它发权分叉叠权。

构建与探：`TestOpenedKnowledgeSetPinDoesNotMoveWithLaterCommit`、`TestKnowledgeSetAuthorizationCoverageIsHonest`、`TestDatasetFileReadDoesNotImplyRepoKnowledge`。
