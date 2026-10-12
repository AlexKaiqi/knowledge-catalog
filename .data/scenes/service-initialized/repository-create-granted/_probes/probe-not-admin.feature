# 在 repository-create-granted 上：建仓准入不是发权或归档。

Feature: probe not admin

  Scenario: repositories.create does not administer
    When I run `kc grant add --as creator --principal other --action inventory.read`
    Then error FORBIDDEN
    When I run `kc catalog archive --as creator`
    Then error USAGE_INVALID
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | creator |
      | rules[].actions.0 | repositories.create |
