# AUTH-REPO-01 · 仓的已认证默认可读

- [x] 已落地：仓可声明已认证默认可读（与 Catalog 公开发现同一模式）；未声明 fail closed。不设仓级 public/private，不按 `kr://kc/system` 授权短路。System Repository 选用该声明；运行时 Writer 仍拒绝写平台仓。验收：`make check-docs` 与相关 `go test` 全绿，无 skip。锚点 `TestUndeclaredSystemRepositoryStillRequiresGrant` / `TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant` / `TestDeclaredSystemRepositoryUsesAuthenticatedDefault` / `TestRuntimeWriterRefusesSystemRepository`。
