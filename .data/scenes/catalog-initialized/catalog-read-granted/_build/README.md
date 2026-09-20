# catalog-read-granted

已认证主体持有该 Catalog 的 `catalog.read`：本 Catalog 库存身份可见。本机 Home 无 Deployment，公开 Catalog 仍要 grant。不放行 `catalog.audit.read`、发权或 `catalog.manage`。System 仓的已认证默认可读声明独立于本 grant。成员仓正文、README 仍要仓级 `knowledge.read`，在 `repository-attached/_probes/probe-inventory-without-body.feature` 观察。

独立 Go 证据（使用自己的 setup，不消费本节点 fixture）：`TestCatalogIsolationDoesNotShareAllow`。
