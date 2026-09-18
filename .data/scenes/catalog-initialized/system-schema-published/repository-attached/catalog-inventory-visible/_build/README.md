# catalog-inventory-visible

已认证主体持有该 Catalog 的 `catalog.read`，且业务仓已 attach：库存列出成员仓身份。不放行成员正文、README、`knowledge.schema.read`、VFS 字节。缺少 `knowledge.read` 不把仓从库存抹掉。

`catalog.read` 发权本身挂在 `catalog-initialized/catalog-allow-ready/catalog-read-granted`。本节点只观察接入完成之后的库存分层。

构建与探：`TestCatalogReadDiscoversWithoutKnowledgeRead`、`TestCatalogInventoryDoesNotHideReposWithoutKnowledgeRead`。
