# 在 catalog-read-granted 上：库存可见，正文仍关闭。

Feature: probe inventory without body

  Scenario: discover without read
    When I run `kc catalog show --as bot`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
    When I run `kc catalog list --as bot`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
    When I run `kc knowledge read --as bot --repo kr://scene/knowledge --object missing`
    Then error FORBIDDEN
