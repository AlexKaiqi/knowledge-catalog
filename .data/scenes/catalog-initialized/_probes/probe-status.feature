# 在 catalog-initialized 上：库存列表可见；尚未 bootstrap 任何 grant。

Feature: probe empty registry

  Scenario: list without grants
    When I run `kc catalog list`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
    When I run `kc admin grant list`
    Then the output has:
      | rules | [] |
