# grant-revoked：catalog.read 规则已删除。

Feature: grant-revoked

  Scenario: construct
    When I run `kc admin grant list`
    Then the output has:
      | rules.0.id        | alw_1 |
      | rules.0.actions.0 | catalog.read |
    When I run `kc admin grant remove --id alw_1`
    Then the output has:
      | revoked | alw_1 |
    When I run `kc admin grant list`
    Then the output has:
      | rules | [] |
