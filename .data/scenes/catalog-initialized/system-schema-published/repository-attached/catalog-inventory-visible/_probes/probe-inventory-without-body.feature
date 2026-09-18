# 在 catalog-inventory-visible 上：成员仓身份可见，正文仍关闭。

Feature: probe inventory without body

  Scenario: discover without read
    When I run `kc show --as inventory-reader`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
    When I run `kc catalog list --as inventory-reader`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
    When I run `kc read --as inventory-reader --repo kr://scene/knowledge --object missing`
    Then error FORBIDDEN
