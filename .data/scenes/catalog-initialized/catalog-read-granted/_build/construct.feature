# catalog-read-granted：bot 持有该 Catalog 的 catalog.read。
# 本机 Home 无 Deployment，已认证主体仍要 grant 才能看见库存。

Feature: catalog-read-granted

  Scenario: construct
    When I run `kc grant add --principal bot --action catalog.read --catalog kr://scene/catalog`
    Then the output has:
      | id        | nonempty |
      | principal | bot |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].catalog   | kr://scene/catalog |
      | rules[].actions.0 | catalog.read |
    When I run `kc catalog use kr://scene/catalog`
    Then the output has:
      | catalogId | kr://scene/catalog |
    When I run `kc show --as bot`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc catalog list --as bot`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
