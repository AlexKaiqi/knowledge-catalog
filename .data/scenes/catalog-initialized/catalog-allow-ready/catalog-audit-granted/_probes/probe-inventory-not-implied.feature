# 在 catalog-audit-granted 上：审计权不放行库存发现。
# 本叉父态是空 allow：show/list 为 FORBIDDEN。公开 Catalog 现场仍可能列出 id（catalog-declared-private）。

Feature: probe inventory not implied

  Scenario: catalog.audit.read does not imply catalog.read
    When I run `kc show --as auditor`
    Then error FORBIDDEN
    When I run `kc catalog list --as auditor`
    Then error FORBIDDEN
