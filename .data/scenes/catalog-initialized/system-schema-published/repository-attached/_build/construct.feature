# repository-attached：Catalog 承认该仓可以进配方（CW1）。只读打开既有 authority 后原子登记。

Feature: repository-attached

  Scenario: construct
    When I run `kc catalog repo attach --repo kr://scene/knowledge`
    Then the output has:
      | catalog      | kr://scene/catalog |
      | repositoryId | kr://scene/knowledge |
    When I run `kc catalog repo list`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
      | repositories[].id | kr://kc/system |
    When I run `kc catalog show`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
      | repositories[].id | kr://kc/system |
