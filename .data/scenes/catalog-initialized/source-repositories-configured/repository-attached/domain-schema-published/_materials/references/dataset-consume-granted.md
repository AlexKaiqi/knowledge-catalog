# dataset-consume-granted：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# dataset-consume-granted

`consumer` 持有 Dataset 的 `file.read`。足以 `search` / `read --dataset`（AUTH-02）。不放行任何仓上的 `knowledge.*` / `writer.*`；`catalog.read` 不能跳过 Dataset `file.read`。

`--repo` SEARCH/READ 仍要仓权。主体不用 `bot`，避免和其它发权分叉叠权。

构建与探：`TestKnowledgeSetConsumeDoesNotImplyKnowledgeActions`、`TestDatasetFileReadDoesNotImplyRepoKnowledge`、`TestAuthorizeKnowledgeSetKnowledgeSeparatesConsumeFromSearch`、`TestKnowledgeSetAuthorizationCoverageIsHonest`。
