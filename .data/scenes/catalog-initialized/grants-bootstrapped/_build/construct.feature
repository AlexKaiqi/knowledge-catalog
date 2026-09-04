# grants-bootstrapped：allow 为空时写入第一个管理员。不是业务 admin grant。

Feature: grants-bootstrapped

  Scenario: construct
    When I run `kc local grant bootstrap --principal user:admin`
    Then the output has:
      | id        | bootstrap-local-admin |
      | principal | user:admin |
      | actions.0 | * |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].id        | bootstrap-local-admin |
      | rules[].principal | user:admin |
