# repository-registered：Catalog 承认该仓可以进配方（CW1）。不是本机 attach。

Feature: repository-registered

  Scenario: construct
    When I run `kc catalog repo register --repo kr://scene/knowledge`
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
