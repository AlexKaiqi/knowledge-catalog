# catalog-allow-ready

Catalog 授权面已经存在，规则仍空。这是登记表出生之后、任何 grant 之前的授权前态。本机 Home 无 Deployment，已认证主体仍要 grant 才能看见库存。

现场 `deployment init` 已经写入 bootstrap 管理主体，live 部署落在 `grants-bootstrapped`，不是本空规则节点。本节点只由 InitHome 夹具构造。

子分叉互不隐含：`grants-bootstrapped` 是部署管理主体；`catalog-read-granted` 是库存发现；`catalog-audit-granted` 是登记表历史；`catalog-create-granted` 是建仓准入；`catalog-declared-private` 是正式配置里的私有 Catalog。仓的已认证默认可读与 Catalog 公开发现同一模式，挂在 `repository-declared-readable`（go-test）；授权器不按 System Repository ID 短路。
