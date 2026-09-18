# catalog-audit-granted

已认证主体持有 `catalog.audit.read`：可读本 Catalog 登记表历史。不放行 `catalog.read` 库存发现，也不放行发权。

本机 Home 无 Deployment，未另授 `catalog.read` 时 `show --as` 仍 FORBIDDEN。
