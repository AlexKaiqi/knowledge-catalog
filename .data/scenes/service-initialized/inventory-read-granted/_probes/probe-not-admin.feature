# 在 inventory-read-granted 上：inventory.read 不是发权或归档。

Feature: probe not admin

  Scenario: inventory.read does not administer
    When I run `kc grant add --as bot --principal other --action inventory.read`
    Then error FORBIDDEN
    When I run `kc catalog archive --as bot`
    Then error USAGE_INVALID
    When I run `kc grant list`
    Then the output has:
      | rules.0.id        | $grantId |
      | rules.0.principal | bot |
      | rules.0.actions.0 | inventory.read |
      | rules.0.catalog   | absent |
      | rules.0.repo      | absent |
      | rules.0.dataset   | absent |
      | rules.0.ref       | absent |
      | rules.0.object    | absent |
      | rules.0.aspect    | absent |
      | rules.1           | absent |
