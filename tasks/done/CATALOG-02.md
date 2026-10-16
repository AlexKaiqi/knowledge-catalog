# CATALOG-02 · Catalog 对所有认证用户可见

- [x] 已落地：已认证用户可发现公开 Catalog（`show` / `catalog list` / `catalog use`），无需 `catalog.read` grant；`private: true` 与未知 Catalog id 仍 fail-closed 要 grant。发现不等于 `knowledge.read`、也不等于 grant/admin。无 `Deployment` 的本地 Home 仍要 grant。验收：`make check-docs` 与 `make test` 全绿，无 skip。锚点 `TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant` / `TestPrivateCatalogStillRequiresCatalogReadGrant` / `TestManagedRepositoryProviderCreatesPublishesAndResumes`。
