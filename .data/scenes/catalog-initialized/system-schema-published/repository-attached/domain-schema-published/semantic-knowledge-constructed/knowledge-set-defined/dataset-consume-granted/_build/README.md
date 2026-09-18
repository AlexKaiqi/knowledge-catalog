# dataset-consume-granted

`consumer` 持有 Dataset 的 `file.read`。足以 `search` / `read --dataset`（AUTH-02）。不放行任何仓上的 `knowledge.*` / `writer.*`；`catalog.read` 不能跳过 Dataset `file.read`。

`--repo` SEARCH/READ 仍要仓权。主体不用 `bot`，避免和其它发权分叉叠权。

构建与探：`TestKnowledgeSetConsumeDoesNotImplyKnowledgeActions`、`TestDatasetFileReadDoesNotImplyRepoKnowledge`、`TestAuthorizeKnowledgeSetKnowledgeSeparatesConsumeFromSearch`、`TestKnowledgeSetAuthorizationCoverageIsHonest`。
