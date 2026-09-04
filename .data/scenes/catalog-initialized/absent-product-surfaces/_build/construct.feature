# absent-product-surfaces：世界仍是已 init 的 home（本分叉不改七列）。

Feature: absent-product-surfaces

  Scenario: construct
    When I run `kc local status`
    Then the output has:
      | catalog.repositoryId | kr://scene/catalog |
      | repos                | [] |
      | workspaces           | [] |
      | archived             | false |
      | home                 | absent |
      | namespace            | absent |
    When I run `kc catalog show`
    Then the output has:
      | catalogId  | kr://scene/catalog |
      | workspaces | [] |
