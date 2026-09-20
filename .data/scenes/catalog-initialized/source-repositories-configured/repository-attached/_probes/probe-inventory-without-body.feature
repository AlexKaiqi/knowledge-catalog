# 从已接入仓出发，在本用例内给 inventory-reader 发权；临时授权不作为后继前态。

Feature: inventory discovery keeps repository body authorization separate

  Scenario: catalog reader discovers members but cannot read their body
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
    When I run `kc catalog list --as inventory-reader`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
    When I run `kc read --as inventory-reader --repo kr://scene/knowledge --object missing`
    Then error FORBIDDEN
