# 在 catalog-read-granted 上：catalog.read 不是发权或归档。

Feature: probe not admin

  Scenario: catalog.read does not administer
    When I run `kc grant add --as bot --principal other --action catalog.read --catalog kr://scene/catalog`
    Then error FORBIDDEN
    When I run `kc catalog archive --as bot`
    Then error FORBIDDEN
    When I run `kc grant list`
    Then the output has:
      | rules.0.id        | $grantId |
      | rules.0.principal | bot |
      | rules.0.actions.0 | catalog.read |
      | rules.0.catalog   | kr://scene/catalog |
      | rules.0.repo      | absent |
      | rules.0.dataset   | absent |
      | rules.0.ref       | absent |
      | rules.0.object    | absent |
      | rules.0.aspect    | absent |
      | rules.1           | absent |
