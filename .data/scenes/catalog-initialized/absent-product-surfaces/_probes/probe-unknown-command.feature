# 冻结入口必须拒绝。

Feature: probe unknown command

  Scenario: checkout is not a product command
    When I run `kc checkout`
    Then error USAGE_INVALID
