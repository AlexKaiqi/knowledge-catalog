# catalog-archived：当前 Catalog 已归档。共享 live 走查仓留到最后再跑；隔离 TestProductScenes 覆盖本节点。

Feature: catalog-archived

  Scenario: construct
    When I run `kc catalog archive`
    Then the output has:
      | catalog  | kr://scene/catalog |
      | archived | true |
    When I run `kc show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | archived  | true |
