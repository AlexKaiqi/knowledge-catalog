# 首次部署的首个管理主体已写入耐久授权状态；这里复用夹具前态。
Feature: grants-bootstrapped

  Scenario: construct
    Given bootstrap principal user:admin
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].id        | bootstrap-deployment-admin |
      | rules[].principal | user:admin |
      | rules[].actions.0 | * |
