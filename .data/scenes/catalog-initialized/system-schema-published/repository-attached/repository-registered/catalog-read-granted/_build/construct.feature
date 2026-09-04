# catalog-read-granted：bot 持有该 Catalog 的 catalog.read。

Feature: catalog-read-granted

  Scenario: construct
    When I run `kc admin grant add --principal bot --action catalog.read --catalog kr://scene/catalog`
    Then the output has:
      | id        | alw_1 |
      | principal | bot |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.read |
    When I run `kc admin grant list`
    Then the output has:
      | rules.0.id        | alw_1 |
      | rules.0.principal | bot |
      | rules.0.actions.0 | catalog.read |
    When I run `kc catalog show --as bot`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
