# catalog-read-granted

已认证主体持有该 Catalog 的 `catalog.read`：本 Catalog 库存身份可见。本机 Home 无 Deployment，公开 Catalog 仍要 grant。不放行 `catalog.audit.read`、发权或 `catalog.manage`。System 仓可读是平台例外，不是本 grant 放行的正文。成员仓正文、README 仍要仓级 `knowledge.read`，在 `catalog-inventory-visible` 观察。

构建与探：`TestCatalogIsolationDoesNotShareAllow`。
