# catalog-read-granted

已认证主体持有该 Catalog 的 `catalog.read`：库存与源说明标题/摘要可见。不放行成员正文、`knowledge.schema.read`、VFS 字节。缺少 `knowledge.read` 不把仓从库存抹掉。Catalog 之间 allow 不继承。

构建与探：`TestCatalogReadDiscoversWithoutKnowledgeRead`、`TestCatalogInventoryDoesNotHideReposWithoutKnowledgeRead`、`TestCatalogIsolationDoesNotShareAllow`。
