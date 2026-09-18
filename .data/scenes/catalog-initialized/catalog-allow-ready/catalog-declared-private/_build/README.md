# 私有 Catalog 仍要 catalog.read

父状态是已初始化的公开 Catalog。本节点用正式 Deployment 配置把 Catalog 标成 `private: true`，不能用 embedded construct 代替：场景夹具是无 Deployment 的本机 Home，公开 Catalog 对 `--as` 仍要 grant。

Oracle：`TestPrivateCatalogStillRequiresCatalogReadGrant`（私有 list/use 失败关闭，授 `catalog.read` 后可见）；`TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant` 对照公开 Catalog 默认可发现且不等于 `knowledge.read`。
