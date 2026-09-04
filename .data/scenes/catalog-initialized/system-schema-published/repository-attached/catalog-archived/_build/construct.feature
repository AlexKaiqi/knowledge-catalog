# catalog-archived：当前 Catalog 已归档。

Feature: catalog-archived

  Scenario: construct
    When I run `kc catalog archive`
    Then the output has:
      | catalog  | kr://scene/catalog |
      | archived | true |
    When I run `kc catalog show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | archived  | true |
