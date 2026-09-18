# catalog-inventory-visible：attach 之后，catalog.read 可见成员仓身份，正文仍关闭。
# 主体用 inventory-reader，不用 bot（其它分叉会给 bot 叠加权）。

Feature: catalog-inventory-visible

  Scenario: construct
    When I run `kc grant add --principal inventory-reader --action catalog.read --catalog kr://scene/catalog`
    Then the output has:
      | id        | nonempty |
      | principal | inventory-reader |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | inventory-reader |
      | rules[].catalog   | kr://scene/catalog |
      | rules[].actions.0 | catalog.read |
    When I run `kc show --as inventory-reader`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
