# 在 catalog-read-granted 上：catalog.read 不是发权或归档。

Feature: probe not admin

  Scenario: catalog.read does not administer
    When I run `kc grant add --as bot --principal other --action catalog.read --catalog kr://scene/catalog`
    Then error FORBIDDEN
    When I run `kc catalog archive --as bot`
    Then error FORBIDDEN
    When I run `kc grant list`
    Then the output has:
      | rules.0.id        | alw_1 |
      | rules.0.principal | bot |
      | rules.0.actions.0 | catalog.read |
