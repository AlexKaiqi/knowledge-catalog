# repository-attached：Catalog 承认该仓可以进配方（CW1）。只读打开既有 authority 后原子登记。

Feature: repository-attached

  Scenario: construct
    When I run `kc attach --repo kr://scene/knowledge`
    Then the output has:
      | catalog      | kr://scene/catalog |
      | repositoryId | kr://scene/knowledge |
    When I run `kc attach --repo kr://scene/graph`
    Then the output has:
      | catalog      | kr://scene/catalog |
      | repositoryId | kr://scene/graph |
    When I run `kc show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | repositories.0.id | kr://kc/system |
      | repositories.0.schemaCount | 4 |
      | repositories.1.id | kr://scene/graph |
      | repositories.1.schemaCount | 0 |
      | repositories.2.id | kr://scene/knowledge |
      | repositories.2.schemaCount | 0 |
