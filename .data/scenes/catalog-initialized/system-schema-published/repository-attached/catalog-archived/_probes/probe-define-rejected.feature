# 在 catalog-archived 上：不能再定义知识集。

Feature: probe define rejected

  Scenario: define after archive
    When I run `kc workspace define --workspace later --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then error CATALOG_ARCHIVED
