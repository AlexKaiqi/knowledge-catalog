# 已有授权不能通过退役的 local 入口重建或覆盖。
Feature: retired bootstrap

  Scenario: retired bootstrap cannot replace grants
    When I run `kc local grant bootstrap --principal agent:other`
    Then error USAGE_INVALID
    When I run `kc grant list`
    Then the output includes:
      | rules[].id        | bootstrap-deployment-admin |
      | rules[].principal | admin |
