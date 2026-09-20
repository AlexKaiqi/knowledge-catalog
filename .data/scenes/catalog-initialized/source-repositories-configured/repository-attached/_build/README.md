# repository-attached

服务只读打开既有 Snapshot，原子提交 Catalog 成员登记。这个可重建前态供普通知识发表、Domain Schema 发表、归档及独立验证复用。

`_probes/probe-inventory-without-body.feature` 从已接入状态开始，在用例内部给 `inventory-reader` 授予 `catalog.read`，观测成员身份可见、Catalog 可发现而正文读取仍被拒绝。该临时授权不被其它用例或子状态继承；没有另建“库存可见”状态。`catalog.read` 的独立授权前态仍在 `catalog-initialized/catalog-read-granted`。

`_meta.yaml` 同时挂载以下 Go 验证证据。各 Go 测试自行构建其正式配置或组件夹具；挂在这里表示风险归属，不表示复用本节点的冻结 home：

- 库存发现与正文隔离：`TestCatalogReadDiscoversWithoutKnowledgeRead`、`TestCatalogInventoryDoesNotHideReposWithoutKnowledgeRead`。
- 仓可声明已认证默认可读动作；未声明仍 fail closed，且不放行写或发权。InitHome 夹具没有 Deployment `repositoryAccess`，授权器不得按 System Repository ID 短路。证据为 `TestUndeclaredSystemRepositoryStillRequiresGrant`、`TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant`、`TestDeclaredSystemRepositoryUsesAuthenticatedDefault`、`TestRuntimeWriterRefusesSystemRepository`。
- 墙外 Collector 对账后产生 ChangeSet（`connector.Preview`），确认后才 Writer COMMIT；没有在 Catalog 注册 runtime。证据为 `connector/preview_test.go`、`TestPreviewThenCommit`。常驻容器与对账程序仍放在 `domain-schema-published/access-handle-published/_materials/accessor/`。
