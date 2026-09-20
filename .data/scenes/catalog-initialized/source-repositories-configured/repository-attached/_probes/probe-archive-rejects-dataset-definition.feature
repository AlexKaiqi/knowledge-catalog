# 归档仅发生在本用例的独立副本上，不作为后继状态。

Feature: 归档 Catalog 后库存标记归档且新知识集定义被拒绝

  Scenario: 归档 Catalog 后库存标记归档且新知识集定义被拒绝
    When I run `kc catalog archive`
    Then the output has:
      | catalog  | kr://scene/catalog |
      | archived | true |
    When I run `kc show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | archived  | true |

    When I run `kc dataset define --dataset later --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then error CATALOG_ARCHIVED
