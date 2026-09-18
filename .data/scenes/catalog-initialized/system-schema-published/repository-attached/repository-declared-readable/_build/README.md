# repository-declared-readable

仓可声明已认证默认可读动作（consume 侧闭集），与 Catalog 公开发现同一模式。未声明仍 fail closed，认 grant。这不是仓类型，不是 SEARCH 过滤键，也不放行写或发权。

本机 InitHome 没有 Deployment `repositoryAccess`，不能用 embedded construct 写入声明。授权器不得按 `kr://kc/system` 短路。

Oracle：`TestUndeclaredSystemRepositoryStillRequiresGrant` / `TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant` / `TestDeclaredSystemRepositoryUsesAuthenticatedDefault` / `TestRuntimeWriterRefusesSystemRepository`。
